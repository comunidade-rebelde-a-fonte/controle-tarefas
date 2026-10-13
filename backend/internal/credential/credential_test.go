package credential_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/seed"
)

type situation struct {
	Code          string `json:"code"`
	Label         string `json:"label"`
	ThresholdDays *int   `json:"threshold_days"`
	DaysLeft      *int   `json:"days_left"`
}

type control struct {
	Type          string    `json:"type"`
	ExpiresAt     *string   `json:"expires_at"`
	RenewalStatus string    `json:"renewal_status"`
	RenewalLabel  string    `json:"renewal_label"`
	Situation     situation `json:"situation"`
	Notes         *string   `json:"notes"`
	Version       int       `json:"version"`
}

type row struct {
	Company struct {
		ID           string `json:"id"`
		InternalCode string `json:"internal_code"`
		TaxRegime    string `json:"tax_regime"`
	} `json:"company"`
	PowerOfAttorney control `json:"rfb_power_of_attorney"`
	Certificate     control `json:"digital_certificate"`
}

type list struct {
	Items      []row `json:"items"`
	Total      int   `json:"total"`
	Thresholds []int `json:"thresholds"`
}

// server sobe a API sobre as 20 empresas fictícias (hoje = 01/10/2026 em São Paulo).
func server(t *testing.T) *testutil.Server {
	t.Helper()
	pool := seeded.NewCompaniesDB(t)
	return testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))
}

