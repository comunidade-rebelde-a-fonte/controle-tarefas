package chart_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-020 [REQ-026, CC-20]: o plano de contas por empresa (M5.1): inclusão, alteração,
// importação CSV tudo ou nada e cópia de outra empresa (o padrão).

var ctx = context.Background()

type env struct {
	srv        *testutil.Server
	pool       *pgxpool.Pool
	company    string
	standard   string
	accountsOf func(company string) []account
}

type account struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	e := &env{srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00")), pool: pool}
	newCompany := func(code, cnpj string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
			VALUES ($1, 'Empresa ' || $1, $2, '2025-01-01', 'PAYROLL', $3, $3) RETURNING id`, code, cnpj, user.AdminID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	e.company, e.standard = newCompany("900", "11222333000181"), newCompany("901", "11444777000161")
	e.accountsOf = func(company string) []account {
		var out struct {
			Items []account `json:"items"`
		}
		e.srv.Get("/api/v1/companies/"+company+"/chart-accounts").MustStatus(t, http.StatusOK).JSON(t, &out)
		return out.Items
	}
	return e
}

func (e *env) upload(t *testing.T, company, content string) testutil.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", "plano.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.srv.URL+"/api/v1/companies/"+company+"/chart-accounts/import", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return testutil.Response{Status: resp.StatusCode, Header: resp.Header, Body: b}
}

func TestTEST020_ChartOfAccounts(t *testing.T) {
	e := setup(t)
	base := "/api/v1/companies/" + e.company + "/chart-accounts"
	var a account
	e.srv.Post(base, map[string]string{"code": " 1.01 ", "name": " Caixa "}).MustStatus(t, http.StatusCreated).JSON(t, &a)
	if a.Code != "1.01" || a.Name != "Caixa" || !a.Active {
		t.Fatalf("conta = %+v", a)
	}
	if code := e.srv.Post(base, map[string]string{"code": "1.01", "name": "Outra"}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "ACCOUNT_CODE_EXISTS" {
		t.Fatalf("código = %s", code)
	}
	for _, bad := range []map[string]string{{"code": "", "name": "x"}, {"code": "1 01", "name": "x"}, {"code": "1;01", "name": "x"}, {"code": "2", "name": " "}} {
		e.srv.Post(base, bad).MustStatus(t, http.StatusBadRequest)
	}
	// Alterar o nome e desativar; o código não muda.
	var upd account
	e.srv.Patch(base+"/"+a.ID, map[string]any{"name": "Caixa geral", "active": false}).MustStatus(t, http.StatusOK).JSON(t, &upd)
	if upd.Code != "1.01" || upd.Name != "Caixa geral" || upd.Active {
		t.Fatalf("alterada = %+v", upd)
	}
	if code := e.srv.Patch(base+"/00000000-0000-4000-8000-000000000000", map[string]any{"active": true}).MustStatus(t, http.StatusNotFound).ErrorCode(t); code != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("código = %s", code)
	}
	var onlyActive struct {
		Items []account `json:"items"`
	}
	e.srv.Get(base+"?active=true").MustStatus(t, http.StatusOK).JSON(t, &onlyActive)
	if len(onlyActive.Items) != 0 {
		t.Fatal("conta desativada listada como ativa")
	}
	// Empresa inexistente: 404.
	e.srv.Get("/api/v1/companies/00000000-0000-4000-8000-000000000000/chart-accounts").MustStatus(t, http.StatusNotFound)
}

// Importação tudo ou nada: com uma linha inválida, nada entra e a resposta traz cada erro; válida,
// inclui as novas, atualiza o nome das que existem e as reativa. Windows-1252 também é lido.
func TestTEST020_ImportChartCSV(t *testing.T) {
	e := setup(t)
	e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts", map[string]string{"code": "1.01", "name": "Caixa"}).MustStatus(t, http.StatusCreated)
	if _, err := e.pool.Exec(ctx, `UPDATE chart_accounts SET active = false WHERE company_id = $1`, e.company); err != nil {
		t.Fatal(err)
	}

	var bad struct {
		Code    string `json:"code"`
		Details struct {
			Lines []struct {
				Line    int    `json:"line"`
				Message string `json:"message"`
			} `json:"lines"`
		} `json:"details"`
	}
	e.upload(t, e.company, "codigo;nome\n2.01;Fornecedores\n2.01;De novo\n3.01;\nsem separador\n").MustStatus(t, http.StatusBadRequest).JSON(t, &bad)
	if bad.Code != "INVALID_ACCOUNTS_CSV" || len(bad.Details.Lines) != 3 || bad.Details.Lines[0].Line != 3 {
		t.Fatalf("erros = %+v", bad)
	}
	if n := len(e.accountsOf(e.company)); n != 1 {
		t.Fatalf("importação parcial: %d contas", n)
	}

	// "Serviços" em Windows-1252 (ç = 0xE7).
	content := "codigo;nome\n1.01;Caixa e bancos\n4.01;Servi\xe7os prestados\n\n5.01;Consultoria;coluna a mais\n"
	var res struct {
		Inserted int `json:"inserted"`
		Updated  int `json:"updated"`
	}
	e.upload(t, e.company, content).MustStatus(t, http.StatusCreated).JSON(t, &res)
	if res.Inserted != 2 || res.Updated != 1 {
		t.Fatalf("importação = %+v", res)
	}
	got := e.accountsOf(e.company)
	if len(got) != 3 || got[0].Name != "Caixa e bancos" || !got[0].Active || got[1].Name != "Serviços prestados" {
		t.Fatalf("plano = %+v", got)
	}
	var logs int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb->>'acao' = 'importar_plano'`).Scan(&logs); err != nil || logs != 1 {
		t.Fatalf("importação fora do log: %d %v", logs, err)
	}
}

