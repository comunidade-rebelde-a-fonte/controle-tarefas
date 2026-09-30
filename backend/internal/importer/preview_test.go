package importer_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"controle/internal/importer"
	"controle/internal/platform/clock"
	"controle/internal/platform/cnpj"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
)

type previewBody struct {
	Token   string `json:"token"`
	Summary struct {
		Total      int            `json:"total"`
		Importable int            `json:"importable"`
		ByStatus   map[string]int `json:"by_status"`
	} `json:"summary"`
	Rows []struct {
		Line        int    `json:"line"`
		Status      string `json:"status"`
		StatusLabel string `json:"status_label"`
		Errors      []struct {
			Field   string `json:"field"`
			Column  string `json:"column"`
			Message string `json:"message"`
		} `json:"errors"`
		ExistingCompanyID *string `json:"existing_company_id"`
	} `json:"rows"`
}

type commitBody struct {
	Created         int `json:"created"`
	Updated         int `json:"updated"`
	SkippedExisting int `json:"skipped_existing"`
	SkippedInvalid  int `json:"skipped_invalid"`
	Companies       []struct {
		Line   int    `json:"line"`
		Action string `json:"action"`
	} `json:"companies"`
}

var (
	cnpjExisting = cnpj.WithCheckDigits("000009990001")
	cnpjNewA     = cnpj.WithCheckDigits("000009910001")
	cnpjNewB     = cnpj.WithCheckDigits("000009920001")
)

func setup(t *testing.T) (*testutil.Server, *clock.Fixed, string) {
	t.Helper()
	clk := testutil.FixedClock(t, "2026-10-01T12:00:00Z")
	srv := testutil.NewServer(t, seeded.NewCompaniesDB(t), clk)
	var created struct {
		ID string `json:"id"`
	}
	srv.Post("/api/v1/companies", map[string]any{
		"internal_code": "499", "legal_name": "Empresa Existente Ltda", "cnpj": cnpjExisting,
		"service_start_date": "2025-01-01", "payroll_type": "PAYROLL", "tax_regime": "LUCRO_PRESUMIDO",
	}).MustStatus(t, http.StatusCreated).JSON(t, &created)
	return srv, clk, created.ID
}

// sheet monta um CSV com ";" usando os cabeçalhos do modelo.
func sheet(rows ...string) []byte {
	headers := []string{"Código interno", "Razão social", "CNPJ", "Regime tributário inicial", "Início do atendimento", "Tipo de folha",
		"Situação", "Módulo Fiscal", "Módulo Folha", "Responsável Fiscal"}
	return []byte(strings.Join(headers, ";") + "\n" + strings.Join(rows, "\n") + "\n")
}

func preview(t *testing.T, srv *testutil.Server, name string, data []byte) testutil.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/api/v1/imports/companies/preview", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return testutil.Response{Status: resp.StatusCode, Header: resp.Header, Body: b}
}

