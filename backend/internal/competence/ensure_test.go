package competence_test

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/competence"
	comp "controle/internal/platform/competence"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
	"controle/seed"
)

func engine(t *testing.T, pool *pgxpool.Pool) *competence.Engine {
	t.Helper()
	return competence.NewEngine(pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
}

func ensure(t *testing.T, e *competence.Engine, key string) competence.Summary {
	t.Helper()
	s, err := e.Ensure(context.Background(), comp.MustParse(key))
	if err != nil {
		t.Fatalf("Ensure(%s): %v", key, err)
	}
	return s
}

func scalar[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

// instance devolve status, aplicabilidade, conta no progresso e vencimento de uma instância.
func instance(t *testing.T, pool *pgxpool.Pool, code, typeCode, key string) (status string, applicable, counts bool, due *string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT i.status_code, i.applicable, i.counts_in_progress, i.due_date_effective::text
		FROM obligation_instances i
		JOIN companies c ON c.id = i.company_id
		JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN competencies k ON k.id = i.competence_id
		WHERE c.internal_code = $1 AND t.code = $2 AND k.competence_key = $3`, code, typeCode, key).Scan(&status, &applicable, &counts, &due)
	if err != nil {
		t.Fatalf("instância %s/%s/%s: %v", code, typeCode, key, err)
	}
	return
}

func addCompany(t *testing.T, pool *pgxpool.Pool, code, cnpj, start string, end *string, active bool) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, service_end_date, active, payroll_type, city, state, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, $2, $3, $4, $5, 'PAYROLL', 'Curitiba', 'PR', $6, $6) RETURNING id`, code, cnpj, start, end, active, user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_tax_regime_history (company_id, tax_regime, effective_from_competence, created_by)
		VALUES ($1, 'LUCRO_PRESUMIDO', date_trunc('month', $2::date), $3)`, id, start, user.AdminID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO company_module_settings (company_id, module_code, enabled, effective_from_competence, created_by)
		SELECT $1, m, true, date_trunc('month', $2::date), $3 FROM unnest(ARRAY['FISCAL','DECLARATIONS','ACCOUNTING','PAYROLL','CREDENTIALS']) m`, id, start, user.AdminID); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCA005_OnlyActiveInServicePeriod(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	ended := "2026-09-30"
	addCompany(t, pool, "901", "90000000000101", "2025-01-01", nil, false)             // inativa
	addCompany(t, pool, "902", "90000000000202", "2025-01-01", &ended, true)           // atendimento encerrado antes de 10/2026
	addCompany(t, pool, "903", "90000000000303", "2026-11-01", nil, true)              // atendimento começa depois
	lateStart := addCompany(t, pool, "904", "90000000000404", "2026-10-20", nil, true) // começa no meio da competência

	s := ensure(t, engine(t, pool), "2026-10")
	if s.CompaniesProcessed != seed.DemoCompanyCount+1 {
		t.Fatalf("empresas processadas = %d, want %d (as do seed + 904)", s.CompaniesProcessed, seed.DemoCompanyCount+1)
	}
	got := scalar[int](t, pool, `SELECT count(*) FROM company_competence_snapshots s JOIN companies c ON c.id = s.company_id WHERE c.internal_code IN ('901','902','903')`)
	if got != 0 {
		t.Fatalf("empresas fora do atendimento receberam snapshot: %d", got)
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM obligation_instances WHERE company_id = $1`, lateStart); n == 0 {
		t.Fatal("empresa com início no meio da competência deveria receber instâncias")
	}
	if n := scalar[int](t, pool, `SELECT count(DISTINCT company_id) FROM obligation_instances`); n != seed.DemoCompanyCount+1 {
		t.Fatalf("empresas com instâncias = %d, want %d", n, seed.DemoCompanyCount+1)
	}
}

func TestCA006_DisabledModuleNoInstances(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	e := engine(t, pool)
	ensure(t, e, "2026-10")

	// 104 não tem o módulo Folha: nenhuma instância nem período de Folha.
	q := `SELECT count(*) FROM obligation_instances i JOIN companies c ON c.id = i.company_id
	      JOIN obligation_types t ON t.id = i.obligation_type_id WHERE c.internal_code = $1 AND t.primary_module = $2`
	if n := scalar[int](t, pool, q, "104", "PAYROLL"); n != 0 {
		t.Fatalf("104 sem Folha recebeu %d instâncias de Folha", n)
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM company_module_periods p JOIN companies c ON c.id = p.company_id WHERE c.internal_code = '104' AND p.module_code = 'PAYROLL'`); n != 0 {
		t.Fatal("104 sem Folha não deveria ter período de Folha")
	}
	if n := scalar[int](t, pool, q, "104", "FISCAL"); n != 13 {
		t.Fatalf("104 (Normal) instâncias fiscais = %d, want 13 (12 controles + Fechamento)", n)
	}

	// Fiscal desativado para 101 a partir de 11/2026: 11/2026 não recebe controles fiscais.
	if _, err := pool.Exec(context.Background(), `UPDATE company_module_settings SET effective_to_competence = '2026-10-01'
		WHERE company_id = (SELECT id FROM companies WHERE internal_code = '101') AND module_code = 'FISCAL'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO company_module_settings (company_id, module_code, enabled, effective_from_competence, created_by)
		SELECT id, 'FISCAL', false, '2026-11-01', $1 FROM companies WHERE internal_code = '101'`, user.AdminID); err != nil {
		t.Fatal(err)
	}
	ensure(t, e, "2026-11")
	fiscalIn := `SELECT count(*) FROM obligation_instances i JOIN companies c ON c.id = i.company_id
	      JOIN obligation_types t ON t.id = i.obligation_type_id JOIN competencies k ON k.id = i.competence_id
	      WHERE c.internal_code = '101' AND t.primary_module = 'FISCAL' AND k.competence_key = $1`
	if n := scalar[int](t, pool, fiscalIn, "2026-11"); n != 0 {
		t.Fatalf("Fiscal desativado gerou %d instâncias em 11/2026", n)
	}
	if n := scalar[int](t, pool, fiscalIn, "2026-10"); n != 13 {
		t.Fatalf("10/2026 (gerada antes) deveria manter 13 instâncias fiscais, tem %d", n)
	}
}

func TestCA007_NotApplicableInstanceGeneratedVisible(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	ensure(t, engine(t, pool), "2026-10")
	status, applicable, _, _ := instance(t, pool, "102", "FISCAL_IPI", "2026-10")
	if applicable || status != "NOT_APPLICABLE" {
		t.Fatalf("IPI da 102 = %s applicable=%v, want NOT_APPLICABLE/false (visível, fora do denominador)", status, applicable)
	}
	if _, applicable, _, _ := instance(t, pool, "101", "FISCAL_IPI", "2026-10"); !applicable {
		t.Fatal("IPI da 101 deveria ser aplicável")
	}
	// ICMS no Simples: gerado e visível, fora do progresso [RN-010].
	if _, applicable, counts, _ := instance(t, pool, "201", "FISCAL_ICMS", "2026-10"); !applicable || counts {
		t.Fatalf("ICMS da 201: applicable=%v counts=%v", applicable, counts)
	}
	// Sem folha (204): Folha = Sem movimento; FGTS gerado como não aplicável (Q5 provisório).
	if status, applicable, _, _ := instance(t, pool, "204", "PAYROLL_PROCESSING", "2026-10"); status != "NO_MOVEMENT" || !applicable {
		t.Fatalf("Folha da 204 = %s/%v", status, applicable)
	}
	if _, applicable, _, _ := instance(t, pool, "204", "FGTS", "2026-10"); applicable {
		t.Fatal("FGTS da 204 (Sem folha) deveria ser não aplicável")
	}
}

func TestEnsure_DueDatesUseCompanyCalendar(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	// Feriado estadual do PR em 10/11/2026: afeta 101 (Curitiba/PR), não 103 (Joinville/SC).
	if _, err := pool.Exec(context.Background(), `
		WITH cal AS (INSERT INTO business_calendars (name, scope, state) VALUES ('UF PR', 'STATE', 'PR') RETURNING id)
		INSERT INTO holidays (calendar_id, date, name, scope) SELECT id, '2026-11-10', 'Feriado estadual de teste', 'STATE' FROM cal`); err != nil {
		t.Fatal(err)
	}
	ensure(t, engine(t, pool), "2026-10")
	cases := []struct{ company, typ, want string }{
		{"103", "FISCAL_ENTRADAS", "2026-11-10"},   // dia 10, terça-feira útil
		{"101", "FISCAL_ENTRADAS", "2026-11-09"},   // feriado estadual → dia útil anterior
		{"101", "DCTFWEB", "2026-11-13"},           // dia 15 = domingo e feriado → sexta 13
		{"101", "EFD_CONTRIBUICOES", "2026-11-09"}, // 5º dia útil (02/11 Finados não conta)
		{"101", "ACCOUNTING_RECONCILIATION", "2026-11-30"},
	}
	for _, tc := range cases {
		if _, _, _, due := instance(t, pool, tc.company, tc.typ, "2026-10"); due == nil || *due != tc.want {
			t.Errorf("%s %s: vencimento = %v, want %s", tc.company, tc.typ, due, tc.want)
		}
	}
}

type counts struct{ competencies, snapshots, periods, events, instances int }

func snapshotCounts(t *testing.T, pool *pgxpool.Pool) counts {
	t.Helper()
	return counts{
		competencies: scalar[int](t, pool, `SELECT count(*) FROM competencies`),
		snapshots:    scalar[int](t, pool, `SELECT count(*) FROM company_competence_snapshots`),
		periods:      scalar[int](t, pool, `SELECT count(*) FROM company_module_periods`),
		events:       scalar[int](t, pool, `SELECT count(*) FROM company_module_period_events`),
		instances:    scalar[int](t, pool, `SELECT count(*) FROM obligation_instances`),
	}
}

func TestEnsure_IdempotentSequentialAndParallel(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	e := engine(t, pool)
	first := ensure(t, e, "2026-10")
	if first.InstancesCreated == 0 || first.PeriodsCreated == 0 {
		t.Fatalf("primeira execução não gerou nada: %+v", first)
	}
	want := snapshotCounts(t, pool)
	if want.events != want.periods {
		t.Fatalf("cada período deveria ter um evento OPENED: %+v", want)
	}
	for range 2 {
		if s := ensure(t, e, "2026-10"); s.InstancesCreated != 0 || s.CompaniesProcessed != 0 {
			t.Fatalf("reexecução gerou dados: %+v", s)
		}
	}
	if got := snapshotCounts(t, pool); got != want {
		t.Fatalf("após 3 execuções: %+v, want %+v", got, want)
	}

	// 5 execuções em paralelo de uma competência nova produzem o mesmo resultado de uma.
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.Ensure(context.Background(), comp.MustParse("2026-11")); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	got := snapshotCounts(t, pool)
	if got.competencies != 2 || got.snapshots != 2*want.snapshots || got.instances != 2*want.instances || got.events != got.periods {
		t.Fatalf("após execuções paralelas: %+v (base %+v)", got, want)
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM (SELECT company_id, competence_id, obligation_type_id FROM obligation_instances
		GROUP BY 1, 2, 3 HAVING count(*) > 1) d`); n != 0 {
		t.Fatalf("%d instâncias duplicadas", n)
	}
}

func TestEnsure_CompanyCreatedAfterGenerationReceivesInstancesOnNextRun(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	e := engine(t, pool)
	ensure(t, e, "2026-10")
	before := scalar[string](t, pool, `SELECT md5(string_agg(id::text || status_code || version::text || updated_at::text, ',' ORDER BY id)) FROM obligation_instances`)
	id := addCompany(t, pool, "905", "90000000000505", "2025-01-01", nil, true)

	s := ensure(t, e, "2026-10")
	if s.CompaniesProcessed != 1 || s.InstancesCreated == 0 {
		t.Fatalf("segunda execução: %+v, want só a empresa nova", s)
	}
	after := scalar[string](t, pool, `SELECT md5(string_agg(id::text || status_code || version::text || updated_at::text, ',' ORDER BY id))
		FROM obligation_instances WHERE company_id <> $1`, id)
	if before != after {
		t.Fatal("instâncias das demais empresas foram alteradas")
	}
}

func TestEnsure_SnapshotPreservesRegimeOfTheCompetence(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	e := engine(t, pool)
	ensure(t, e, "2025-06")
	ensure(t, e, "2026-10")
	q := `SELECT s.tax_regime || '/' || s.regime_group FROM company_competence_snapshots s JOIN companies c ON c.id = s.company_id
	      JOIN competencies k ON k.id = s.competence_id WHERE c.internal_code = '205' AND k.competence_key = $1`
	if got := scalar[string](t, pool, q, "2025-06"); got != "LUCRO_PRESUMIDO/NORMAL" {
		t.Fatalf("205 em 06/2025 = %s", got)
	}
	if got := scalar[string](t, pool, q, "2026-10"); got != "SIMPLES_NACIONAL/SIMPLES" {
		t.Fatalf("205 em 10/2026 = %s", got)
	}
}
