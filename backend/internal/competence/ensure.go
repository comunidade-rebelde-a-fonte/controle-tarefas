package competence

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/calendar"
	"controle/internal/company"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/store"
	"controle/internal/user"
)

// PeriodModules são os módulos com fechamento mensal (Procurações/Certificados não tem período).
var PeriodModules = []string{company.ModuleFiscal, company.ModuleDeclarations, company.ModuleAccounting, company.ModulePayroll}

// Summary resume uma execução de Ensure.
type Summary struct {
	Competence         comp.Competence `json:"competence"`
	CompaniesProcessed int             `json:"companies_processed"`
	PeriodsCreated     int             `json:"periods_created"`
	InstancesCreated   int             `json:"instances_created"`
}

// Engine gera competências: snapshot, períodos de módulo e instâncias.
type Engine struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewEngine cria o motor.
func NewEngine(pool *pgxpool.Pool, clk clock.Clock) *Engine { return &Engine{pool: pool, clock: clk} }

// Current devolve a competência corrente no fuso de negócio.
func (e *Engine) Current() comp.Competence { return comp.Current(e.clock) }

// EnsureCurrent garante a competência corrente [PRD §10.2, RN-005].
func (e *Engine) EnsureCurrent(ctx context.Context) (Summary, error) {
	return e.Ensure(ctx, e.Current())
}

// Ensure garante que a competência existe e que toda empresa elegível tem snapshot,
// períodos de módulo e instâncias [PRD §10.1, §39]. É transacional e idempotente:
//   - pg_advisory_xact_lock por competência serializa execuções concorrentes;
//   - só empresas ativas, dentro do período de atendimento e ainda sem snapshot na
//     competência são processadas (R5): competências já geradas não são reescritas [RN-004];
//   - inserções usam ON CONFLICT DO NOTHING como segunda barreira contra duplicação.
func (e *Engine) Ensure(ctx context.Context, c comp.Competence) (Summary, error) {
	s := Summary{Competence: c}
	err := db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockCompetence(ctx, c.String()); err != nil {
			return fmt.Errorf("lock da competência: %w", err)
		}
		if err := q.InsertCompetence(ctx, store.InsertCompetenceParams{CompetenceKey: c.String(), FirstDay: c.FirstDay()}); err != nil {
			return fmt.Errorf("criar competência: %w", err)
		}
		row, err := q.GetCompetenceByKey(ctx, c.String())
		if err != nil {
			return err
		}
		companies, err := q.EligibleCompaniesWithoutSnapshot(ctx, store.EligibleCompaniesWithoutSnapshotParams{
			CompetenceID: row.ID, FirstDay: c.FirstDay(), LastDay: c.LastDay(),
		})
		if err != nil || len(companies) == 0 {
			return err
		}
		g, err := e.loadGenerationData(ctx, q, c)
		if err != nil {
			return err
		}
		return e.generate(ctx, tx, row.ID, c, companies, g, &s)
	})
	return s, err
}

// generationData é o que a geração precisa, carregado uma única vez por execução.
type generationData struct {
	catalog   []CatalogType
	rules     map[string]calendar.Rule // due_date_rule_id → regra
	typeRule  map[string]string        // obligation_type_id → due_date_rule_id
	regimes   map[string]string        // company_id → regime vigente
	modules   map[string][]string      // company_id → módulos habilitados
	overrides map[string]map[string]string
	calendars *calendarIndex
}

func (e *Engine) loadGenerationData(ctx context.Context, q *store.Queries, c comp.Competence) (*generationData, error) {
	g := &generationData{
		rules: map[string]calendar.Rule{}, typeRule: map[string]string{}, regimes: map[string]string{},
		modules: map[string][]string{}, overrides: map[string]map[string]string{},
	}
	var err error
	if g.catalog, err = loadCatalog(ctx, q, g.typeRule); err != nil {
		return nil, err
	}
	dueRules, err := q.ListDueDateRules(ctx)
	if err != nil {
		return nil, err
	}
	ruleCalendar := map[string]*string{}
	for _, r := range dueRules {
		g.rules[r.ID] = calendar.Rule{MonthOffset: int(r.MonthOffset), BaseDay: int(r.BaseDay), DayType: r.DayType, Adjustment: r.NonBusinessDayAdjustment}
		ruleCalendar[r.ID] = r.CalendarID
	}
	first := c.FirstDay()
	regimes, err := q.RegimesAtCompetence(ctx, first)
	if err != nil {
		return nil, err
	}
	for _, r := range regimes {
		g.regimes[r.CompanyID] = r.TaxRegime
	}
	mods, err := q.ModulesAtCompetence(ctx, first)
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		g.modules[m.CompanyID] = append(g.modules[m.CompanyID], m.ModuleCode)
	}
	ovs, err := q.OverridesAtCompetence(ctx, first)
	if err != nil {
		return nil, err
	}
	for _, o := range ovs {
		if g.overrides[o.CompanyID] == nil {
			g.overrides[o.CompanyID] = map[string]string{}
		}
		g.overrides[o.CompanyID][o.ObligationTypeID] = o.Applicability
	}
	g.calendars, err = loadCalendars(ctx, q, c, ruleCalendar)
	return g, err
}

