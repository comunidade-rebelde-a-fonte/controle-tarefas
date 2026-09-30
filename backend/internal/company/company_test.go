package company_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"controle/internal/platform/cnpj"
	"controle/internal/testutil"
	"controle/internal/user"
)

type companyDTO struct {
	ID                 string   `json:"id"`
	InternalCode       string   `json:"internal_code"`
	LegalName          string   `json:"legal_name"`
	TradeName          *string  `json:"trade_name"`
	Cnpj               string   `json:"cnpj"`
	State              *string  `json:"state"`
	ServiceStartDate   string   `json:"service_start_date"`
	Active             bool     `json:"active"`
	PayrollType        string   `json:"payroll_type"`
	CurrentTaxRegime   *string  `json:"current_tax_regime"`
	CurrentRegimeGroup *string  `json:"current_regime_group"`
	ActiveModules      []string `json:"active_modules"`
	Version            int      `json:"version"`
	Responsibles       struct {
		Fiscal *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"fiscal"`
	} `json:"responsibles"`
}

type listDTO struct {
	Items []companyDTO `json:"items"`
	Total int          `json:"total"`
}

type errorDTO struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// validCnpj gera um CNPJ fictício com dígitos verificadores válidos.
func validCnpj(base12 string) string { return cnpj.WithCheckDigits(base12) }

func newServer(t *testing.T) *testutil.Server {
	t.Helper()
	// 01/10/2026 09:00 em São Paulo → competência corrente 2026-10.
	return testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
}

func companyPayload(code, name, cnpjValue string) map[string]any {
	return map[string]any{
		"internal_code":      code,
		"legal_name":         name,
		"trade_name":         name + " Fantasia",
		"cnpj":               cnpjValue,
		"city":               "Curitiba",
		"state":              "pr",
		"service_start_date": "2025-01-15",
		"payroll_type":       "PAYROLL",
		"tax_regime":         "LUCRO_PRESUMIDO",
	}
}

func TestCA001_CreateCompany(t *testing.T) {
	srv := newServer(t)
	raw := validCnpj("112223330001")
	masked := cnpj.Format(raw)

	var created companyDTO
	srv.Post("/api/v1/companies", companyPayload("301", "Empresa Nova Ltda", masked)).
		MustStatus(t, http.StatusCreated).JSON(t, &created)

	if created.Cnpj != raw {
		t.Fatalf("CNPJ armazenado = %q, want só dígitos %q", created.Cnpj, raw)
	}
	if created.State == nil || *created.State != "PR" || !created.Active || created.Version != 1 {
		t.Fatalf("empresa criada = %+v", created)
	}
	if created.CurrentTaxRegime == nil || *created.CurrentTaxRegime != "LUCRO_PRESUMIDO" || *created.CurrentRegimeGroup != "NORMAL" {
		t.Fatalf("regime = %v / %v", created.CurrentTaxRegime, created.CurrentRegimeGroup)
	}
	if len(created.ActiveModules) != 5 {
		t.Fatalf("módulos = %v, want os 5 módulos disponíveis para configuração", created.ActiveModules)
	}

	var got companyDTO
	srv.Get("/api/v1/companies/"+created.ID).MustStatus(t, http.StatusOK).JSON(t, &got)
	if got.ID != created.ID || got.LegalName != "Empresa Nova Ltda" {
		t.Fatalf("GET = %+v", got)
	}

	// O regime inicial vale a partir da competência de início do atendimento (01/2025).
	var from string
	if err := srv.Pool.QueryRow(context.Background(),
		`SELECT effective_from_competence::text FROM company_tax_regime_history WHERE company_id = $1`, created.ID).Scan(&from); err != nil {
		t.Fatal(err)
	}
	if from != "2025-01-01" {
		t.Fatalf("vigência inicial do regime = %s, want 2025-01-01", from)
	}
}

func TestCA001_CreateCompany_WithSelectedModulesAndResponsible(t *testing.T) {
	srv := newServer(t)
	var resp struct{ ID string }
	srv.Post("/api/v1/users", map[string]any{"name": "Ana Fiscal"}).MustStatus(t, http.StatusCreated).JSON(t, &resp)

	p := companyPayload("302", "Simples Ltda", validCnpj("998887770001"))
	p["tax_regime"] = "SIMPLES_NACIONAL"
	p["modules"] = []string{"FISCAL", "PAYROLL"}
	p["fiscal_responsible_user_id"] = resp.ID
	p["factor_r"] = true

	var created companyDTO
	srv.Post("/api/v1/companies", p).MustStatus(t, http.StatusCreated).JSON(t, &created)
	if len(created.ActiveModules) != 2 || *created.CurrentRegimeGroup != "SIMPLES" {
		t.Fatalf("empresa = %+v", created)
	}
	if created.Responsibles.Fiscal == nil || created.Responsibles.Fiscal.Name != "Ana Fiscal" {
		t.Fatalf("responsável fiscal = %+v", created.Responsibles.Fiscal)
	}
}

func TestCA002_DuplicateCnpj(t *testing.T) {
	srv := newServer(t)
	raw := validCnpj("112223330001")
	var first companyDTO
	srv.Post("/api/v1/companies", companyPayload("101", "Empresa Alfa Ltda", raw)).MustStatus(t, http.StatusCreated).JSON(t, &first)

	// Mesmo CNPJ, agora com máscara e espaços: deve ser bloqueado indicando a conflitante.
	var e errorDTO
	srv.Post("/api/v1/companies", companyPayload("999", "Outra Ltda", " "+cnpj.Format(raw)+" ")).
		MustStatus(t, http.StatusConflict).JSON(t, &e)
	if e.Code != "CNPJ_CONFLICT" {
		t.Fatalf("code = %s", e.Code)
	}
	cc, _ := e.Details["conflicting_company"].(map[string]any)
	if cc["id"] != first.ID || cc["internal_code"] != "101" || cc["legal_name"] != "Empresa Alfa Ltda" {
		t.Fatalf("empresa conflitante = %v", cc)
	}

	// Código interno duplicado também é bloqueado.
	srv.Post("/api/v1/companies", companyPayload("101", "Outra Ltda", validCnpj("445556660001"))).
		MustStatus(t, http.StatusConflict).JSON(t, &e)
	if e.Code != "CODE_CONFLICT" {
		t.Fatalf("code = %s", e.Code)
	}

	// Nada foi gravado.
	var list listDTO
	srv.Get("/api/v1/companies").MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Total != 1 {
		t.Fatalf("empresas = %d, want 1", list.Total)
	}
}

func TestCompanies_SearchByCnpjWithAndWithoutMask(t *testing.T) {
	srv := newServer(t)
	ctx := context.Background()
	// CNPJ fictício do PRD (sem DV válido) inserido diretamente, como faz o seed.
	var id string
	if err := srv.Pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, trade_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('101', 'Empresa Fictícia Alfa Comércio Ltda', 'Alfa', '00000001000101', '2025-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Pool.Exec(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('102', 'Empresa Fictícia Beta Serviços Ltda', '00000002000102', '2025-01-01', 'PAYROLL', $1, $1)`, user.AdminID); err != nil {
		t.Fatal(err)
	}

	search := func(query string) []companyDTO {
		var l listDTO
		srv.Get("/api/v1/companies?"+query).MustStatus(t, http.StatusOK).JSON(t, &l)
		return l.Items
	}
	for _, q := range []string{"q=" + url.QueryEscape("00.000.001/0001-01"), "q=00000001000101", "cnpj=" + url.QueryEscape("00.000.001/0001-01"),
		"q=alfa", "q=ALFA+COM", "code=101", "q=" + url.QueryEscape("Fictícia Alfa")} {
		items := search(q)
		if len(items) != 1 || items[0].ID != id {
			t.Errorf("busca %q retornou %d itens", q, len(items))
		}
	}
	if items := search("q=Fict"); len(items) != 2 {
		t.Errorf("busca parcial retornou %d itens, want 2", len(items))
	}
	if items := search("q=%25"); len(items) != 0 {
		t.Errorf("curinga %% não deve casar tudo: %d itens", len(items))
	}
}

