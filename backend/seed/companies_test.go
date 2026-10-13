package seed_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/seed"
)

func TestDemoSeed_TwentyCompaniesIdempotent(t *testing.T) {
	pool := testutil.NewDB(t)
	ctx := context.Background()
	clk := testutil.FixedClock(t, "2026-10-01T12:00:00Z")
	for range 2 {
		if err := seed.Reference(ctx, pool, clk, io.Discard); err != nil {
			t.Fatal(err)
		}
		if err := seed.Demo(ctx, pool, clk, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	// As 20 empresas do PRD §37 e a 111 Construtora Horizonte, a do cenário da Conciliação (D-041),
	// com o nome que os documentos do cenário imprimem.
	if n := count(t, pool, `SELECT count(*) FROM companies`); n != seed.DemoCompanyCount || n != 21 {
		t.Fatalf("empresas = %d, want 21", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM companies WHERE internal_code <> $1`, seed.HorizonteCode); n != 20 {
		t.Fatalf("empresas do PRD = %d, want 20", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM companies c JOIN company_tax_regime_history h ON h.company_id = c.id
		WHERE c.internal_code = $1 AND c.legal_name = 'Construtora Horizonte Ltda' AND c.cnpj = '12345678000190'
		  AND h.tax_regime = 'LUCRO_PRESUMIDO' AND h.effective_to_competence IS NULL`, seed.HorizonteCode); n != 1 {
		t.Fatal("a 111 deveria ser a Construtora Horizonte Ltda, no Lucro Presumido")
	}
	if n := count(t, pool, `SELECT count(*) FROM companies WHERE cnpj ~ '^[0-9]{14}$'`); n != 21 {
		t.Fatalf("CNPJs com 14 dígitos = %d", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM users WHERE email LIKE '%@escritorio.exemplo'`); n != 4 {
		t.Fatalf("responsáveis = %d, want 4", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM company_tax_regime_history h JOIN companies c ON c.id = h.company_id WHERE c.internal_code = '205'`); n != 2 {
		t.Fatalf("regimes da 205 = %d, want 2 (troca histórica)", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM company_obligation_overrides o JOIN companies c ON c.id = o.company_id
		JOIN obligation_types t ON t.id = o.obligation_type_id WHERE c.internal_code = '102' AND t.code = 'FISCAL_IPI' AND o.applicability = 'NOT_APPLICABLE'`); n != 1 {
		t.Fatal("102 deveria ter IPI não aplicável")
	}
	if n := count(t, pool, `SELECT count(DISTINCT payroll_type) FROM companies`); n != 3 {
		t.Fatalf("tipos de folha distintos = %d, want 3", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM company_module_settings WHERE NOT enabled`); n == 0 {
		t.Fatal("algumas empresas deveriam ter módulos desativados")
	}

	// A API lista as 21 empresas: 11 Normais (com a 111) e 10 do Simples na competência corrente (10/2026).
	srv := testutil.NewServer(t, pool, clk)
	var all, simples struct {
		Total int `json:"total"`
	}
	srv.Get("/api/v1/companies").MustStatus(t, http.StatusOK).JSON(t, &all)
	srv.Get("/api/v1/companies?tax_regime=SIMPLES_NACIONAL").MustStatus(t, http.StatusOK).JSON(t, &simples)
	if all.Total != 21 || simples.Total != 10 {
		t.Fatalf("GET /companies = %d (Simples %d), want 21 (10)", all.Total, simples.Total)
	}
}