// loadCatalog monta o catálogo no formato do resolvedor (Resolve).
func loadCatalog(ctx context.Context, q *store.Queries, typeRule map[string]string) ([]CatalogType, error) {
	types, err := q.EngineObligationTypes(ctx)
	if err != nil {
		return nil, err
	}
	codes, err := q.EngineStatusCodes(ctx)
	if err != nil {
		return nil, err
	}
	bySet := map[string][]string{}
	for _, s := range codes {
		bySet[s.StatusSetID] = append(bySet[s.StatusSetID], s.Code)
	}
	rules, err := q.EngineRegimeRules(ctx)
	if err != nil {
		return nil, err
	}
	byType := map[string]map[string]RegimeRule{}
	for _, r := range rules {
		if byType[r.ObligationTypeID] == nil {
			byType[r.ObligationTypeID] = map[string]RegimeRule{}
		}
		byType[r.ObligationTypeID][r.TaxRegime] = RegimeRule{DefaultApplicable: r.DefaultApplicable, CountsInProgress: r.CountsInProgress, Active: r.Active}
	}
	out := make([]CatalogType, 0, len(types))
	for _, t := range types {
		ct := CatalogType{
			ID: t.ID, Code: t.Code, PrimaryModule: t.PrimaryModule, Active: t.Active,
			InitialStatusCode: t.InitialStatusCode, StatusCodes: bySet[t.StatusSetID], RegimeRules: byType[t.ID],
		}
		if t.StatusWhenNoPayroll != nil {
			ct.StatusWhenNoPayroll = *t.StatusWhenNoPayroll
		}
		if t.DueDateRuleID != nil {
			typeRule[t.ID] = *t.DueDateRuleID
		}
		out = append(out, ct)
	}
	return out, nil
}

// calendarIndex monta o calendário efetivo de cada empresa: nacional + UF + município
// (+ calendário específico da regra, se houver) [PRD §21.2].
type calendarIndex struct {
	national     []string
	byState      map[string][]string
	byCity       map[string][]string
	holidays     map[string][]time.Time // calendar_id → datas
	ruleCalendar map[string]*string
	cache        map[string]calendar.BusinessCalendar
}

func loadCalendars(ctx context.Context, q *store.Queries, c comp.Competence, ruleCalendar map[string]*string) (*calendarIndex, error) {
	idx := &calendarIndex{
		byState: map[string][]string{}, byCity: map[string][]string{}, holidays: map[string][]time.Time{},
		ruleCalendar: ruleCalendar, cache: map[string]calendar.BusinessCalendar{},
	}
	cals, err := q.EngineCalendars(ctx)
	if err != nil {
		return nil, err
	}
	for _, cal := range cals {
		switch cal.Scope {
		case "NATIONAL":
			idx.national = append(idx.national, cal.ID)
		case "STATE":
			if cal.State != nil {
				idx.byState[*cal.State] = append(idx.byState[*cal.State], cal.ID)
			}
		case "MUNICIPAL":
			if cal.State != nil && cal.City != nil {
				k := cityKey(*cal.State, *cal.City)
				idx.byCity[k] = append(idx.byCity[k], cal.ID)
			}
		}
	}
	// Vencimentos caem até 12 meses após a competência; a busca de dia útil pode avançar mais um mês.
	hs, err := q.EngineHolidaysInRange(ctx, store.EngineHolidaysInRangeParams{FromDate: c.FirstDay(), ToDate: c.AddMonths(14).LastDay()})
	if err != nil {
		return nil, err
	}
	for _, h := range hs {
		idx.holidays[h.CalendarID] = append(idx.holidays[h.CalendarID], h.Date)
	}
	return idx, nil
}

func cityKey(state, city string) string {
	return state + "|" + strings.ToLower(strings.TrimSpace(city))
}

