package catalog_test

import (
	"net/http"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

type typeDTO struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	PrimaryModule   string `json:"primary_module"`
	StatusSetCode   string `json:"status_set_code"`
	AllowsProtocol  bool   `json:"allows_protocol"`
	Version         int    `json:"version"`
	DisplayContexts []struct {
		ModuleCode string `json:"module_code"`
	} `json:"display_contexts"`
	RegimeRules []struct {
		TaxRegime string `json:"tax_regime"`
	} `json:"regime_rules"`
	Dependencies []struct {
		PrerequisiteCode string `json:"prerequisite_code"`
		DependencyMode   string `json:"dependency_mode"`
	} `json:"dependencies"`
}

type typeList struct {
	Items []typeDTO `json:"items"`
}

func newServer(t *testing.T) *testutil.Server {
	t.Helper()
	return testutil.NewServer(t, seeded.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
}

func findType(l typeList, code string) *typeDTO {
	for i := range l.Items {
		if l.Items[i].Code == code {
			return &l.Items[i]
		}
	}
	return nil
}

// [PRD §15.2]: nova declaração entra por cadastro, sem migration.
func TestCatalog_CreateDeclarationTypeWithoutMigration(t *testing.T) {
	srv := newServer(t)
	var created typeDTO
	srv.Post("/api/v1/obligation-types", map[string]any{
		"code": "nova_declaracao", "name": "Nova Declaração", "primary_module": "DECLARATIONS",
		"status_set_code": "DECLARATION", "sort_order": 300, "allows_protocol": true, "allows_transmission_date": true,
		"regime_rules": []map[string]any{{"tax_regime": "LUCRO_REAL"}, {"tax_regime": "LUCRO_PRESUMIDO"}},
	}).MustStatus(t, http.StatusCreated).JSON(t, &created)
	if created.Code != "NOVA_DECLARACAO" || !created.AllowsProtocol || len(created.RegimeRules) != 2 {
		t.Fatalf("criado = %+v", created)
	}
	if len(created.DisplayContexts) != 1 || created.DisplayContexts[0].ModuleCode != "DECLARATIONS" {
		t.Fatalf("contexto padrão = %+v", created.DisplayContexts)
	}

	var l typeList
	srv.Get("/api/v1/obligation-types").MustStatus(t, http.StatusOK).JSON(t, &l)
	if findType(l, "NOVA_DECLARACAO") == nil || len(l.Items) != 35 {
		t.Fatalf("catálogo com %d tipos; nova declaração presente? %v", len(l.Items), findType(l, "NOVA_DECLARACAO") != nil)
	}
	if code := srv.Post("/api/v1/obligation-types", map[string]any{
		"code": "NOVA_DECLARACAO", "name": "Dup", "primary_module": "DECLARATIONS", "status_set_code": "DECLARATION",
	}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "CODE_CONFLICT" {
		t.Fatalf("code = %s", code)
	}
}

func TestCatalog_Validation(t *testing.T) {
	srv := newServer(t)
	base := func() map[string]any {
		return map[string]any{"code": "X_TIPO", "name": "X", "primary_module": "DECLARATIONS", "status_set_code": "DECLARATION"}
	}
	cases := map[string]func(map[string]any){
		"código inválido":      func(p map[string]any) { p["code"] = "com espaço" },
		"módulo inválido":      func(p map[string]any) { p["primary_module"] = "DCTFWEB" },
		"conjunto inexistente": func(p map[string]any) { p["status_set_code"] = "NAO_EXISTE" },
		"status inicial fora":  func(p map[string]any) { p["initial_status_code"] = "OK" },
		"sem folha sem NO_MOVEMENT": func(p map[string]any) {
			p["status_set_code"] = "FISCAL_CLOSING"
			p["status_when_no_payroll"] = "NO_MOVEMENT"
		},
		"regime repetido": func(p map[string]any) {
			p["regime_rules"] = []map[string]any{{"tax_regime": "LUCRO_REAL"}, {"tax_regime": "LUCRO_REAL"}}
		},
		"contexto inválido": func(p map[string]any) { p["display_contexts"] = []map[string]any{{"module_code": "CREDENTIALS"}} },
		"pré-requisito inexistente": func(p map[string]any) {
			p["dependencies"] = []map[string]any{{"prerequisite_obligation_type_id": "7b0f8d7e-0000-4000-8000-000000000000", "dependency_mode": "SOFT"}}
		},
		"regra de vencimento inexistente": func(p map[string]any) { p["due_date_rule_id"] = "7b0f8d7e-0000-4000-8000-000000000000" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := base()
			mutate(p)
			srv.Post("/api/v1/obligation-types", p).MustStatus(t, http.StatusBadRequest)
		})
	}
}

func TestCatalog_UpdateReplacesListsWithVersion(t *testing.T) {
	srv := newServer(t)
	var l typeList
	srv.Get("/api/v1/obligation-types").MustStatus(t, http.StatusOK).JSON(t, &l)
	sped := findType(l, "SPED_FISCAL")
	mit := findType(l, "MIT")
	if sped == nil || mit == nil || len(sped.RegimeRules) != 4 {
		t.Fatalf("SPED_FISCAL = %+v", sped)
	}

	var upd typeDTO
	srv.Patch("/api/v1/obligation-types/"+sped.ID, map[string]any{
		"version": sped.Version, "name": "SPED Fiscal (ICMS/IPI)",
		"regime_rules": []map[string]any{{"tax_regime": "LUCRO_REAL"}},
		"dependencies": []map[string]any{{"prerequisite_obligation_type_id": mit.ID, "dependency_mode": "SOFT"}},
	}).MustStatus(t, http.StatusOK).JSON(t, &upd)
	if upd.Name != "SPED Fiscal (ICMS/IPI)" || len(upd.RegimeRules) != 1 || len(upd.Dependencies) != 1 || upd.Version != sped.Version+1 {
		t.Fatalf("atualizado = %+v", upd)
	}
	srv.Patch("/api/v1/obligation-types/"+sped.ID, map[string]any{"version": sped.Version, "name": "Y"}).MustStatus(t, http.StatusConflict)
	srv.Patch("/api/v1/obligation-types/"+sped.ID, map[string]any{
		"version": upd.Version, "dependencies": []map[string]any{{"prerequisite_obligation_type_id": sped.ID, "dependency_mode": "HARD"}},
	}).MustStatus(t, http.StatusBadRequest)
	srv.Patch("/api/v1/obligation-types/7b0f8d7e-0000-4000-8000-000000000000", map[string]any{"version": 1}).MustStatus(t, http.StatusNotFound)
}

func TestCatalog_StatusSets(t *testing.T) {
	srv := newServer(t)
	var sets struct {
		Items []struct {
			Code     string `json:"code"`
			Statuses []struct {
				Code    string `json:"code"`
				IsFinal bool   `json:"is_final"`
			} `json:"statuses"`
		} `json:"items"`
	}
	srv.Get("/api/v1/status-sets").MustStatus(t, http.StatusOK).JSON(t, &sets)
	total := 0
	for _, s := range sets.Items {
		total += len(s.Statuses)
	}
	if len(sets.Items) != 5 || total != 31 {
		t.Fatalf("conjuntos = %d, status = %d", len(sets.Items), total)
	}
}