// Copiar de um padrão: as contas ativas da outra empresa que esta ainda não tem.
func TestTEST020_CopyChartFromStandard(t *testing.T) {
	e := setup(t)
	for code, name := range map[string]string{"1.01": "Caixa", "2.01": "Fornecedores", "3.01": "Desativada"} {
		e.srv.Post("/api/v1/companies/"+e.standard+"/chart-accounts", map[string]string{"code": code, "name": name}).MustStatus(t, http.StatusCreated)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE chart_accounts SET active = false WHERE company_id = $1 AND code = '3.01'`, e.standard); err != nil {
		t.Fatal(err)
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts", map[string]string{"code": "1.01", "name": "Caixa da empresa"}).MustStatus(t, http.StatusCreated)
	var res struct {
		Inserted int `json:"inserted"`
		Skipped  int `json:"skipped"`
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts/copy", map[string]string{"source_company_id": e.standard}).
		MustStatus(t, http.StatusCreated).JSON(t, &res)
	if res.Inserted != 1 || res.Skipped != 1 {
		t.Fatalf("cópia = %+v", res)
	}
	got := e.accountsOf(e.company)
	if len(got) != 2 || got[0].Name != "Caixa da empresa" || got[1].Code != "2.01" {
		t.Fatalf("plano = %+v", got)
	}
	for _, src := range []string{e.company, "x", "00000000-0000-4000-8000-000000000000"} {
		var errOut struct {
			Details struct {
				Fields map[string]string `json:"fields"`
			} `json:"details"`
		}
		e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts/copy", map[string]string{"source_company_id": src}).
			MustStatus(t, http.StatusBadRequest).JSON(t, &errOut)
		if errOut.Details.Fields["source_company_id"] == "" {
			t.Errorf("origem %q: %+v", src, errOut)
		}
	}
	raw, _ := json.Marshal(res)
	if string(raw) != `{"inserted":1,"skipped":1}` {
		t.Fatalf("JSON = %s", raw)
	}
}
