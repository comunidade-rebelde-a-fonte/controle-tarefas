// Package companyhub é a ficha da empresa como hub (TASK-025, D-039): a Visão geral da empresa
// na competência (o progresso por módulo, os próximos vencimentos, o Fator R e a tendência, a
// Conciliação e as pendências) e os indicadores compactos da listagem de empresas (08 §4 e §5).
// Tudo sai das mesmas consultas das telas de cada módulo, sempre pela mesma empresa
// (companies.id, REQ-002).
package companyhub

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/credential"
	"controle/internal/dashboard"
	"controle/internal/factorr"
	"controle/internal/factorr/engine"
	"controle/internal/obligation"
	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation/overview"
	"controle/internal/store"
)

// Service monta a Visão geral e os indicadores.
type Service struct {
	pool        *pgxpool.Pool
	clock       clock.Clock
	dashboard   *dashboard.Service
	factorR     *factorr.Service
	credentials *credential.Service
}

// NewService cria o serviço com os serviços dos módulos de que a ficha depende.
func NewService(pool *pgxpool.Pool, clk clock.Clock, dash *dashboard.Service, fr *factorr.Service, credentials *credential.Service) *Service {
	return &Service{pool: pool, clock: clk, dashboard: dash, factorR: fr, credentials: credentials}
}

// CompanyRef identifica a empresa.
type CompanyRef struct {
	ID           string `json:"id"`
	InternalCode string `json:"internal_code"`
	LegalName    string `json:"legal_name"`
}

// ModuleProgress é um módulo da empresa na competência.
type ModuleProgress struct {
	Module              string               `json:"module"`
	Label               string               `json:"label"`
	PeriodState         string               `json:"period_state"`
	Applicable          int                  `json:"applicable"`
	Done                int                  `json:"done"`
	Pending             int                  `json:"pending"`
	Overdue             int                  `json:"overdue"`
	OperationalProgress float64              `json:"operational_progress"`
	FinalCompletion     float64              `json:"final_completion"`
	Drilldown           *dashboard.Drilldown `json:"drilldown"`
}

// DueItem é uma obrigação aplicável e não final da competência.
type DueItem struct {
	InstanceID  string               `json:"instance_id"`
	Module      string               `json:"module"`
	ModuleLabel string               `json:"module_label"`
	Obligation  string               `json:"obligation"`
	StatusLabel string               `json:"status_label"`
	DueDate     *civil.Date          `json:"due_date"`
	Overdue     bool                 `json:"overdue"`
	Drilldown   *dashboard.Drilldown `json:"drilldown"`
}

// TrendPoint é o Fator R de um PA da tendência.
type TrendPoint struct {
	PA      competence.Competence `json:"pa"`
	Light   *engine.Light         `json:"semaforo"`
	FactorR *string               `json:"fator_r"`
}

// FactorR é o Fator R da empresa no PA igual à competência, com a ação sugerida e a tendência
// dos últimos PAs (o mesmo cálculo da aba Fator R).
type FactorR struct {
	PA         competence.Competence `json:"pa"`
	Status     engine.Status         `json:"status"`
	Reason     *engine.Reason        `json:"motivo"`
	Light      *engine.Light         `json:"semaforo"`
	FactorR    *string               `json:"fator_r"`
	Annex      *engine.Annex         `json:"anexo"`
	Savings12m *string               `json:"economia_12m"`
	Action     engine.Action         `json:"acao"`
	ActionText string                `json:"acao_texto"`
	Trend      []TrendPoint          `json:"trend"`
}

// Overview é a Visão geral da empresa na competência.
type Overview struct {
	Company    CompanyRef `json:"company"`
	Competence string     `json:"competence"`
	// CompetenceGenerated é falso numa competência ainda não gerada (um PA futuro do Fator R):
	// sem módulos, vencimentos e pendências; o Fator R e a Conciliação seguem.
	CompetenceGenerated bool             `json:"competence_generated"`
	Modules             []ModuleProgress `json:"modules"`
	NextDue             []DueItem        `json:"next_due"`
	// FactorR é nulo quando a empresa não está na carteira do Fator R no PA.
	FactorR *FactorR `json:"factor_r"`
	// Reconciliation é nula quando a empresa não usa a Conciliação na competência nem tem item na
	// fila dela.
	Reconciliation *overview.Company             `json:"reconciliation"`
	Attention      []dashboard.AttentionCategory `json:"attention"`
}

const (
	nextDueLimit = 5
	trendMonths  = 6
)

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

func text(d interface{ Text() string }) *string {
	v := d.Text()
	return &v
}