func TestCompanies_Validation(t *testing.T) {
	srv := newServer(t)
	cases := map[string]func(map[string]any){
		"CNPJ com DV inválido": func(p map[string]any) { p["cnpj"] = "11.222.333/0001-82" },
		"sem regime":           func(p map[string]any) { delete(p, "tax_regime") },
		"regime MEI":           func(p map[string]any) { p["tax_regime"] = "MEI" },
		"sem razão social":     func(p map[string]any) { p["legal_name"] = "  " },
		"UF inválida":          func(p map[string]any) { p["state"] = "Paraná" },
		"fim antes do início":  func(p map[string]any) { p["service_end_date"] = "2024-12-31" },
		"responsável inexistente": func(p map[string]any) {
			p["fiscal_responsible_user_id"] = "7b0f8d7e-0000-4000-8000-000000000000"
		},
		"módulo inválido":  func(p map[string]any) { p["modules"] = []string{"DCTFWEB"} },
		"tipo de folha":    func(p map[string]any) { p["payroll_type"] = "X" },
		"data mal formada": func(p map[string]any) { p["service_start_date"] = "15/01/2025" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := companyPayload("401", "Teste", validCnpj("112223330001"))
			mutate(p)
			srv.Post("/api/v1/companies", p).MustStatus(t, http.StatusBadRequest)
		})
	}
}

