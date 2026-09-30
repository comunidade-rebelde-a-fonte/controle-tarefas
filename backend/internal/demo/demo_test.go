package demo_test

import (
	"net/http"
	"testing"

	"controle/internal/platform/config"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
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
		if res.Companies != 20 {
			t.Fatalf("empresas após o seed = %d, want 20", res.Companies)
		}
	}
	srv.Get("/api/v1/companies").MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Total != 20 {
		t.Fatalf("GET /companies = %d, want 20", list.Total)
	}
}

func TestDemoSeed_NotAvailableInProduction(t *testing.T) {
	srv := testutil.NewServer(t, seeded.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"),
		func(c *config.Config) { c.Env = config.EnvProduction })
	srv.Post("/api/v1/demo/seed", nil).MustStatus(t, http.StatusNotFound)
}
