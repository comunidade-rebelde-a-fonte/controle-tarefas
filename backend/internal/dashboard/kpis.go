// Package dashboard agrega a competência para a gestão [PRD §24]: KPIs, cards por módulo,
// vencimentos de credenciais, progresso por responsável, "Exige atenção", comparação
// histórica e backlog. Cada número traz o drill-down (rota e filtros) da lista que o explica.
package dashboard

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/company"
	"controle/internal/credential"
	"controle/internal/obligation"
	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/store"
)

// Módulos dos cards, na ordem de exibição [PRD §24.3].
var Modules = []string{"FISCAL", "DECLARATIONS", "PAYROLL", "ACCOUNTING"}

var moduleLabels = map[string]string{"FISCAL": "Fiscal", "DECLARATIONS": "Declarações", "PAYROLL": "Folha", "ACCOUNTING": "Contábil"}

// Rota da grade de cada módulo na interface.
var modulePaths = map[string]string{"FISCAL": "/fiscal", "DECLARATIONS": "/declarations", "PAYROLL": "/payroll", "ACCOUNTING": "/accounting"}

// Classes de obrigação usadas no drill-down da lista de obrigações (state=...).
const (
	StateApplicable = "applicable"
	StateDone       = "done"
	StateInProgress = "in_progress"
	StateNotStarted = "not_started"
	StatePending    = "pending"
	StateOverdue    = "overdue"
)

// Filters são os filtros do dashboard [PRD §24.4, §24.5]: os filtros de empresa (company.Filter).
type Filters struct {
	RegimeGroup   *string `json:"regime_group"`   // NORMAL | SIMPLES
	TaxRegime     *string `json:"tax_regime"`     // regime específico
	ResponsibleID *string `json:"responsible_id"` // responsável do módulo (snapshot)
}

// Drilldown é a rota da interface que explica o número, com os filtros na query [PRD §24.7].
type Drilldown struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query"`
}

// Metric é um número clicável.
type Metric struct {
	Value     int        `json:"value"`
	Drilldown *Drilldown `json:"drilldown"`
}

// Counts são as contagens de obrigações [PRD §24.2]. Concluídas, em andamento, não iniciadas e
// pendentes particionam as aplicáveis que contam no progresso.
type Counts struct {
	Applicable          Metric  `json:"applicable"`
	Done                Metric  `json:"done"`
	InProgress          Metric  `json:"in_progress"`
	NotStarted          Metric  `json:"not_started"`
	Pending             Metric  `json:"pending"`
	Overdue             Metric  `json:"overdue"`
	OperationalProgress float64 `json:"operational_progress"`
	FinalCompletion     float64 `json:"final_completion"`
}

// KPIs são os indicadores principais [PRD §24.2].
type KPIs struct {
	Counts
	// Empresas com ao menos um módulo pronto para fechar (todos os itens finais, módulo aberto).
	ReadyToClose Metric `json:"ready_to_close"`
	// Empresas com módulo aberto em competência anterior (plan R8).
	PreviousOpen Metric `json:"previous_open"`
}

// ModuleCard é o resumo de um módulo [PRD §24.3].
type ModuleCard struct {
	Module string `json:"module"`
	Label  string `json:"label"`
	Counts
	Companies    int    `json:"companies"`
	ReadyToClose Metric `json:"ready_to_close"`
	Closed       Metric `json:"closed"`
}

// CredentialCard resume os vencimentos de procurações ou certificados [PRD §24.3, §19.3].
type CredentialCard struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Expired Metric `json:"expired"`
	// Vencem dentro da maior faixa configurada (DueSoonDays).
	DueSoon     Metric `json:"due_soon"`
	DueSoonDays int    `json:"due_soon_days"`
	Current     Metric `json:"current"`
	NoDate      Metric `json:"no_date"`
}

