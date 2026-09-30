package seed_test

import (
	"context"
	"io"
	"testing"

	"controle/internal/testutil"
	"controle/seed"
)

// Critérios da T-M2-06: após o seed de demonstração há backlog, dados para comparar com a
// competência anterior e ao menos um evento REOPENED; rodar de novo não altera nada.
func TestDemoHistory_BacklogComparisonAndReopen(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	clk := testutil.FixedClock(t, "2026-10-15T12:00:00Z") // corrente 10/2026; histórico 07, 08 e 09/2026
	if err := seed.Reference(ctx, pool, clk, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := seed.Demo(ctx, pool, clk, io.Discard); err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, `SELECT count(*) FROM competencies`); n != 4 {
		t.Fatalf("competências = %d, want 4", n)
	}
	backlog := count(t, pool, `
		SELECT count(*) FROM obligation_instances i
		JOIN competencies k ON k.id = i.competence_id
		JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
		WHERE k.competence_key < '2026-10' AND i.applicable AND NOT d.is_final`)
	if backlog == 0 {
		t.Fatal("backlog vazio: deveria haver obrigações abertas em competências anteriores")
	}
	finalPrev := count(t, pool, `
		SELECT count(*) FROM obligation_instances i JOIN competencies k ON k.id = i.competence_id
		JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
		WHERE k.competence_key = '2026-09' AND d.is_final`)
	if finalPrev == 0 {
		t.Fatal("competência anterior sem obrigações concluídas para comparar")
	}
	if n := count(t, pool, `SELECT count(*) FROM company_module_period_events WHERE event_type = 'REOPENED' AND reason IS NOT NULL`); n < 1 {
		t.Fatal("deveria existir ao menos um evento REOPENED com motivo")
	}
	if n := count(t, pool, `SELECT count(*) FROM company_module_periods WHERE state = 'CLOSED'`); n == 0 {
		t.Fatal("deveria haver módulos fechados no histórico")
	}
	// Uma prorrogação (competência anterior) e dois vencimentos antecipados (atrasadas na
	// corrente, para o "Exige atenção").
	if n := count(t, pool, `SELECT count(*) FROM obligation_instances WHERE due_date_override_reason IS NOT NULL`); n != 3 {
		t.Fatalf("overrides de vencimento = %d, want 3", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_instances i JOIN competencies k ON k.id = i.competence_id
		WHERE k.competence_key = '2026-10' AND i.due_date_override_reason = '`+seed.AttentionDueReason+`'
		AND i.due_date_effective < '2026-10-15'`); n != 2 {
		t.Fatalf("atrasadas de demonstração na corrente = %d, want 2", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_instances WHERE status_code = 'ERROR'`); n != 1 {
		t.Fatalf("Fechamentos com Erro = %d, want 1", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_status_history WHERE change_source = 'SYSTEM'`); n == 0 {
		t.Fatal("histórico SYSTEM ausente")
	}
	// Sequência contábil respeitada: nenhuma etapa OK com anterior aplicável não final.
	if n := count(t, pool, `
		SELECT count(*) FROM obligation_instances i
		JOIN obligation_dependencies dep ON dep.obligation_type_id = i.obligation_type_id AND dep.dependency_mode = 'HARD'
		JOIN obligation_instances p ON p.company_id = i.company_id AND p.competence_id = i.competence_id
		     AND p.obligation_type_id = dep.prerequisite_obligation_type_id
		WHERE i.status_code = 'OK' AND p.applicable AND p.status_code NOT IN ('OK', 'NO_MOVEMENT')`); n != 0 {
		t.Fatalf("%d etapas contábeis concluídas fora de ordem", n)
	}

	// Idempotência: rodar de novo não muda nada.
	before := count(t, pool, `SELECT count(*) FROM obligation_status_history`)
	if err := seed.Demo(ctx, pool, clk, io.Discard); err != nil {
		t.Fatal(err)
	}
	if after := count(t, pool, `SELECT count(*) FROM obligation_status_history`); after != before {
		t.Fatalf("histórico mudou na segunda execução: %d → %d", before, after)
	}
}