func count(t *testing.T, srv *testutil.Server, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := srv.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var fiveRows = sheet(
	"401;Nova Alfa Ltda;"+cnpj.Format(cnpjNewA)+";Simples Nacional;01/02/2026;Sem folha;;Sim;Não;Ana Souza",               // 2: nova
	"402;CNPJ Errado Ltda;00.000.001/0001-99;Lucro Real;01/02/2026;Folha;;Sim;Sim;",                                       // 3: CNPJ inválido
	"403;Repetida Ltda;"+cnpjNewA+";Lucro Real;01/02/2026;Folha;;Sim;Sim;",                                                // 4: CNPJ repetido no arquivo
	"499;Empresa Existente Renomeada Ltda;"+cnpjExisting+";Lucro Real;01/01/2025;Folha;;Sim;Sim;",                         // 5: existente
	"404;Nova Beta Ltda;"+cnpjNewB+";Lucro Presumido;15/03/2026;Pró-labore;Inativa;Sim;Não;bruno.lima@escritorio.exemplo", // 6: nova inativa
)

// CA-018: CNPJ inválido ou repetido aparece na prévia, antes da confirmação; só as linhas
// válidas são importadas e a empresa existente não confirmada não é alterada.
func TestCA018_ImportRejectsInvalidBeforeCommit(t *testing.T) {
	srv, _, existingID := setup(t)
	before := count(t, srv, `SELECT count(*) FROM companies`)

	var p previewBody
	preview(t, srv, "empresas.csv", fiveRows).MustStatus(t, http.StatusOK).JSON(t, &p)
	statuses := make([]string, len(p.Rows))
	for i, r := range p.Rows {
		statuses[i] = r.Status
	}
	want := []string{importer.StatusNew, importer.StatusInvalidCnpj, importer.StatusInvalidValue, importer.StatusExisting, importer.StatusNew}
	if strings.Join(statuses, ",") != strings.Join(want, ",") {
		t.Fatalf("status = %v, want %v", statuses, want)
	}
	if p.Summary.Total != 5 || p.Summary.Importable != 3 || p.Summary.ByStatus[importer.StatusInvalidCnpj] != 1 {
		t.Fatalf("resumo = %+v", p.Summary)
	}
	if e := p.Rows[1].Errors; len(e) != 1 || e[0].Column != "CNPJ" || p.Rows[1].StatusLabel != "CNPJ inválido" {
		t.Fatalf("linha 3 = %+v", p.Rows[1])
	}
	if e := p.Rows[2].Errors; len(e) != 1 || !strings.Contains(e[0].Message, "CNPJ repetido no arquivo (linha 2)") {
		t.Fatalf("linha 4 = %+v", p.Rows[2])
	}
	if p.Rows[3].ExistingCompanyID == nil || *p.Rows[3].ExistingCompanyID != existingID {
		t.Fatal("linha 5 deveria apontar a empresa existente")
	}
	// Nada foi gravado antes da confirmação.
	if n := count(t, srv, `SELECT count(*) FROM companies`); n != before {
		t.Fatalf("empresas antes da confirmação = %d, want %d", n, before)
	}

	var c commitBody
	srv.Post("/api/v1/imports/companies/commit", map[string]any{"token": p.Token}).MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.Created != 2 || c.Updated != 0 || c.SkippedExisting != 1 || c.SkippedInvalid != 2 {
		t.Fatalf("resultado = %+v", c)
	}
	if n := count(t, srv, `SELECT count(*) FROM companies`); n != before+2 {
		t.Fatalf("empresas = %d, want %d", n, before+2)
	}
	if n := count(t, srv, `SELECT count(*) FROM companies WHERE internal_code IN ('402', '403')`); n != 0 {
		t.Fatal("linhas inválidas não podem ser importadas")
	}
	if n := count(t, srv, `SELECT count(*) FROM companies WHERE id = $1 AND legal_name = 'Empresa Existente Ltda'`, existingID); n != 1 {
		t.Fatal("empresa existente não confirmada foi alterada")
	}
	// Nova com regime, módulos, responsável e situação da planilha.
	if n := count(t, srv, `SELECT count(*) FROM companies c JOIN users u ON u.id = c.fiscal_responsible_user_id
		WHERE c.internal_code = '401' AND c.active AND c.payroll_type = 'NO_PAYROLL' AND u.name = 'Ana Souza'`); n != 1 {
		t.Fatal("empresa 401 não gravada como na planilha")
	}
	if n := count(t, srv, `SELECT count(*) FROM company_tax_regime_history h JOIN companies c ON c.id = h.company_id
		WHERE c.internal_code = '401' AND h.tax_regime = 'SIMPLES_NACIONAL' AND h.effective_from_competence = '2026-02-01'`); n != 1 {
		t.Fatal("regime inicial da 401 ausente")
	}
	if n := count(t, srv, `SELECT count(*) FROM company_module_settings s JOIN companies c ON c.id = s.company_id
		WHERE c.internal_code = '401' AND s.enabled`); n != 1 {
		t.Fatal("a 401 deveria ter só o módulo Fiscal")
	}
	if n := count(t, srv, `SELECT count(*) FROM companies WHERE internal_code = '404' AND NOT active`); n != 1 {
		t.Fatal("empresa 404 deveria ser importada inativa")
	}

	// Não confirma duas vezes.
	if code := srv.Post("/api/v1/imports/companies/commit", map[string]any{"token": p.Token}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "IMPORT_ALREADY_COMMITTED" {
		t.Fatalf("code = %s", code)
	}
}

func TestImportCommit_UpdatesOnlyConfirmedExisting(t *testing.T) {
	srv, _, existingID := setup(t)
	var p previewBody
	preview(t, srv, "empresas.csv", fiveRows).MustStatus(t, http.StatusOK).JSON(t, &p)

	// Confirmar uma linha que não é "existente" é recusado.
	if code := srv.Post("/api/v1/imports/companies/commit", map[string]any{"token": p.Token, "update_existing_rows": []int{2}}).
		MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "INVALID_CONFIRMATION" {
		t.Fatalf("code = %s", code)
	}

	var c commitBody
	srv.Post("/api/v1/imports/companies/commit", map[string]any{"token": p.Token, "update_existing_rows": []int{5}}).MustStatus(t, http.StatusOK).JSON(t, &c)
	if c.Created != 2 || c.Updated != 1 || c.SkippedExisting != 0 {
		t.Fatalf("resultado = %+v", c)
	}
	if n := count(t, srv, `SELECT count(*) FROM companies WHERE id = $1 AND legal_name = 'Empresa Existente Renomeada Ltda' AND version = 2`, existingID); n != 1 {
		t.Fatal("empresa existente confirmada deveria ser atualizada")
	}
	// Regime não muda pela importação (continua Presumido; a planilha dizia Lucro Real).
	if n := count(t, srv, `SELECT count(*) FROM company_tax_regime_history WHERE company_id = $1 AND tax_regime = 'LUCRO_PRESUMIDO'`, existingID); n != 1 {
		t.Fatal("regime da existente não pode mudar pela importação")
	}
}

func TestImportCommit_ExpiredTokenIs410(t *testing.T) {
	srv, clk, _ := setup(t)
	var p previewBody
	preview(t, srv, "empresas.csv", fiveRows).MustStatus(t, http.StatusOK).JSON(t, &p)
	clk.Advance(importer.PreviewTTL + time.Minute)
	if code := srv.Post("/api/v1/imports/companies/commit", map[string]any{"token": p.Token}).MustStatus(t, http.StatusGone).ErrorCode(t); code != "IMPORT_TOKEN_EXPIRED" {
		t.Fatalf("code = %s", code)
	}
	if n := count(t, srv, `SELECT count(*) FROM companies WHERE internal_code IN ('401', '404')`); n != 0 {
		t.Fatal("nada pode ser importado com o token expirado")
	}
}

func TestImportPreview_Errors(t *testing.T) {
	srv, _, _ := setup(t)
	if code := preview(t, srv, "empresas.pdf", []byte("%PDF-1.4")).MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "INVALID_IMPORT_FILE" {
		t.Fatalf("code = %s", code)
	}
	if code := preview(t, srv, "x.csv", []byte("Razão social\nX\n")).MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "INVALID_IMPORT_FILE" {
		t.Fatalf("code = %s", code)
	}
	srv.Post("/api/v1/imports/companies/commit", map[string]any{"token": "00000000-0000-4000-8000-000000000000"}).MustStatus(t, http.StatusNotFound)
	srv.Post("/api/v1/imports/companies/commit", map[string]any{"token": "x"}).MustStatus(t, http.StatusBadRequest)
}
