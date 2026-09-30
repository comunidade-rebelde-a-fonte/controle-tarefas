package company_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"controle/internal/company"
	"controle/internal/platform/competence"
	"controle/internal/store"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

func newSeededServer(t *testing.T) *testutil.Server {
	t.Helper()
	return testutil.NewServer(t, seeded.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
}

type moduleStateDTO struct {
	Module   string `json:"module"`
	Enabled  bool   `json:"enabled"`
	Timeline []struct {
		Enabled       bool    `json:"enabled"`
		EffectiveFrom string  `json:"effective_from"`
		EffectiveTo   *string `json:"effective_to"`
	} `json:"timeline"`
}

type modulesDTO struct {
	ReferenceCompetence    string           `json:"reference_competence"`
	SuggestedEffectiveFrom string           `json:"suggested_effective_from"`
	Items                  []moduleStateDTO `json:"items"`
}

func TestModules_ValidityAndPersistence(t *testing.T) {
	srv := newSeededServer(t)
	var c companyDTO
	srv.Post("/api/v1/companies", companyPayload("103", "Gama Indústria Ltda", validCnpj("000000030001"))).MustStatus(t, http.StatusCreated).JSON(t, &c)

	var m modulesDTO
	srv.Get("/api/v1/companies/"+c.ID+"/modules").MustStatus(t, http.StatusOK).JSON(t, &m)
	if len(m.Items) != 5 || m.SuggestedEffectiveFrom != "2026-10" || m.ReferenceCompetence != "2026-10" {
		t.Fatalf("módulos = %+v", m)
	}

	path := "/api/v1/companies/" + c.ID + "/modules/FISCAL"
	srv.Put(path, map[string]any{"enabled": false, "effective_from": "2026-11"}).MustStatus(t, http.StatusOK).JSON(t, &m)
	fiscal := m.Items[slices.IndexFunc(m.Items, func(i moduleStateDTO) bool { return i.Module == "FISCAL" })]
	if !fiscal.Enabled || len(fiscal.Timeline) != 2 || *fiscal.Timeline[0].EffectiveTo != "2026-10" || fiscal.Timeline[1].Enabled {
		t.Fatalf("Fiscal = %+v", fiscal)
	}

	q := store.New(srv.Pool)
	at := func(comp string) []string {
		mods, err := company.ModulesAt(context.Background(), q, c.ID, competence.MustParse(comp))
		if err != nil {
			t.Fatal(err)
		}
		return mods
	}
	if !slices.Contains(at("2026-10"), "FISCAL") {
		t.Fatal("Fiscal deveria estar ativo em 10/2026")
	}
	for _, comp := range []string{"2026-11", "2027-06"} {
		if slices.Contains(at(comp), "FISCAL") {
			t.Fatalf("Fiscal deveria estar inativo em %s", comp)
		}
	}

	// Vigência anterior ao início da configuração aberta → 422; mesmo valor → sem alteração.
	srv.Put(path, map[string]any{"enabled": true, "effective_from": "2026-09"}).MustStatus(t, http.StatusUnprocessableEntity)
	srv.Put(path, map[string]any{"enabled": false, "effective_from": "2027-01"}).MustStatus(t, http.StatusOK)
	// Mesma vigência da configuração aberta: corrige o valor sem criar novo intervalo.
	srv.Put(path, map[string]any{"enabled": true, "effective_from": "2026-11"}).MustStatus(t, http.StatusOK).JSON(t, &m)
	if !slices.Contains(at("2026-12"), "FISCAL") {
		t.Fatal("correção na mesma vigência deveria religar o Fiscal")
	}
	srv.Put("/api/v1/companies/"+c.ID+"/modules/DCTFWEB", map[string]any{"enabled": true, "effective_from": "2026-11"}).MustStatus(t, http.StatusNotFound)
	srv.Put(path, map[string]any{"enabled": true}).MustStatus(t, http.StatusBadRequest)
}

type overridesDTO struct {
	TaxRegime *string `json:"tax_regime"`
	Items     []struct {
		ObligationType struct {
			ID   string `json:"id"`
			Code string `json:"code"`
		} `json:"obligation_type"`
		Applicability       string `json:"applicability"`
		DefaultApplicable   *bool  `json:"default_applicable"`
		EffectiveApplicable *bool  `json:"effective_applicable"`
	} `json:"items"`
}

func (o overridesDTO) find(code string) (id, applicability string, def, eff *bool) {
	for _, i := range o.Items {
		if i.ObligationType.Code == code {
			return i.ObligationType.ID, i.Applicability, i.DefaultApplicable, i.EffectiveApplicable
		}
	}
	return "", "", nil, nil
}

func TestOverrides_ValidityAndPersistence(t *testing.T) {
	srv := newSeededServer(t)
	var c companyDTO
	srv.Post("/api/v1/companies", companyPayload("102", "Beta Serviços Ltda", validCnpj("000000020001"))).MustStatus(t, http.StatusCreated).JSON(t, &c)

	var o overridesDTO
	srv.Get("/api/v1/companies/"+c.ID+"/obligation-overrides").MustStatus(t, http.StatusOK).JSON(t, &o)
	ipiID, app, def, eff := o.find("FISCAL_IPI")
	if app != "DEFAULT" || def == nil || !*def || eff == nil || !*eff {
		t.Fatalf("IPI inicial: %s %v %v", app, def, eff)
	}
	if _, _, def, eff := o.find("PGDAS"); def != nil || eff != nil {
		t.Fatal("PGDAS não é gerado para Lucro Presumido (sem regra de regime)")
	}

	// Empresa Beta: IPI = não se aplica a partir de 10/2026 [PRD §9.1].
	path := "/api/v1/companies/" + c.ID + "/obligation-overrides/" + ipiID
	srv.Put(path, map[string]any{"applicability": "NOT_APPLICABLE", "effective_from": "2026-10"}).MustStatus(t, http.StatusOK).JSON(t, &o)
	if _, app, _, eff := o.find("FISCAL_IPI"); app != "NOT_APPLICABLE" || *eff {
		t.Fatalf("IPI após override: %s %v", app, *eff)
	}

	q := store.New(srv.Pool)
	at := func(comp string) string {
		m, err := company.OverridesAt(context.Background(), q, c.ID, competence.MustParse(comp))
		if err != nil {
			t.Fatal(err)
		}
		return m[ipiID]
	}
	if got := at("2026-09"); got != "" {
		t.Fatalf("override de 10/2026 não pode aparecer em 09/2026 (got %q)", got)
	}
	for _, comp := range []string{"2026-10", "2026-11", "2027-06"} {
		if got := at(comp); got != "NOT_APPLICABLE" {
			t.Fatalf("dispensa deveria persistir em %s (got %q)", comp, got)
		}
	}

	// Volta ao padrão a partir de 01/2027; 12/2026 continua dispensado.
	srv.Put(path, map[string]any{"applicability": "DEFAULT", "effective_from": "2027-01"}).MustStatus(t, http.StatusOK)
	if at("2026-12") != "NOT_APPLICABLE" || at("2027-01") != "DEFAULT" {
		t.Fatalf("12/2026=%s 01/2027=%s", at("2026-12"), at("2027-01"))
	}

	srv.Put("/api/v1/companies/"+c.ID+"/obligation-overrides/7b0f8d7e-0000-4000-8000-000000000000",
		map[string]any{"applicability": "NOT_APPLICABLE", "effective_from": "2026-10"}).MustStatus(t, http.StatusNotFound)
	srv.Put(path, map[string]any{"applicability": "TALVEZ", "effective_from": "2027-02"}).MustStatus(t, http.StatusBadRequest)
}

func TestResolveApplicable(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		def  *bool
		app  string
		want *bool
	}{
		{&yes, "DEFAULT", &yes}, {&yes, "NOT_APPLICABLE", &no}, {&no, "APPLICABLE", &yes},
		{&no, "DEFAULT", &no}, {nil, "APPLICABLE", nil},
	}
	for _, tc := range cases {
		got := company.ResolveApplicable(tc.def, tc.app)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("ResolveApplicable(%v, %s) = %v", tc.def, tc.app, got)
		}
	}
}
