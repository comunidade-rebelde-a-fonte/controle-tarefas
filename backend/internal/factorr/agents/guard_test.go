package agents_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai/tracer"
	"controle/internal/factorr/agents"
	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/store"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

func isNoSimulation(err error) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == "RECOMMENDATION_WITHOUT_SIMULATION"
}

// Guarda do decide (T-608 da origem): "corrigir" só com simulação persistida com esse veredito;
// sem ela, a corrida termina em erro e nada da decisão fica.
func TestGuard_CorrectRequiresPersistedSimulation(t *testing.T) {
	pool := seeded.NewDB(t)
	ctx := context.Background()
	clk := clock.NewFixed(time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC), time.UTC)
	tr := tracer.New(clk, jobs.NewQueue(clk, 3))
	q := store.New(pool)
	if err := agents.EnsureSimulationForCorrect(ctx, q, map[string]any{"veredito": "corrigir"}); !isNoSimulation(err) {
		t.Fatalf("corrigir sem simulation_id = %v", err)
	}
	if err := agents.EnsureSimulationForCorrect(ctx, q, map[string]any{"acao": "corrigir", "simulation_id": "00000000-0000-0000-0000-000000000000"}); !isNoSimulation(err) {
		t.Fatalf("corrigir com simulação inexistente = %v", err)
	}
	if err := agents.EnsureSimulationForCorrect(ctx, q, map[string]any{"veredito": "nao_forcar"}); err != nil {
		t.Fatalf("não forçar não exige simulação: %v", err)
	}
	// Simulação gravada com outro veredito também não autoriza o "corrigir".
	var companyID, simID string
	_, err := tr.Do(ctx, pool, tracer.Spec{Agent: "consultor", Trigger: "ficha"}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		if err := tx.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
			VALUES ('G1', 'Guarda Ltda', '11222333000181', '2024-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID).Scan(&companyID); err != nil {
			return tracer.Result{}, err
		}
		err := tx.QueryRow(ctx, `INSERT INTO factor_r_simulations (company_id, pa, meta, fracao_pro_labore, inss, irrf, horizonte_meses,
			piso_economia_anual, status, veredito, reforco_12m, reforco_mensal, custo_total, parametros_json, resultado_json, trace_id)
			VALUES ($1, '2026-09-01', 0.3, 1, 0.11, 0.275, 12, 6000, 'ok', 'nao_forcar', 0, 0, 0, '{}', '{}', $2) RETURNING id`,
			companyID, run.TraceID()).Scan(&simID)
		return tracer.Result{Status: tracer.StatusOK}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := agents.EnsureSimulationForCorrect(ctx, q, map[string]any{"veredito": "corrigir", "simulation_id": simID}); !isNoSimulation(err) {
		t.Fatalf("simulação com outro veredito = %v", err)
	}
	// Na corrida: o decide falha, a corrida vira erro com o código e nada da decisão fica gravado.
	trace, err := tr.Do(ctx, pool, tracer.Spec{Agent: "consultor", Trigger: "chat"}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		decision := map[string]any{"recomendacao": "corrigir"}
		if err := agents.Decide(ctx, run, store.New(tx), decision); err != nil {
			return tracer.Result{}, err
		}
		return tracer.Result{Status: tracer.StatusOK, Decision: decision}, run.RecordDecision(ctx, "consultor_resposta", decision)
	})
	if !isNoSimulation(err) {
		t.Fatalf("corrida = %v", err)
	}
	var status, code string
	if err := pool.QueryRow(ctx, `SELECT status, error_code FROM ai_runs WHERE trace_id = $1`, trace).Scan(&status, &code); err != nil ||
		status != "error" || code != "RECOMMENDATION_WITHOUT_SIMULATION" {
		t.Fatalf("corrida de erro = %s/%s, %v", status, code, err)
	}
	var decisions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1`, trace).Scan(&decisions); err != nil || decisions != 0 {
		t.Fatalf("decisões gravadas = %d, %v", decisions, err)
	}
	_ = testutil.SaoPaulo
}

func TestRoutineDueAndWeekStart(t *testing.T) {
	loc := testutil.SaoPaulo
	cases := []struct {
		now  time.Time
		want bool
	}{
		{time.Date(2026, 9, 20, 7, 0, 0, 0, loc), true},   // domingo 7h
		{time.Date(2026, 9, 20, 6, 59, 0, 0, loc), false}, // domingo antes da hora
		{time.Date(2026, 9, 20, 23, 30, 0, 0, loc), true}, // ainda domingo
		{time.Date(2026, 9, 21, 7, 0, 0, 0, loc), false},  // segunda
	}
	for _, c := range cases {
		if got := agents.RoutineDue(c.now, 7); got != c.want {
			t.Errorf("RoutineDue(%s) = %v", c.now, got)
		}
	}
	// A semana começa na segunda 00:00 (o domingo é o último dia dela).
	if got := agents.WeekStart(time.Date(2026, 9, 20, 10, 0, 0, 0, loc)); !got.Equal(time.Date(2026, 9, 14, 0, 0, 0, 0, loc)) {
		t.Errorf("weekStart(domingo) = %s", got)
	}
	if got := agents.WeekStart(time.Date(2026, 9, 14, 0, 0, 0, 0, loc)); !got.Equal(time.Date(2026, 9, 14, 0, 0, 0, 0, loc)) {
		t.Errorf("weekStart(segunda) = %s", got)
	}
}
