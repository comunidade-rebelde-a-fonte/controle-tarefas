package seed_test

import (
	"context"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/testutil"
	"controle/seed"
)

func seededPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testutil.NewDB(t)
	clk := testutil.FixedClock(t, "2026-10-01T12:00:00Z")
	for range 2 { // idempotência
		if err := seed.Run(context.Background(), pool, clk, seed.ReferenceSteps(), io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func count(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCatalogSeed_SimplesHasEightFiscalControlsSevenInProgress(t *testing.T) {
	pool := seededPool(t)
	const q = `SELECT count(*) FROM obligation_type_regime_rules r JOIN obligation_types t ON t.id = r.obligation_type_id
	           WHERE r.tax_regime = $1 AND t.primary_module = 'FISCAL' AND NOT t.is_closing_stage AND r.active`
	if n := count(t, pool, q, "SIMPLES_NACIONAL"); n != 8 {
		t.Fatalf("controles fiscais do Simples = %d, want 8 [§14.2]", n)
	}
	if n := count(t, pool, q+` AND r.counts_in_progress`, "SIMPLES_NACIONAL"); n != 7 {
		t.Fatalf("controles do Simples no progresso = %d, want 7 [§14.4]", n)
	}
	var icmsCounts bool
	if err := pool.QueryRow(context.Background(), `SELECT r.counts_in_progress FROM obligation_type_regime_rules r
		JOIN obligation_types t ON t.id = r.obligation_type_id WHERE t.code = 'FISCAL_ICMS' AND r.tax_regime = 'SIMPLES_NACIONAL'`).Scan(&icmsCounts); err != nil {
		t.Fatal(err)
	}
	if icmsCounts {
		t.Fatal("ICMS no Simples não pode contar no progresso [RN-010]")
	}
	for _, regime := range []string{"LUCRO_PRESUMIDO", "LUCRO_REAL", "LUCRO_ARBITRADO", "IMUNE_ISENTA"} {
		if n := count(t, pool, q+` AND r.counts_in_progress`, regime); n != 12 {
			t.Fatalf("%s: controles fiscais no progresso = %d, want 12 [§13.3]", regime, n)
		}
	}
	// Fechamento existe nos dois grupos e fica fora dos percentuais (R3).
	if n := count(t, pool, `SELECT count(*) FROM obligation_type_regime_rules r JOIN obligation_types t ON t.id = r.obligation_type_id
		WHERE t.code = 'FISCAL_FECHAMENTO' AND NOT r.counts_in_progress`); n != 5 {
		t.Fatalf("regras do Fechamento fora do progresso = %d, want 5", n)
	}
}

func TestCatalogSeed_DisplayContextsAndDependencies(t *testing.T) {
	pool := seededPool(t)
	ctxOf := `SELECT count(*) FROM obligation_display_contexts c JOIN obligation_types t ON t.id = c.obligation_type_id WHERE t.code = $1 AND c.active`
	if n := count(t, pool, ctxOf, "ESOCIAL"); n != 2 {
		t.Fatalf("contextos do eSocial = %d, want 2 (Folha e DCTFWeb)", n)
	}
	if n := count(t, pool, ctxOf, "MIT"); n != 2 {
		t.Fatalf("contextos do MIT = %d, want 2", n)
	}
	// REINF Retidos/Lucros: primary DECLARATIONS, exibidos só na DCTFWeb (D5).
	if n := count(t, pool, `SELECT count(*) FROM obligation_display_contexts c JOIN obligation_types t ON t.id = c.obligation_type_id
		WHERE t.code IN ('REINF_RETIDOS','REINF_LUCROS') AND c.module_code = 'DCTFWEB' AND t.primary_module = 'DECLARATIONS'`); n != 2 {
		t.Fatalf("REINF Retidos/Lucros na DCTFWeb = %d, want 2", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_display_contexts WHERE module_code = 'DCTFWEB'`); n != 5 {
		t.Fatalf("colunas da DCTFWeb = %d, want 5 [§16.2]", n)
	}
	hard := `SELECT count(*) FROM obligation_dependencies d JOIN obligation_types t ON t.id = d.obligation_type_id
	         WHERE d.dependency_mode = $1 AND t.primary_module = $2 AND d.active`
	if n := count(t, pool, hard, "HARD", "ACCOUNTING"); n != 5 {
		t.Fatalf("arestas HARD do Contábil = %d, want 5", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_dependencies d JOIN obligation_types t ON t.id = d.obligation_type_id
		WHERE t.code = 'DCTFWEB' AND d.dependency_mode = 'SOFT'`); n != 4 {
		t.Fatalf("dependências SOFT da DCTFWeb = %d, want 4", n)
	}
}

func TestCatalogSeed_CatalogShape(t *testing.T) {
	pool := seededPool(t)
	cases := map[string]int{
		"FISCAL": 14, "DECLARATIONS": 10, "PAYROLL": 4, "ACCOUNTING": 6,
	}
	for module, want := range cases {
		if n := count(t, pool, `SELECT count(*) FROM obligation_types WHERE primary_module = $1`, module); n != want {
			t.Errorf("%s: %d tipos, want %d", module, n, want)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_types WHERE due_date_rule_id IS NULL`); n != 0 {
		t.Fatalf("%d tipos sem regra de vencimento", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_types WHERE status_when_no_payroll IS NOT NULL`); n != 4 {
		t.Fatalf("tipos com status 'sem folha' = %d, want 4", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM obligation_types WHERE primary_module = 'DECLARATIONS' AND NOT (allows_protocol AND allows_transmission_date)`); n != 0 {
		t.Fatal("declarações devem permitir protocolo e data de transmissão")
	}
}