func (idx *calendarIndex) forCompany(state, city *string, ruleID string) calendar.BusinessCalendar {
	var extra string
	if rc := idx.ruleCalendar[ruleID]; rc != nil {
		extra = *rc
	}
	key := fmt.Sprintf("%v|%v|%s", deref(state), strings.ToLower(deref(city)), extra)
	if cal, ok := idx.cache[key]; ok {
		return cal
	}
	ids := slices.Clone(idx.national)
	if state != nil {
		ids = append(ids, idx.byState[*state]...)
		if city != nil {
			ids = append(ids, idx.byCity[cityKey(*state, *city)]...)
		}
	}
	if extra != "" {
		ids = append(ids, extra)
	}
	sets := make([][]time.Time, 0, len(ids))
	for _, id := range ids {
		sets = append(sets, idx.holidays[id])
	}
	cal := calendar.NewBusinessCalendar(sets...)
	idx.cache[key] = cal
	return cal
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

const (
	insertSnapshotSQL = `
INSERT INTO company_competence_snapshots (
    company_id, competence_id, tax_regime, regime_group, payroll_type, factor_r, active_modules,
    fiscal_responsible_user_id, accounting_responsible_user_id, payroll_responsible_user_id,
    declarations_responsible_user_id, city, state)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (company_id, competence_id) DO NOTHING`

	insertPeriodSQL = `
WITH p AS (
    INSERT INTO company_module_periods (company_id, competence_id, module_code)
    VALUES ($1, $2, $3)
    ON CONFLICT (company_id, competence_id, module_code) DO NOTHING
    RETURNING id)
INSERT INTO company_module_period_events (company_module_period_id, event_type, user_id, created_at)
SELECT id, 'OPENED', $4, $5 FROM p`

	insertInstanceSQL = `
INSERT INTO obligation_instances (
    company_id, competence_id, obligation_type_id, status_code, applicable, counts_in_progress,
    regime_snapshot, due_date_original, due_date_effective)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
ON CONFLICT (company_id, competence_id, obligation_type_id) DO NOTHING`
)

type batchKind int

const (
	kindSnapshot batchKind = iota
	kindPeriod
	kindInstance
)

func (e *Engine) generate(ctx context.Context, tx pgx.Tx, competenceID string, c comp.Competence,
	companies []store.EligibleCompaniesWithoutSnapshotRow, g *generationData, s *Summary) error {
	actor := user.FromContext(ctx)
	now := e.clock.Now()
	batch := &pgx.Batch{}
	var kinds []batchKind
	for _, co := range companies {
		regime := g.regimes[co.ID]
		if regime == "" {
			continue // sem regime vigente na competência: nada a gerar
		}
		mods := g.modules[co.ID]
		if mods == nil {
			mods = []string{}
		}
		s.CompaniesProcessed++
		batch.Queue(insertSnapshotSQL, co.ID, competenceID, regime, company.RegimeGroup(regime), co.PayrollType, co.FactorR, mods,
			co.FiscalResponsibleUserID, co.AccountingResponsibleUserID, co.PayrollResponsibleUserID, co.DeclarationsResponsibleUserID,
			co.City, co.State)
		kinds = append(kinds, kindSnapshot)
		for _, m := range PeriodModules {
			if slices.Contains(mods, m) {
				batch.Queue(insertPeriodSQL, co.ID, competenceID, m, actor, now)
				kinds = append(kinds, kindPeriod)
			}
		}
		planned := Resolve(Snapshot{TaxRegime: regime, PayrollType: co.PayrollType, ActiveModules: mods}, g.catalog, g.overrides[co.ID])
		for _, p := range planned {
			var due *time.Time
			if ruleID, ok := g.typeRule[p.ObligationTypeID]; ok {
				if rule, ok := g.rules[ruleID]; ok {
					d, err := calendar.DueDate(rule, c, g.calendars.forCompany(co.State, co.City, ruleID))
					if err != nil {
						return fmt.Errorf("vencimento de %s: %w", p.Code, err)
					}
					due = &d
				}
			}
			batch.Queue(insertInstanceSQL, co.ID, competenceID, p.ObligationTypeID, p.StatusCode, p.Applicable, p.CountsInProgress, p.RegimeSnapshot, due)
			kinds = append(kinds, kindInstance)
		}
	}
	br := tx.SendBatch(ctx, batch)
	for _, k := range kinds {
		tag, err := br.Exec()
		if err != nil {
			_ = br.Close()
			return fmt.Errorf("gerar competência %s: %w", c, err)
		}
		switch k {
		case kindPeriod:
			s.PeriodsCreated += int(tag.RowsAffected())
		case kindInstance:
			s.InstancesCreated += int(tag.RowsAffected())
		}
	}
	return br.Close()
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
