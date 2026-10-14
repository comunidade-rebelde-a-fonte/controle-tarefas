package demo_test

import (
	"net/http"
	"testing"

	"controle/internal/platform/config"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/seed"
)

func TestDemoSeed_LoadsTwentyCompaniesOnEmptyDatabase(t *testing.T) {
	srv := testutil.NewServer(t, seeded.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
	var list struct {
		Total int `json:"total"`
	}
	srv.Get("/api/v1/companies").MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Total != 0 {
		t.Fatalf("banco deveria começar vazio, tem %d empresas", list.Total)
	}

	var res struct {
		Companies int `json:"companies"`
	}
	for range 2 { // idempotente
		srv.Post("/api/v1/demo/seed", nil).MustStatus(t, http.StatusOK).JSON(t, &res)
		// As 20 do PRD e a 111 Construtora Horizonte (D-041).
		if res.Companies != seed.DemoCompanyCount {
			t.Fatalf("empresas após o seed = %d, want %d", res.Companies, seed.DemoCompanyCount)
		}
	}
	srv.Get("/api/v1/companies").MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Total != seed.DemoCompanyCount {
		t.Fatalf("GET /companies = %d, want %d", list.Total, seed.DemoCompanyCount)
	}
}

func TestDemoSeed_NotAvailableInProduction(t *testing.T) {
	srv := testutil.NewServer(t, seeded.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"),
		func(c *config.Config) { c.Env = config.EnvProduction })
	srv.Post("/api/v1/demo/seed", nil).MustStatus(t, http.StatusNotFound)
}
