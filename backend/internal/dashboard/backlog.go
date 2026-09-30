package dashboard

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	"controle/internal/platform/cnpj"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/validate"
)

// Tipos de item do backlog.
const (
	BacklogObligation = "OBLIGATION" // obrigação aplicável não final
	BacklogModule     = "MODULE"     // módulo aberto sem obrigação pendente (falta fechar)
)

// Ordenações do backlog.
const (
	SortDaysOverdue = "days_overdue" // padrão: mais atrasadas primeiro [UC-03]
	SortDueDate     = "due_date"
	SortCompetence  = "competence"
	SortCompany     = "company"
)

const (
	defaultPageSize = 100
	maxPageSize     = 500
)

// BacklogParams são os filtros do §25.3 (backlog) ou da lista filtrada do drill-down.
type BacklogParams struct {
	Before        *comp.Competence // backlog: competências anteriores a esta
	Competence    *comp.Competence // lista: exatamente esta competência
	State         *string          // open (padrão), applicable, done, in_progress, not_started, pending, overdue
	Module        *string
	Status        *string
	ResponsibleID *string
	RegimeGroup   *string
	TaxRegime     *string
	Q             *string // empresa, código ou CNPJ (com ou sem máscara)
	OverdueMin    *int32
	OverdueMax    *int32
	Sort          string
	Page          int
	PageSize      int
}

// BacklogCompany identifica a empresa do item.
type BacklogCompany struct {
	ID           string  `json:"id"`
	InternalCode string  `json:"internal_code"`
	LegalName    string  `json:"legal_name"`
	TradeName    *string `json:"trade_name"`
	Cnpj         string  `json:"cnpj"`
}

// BacklogType identifica a obrigação do item.
type BacklogType struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// BacklogItem é uma linha do backlog [PRD §25.2].
type BacklogItem struct {
	Kind           string          `json:"kind"`
	InstanceID     *string         `json:"instance_id"`
	Competence     comp.Competence `json:"competence"`
	Company        BacklogCompany  `json:"company"`
	Module         string          `json:"module"`
	ObligationType *BacklogType    `json:"obligation_type"`
	StatusCode     *string         `json:"status_code"`
	StatusLabel    string          `json:"status_label"`
	DueDate        *civil.Date     `json:"due_date"`
	DaysOverdue    int             `json:"days_overdue"`
	Responsible    *UserRef        `json:"responsible"`
	// Grade do módulo na competência do item, com a busca na empresa.
	Drilldown *Drilldown `json:"drilldown"`
}

