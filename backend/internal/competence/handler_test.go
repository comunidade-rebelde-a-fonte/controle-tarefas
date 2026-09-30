package competence_test

import (
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

func TestCompetencies_LatestTriggersGenerationAndListCounts(t *testing.T) {
	pool := seeded.NewDemoDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))

	// Acessar a competência corrente inexistente dispara a geração (3º gatilho).
	var latest struct{ Competence, Current string }
	srv.Get("/api/v1/competencies/latest").MustStatus(t, http.StatusOK).JSON(t, &latest)
	if latest.Competence != "2026-10" || latest.Current != "2026-10" {
		t.Fatalf("latest = %+v", latest)
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM obligation_instances`); n == 0 {
		t.Fatal("a competência corrente deveria ter sido gerada")
	}

	var list struct {
		Items []struct {
			Competence  string `json:"competence"`
			Companies   int    `json:"companies"`
			Instances   int    `json:"instances"`
			OpenPeriods int    `json:"open_periods"`
		} `json:"items"`
	}
	srv.Get("/api/v1/competencies").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 1 || list.Items[0].Companies != 20 || list.Items[0].OpenPeriods == 0 {
		t.Fatalf("competências = %+v", list.Items)
	}

	var s struct {
		Competence       string `json:"competence"`
		InstancesCreated int    `json:"instances_created"`
	}
	srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK).JSON(t, &s)
	if s.Competence != "2026-10" || s.InstancesCreated != 0 {
		t.Fatalf("ensure-current = %+v (deveria ser idempotente)", s)
	}

	// Com 10/2026 gerada, a vigência sugerida para alterações de cadastro passa a 11/2026 (R6).
	var companies struct{ Items []struct{ ID string } }
	srv.Get("/api/v1/companies?code=101").MustStatus(t, http.StatusOK).JSON(t, &companies)
	var modules struct {
		SuggestedEffectiveFrom string `json:"suggested_effective_from"`
	}
	srv.Get("/api/v1/companies/"+companies.Items[0].ID+"/modules").MustStatus(t, http.StatusOK).JSON(t, &modules)
	if modules.SuggestedEffectiveFrom != "2026-11" {
		t.Fatalf("vigência sugerida = %s, want 2026-11", modules.SuggestedEffectiveFrom)
	}
}