// UserRef identifica o responsável; nil = sem responsável definido.
type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ResponsibleProgress é o progresso de um responsável num módulo [PRD §24.5].
type ResponsibleProgress struct {
	User                *UserRef `json:"user"`
	Module              string   `json:"module"`
	Applicable          int      `json:"applicable"`
	Done                int      `json:"done"`
	Pending             int      `json:"pending"`
	Overdue             int      `json:"overdue"`
	OperationalProgress float64  `json:"operational_progress"`
	FinalCompletion     float64  `json:"final_completion"`
}

// Dashboard é a resposta de GET /dashboard.
type Dashboard struct {
	Competence   comp.Competence       `json:"competence"`
	Today        civil.Date            `json:"today"`
	Filters      Filters               `json:"filters"`
	KPIs         KPIs                  `json:"kpis"`
	Modules      []ModuleCard          `json:"modules"`
	Credentials  []CredentialCard      `json:"credentials"`
	Responsibles []ResponsibleProgress `json:"responsibles"`
	// As seções da consolidação (PRD §8; TASK-024, D-038), com os mesmos filtros.
	FactorR        FactorRSection        `json:"factor_r"`
	Reconciliation ReconciliationSection `json:"reconciliation"`
}

// Service monta os agregados do dashboard.
type Service struct {
	pool        *pgxpool.Pool
	clock       clock.Clock
	credentials *credential.Service
}

// NewService cria o serviço. credentials alimenta o card de vencimentos.
func NewService(pool *pgxpool.Pool, clk clock.Clock, credentials *credential.Service) *Service {
	return &Service{pool: pool, clock: clk, credentials: credentials}
}

// competenceID resolve a competência gerada (404 se ainda não gerada).
func competenceID(ctx context.Context, q *store.Queries, c comp.Competence) (string, error) {
	k, err := q.GetCompetenceByKey(ctx, c.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperr.NotFound("COMPETENCE_NOT_FOUND", fmt.Sprintf("A competência %s ainda não foi gerada.", c.Display()))
		}
		return "", err
	}
	return k.ID, nil
}

// company são os filtros de empresa, para as seções Fator R e Conciliação (D-038).
func (f Filters) company() company.Filter { return company.Filter(f) }

// baseQuery são os filtros comuns a todos os drill-downs.
func (f Filters) baseQuery(c comp.Competence) map[string]string {
	q := map[string]string{"competence": c.String()}
	if f.RegimeGroup != nil {
		q["regime_group"] = *f.RegimeGroup
	}
	if f.TaxRegime != nil {
		q["tax_regime"] = *f.TaxRegime
	}
	if f.ResponsibleID != nil {
		q["responsible_id"] = *f.ResponsibleID
	}
	return q
}