func TestCompanies_UpdateWithVersion(t *testing.T) {
	srv := newServer(t)
	var c companyDTO
	srv.Post("/api/v1/companies", companyPayload("501", "Gama Ltda", validCnpj("112223330001"))).MustStatus(t, http.StatusCreated).JSON(t, &c)

	var updated companyDTO
	srv.Patch("/api/v1/companies/"+c.ID, map[string]any{"version": c.Version, "legal_name": "Gama Indústria Ltda", "trade_name": nil, "payroll_type": "NO_PAYROLL"}).
		MustStatus(t, http.StatusOK).JSON(t, &updated)
	if updated.LegalName != "Gama Indústria Ltda" || updated.TradeName != nil || updated.PayrollType != "NO_PAYROLL" || updated.Version != 2 {
		t.Fatalf("atualizada = %+v", updated)
	}
	if code := srv.Patch("/api/v1/companies/"+c.ID, map[string]any{"version": c.Version, "legal_name": "X"}).
		MustStatus(t, http.StatusConflict).ErrorCode(t); code != "VERSION_CONFLICT" {
		t.Fatalf("code = %s", code)
	}
	srv.Patch("/api/v1/companies/"+c.ID, map[string]any{"legal_name": "sem versão"}).MustStatus(t, http.StatusBadRequest)
	srv.Patch("/api/v1/companies/"+c.ID, map[string]any{"version": 2, "cnpj": "123"}).MustStatus(t, http.StatusBadRequest)
}

func TestCompanies_DeactivateFiltersAndNoDelete(t *testing.T) {
	srv := newServer(t)
	var a, b companyDTO
	srv.Post("/api/v1/companies", companyPayload("601", "Delta Ltda", validCnpj("112223330001"))).MustStatus(t, http.StatusCreated).JSON(t, &a)
	pb := companyPayload("602", "Épsilon Simples", validCnpj("998887770001"))
	pb["tax_regime"] = "SIMPLES_NACIONAL"
	pb["modules"] = []string{"FISCAL"}
	srv.Post("/api/v1/companies", pb).MustStatus(t, http.StatusCreated).JSON(t, &b)

	var d companyDTO
	srv.Post("/api/v1/companies/"+a.ID+"/deactivate", nil).MustStatus(t, http.StatusOK).JSON(t, &d)
	if d.Active {
		t.Fatal("empresa deveria estar inativa")
	}
	srv.Do(http.MethodDelete, "/api/v1/companies/"+a.ID, nil).MustStatus(t, http.StatusMethodNotAllowed)

	count := func(query string) int {
		var l listDTO
		srv.Get("/api/v1/companies?"+query).MustStatus(t, http.StatusOK).JSON(t, &l)
		return l.Total
	}
	checks := map[string]int{
		"active=false": 1, "active=true": 1, "tax_regime=SIMPLES_NACIONAL": 1, "tax_regime=LUCRO_PRESUMIDO": 1,
		"module=PAYROLL": 1, "module=FISCAL": 2, "": 2,
	}
	for q, want := range checks {
		if got := count(q); got != want {
			t.Errorf("filtro %q: %d, want %d", q, got, want)
		}
	}
	srv.Get("/api/v1/companies?module=DCTFWEB").MustStatus(t, http.StatusBadRequest)
	srv.Get("/api/v1/companies/nao-existe").MustStatus(t, http.StatusNotFound)
}
