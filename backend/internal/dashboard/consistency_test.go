package dashboard_test

import (
	"context"
	"io"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/loadgen"
	"controle/internal/testutil"
)

// independentKPIs recalcula os KPIs com SQL simples, direto das tabelas (sem a view
// v_obligation_instances nem as consultas do dashboard) [T-M5-09, PRD §54].
func independentKPIs(t *testing.T, pool *pgxpool.Pool, key, today string) map[string]int {
	t.Helper()
	const base = `
		FROM obligation_instances i
		JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
		JOIN competencies k ON k.id = i.competence_id
		WHERE k.competence_key = $1`
	const relevant = ` AND i.applicable AND i.counts_in_progress AND NOT d.excluded_from_denominator`
	queries := map[string]string{
		"applicable":       `SELECT count(*)` + base + relevant,
		"done":             `SELECT count(*)` + base + relevant + ` AND d.is_final`,
		"operational_done": `SELECT count(*)` + base + relevant + ` AND d.counts_as_operational_progress`,
		"in_progress":      `SELECT count(*)` + base + relevant + ` AND d.counts_as_started AND NOT d.is_final AND NOT d.is_blocking`,
		"not_started":      `SELECT count(*)` + base + relevant + ` AND NOT d.counts_as_started AND NOT d.is_final AND NOT d.is_blocking`,
		"pending":          `SELECT count(*)` + base + relevant + ` AND d.is_blocking`,
		"overdue":          `SELECT count(*)` + base + ` AND i.applicable AND NOT d.is_final AND i.due_date_effective < $2::date`,
		"ready_to_close": `
			SELECT count(DISTINCT p.company_id) FROM company_module_periods p
			JOIN competencies k ON k.id = p.competence_id
			WHERE k.competence_key = $1 AND p.state = 'OPEN' AND NOT EXISTS (
			    SELECT 1 FROM obligation_instances i
			    JOIN obligation_types t ON t.id = i.obligation_type_id
			    JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
			    WHERE i.company_id = p.company_id AND i.competence_id = p.competence_id AND t.primary_module = p.module_code
			      AND i.applicable AND i.counts_in_progress AND NOT d.excluded_from_denominator AND NOT d.is_final)`,
		"previous_open": `
			SELECT count(DISTINCT p.company_id) FROM company_module_periods p
			JOIN competencies k ON k.id = p.competence_id
			WHERE k.competence_key < $1 AND p.state = 'OPEN'`,
	}
	out := map[string]int{}
	for name, sql := range queries {
		args := []any{key}
		if name == "overdue" {
			args = append(args, today)
		}
		var n int
		if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name] = n
	}
	return out
}

func assertConsistent(t *testing.T, srv *testutil.Server, key, today string) {
	t.Helper()
	want := independentKPIs(t, srv.Pool, key, today)
	d := get(t, srv, "competence="+key)
	k := d.KPIs
	got := map[string]int{
		"applicable": k.Applicable.Value, "done": k.Done.Value, "in_progress": k.InProgress.Value, "not_started": k.NotStarted.Value,
		"pending": k.Pending.Value, "overdue": k.Overdue.Value, "ready_to_close": k.ReadyToClose.Value, "previous_open": k.PreviousOpen.Value,
	}
	for name, v := range got {
		if v != want[name] {
			t.Errorf("%s %s: dashboard %d × SQL independente %d", key, name, v, want[name])
		}
	}
	if want["applicable"] == 0 {
		t.Fatalf("%s sem obrigações aplicáveis na massa", key)
	}
	if op := float64(want["operational_done"]) / float64(want["applicable"]); math.Abs(op-k.OperationalProgress) > 1e-9 {
		t.Errorf("%s progresso operacional: %v × %v", key, k.OperationalProgress, op)
	}
	if fc := float64(want["done"]) / float64(want["applicable"]); math.Abs(fc-k.FinalCompletion) > 1e-9 {
		t.Errorf("%s conclusão final: %v × %v", key, k.FinalCompletion, fc)
	}
	// A lista do drill-down de cada número tem o mesmo total.
	for _, state := range []string{"applicable", "done", "in_progress", "not_started", "pending", "overdue"} {
		if n := backlog(t, srv, "competence="+key+"&state="+state+"&page_size=1").Total; n != got[state] {
			t.Errorf("%s lista %s = %d × KPI %d", key, state, n, got[state])
		}
	}
}

func TestDashboardConsistency_DemoSeed(t *testing.T) {
	srv := server(t) // hoje = 01/10/2026
	for _, key := range []string{"2026-10", "2026-09"} {
		assertConsistent(t, srv, key, "2026-10-01")
	}
}

func TestDashboardConsistency_LoadgenSeed42(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := testutil.FixedClock(t, "2026-10-20T12:00:00Z")
	if _, err := loadgen.Generate(context.Background(), pool, clk, 80, 3, 42, io.Discard); err != nil {
		t.Fatal(err)
	}
	srv := testutil.NewServer(t, pool, clk)
	for _, key := range []string{"2026-10", "2026-09"} {
		assertConsistent(t, srv, key, "2026-10-20")
	}
}
