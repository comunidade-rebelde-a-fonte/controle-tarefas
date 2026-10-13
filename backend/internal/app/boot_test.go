package app_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"controle/internal/app"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/seed"
)

// Critério da T-M2-05: a API iniciada com o relógio em 01/10/2026 cria 10/2026 sem chamada manual.
func TestBoot_CreatesCurrentCompetenceWithoutManualCall(t *testing.T) {
	pool := seeded.NewCompaniesDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := app.New(app.Deps{
		Config: testutil.TestConfig(t),
		Pool:   pool,
		Clock:  testutil.FixedClock(t, "2026-10-01T12:00:00Z"),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	a.Boot(ctx)

	var key string
	var companies, instances int
	if err := pool.QueryRow(ctx, `
		SELECT k.competence_key,
		       (SELECT count(*) FROM company_competence_snapshots s WHERE s.competence_id = k.id),
		       (SELECT count(*) FROM obligation_instances i WHERE i.competence_id = k.id)
		FROM competencies k`).Scan(&key, &companies, &instances); err != nil {
		t.Fatal(err)
	}
	if key != "2026-10" || companies != seed.DemoCompanyCount || instances == 0 {
		t.Fatalf("competência %s com %d empresas e %d instâncias", key, companies, instances)
	}
}