// Backlog é a resposta de GET /backlog.
type Backlog struct {
	Items    []BacklogItem `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

const backlogSQL = `
WITH items AS (
    SELECT 'OBLIGATION'::text AS kind, v.id AS instance_id, k.competence_key, c.id AS company_id, c.internal_code,
           c.legal_name, c.trade_name, c.cnpj, s.regime_group, s.tax_regime, v.primary_module AS module,
           t.code AS type_code, t.name AS type_name, v.status_code, v.status_label, v.due_date_effective,
           CASE WHEN v.applicable AND NOT v.is_final AND v.due_date_effective < $1::date
                THEN $1::date - v.due_date_effective ELSE 0 END AS days_overdue,
           u.id AS responsible_id, u.name AS responsible_name,
           v.applicable, v.in_denominator, v.is_final, v.counts_as_started, v.is_blocking
    FROM v_obligation_instances v
    JOIN competencies k ON k.id = v.competence_id
    JOIN companies c ON c.id = v.company_id
    JOIN obligation_types t ON t.id = v.obligation_type_id
    JOIN company_competence_snapshots s ON s.company_id = v.company_id AND s.competence_id = v.competence_id
    LEFT JOIN users u ON u.id = CASE v.primary_module
        WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
        WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END
    WHERE ($2::text IS NOT NULL AND k.competence_key < $2::text)
       OR ($3::text IS NOT NULL AND k.competence_key = $3::text)
    UNION ALL
    SELECT 'MODULE'::text, NULL::uuid, k.competence_key, c.id, c.internal_code, c.legal_name, c.trade_name, c.cnpj,
           s.regime_group, s.tax_regime, p.module_code, NULL::text, NULL::text, NULL::text, 'Módulo aberto'::text, NULL::date,
           0, u.id, u.name, true, false, false, true, false
    FROM company_module_periods p
    JOIN competencies k ON k.id = p.competence_id
    JOIN companies c ON c.id = p.company_id
    JOIN company_competence_snapshots s ON s.company_id = p.company_id AND s.competence_id = p.competence_id
    LEFT JOIN users u ON u.id = CASE p.module_code
        WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
        WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END
    WHERE $2::text IS NOT NULL AND k.competence_key < $2::text AND p.state = 'OPEN'
      AND NOT EXISTS (
          SELECT 1 FROM v_obligation_instances o
          WHERE o.company_id = p.company_id AND o.competence_id = p.competence_id AND o.primary_module = p.module_code
            AND o.applicable AND NOT o.is_final)
), filtered AS (
    SELECT * FROM items i
    WHERE ($4::text IS NULL OR i.module = $4::text)
      AND ($5::text IS NULL OR i.status_code = $5::text)
      AND ($6::uuid IS NULL OR i.responsible_id = $6::uuid)
      AND ($7::text IS NULL OR i.regime_group = $7::text)
      AND ($8::text IS NULL OR i.tax_regime = $8::text)
      AND ($9::text IS NULL
           OR i.internal_code ILIKE '%' || $9::text || '%'
           OR i.legal_name ILIKE '%' || $9::text || '%'
           OR coalesce(i.trade_name, '') ILIKE '%' || $9::text || '%'
           OR ($10::text IS NOT NULL AND i.cnpj LIKE '%' || $10::text || '%'))
      AND ($11::int IS NULL OR i.days_overdue >= $11::int)
      AND ($12::int IS NULL OR i.days_overdue <= $12::int)
      AND CASE coalesce($13::text, 'open')
            WHEN 'open'        THEN i.applicable AND NOT i.is_final
            WHEN 'applicable'  THEN i.in_denominator
            WHEN 'done'        THEN i.in_denominator AND i.is_final
            WHEN 'in_progress' THEN i.in_denominator AND i.counts_as_started AND NOT i.is_final AND NOT i.is_blocking
            WHEN 'not_started' THEN i.in_denominator AND NOT i.counts_as_started AND NOT i.is_final AND NOT i.is_blocking
            WHEN 'pending'     THEN i.in_denominator AND i.is_blocking
            WHEN 'overdue'     THEN i.days_overdue > 0
          END
      AND (i.kind = 'OBLIGATION' OR ($13::text IS NULL AND $5::text IS NULL))
)
SELECT f.kind, CASE WHEN f.kind = 'OBLIGATION' THEN f.instance_id END AS instance_id, f.competence_key, f.company_id,
       f.internal_code, f.legal_name, f.trade_name, f.cnpj, f.module,
       CASE WHEN f.kind = 'OBLIGATION' THEN f.type_code END AS type_code,
       CASE WHEN f.kind = 'OBLIGATION' THEN f.type_name END AS type_name,
       CASE WHEN f.kind = 'OBLIGATION' THEN f.status_code END AS status_code,
       f.status_label, f.due_date_effective, f.days_overdue::int AS days_overdue,
       f.responsible_id, f.responsible_name, count(*) OVER ()::int AS total
FROM filtered f
ORDER BY
    CASE WHEN $14::text = 'days_overdue' THEN f.days_overdue END DESC NULLS LAST,
    CASE WHEN $14::text = 'due_date' THEN f.due_date_effective END ASC NULLS LAST,
    CASE WHEN $14::text IN ('competence', 'days_overdue') THEN f.competence_key END ASC,
    CASE WHEN $14::text = 'company' THEN f.legal_name END ASC,
    f.internal_code, f.module, f.type_code NULLS FIRST
LIMIT $15::int OFFSET $16::int`

// Backlog lista as pendências de competências anteriores (plan R8) ou as obrigações de uma
// competência por classe, com filtros, ordenação por atraso e paginação [PRD §25, UC-03].
func (s *Service) Backlog(ctx context.Context, p BacklogParams) (Backlog, error) {
	today := clock.Today(s.clock)
	var before, competence, qDigits *string
	if p.Before != nil {
		v := p.Before.String()
		before = &v
	}
	if p.Competence != nil {
		v := p.Competence.String()
		competence = &v
	}
	if p.Q != nil {
		if d := cnpj.Sanitize(*p.Q); len(d) >= 3 {
			qDigits = &d
		}
	}
	rows, err := s.pool.Query(ctx, backlogSQL, today, before, competence, p.Module, p.Status, p.ResponsibleID,
		p.RegimeGroup, p.TaxRegime, p.Q, qDigits, p.OverdueMin, p.OverdueMax, p.State, p.Sort, p.PageSize, (p.Page-1)*p.PageSize)
	if err != nil {
		return Backlog{}, err
	}
	out := Backlog{Items: []BacklogItem{}, Page: p.Page, PageSize: p.PageSize}
	type row struct {
		kind, key, companyID, code, legal, cnpjDigits, module, label string
		instanceID, trade, typeCode, typeName, status, respID, resp  *string
		due                                                          *time.Time
		days, total                                                  int32
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		err := r.Scan(&x.kind, &x.instanceID, &x.key, &x.companyID, &x.code, &x.legal, &x.trade, &x.cnpjDigits, &x.module,
			&x.typeCode, &x.typeName, &x.status, &x.label, &x.due, &x.days, &x.respID, &x.resp, &x.total)
		return x, err
	})
	if err != nil {
		return Backlog{}, err
	}
	for _, x := range items {
		c, err := comp.Parse(x.key)
		if err != nil {
			return Backlog{}, err
		}
		out.Total = int(x.total)
		it := BacklogItem{
			Kind: x.kind, InstanceID: x.instanceID, Competence: c,
			Company: BacklogCompany{ID: x.companyID, InternalCode: x.code, LegalName: x.legal, TradeName: x.trade, Cnpj: x.cnpjDigits},
			Module:  x.module, StatusCode: x.status, StatusLabel: x.label, DueDate: civil.Ptr(x.due), DaysOverdue: int(x.days),
			Drilldown: &Drilldown{Path: modulePaths[x.module], Query: map[string]string{"competence": c.String(), "q": x.code}},
		}
		if x.typeCode != nil && x.typeName != nil {
			it.ObligationType = &BacklogType{Code: *x.typeCode, Name: *x.typeName}
		}
		if x.respID != nil && x.resp != nil {
			it.Responsible = &UserRef{ID: *x.respID, Name: *x.resp}
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

var (
	backlogStates  = []string{"open", StateApplicable, StateDone, StateInProgress, StateNotStarted, StatePending, StateOverdue}
	backlogSorts   = []string{SortDaysOverdue, SortDueDate, SortCompetence, SortCompany}
	backlogModules = []string{"FISCAL", "DECLARATIONS", "PAYROLL", "ACCOUNTING"}
)

func oneOf(v string, allowed []string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// ParseBacklog lê os parâmetros de GET /backlog: before ou competence (exatamente um).
func ParseBacklog(q url.Values) (BacklogParams, error) {
	p := BacklogParams{Sort: SortDaysOverdue, Page: 1, PageSize: defaultPageSize}
	opt := func(name string) *string {
		if v := strings.TrimSpace(q.Get(name)); v != "" {
			return &v
		}
		return nil
	}
	for name, dst := range map[string]**comp.Competence{"before": &p.Before, "competence": &p.Competence} {
		if v := opt(name); v != nil {
			c, err := comp.Parse(*v)
			if err != nil {
				return p, validate.Field(name, "Informe a competência no formato AAAA-MM.")
			}
			*dst = &c
		}
	}
	if (p.Before == nil) == (p.Competence == nil) {
		return p, validate.Field("before", "Informe before (backlog) ou competence (lista da competência).")
	}
	if v := opt("state"); v != nil {
		if !oneOf(*v, backlogStates) {
			return p, validate.Field("state", "Classe inválida.")
		}
		p.State = v
	}
	if v := opt("module"); v != nil {
		up := strings.ToUpper(*v)
		if !oneOf(up, backlogModules) {
			return p, validate.Field("module", "Módulo inválido.")
		}
		p.Module = &up
	}
	if v := opt("status"); v != nil {
		up := strings.ToUpper(*v)
		p.Status = &up
	}
	f, err := parseFilters(q)
	if err != nil {
		return p, err
	}
	p.RegimeGroup, p.TaxRegime, p.ResponsibleID = f.RegimeGroup, f.TaxRegime, f.ResponsibleID
	if v := opt("q"); v != nil {
		p.Q = v
	}
	for name, dst := range map[string]**int32{"overdue_min": &p.OverdueMin, "overdue_max": &p.OverdueMax} {
		if v := opt(name); v != nil {
			n, err := strconv.ParseInt(*v, 10, 32)
			if err != nil || n < 0 {
				return p, validate.Field(name, "Informe um número de dias maior ou igual a zero.")
			}
			d := int32(n)
			*dst = &d
		}
	}
	if v := opt("sort"); v != nil {
		if !oneOf(*v, backlogSorts) {
			return p, validate.Field("sort", "Ordenação inválida.")
		}
		p.Sort = *v
	}
	for name, dst := range map[string]*int{"page": &p.Page, "page_size": &p.PageSize} {
		if v := opt(name); v != nil {
			n, err := strconv.Atoi(*v)
			if err != nil || n < 1 || (name == "page_size" && n > maxPageSize) {
				return p, validate.Field(name, "Valor inválido.")
			}
			*dst = n
		}
	}
	return p, nil
}