// Overview monta a Visão geral da empresa na competência.
func (s *Service) Overview(ctx context.Context, companyID string, c competence.Competence) (Overview, error) {
	if !httpx.IsUUID(companyID) {
		return Overview{}, companyNotFound()
	}
	q := store.New(s.pool)
	co, err := q.GetCompanyRef(ctx, companyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Overview{}, companyNotFound()
	} else if err != nil {
		return Overview{}, err
	}
	code := co.InternalCode
	out := Overview{
		Company: CompanyRef{ID: co.ID, InternalCode: code, LegalName: co.LegalName}, Competence: c.String(),
		Modules: []ModuleProgress{}, NextDue: []DueItem{}, Attention: []dashboard.AttentionCategory{},
	}
	kid, err := dashboard.CompetenceID(ctx, q, c)
	var ae *apperr.Error
	switch {
	case errors.As(err, &ae) && ae.Code == codeCompetenceNotFound:
		return s.withModules(ctx, q, out, c)
	case err != nil:
		return Overview{}, err
	}
	out.CompetenceGenerated = true
	today := clock.Today(s.clock)
	group := ""
	if snap, err := q.GetCompanyCompetenceSnapshot(ctx, store.GetCompanyCompetenceSnapshotParams{CompanyID: companyID, CompetenceID: kid}); err == nil {
		group = snap.RegimeGroup
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Overview{}, err
	}

	modules, err := q.CompanyModuleProgress(ctx, store.CompanyModuleProgressParams{Today: today, CompanyID: companyID, CompetenceID: kid})
	if err != nil {
		return Overview{}, err
	}
	for _, m := range modules {
		p := obligation.ProgressFromCounts(int(m.Applicable), int(m.OperationalDone), int(m.Done), int(m.Pending))
		out.Modules = append(out.Modules, ModuleProgress{
			Module: m.Module, Label: dashboard.ModuleLabel(m.Module), PeriodState: m.State, Applicable: int(m.Applicable),
			Done: int(m.Done), Pending: int(m.Pending), Overdue: int(m.Overdue), OperationalProgress: p.OperationalPct,
			FinalCompletion: p.FinalPct, Drilldown: dashboard.CompanyGrid(m.Module, code, group, c),
		})
	}

	due, err := q.CompanyNextDue(ctx, store.CompanyNextDueParams{Today: today, CompanyID: companyID, CompetenceID: kid, RowLimit: nextDueLimit})
	if err != nil {
		return Overview{}, err
	}
	for _, d := range due {
		out.NextDue = append(out.NextDue, DueItem{
			InstanceID: d.ID, Module: d.Module, ModuleLabel: dashboard.ModuleLabel(d.Module), Obligation: d.TypeName,
			StatusLabel: d.StatusLabel, DueDate: civil.Ptr(d.DueDateEffective), Overdue: d.Overdue,
			Drilldown: dashboard.CompanyGrid(d.Module, code, group, c),
		})
	}

	att, err := s.dashboard.CompanyAttention(ctx, c, companyID)
	if err != nil {
		return Overview{}, err
	}
	out.Attention = att.Categories
	return s.withModules(ctx, q, out, c)
}

// codeCompetenceNotFound é a recusa da competência ainda não gerada.
const codeCompetenceNotFound = "COMPETENCE_NOT_FOUND"

// withModules acrescenta o Fator R e a Conciliação, que não dependem da competência gerada.
func (s *Service) withModules(ctx context.Context, q *store.Queries, out Overview, c competence.Competence) (Overview, error) {
	var err error
	if out.FactorR, err = s.factorRFor(ctx, out.Company.ID, c); err != nil {
		return Overview{}, err
	}
	rec, err := overview.Load(ctx, q, c, []string{out.Company.ID})
	if err != nil {
		return Overview{}, err
	}
	if len(rec) > 0 {
		out.Reconciliation = &rec[0]
	}
	return out, nil
}