func with(base map[string]string, kv ...string) map[string]string {
	out := make(map[string]string, len(base)+len(kv)/2)
	for k, v := range base {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

// obligationsLink leva à lista de obrigações da competência filtrada pela classe.
func obligationsLink(base map[string]string, state string, module string) *Drilldown {
	q := with(base, "state", state)
	if module != "" {
		q["module"] = module
	}
	return &Drilldown{Path: "/obligations", Query: q}
}

// gridLink leva à grade do módulo com os filtros derivados (os nomes da URL da grade).
func gridLink(module string, f Filters, c comp.Competence, kv ...string) *Drilldown {
	q := map[string]string{"competence": c.String()}
	if f.TaxRegime != nil {
		q["tax_regime"] = *f.TaxRegime
	}
	if f.ResponsibleID != nil {
		q["responsible_id"] = *f.ResponsibleID
	}
	if module == "FISCAL" && f.RegimeGroup != nil {
		if *f.RegimeGroup == "SIMPLES" {
			q["group"] = "simples"
		} else {
			q["group"] = "normal"
		}
	}
	return &Drilldown{Path: modulePaths[module], Query: with(q, kv...)}
}

func counts(r store.DashboardModuleCountsRow, base map[string]string, module string) Counts {
	p := obligation.ProgressFromCounts(int(r.Applicable), int(r.OperationalDone), int(r.Done), int(r.Pending))
	return Counts{
		Applicable:          Metric{int(r.Applicable), obligationsLink(base, StateApplicable, module)},
		Done:                Metric{int(r.Done), obligationsLink(base, StateDone, module)},
		InProgress:          Metric{int(r.InProgress), obligationsLink(base, StateInProgress, module)},
		NotStarted:          Metric{int(r.NotStarted), obligationsLink(base, StateNotStarted, module)},
		Pending:             Metric{int(r.Pending), obligationsLink(base, StatePending, module)},
		Overdue:             Metric{int(r.Overdue), obligationsLink(base, StateOverdue, module)},
		OperationalProgress: p.OperationalPct,
		FinalCompletion:     p.FinalPct,
	}
}

// Build monta o dashboard da competência [PRD §24.1-§24.5].
func (s *Service) Build(ctx context.Context, c comp.Competence, f Filters) (Dashboard, error) {
	today := clock.Today(s.clock)
	d := Dashboard{Competence: c, Today: civil.Date{Time: today}, Filters: f, Modules: []ModuleCard{},
		Credentials: []CredentialCard{}, Responsibles: []ResponsibleProgress{}}
	q := store.New(s.pool)
	kid, err := competenceID(ctx, q, c)
	if err != nil {
		return d, err
	}
	base := f.baseQuery(c)

	rows, err := q.DashboardModuleCounts(ctx, store.DashboardModuleCountsParams{
		Today: today, CompetenceID: kid, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return d, err
	}
	byModule := map[string]store.DashboardModuleCountsRow{}
	var total store.DashboardModuleCountsRow
	for _, r := range rows {
		byModule[r.Module] = r
		total.Applicable += r.Applicable
		total.Done += r.Done
		total.OperationalDone += r.OperationalDone
		total.InProgress += r.InProgress
		total.NotStarted += r.NotStarted
		total.Pending += r.Pending
		total.Overdue += r.Overdue
	}
	d.KPIs.Counts = counts(total, base, "")

	periods, err := q.DashboardCompanyModules(ctx, store.DashboardCompanyModulesParams{
		CompetenceID: kid, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return d, err
	}
	type moduleAgg struct{ companies, ready, closed int }
	agg := map[string]*moduleAgg{}
	readyCompanies := map[string]bool{}
	for _, p := range periods {
		a := agg[p.ModuleCode]
		if a == nil {
			a = &moduleAgg{}
			agg[p.ModuleCode] = a
		}
		a.companies++
		switch {
		case p.State == "CLOSED":
			a.closed++
		case obligation.ProgressFromCounts(int(p.Denominator), 0, int(p.FinalDone), 0).ReadyToClose:
			a.ready++
			readyCompanies[p.CompanyID] = true
		}
	}
	d.KPIs.ReadyToClose = Metric{Value: len(readyCompanies)}

	prev, err := q.DashboardPreviousOpenCompanies(ctx, store.DashboardPreviousOpenCompaniesParams{
		CompetenceKey: c.String(), RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return d, err
	}
	d.KPIs.PreviousOpen = Metric{Value: int(prev), Drilldown: &Drilldown{Path: "/backlog", Query: with(base, "before", c.String())}}
	delete(d.KPIs.PreviousOpen.Drilldown.Query, "competence")

	for _, m := range Modules {
		a := agg[m]
		if a == nil {
			a = &moduleAgg{}
		}
		d.Modules = append(d.Modules, ModuleCard{
			Module: m, Label: moduleLabels[m], Counts: counts(byModule[m], base, m), Companies: a.companies,
			ReadyToClose: Metric{a.ready, gridLink(m, f, c, "ready_to_close", "true")},
			Closed:       Metric{a.closed, gridLink(m, f, c, "period_state", "CLOSED")},
		})
	}

	if d.Credentials, err = s.credentialCards(ctx, f); err != nil {
		return d, err
	}

	resp, err := q.DashboardResponsibles(ctx, store.DashboardResponsiblesParams{
		Today: today, CompetenceID: kid, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID,
	})
	if err != nil {
		return d, err
	}
	for _, r := range resp {
		p := obligation.ProgressFromCounts(int(r.Applicable), int(r.OperationalDone), int(r.Done), int(r.Pending))
		var u *UserRef
		if r.UserID != nil && r.UserName != nil {
			u = &UserRef{ID: *r.UserID, Name: *r.UserName}
		}
		d.Responsibles = append(d.Responsibles, ResponsibleProgress{
			User: u, Module: r.Module, Applicable: int(r.Applicable), Done: int(r.Done), Pending: int(r.Pending),
			Overdue: int(r.Overdue), OperationalProgress: p.OperationalPct, FinalCompletion: p.FinalPct,
		})
	}

	if d.FactorR, err = factorRSection(ctx, q, c, f.company()); err != nil {
		// Sem tabela do Simples vigente no PA, a seção Fator R fica indisponível, com o motivo; o
		// resto do Dashboard segue.
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != codeSimplesTableNotInForce {
			return d, err
		}
		d.FactorR = FactorRSection{PA: c, Unavailable: &ae.Message, SavingsAtPlay: MoneyMetric{Value: "0"}, Priorities: []FactorRPriority{}}
	}
	if d.Reconciliation, err = reconciliationSection(ctx, q, c, f.company()); err != nil {
		return d, err
	}
	return d, nil
}

// codeSimplesTableNotInForce é a recusa do motor do Fator R sem tabela do Simples no PA.
const codeSimplesTableNotInForce = "SIMPLES_TABLE_NOT_IN_FORCE"

var credentialLabels = map[string]string{
	credential.TypePowerOfAttorney: "Procurações RFB",
	credential.TypeCertificate:     "Certificados digitais",
}

// credentialCards conta a situação de validade das procurações e dos certificados das empresas
// com o módulo, pelas faixas configuradas [PRD §19.3]. Filtra pelo regime atual da empresa.
func (s *Service) credentialCards(ctx context.Context, f Filters) ([]CredentialCard, error) {
	list, err := s.credentials.List(ctx, credential.ListParams{})
	if err != nil {
		return nil, err
	}
	maxDays := 0
	for _, t := range list.Thresholds {
		maxDays = max(maxDays, t)
	}
	cards := make([]CredentialCard, 0, len(credential.Types))
	for _, typ := range credential.Types {
		card := CredentialCard{Type: typ, Label: credentialLabels[typ], DueSoonDays: maxDays}
		link := func(filter string) *Drilldown {
			q := map[string]string{"type": typ}
			if filter != "" {
				q["filter"] = filter
			}
			return &Drilldown{Path: "/credentials", Query: q}
		}
		for _, row := range list.Items {
			if !regimeMatches(row.Company.TaxRegime, f) {
				continue
			}
			ctl := row.RFBPowerOfAttorney
			if typ == credential.TypeCertificate {
				ctl = row.DigitalCertificate
			}
			switch ctl.Situation.Code {
			case credential.SituationExpired:
				card.Expired.Value++
			case credential.SituationDueSoon:
				card.DueSoon.Value++
			case credential.SituationNoDate:
				card.NoDate.Value++
			default:
				card.Current.Value++
			}
		}
		card.Expired.Drilldown = link(credential.FilterExpired)
		card.DueSoon.Drilldown = link(fmt.Sprintf("due_%d", maxDays))
		card.NoDate.Drilldown = link(credential.FilterNoDate)
		card.Current.Drilldown = link("")
		cards = append(cards, card)
	}
	return cards, nil
}

func regimeMatches(regime string, f Filters) bool {
	if f.TaxRegime != nil && regime != *f.TaxRegime {
		return false
	}
	if f.RegimeGroup != nil {
		simples := regime == "SIMPLES_NACIONAL"
		if (*f.RegimeGroup == "SIMPLES") != simples {
			return false
		}
	}
	return true
}
