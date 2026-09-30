package company_test

import (
	"context"
	"net/http"
	"testing"

	"controle/internal/company"
	"controle/internal/platform/competence"
	"controle/internal/store"
)

type regimeDTO struct {
	TaxRegime     string  `json:"tax_regime"`
	RegimeGroup   string  `json:"regime_group"`
	EffectiveFrom string  `json:"effective_from"`
	EffectiveTo   *string `json:"effective_to"`
	CreatedBy     struct {
		Name string `json:"name"`
	} `json:"created_by"`
}

type regimeList struct {
	Items []regimeDTO `json:"items"`
}

// Exemplo do PRD §8.2: Simples de 01/2025 a 06/2026; Lucro Presumido a partir de 07/2026.
func TestRegime_ChangeSimplesToPresumido(t *testing.T) {
	srv := newServer(t)
	p := companyPayload("205", "Estação Criativa Publicidade Ltda", validCnpj("001000050001"))
	p["tax_regime"] = "SIMPLES_NACIONAL"
	var c companyDTO
	srv.Post("/api/v1/companies", p).MustStatus(t, http.StatusCreated).JSON(t, &c)

	var tl regimeList
	srv.Post("/api/v1/companies/"+c.ID+"/tax-regimes", map[string]any{"tax_regime": "LUCRO_PRESUMIDO", "effective_from": "2026-07"}).
		MustStatus(t, http.StatusCreated).JSON(t, &tl)
	if len(tl.Items) != 2 {
		t.Fatalf("linha do tempo = %+v", tl.Items)
	}
	simples, presumido := tl.Items[0], tl.Items[1]
	if simples.TaxRegime != "SIMPLES_NACIONAL" || simples.EffectiveFrom != "2025-01" || simples.EffectiveTo == nil || *simples.EffectiveTo != "2026-06" {
		t.Fatalf("Simples = %+v", simples)
	}
	if presumido.TaxRegime != "LUCRO_PRESUMIDO" || presumido.RegimeGroup != "NORMAL" || presumido.EffectiveFrom != "2026-07" || presumido.EffectiveTo != nil {
		t.Fatalf("Presumido = %+v", presumido)
	}
	if presumido.CreatedBy.Name != "Administrador" {
		t.Fatalf("usuário responsável = %q", presumido.CreatedBy.Name)
	}

	// GET devolve a mesma linha do tempo.
	var got regimeList
	srv.Get("/api/v1/companies/"+c.ID+"/tax-regimes").MustStatus(t, http.StatusOK).JSON(t, &got)
	if len(got.Items) != 2 {
		t.Fatalf("GET = %+v", got.Items)
	}

	// RegimeAt antes e depois da troca.
	q := store.New(srv.Pool)
	cases := map[string]string{"2024-12": "", "2025-01": "SIMPLES_NACIONAL", "2026-06": "SIMPLES_NACIONAL", "2026-07": "LUCRO_PRESUMIDO", "2030-01": "LUCRO_PRESUMIDO"}
	for comp, want := range cases {
		got, err := company.RegimeAt(context.Background(), q, c.ID, competence.MustParse(comp))
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("RegimeAt(%s) = %q, want %q", comp, got, want)
		}
	}

	// A empresa passa a exibir o regime vigente na competência corrente (10/2026).
	srv.Get("/api/v1/companies/"+c.ID).MustStatus(t, http.StatusOK).JSON(t, &c)
	if *c.CurrentTaxRegime != "LUCRO_PRESUMIDO" {
		t.Fatalf("regime vigente = %s", *c.CurrentTaxRegime)
	}
}

func TestRegime_RejectsInvalidEffectiveFrom(t *testing.T) {
	srv := newServer(t)
	var c companyDTO
	srv.Post("/api/v1/companies", companyPayload("206", "Fênix Ltda", validCnpj("001000060001"))).MustStatus(t, http.StatusCreated).JSON(t, &c)
	path := "/api/v1/companies/" + c.ID + "/tax-regimes"

	// Igual ou anterior ao início da vigência aberta (01/2025) → 422.
	for _, from := range []string{"2025-01", "2024-06"} {
		if code := srv.Post(path, map[string]any{"tax_regime": "LUCRO_REAL", "effective_from": from}).
			MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "INVALID_EFFECTIVE_FROM" {
			t.Fatalf("%s: code = %s", from, code)
		}
	}
	srv.Post(path, map[string]any{"tax_regime": "LUCRO_PRESUMIDO", "effective_from": "2026-11"}).MustStatus(t, http.StatusUnprocessableEntity)
	srv.Post(path, map[string]any{"tax_regime": "LUCRO_REAL", "effective_from": "11/2026"}).MustStatus(t, http.StatusBadRequest)
	srv.Post(path, map[string]any{"tax_regime": "MEI", "effective_from": "2026-11"}).MustStatus(t, http.StatusBadRequest)
	srv.Post("/api/v1/companies/7b0f8d7e-0000-4000-8000-000000000000/tax-regimes",
		map[string]any{"tax_regime": "LUCRO_REAL", "effective_from": "2026-11"}).MustStatus(t, http.StatusNotFound)

	// Trocas sucessivas mantêm a linha do tempo contígua.
	srv.Post(path, map[string]any{"tax_regime": "LUCRO_REAL", "effective_from": "2026-11"}).MustStatus(t, http.StatusCreated)
	var tl regimeList
	srv.Post(path, map[string]any{"tax_regime": "LUCRO_ARBITRADO", "effective_from": "2027-03"}).MustStatus(t, http.StatusCreated).JSON(t, &tl)
	if len(tl.Items) != 3 || *tl.Items[0].EffectiveTo != "2026-10" || *tl.Items[1].EffectiveTo != "2027-02" || tl.Items[2].EffectiveTo != nil {
		t.Fatalf("linha do tempo = %+v", tl.Items)
	}
}