// factorRFor é o Fator R da empresa no PA e a tendência, só se ela está na carteira do PA. Um PA
// sem tabela do Simples vigente fica fora da tendência (e, sendo o próprio PA, sem Fator R).
func (s *Service) factorRFor(ctx context.Context, companyID string, pa competence.Competence) (*FactorR, error) {
	calc, err := s.factorR.Calc(ctx, companyID, pa)
	var ae *apperr.Error
	if errors.As(err, &ae) && ae.Code == "SIMPLES_TABLE_NOT_IN_FORCE" {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !calc.Monitorada {
		return nil, nil
	}
	r := calc.Result
	action := engine.SuggestedAction(r)
	out := &FactorR{PA: pa, Status: r.Status, Reason: r.Reason, Light: r.Light, Annex: r.Annex, Action: action,
		ActionText: engine.ActionTexts[action], Trend: []TrendPoint{}}
	if r.FactorR != nil {
		out.FactorR = text(*r.FactorR)
	}
	if r.Savings12m != nil {
		out.Savings12m = text(*r.Savings12m)
	}
	for i := trendMonths - 1; i >= 0; i-- {
		p := pa.AddMonths(-i)
		point := TrendPoint{PA: p}
		if i == 0 {
			point.Light, point.FactorR = out.Light, out.FactorR
		} else {
			past, err := s.factorR.Calc(ctx, companyID, p)
			if errors.As(err, &ae) && ae.Code == "SIMPLES_TABLE_NOT_IN_FORCE" {
				continue
			}
			if err != nil {
				return nil, err
			}
			point.Light = past.Light
			if past.FactorR != nil {
				point.FactorR = text(*past.FactorR)
			}
		}
		out.Trend = append(out.Trend, point)
	}
	return out, nil
}

// FactorRIndicator é o semáforo da carteira (nulo sem dados suficientes) e a ação sugerida.
type FactorRIndicator struct {
	Light  *engine.Light `json:"semaforo"`
	Action engine.Action `json:"acao"`
}

// ReconciliationIndicator é o estado da conciliação na competência.
type ReconciliationIndicator struct {
	State            string `json:"state"`
	BlocksAccounting bool   `json:"blocks_accounting"`
}

// CompanyIndicators são os indicadores compactos de uma empresa na listagem (08 §4).
type CompanyIndicators struct {
	CompanyID string `json:"company_id"`
	// FactorR só para as monitoradas no PA; Reconciliation só para quem usa a Conciliação.
	FactorR        *FactorRIndicator        `json:"factor_r"`
	Reconciliation *ReconciliationIndicator `json:"reconciliation"`
	// Os alertas críticos: obrigações atrasadas na competência e credenciais vencidas.
	Overdue            int `json:"overdue"`
	CredentialsExpired int `json:"credentials_expired"`
}

// Indicators são os indicadores da listagem na competência.
type Indicators struct {
	Competence string              `json:"competence"`
	Items      []CompanyIndicators `json:"items"`
}

// Indicators monta os indicadores de todas as empresas com algum indicador, com uma consulta por
// fonte (a carteira, a Conciliação por empresa, as atrasadas e as credenciais), sem N+1.
func (s *Service) Indicators(ctx context.Context, c competence.Competence) (Indicators, error) {
	q := store.New(s.pool)
	kid, err := dashboard.CompetenceID(ctx, q, c)
	if err != nil {
		return Indicators{}, err
	}
	byID := map[string]*CompanyIndicators{}
	var order []string
	get := func(id string) *CompanyIndicators {
		it, ok := byID[id]
		if !ok {
			it = &CompanyIndicators{CompanyID: id}
			byID[id] = it
			order = append(order, id)
		}
		return it
	}

	portfolio, err := factorr.PortfolioWith(ctx, q, c, nil)
	var ae *apperr.Error
	if err != nil && (!errors.As(err, &ae) || ae.Code != "SIMPLES_TABLE_NOT_IN_FORCE") {
		return Indicators{}, err
	}
	for _, l := range portfolio.Linhas {
		it := get(l.Company.ID)
		it.FactorR = &FactorRIndicator{Light: l.Semaforo, Action: l.Acao}
	}

	rec, err := overview.Load(ctx, q, c, nil)
	if err != nil {
		return Indicators{}, err
	}
	for _, r := range rec {
		it := get(r.Company.ID)
		it.Reconciliation = &ReconciliationIndicator{State: r.State, BlocksAccounting: r.BlocksAccounting}
	}

	overdue, err := q.CompanyOverdueCounts(ctx, store.CompanyOverdueCountsParams{CompetenceID: kid, Today: clock.Today(s.clock)})
	if err != nil {
		return Indicators{}, err
	}
	for _, o := range overdue {
		get(o.CompanyID).Overdue = int(o.Overdue)
	}

	creds, err := s.credentials.List(ctx, credential.ListParams{})
	if err != nil {
		return Indicators{}, err
	}
	for _, row := range creds.Items {
		n := 0
		for _, ctl := range []credential.Control{row.DigitalCertificate, row.RFBPowerOfAttorney} {
			if ctl.Situation.Code == credential.SituationExpired {
				n++
			}
		}
		if n > 0 {
			get(row.Company.ID).CredentialsExpired = n
		}
	}

	out := Indicators{Competence: c.String(), Items: make([]CompanyIndicators, 0, len(order))}
	for _, id := range order {
		out.Items = append(out.Items, *byID[id])
	}
	return out, nil
}
