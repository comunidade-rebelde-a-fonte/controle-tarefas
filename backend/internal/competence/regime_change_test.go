package competence_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

// companyInstances devolve "tipo=status" das instâncias da empresa na competência, ordenado.
func companyInstances(t *testing.T, pool *pgxpool.Pool, code, key string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT t.code || '=' || i.status_code || '/' || i.applicable || '/' || i.counts_in_progress || '/' || i.version
		FROM obligation_instances i
		JOIN companies c ON c.id = i.company_id
		JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN competencies k ON k.id = i.competence_id
		WHERE c.internal_code = $1 AND k.competence_key = $2
		ORDER BY t.code`, code, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func hasType(items []string, typeCode string) bool {
	return slices.ContainsFunc(items, func(s string) bool { return len(s) > len(typeCode) && s[:len(typeCode)+1] == typeCode+"=" })
}

func TestCA004_FutureRegimeChange(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	clk := testutil.FixedClock(t, "2026-09-15T12:00:00Z")
	srv := testutil.NewServer(t, pool, clk)

	// Gera 09/2026 com a 201 no Simples.
	srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)
	snapshotQ := `SELECT s.tax_regime || '/' || s.regime_group FROM company_competence_snapshots s
		JOIN companies c ON c.id = s.company_id JOIN competencies k ON k.id = s.competence_id
		WHERE c.internal_code = '201' AND k.competence_key = $1`
	sep := companyInstances(t, pool, "201", "2026-09")
	if !hasType(sep, "PGDAS") || !hasType(sep, "FISCAL_SIMPLES_NACIONAL") || hasType(sep, "EFD_CONTRIBUICOES") || hasType(sep, "FISCAL_PIS") {
		t.Fatalf("09/2026 deveria usar o catálogo do Simples: %v", sep)
	}

	// Registra Lucro Presumido a partir de 10/2026 [UC-04].
	companyID := scalar[string](t, pool, `SELECT id FROM companies WHERE internal_code = '201'`)
	srv.Post("/api/v1/companies/"+companyID+"/tax-regimes", map[string]any{"tax_regime": "LUCRO_PRESUMIDO", "effective_from": "2026-10"}).
		MustStatus(t, http.StatusCreated)

	// 09/2026 continua intacta mesmo com nova execução do motor (RN-004).
	srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)
	if got := companyInstances(t, pool, "201", "2026-09"); !slices.Equal(got, sep) {
		t.Fatalf("09/2026 foi alterada após a mudança de regime:\nantes  %v\ndepois %v", sep, got)
	}

	// Gera 10/2026.
	clk.Set(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)

	// Instâncias e snapshot de 09/2026 intactos: regime e tipos do Simples [CA-004].
	if got := companyInstances(t, pool, "201", "2026-09"); !slices.Equal(got, sep) {
		t.Fatalf("09/2026 foi alterada após gerar 10/2026:\nantes  %v\ndepois %v", sep, got)
	}
	if got := scalar[string](t, pool, snapshotQ, "2026-09"); got != "SIMPLES_NACIONAL/SIMPLES" {
		t.Fatalf("snapshot 09/2026 = %s", got)
	}

	// 10/2026 usa o catálogo Normal.
	if got := scalar[string](t, pool, snapshotQ, "2026-10"); got != "LUCRO_PRESUMIDO/NORMAL" {
		t.Fatalf("snapshot 10/2026 = %s", got)
	}
	oct := companyInstances(t, pool, "201", "2026-10")
	for _, want := range []string{"EFD_CONTRIBUICOES", "SPED_FISCAL", "MIT", "FISCAL_PIS", "FISCAL_COFINS", "FISCAL_IRPJ", "FISCAL_CSLL"} {
		if !hasType(oct, want) {
			t.Fatalf("10/2026 deveria ter %s: %v", want, oct)
		}
	}
	for _, notWant := range []string{"PGDAS", "DESTDA", "FISCAL_SIMPLES_NACIONAL"} {
		if hasType(oct, notWant) {
			t.Fatalf("10/2026 não deveria ter %s: %v", notWant, oct)
		}
	}
}