func companyID(t *testing.T, srv *testutil.Server, code string) string {
	t.Helper()
	var id string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT id FROM companies WHERE internal_code = $1`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func getList(t *testing.T, srv *testutil.Server, query string) list {
	t.Helper()
	var l list
	srv.Get("/api/v1/credential-controls"+query).MustStatus(t, http.StatusOK).JSON(t, &l)
	return l
}

func codes(l list) []string {
	out := make([]string, len(l.Items))
	for i, r := range l.Items {
		out[i] = r.Company.InternalCode
	}
	return out
}

func put(srv *testutil.Server, company, typ string, body map[string]any) testutil.Response {
	return srv.Put("/api/v1/companies/"+company+"/credential-controls/"+typ, body)
}

func TestCredentialControls_ListDefaults(t *testing.T) {
	srv := server(t)
	l := getList(t, srv, "")
	// Todas as empresas do seed têm o módulo Procurações/Certificados.
	if l.Total != seed.DemoCompanyCount || len(l.Items) != seed.DemoCompanyCount {
		t.Fatalf("total = %d/%d, want %d empresas com o módulo", l.Total, len(l.Items), seed.DemoCompanyCount)
	}
	if len(l.Thresholds) != 4 || l.Thresholds[0] != 7 || l.Thresholds[3] != 60 {
		t.Fatalf("faixas = %v", l.Thresholds)
	}
	first := l.Items[0]
	for _, c := range []control{first.PowerOfAttorney, first.Certificate} {
		if c.Version != 0 || c.ExpiresAt != nil || c.RenewalStatus != "CURRENT" || c.RenewalLabel != "Vigente" ||
			c.Situation.Code != "NO_DATE" || c.Situation.Label != "Sem data cadastrada" {
			t.Fatalf("item padrão = %+v", c)
		}
	}
	if first.Company.TaxRegime == "" {
		t.Fatal("regime vigente deveria vir na linha")
	}
	if n := getList(t, srv, "?filter=no_date").Total; n != seed.DemoCompanyCount {
		t.Fatalf("sem data = %d, want %d", n, seed.DemoCompanyCount)
	}
	if n := getList(t, srv, "?filter=expired").Total; n != 0 {
		t.Fatalf("vencidos = %d, want 0", n)
	}
}

// UC-05: renovar certificado (e procuração) com as etapas e a situação calculada.
func TestUC05_RenewalFlowAndSituation(t *testing.T) {
	srv := server(t)
	alfa := companyID(t, srv, "101")

	var c control
	put(srv, alfa, "RFB_POWER_OF_ATTORNEY", map[string]any{"version": 0, "expires_at": "2026-10-05", "renewal_status": "CURRENT", "notes": "  Procuração eletrônica  "}).
		MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.Version != 1 || *c.ExpiresAt != "2026-10-05" || c.Situation.Code != "DUE_SOON" || *c.Situation.ThresholdDays != 7 ||
		*c.Situation.DaysLeft != 4 || c.Situation.Label != "Vence em até 7 dias" || *c.Notes != "Procuração eletrônica" {
		t.Fatalf("criação = %+v", c)
	}
	// Criar de novo com version 0 é conflito.
	if code := put(srv, alfa, "RFB_POWER_OF_ATTORNEY", map[string]any{"version": 0, "renewal_status": "CURRENT"}).
		MustStatus(t, http.StatusConflict).ErrorCode(t); code != "VERSION_CONFLICT" {
		t.Fatalf("code = %s", code)
	}

	put(srv, alfa, "RFB_POWER_OF_ATTORNEY", map[string]any{"version": 1, "expires_at": "2026-10-05", "renewal_status": "RENEWAL_REQUESTED"}).
		MustStatus(t, http.StatusOK).JSON(t, &c)
	put(srv, alfa, "RFB_POWER_OF_ATTORNEY", map[string]any{"version": 2, "expires_at": "2026-10-05", "renewal_status": "AWAITING_CLIENT"}).
		MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.Version != 3 || c.RenewalStatus != "AWAITING_CLIENT" || c.RenewalLabel != "Aguardando cliente" || c.Notes != nil {
		t.Fatalf("aguardando cliente = %+v", c)
	}
	if got := codes(getList(t, srv, "?filter=awaiting_client")); len(got) != 1 || got[0] != "101" {
		t.Fatalf("aguardando cliente = %v", got)
	}
	if got := codes(getList(t, srv, "?filter=due_7")); len(got) != 1 || got[0] != "101" {
		t.Fatalf("vence em 7 dias = %v", got)
	}
	if n := getList(t, srv, "?filter=due_7&type=DIGITAL_CERTIFICATE").Total; n != 0 {
		t.Fatalf("filtro restrito ao certificado = %d, want 0", n)
	}

	// Renovado exige nova data [PRD §19.4].
	res := put(srv, alfa, "RFB_POWER_OF_ATTORNEY", map[string]any{"version": 3, "renewal_status": "RENEWED"})
	if code := res.MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "RENEWAL_DATE_REQUIRED" {
		t.Fatalf("renovado sem data: code = %s", code)
	}
	if code := put(srv, alfa, "RFB_POWER_OF_ATTORNEY", map[string]any{"version": 3, "expires_at": "2026-10-05", "renewal_status": "RENEWED"}).
		MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "RENEWAL_DATE_REQUIRED" {
		t.Fatalf("renovado com a mesma data: code = %s", code)
	}
	// Com a nova validade, volta ao acompanhamento normal (Vigente).
	put(srv, alfa, "RFB_POWER_OF_ATTORNEY", map[string]any{"version": 3, "expires_at": "2027-10-05", "renewal_status": "RENEWED"}).
		MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.Version != 4 || c.RenewalStatus != "CURRENT" || c.Situation.Code != "CURRENT" || c.Situation.Label != "Vigente" {
		t.Fatalf("renovado = %+v", c)
	}
	if n := getList(t, srv, "?filter=awaiting_client").Total; n != 0 {
		t.Fatalf("aguardando cliente após renovar = %d", n)
	}

	// Certificado vencido de outra empresa.
	beta := companyID(t, srv, "102")
	put(srv, beta, "DIGITAL_CERTIFICATE", map[string]any{"version": 0, "expires_at": "2026-09-15", "renewal_status": "CURRENT"}).
		MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.Situation.Code != "EXPIRED" || *c.Situation.DaysLeft != -16 {
		t.Fatalf("vencido = %+v", c)
	}
	if got := codes(getList(t, srv, "?filter=expired")); len(got) != 1 || got[0] != "102" {
		t.Fatalf("vencidos = %v", got)
	}
	if n := getList(t, srv, "?filter=no_date").Total; n != seed.DemoCompanyCount {
		// Toda empresa ainda tem ao menos um item sem data.
		t.Fatalf("sem data = %d, want %d", n, seed.DemoCompanyCount)
	}
}

func TestCredentialControls_CompanyEndpointAndConfiguredThresholds(t *testing.T) {
	srv := server(t)
	alfa := companyID(t, srv, "101")
	put(srv, alfa, "DIGITAL_CERTIFICATE", map[string]any{"version": 0, "expires_at": "2026-10-21", "renewal_status": "CURRENT"}).MustStatus(t, http.StatusOK)

	var r row
	srv.Get("/api/v1/companies/"+alfa+"/credential-controls").MustStatus(t, http.StatusOK).JSON(t, &r)
	if r.Company.InternalCode != "101" || r.Certificate.Situation.Code != "DUE_SOON" || *r.Certificate.Situation.ThresholdDays != 30 ||
		r.PowerOfAttorney.Situation.Code != "NO_DATE" {
		t.Fatalf("empresa = %+v", r)
	}

	// Faixas configuradas mudam a classificação [PRD §19.3].
	srv.Put("/api/v1/settings/credential-thresholds", map[string]any{"days": []int{10, 90}}).MustStatus(t, http.StatusOK)
	srv.Get("/api/v1/companies/"+alfa+"/credential-controls").MustStatus(t, http.StatusOK).JSON(t, &r)
	if *r.Certificate.Situation.ThresholdDays != 90 || r.Certificate.Situation.Label != "Vence em até 90 dias" {
		t.Fatalf("com faixas 10/90 = %+v", r.Certificate.Situation)
	}

	// Empresa inativa ou sem o módulo sai da grade, mas continua consultável na ficha.
	if _, err := srv.Pool.Exec(context.Background(), `UPDATE companies SET active = false WHERE id = $1`, alfa); err != nil {
		t.Fatal(err)
	}
	if n := getList(t, srv, "").Total; n != seed.DemoCompanyCount-1 {
		t.Fatalf("grade com empresa inativa = %d, want %d", n, seed.DemoCompanyCount-1)
	}
	srv.Get("/api/v1/companies/"+alfa+"/credential-controls").MustStatus(t, http.StatusOK)
}

func TestCredentialControls_Validation(t *testing.T) {
	srv := server(t)
	alfa := companyID(t, srv, "101")
	cases := []struct {
		name   string
		path   string
		body   map[string]any
		status int
		code   string
	}{
		{"tipo desconhecido", "/api/v1/companies/" + alfa + "/credential-controls/PASSWORD", map[string]any{"version": 0, "renewal_status": "CURRENT"}, http.StatusNotFound, "CREDENTIAL_TYPE_NOT_FOUND"},
		{"empresa inexistente", "/api/v1/companies/00000000-0000-4000-8000-000000000000/credential-controls/DIGITAL_CERTIFICATE", map[string]any{"version": 0, "renewal_status": "CURRENT"}, http.StatusNotFound, "COMPANY_NOT_FOUND"},
		{"etapa inválida", "/api/v1/companies/" + alfa + "/credential-controls/DIGITAL_CERTIFICATE", map[string]any{"version": 0, "renewal_status": "DONE"}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"observação longa", "/api/v1/companies/" + alfa + "/credential-controls/DIGITAL_CERTIFICATE", map[string]any{"version": 0, "renewal_status": "CURRENT", "notes": strings.Repeat("x", 2001)}, http.StatusBadRequest, "VALIDATION_ERROR"},
		{"data inválida", "/api/v1/companies/" + alfa + "/credential-controls/DIGITAL_CERTIFICATE", map[string]any{"version": 0, "renewal_status": "CURRENT", "expires_at": "31/12/2026"}, http.StatusBadRequest, ""},
		{"versão divergente", "/api/v1/companies/" + alfa + "/credential-controls/DIGITAL_CERTIFICATE", map[string]any{"version": 7, "renewal_status": "CURRENT"}, http.StatusConflict, "VERSION_CONFLICT"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := srv.Put(c.path, c.body).MustStatus(t, c.status)
			if c.code != "" {
				if got := res.ErrorCode(t); got != c.code {
					t.Fatalf("code = %s, want %s", got, c.code)
				}
			}
		})
	}
	for _, q := range []string{"?filter=due_x", "?filter=vencidos", "?type=PASSWORD"} {
		srv.Get("/api/v1/credential-controls"+q).MustStatus(t, http.StatusBadRequest)
	}
	srv.Get("/api/v1/companies/not-a-uuid/credential-controls").MustStatus(t, http.StatusNotFound)
}
