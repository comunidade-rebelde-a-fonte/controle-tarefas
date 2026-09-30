package obligation_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/competence"
	"controle/internal/obligation"
	comp "controle/internal/platform/competence"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

func generated(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	pool := seeded.NewCompaniesDB(t)
	e := competence.NewEngine(pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
	if _, err := e.Ensure(context.Background(), comp.MustParse("2026-10")); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM competencies WHERE competence_key = '2026-10'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return pool, id
}

func setStatus(t *testing.T, pool *pgxpool.Pool, company, typeFilter, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		UPDATE obligation_instances i SET status_code = $3
		FROM companies c, obligation_types t
		WHERE c.id = i.company_id AND t.id = i.obligation_type_id AND c.internal_code = $1 AND t.code LIKE $2 AND i.applicable`,
		company, typeFilter, status); err != nil {
		t.Fatal(err)
	}
}

func progressOf(t *testing.T, pool *pgxpool.Pool, compID, company, module string, today string) obligation.ModuleProgressRow {
	t.Helper()
	d, _ := time.Parse(time.DateOnly, today)
	rows, err := obligation.ModuleProgress(context.Background(), pool, compID, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM companies WHERE internal_code = $1`, company).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.CompanyID == id && r.Module == module {
			return r
		}
	}
	t.Fatalf("sem progresso para %s/%s", company, module)
	return obligation.ModuleProgressRow{}
}

func TestCA007_NotApplicableKeepsPercent_Integration(t *testing.T) {
	pool, compID := generated(t)
	// 102 tem IPI não aplicável: o Fiscal tem 11 controles relevantes (Fechamento fora, R3).
	setStatus(t, pool, "102", "FISCAL_%", "CLOSED")
	setStatus(t, pool, "102", "FISCAL_FECHAMENTO", "NOT_STARTED")
	p := progressOf(t, pool, compID, "102", "FISCAL", "2026-10-15")
	if p.Denominator != 11 || p.FinalDone != 11 {
		t.Fatalf("102 Fiscal: denominador %d, finais %d, want 11/11 (IPI fora do percentual)", p.Denominator, p.FinalDone)
	}
}

func TestCA012_OverdueComputed_Integration(t *testing.T) {
	pool, compID := generated(t)
	// ENTRADAS vence em 10/11/2026 (dia 10 do mês seguinte, provisório Q1).
	before := progressOf(t, pool, compID, "103", "FISCAL", "2026-11-10")
	after := progressOf(t, pool, compID, "103", "FISCAL", "2026-11-11")
	if after.Overdue <= before.Overdue {
		t.Fatalf("atrasos em 10/11 = %d, em 11/11 = %d: deveriam aumentar sem mudança de status", before.Overdue, after.Overdue)
	}
	setStatus(t, pool, "103", "FISCAL_ENTRADAS", "CLOSED")
	if closed := progressOf(t, pool, compID, "103", "FISCAL", "2026-11-11"); closed.Overdue != after.Overdue-1 {
		t.Fatalf("concluir ENTRADAS deveria tirar 1 atraso: %d → %d", after.Overdue, closed.Overdue)
	}
}

func TestCA014_ReadyToClose_Integration(t *testing.T) {
	pool, compID := generated(t)
	// Simples (201): 8 controles, ICMS fora do progresso (R4) → 7 relevantes; Fechamento fora (R2).
	setStatus(t, pool, "201", "FISCAL_%", "CLOSED")
	setStatus(t, pool, "201", "FISCAL_ICMS", "NOT_STARTED")
	setStatus(t, pool, "201", "FISCAL_FECHAMENTO", "NOT_STARTED")
	p := progressOf(t, pool, compID, "201", "FISCAL", "2026-10-15")
	if p.Denominator != 7 || p.FinalDone != 7 {
		t.Fatalf("201 Fiscal: %d/%d, want 7/7 (pronto para fechar)", p.FinalDone, p.Denominator)
	}
	if p.NearestDeadline == nil {
		t.Fatal("ICMS e Fechamento em aberto ainda têm prazo crítico")
	}
}

// A agregação em SQL (grades/dashboard) bate com a função pura Aggregate.
func TestModuleProgress_SQLMatchesAggregate(t *testing.T) {
	pool, compID := generated(t)
	setStatus(t, pool, "101", "FISCAL_E%", "CLOSED")
	setStatus(t, pool, "101", "FISCAL_S%", "DOWNLOADED")
	setStatus(t, pool, "104", "FISCAL_P%", "MISSING_DOCUMENTS")
	setStatus(t, pool, "205", "ACCOUNTING_MOVEMENT_RECEIVED", "OK")
	rows, err := pool.Query(context.Background(), `
		SELECT company_id, primary_module, applicable, counts_in_progress, counts_as_operational_progress, is_final,
		       is_blocking, excluded_from_denominator
		FROM v_obligation_instances WHERE competence_id = $1`, compID)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string][]obligation.Item{}
	for rows.Next() {
		var company, module string
		var it obligation.Item
		if err := rows.Scan(&company, &module, &it.Applicable, &it.CountsInProgress, &it.CountsAsProgress, &it.IsFinal, &it.IsBlocking, &it.ExcludedFromDenominator); err != nil {
			t.Fatal(err)
		}
		groups[company+module] = append(groups[company+module], it)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sqlRows, err := obligation.ModuleProgress(context.Background(), pool, compID, time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range sqlRows {
		if p := obligation.Aggregate(groups[r.CompanyID+r.Module]); p != r.Progress() {
			t.Fatalf("%s/%s: SQL %+v × Go %+v", r.CompanyID, r.Module, r.Progress(), p)
		}
	}
}
