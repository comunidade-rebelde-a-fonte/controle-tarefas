package entries_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/encoding/charmap"

	"controle/internal/reconciliation/entries"
	"controle/internal/testutil"
	"controle/internal/user"
)

// CC-12 [REQ-023, AC-025]: lançamentos do cliente inseridos à mão, pelo formulário e pela
// importação CSV (rotas /lancamentos e /lancamentos/csv da origem).

var ctx = context.Background()

type env struct {
	srv  *testutil.Server
	pool *pgxpool.Pool
}

func setup(t *testing.T) env {
	t.Helper()
	pool := testutil.NewDB(t)
	return env{srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00")), pool: pool}
}

func (e env) company(t *testing.T, code, cnpj string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(ctx, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, $2, '2025-01-01', 'PAYROLL', $3, $3) RETURNING id`, code, cnpj, user.AdminID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e env) closeAccounting(t *testing.T, companyID, comp string) {
	t.Helper()
	var compID string
	if err := e.pool.QueryRow(ctx, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ($1, $2::int, $3::int, $4::date)
		ON CONFLICT (competence_key) DO UPDATE SET competence_key = EXCLUDED.competence_key RETURNING id`, comp, comp[:4], comp[5:], comp+"-01").Scan(&compID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', 'CLOSED', now(), $3)`, companyID, compID, user.AdminID); err != nil {
		t.Fatal(err)
	}
}

func (e env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// importCSV envia o CSV no multipart, como a tela (e a origem) enviam.
func (e env) importCSV(t *testing.T, companyID string, data []byte) testutil.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if companyID != "" {
		if err := mw.WriteField("company_id", companyID); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", "lancamentos.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.srv.URL+"/api/v1/reconciliation/entries/import", &body)
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

type errorBody struct {
	Code    string `json:"code"`
	Details struct {
		Fields map[string]string `json:"fields"`
		Lines  []struct {
			Line    int    `json:"line"`
			Message string `json:"message"`
		} `json:"lines"`
	} `json:"details"`
}

func ptr(s string) *string { return &s }

// O lançamento do formulário nasce manual, normalizado (valor com 2 casas, sentido em maiúscula,
// documento só com dígitos) e fica no log de extração como intervenção humana.
func TestCC12_CreateManualEntry(t *testing.T) {
	e := setup(t)
	company := e.company(t, "601", "11222333000181")
	var en entries.Entry
	e.srv.Post("/api/v1/reconciliation/entries", entries.CreateInput{
		CompanyID: company, Date: "2026-08-05", Amount: "1234,5", Direction: "d", Counterparty: ptr("  Aço Brasil Ltda "),
		CounterpartyDoc: ptr("89.012.345/0001-67"), DocumentRef: ptr("NF 4410"),
	}).MustStatus(t, http.StatusCreated).JSON(t, &en)
	if en.Status != entries.StatusManual || *en.Amount != "1234.50" || *en.Direction != "D" || *en.Counterparty != "Aço Brasil Ltda" ||
		*en.CounterpartyDoc != "89012345000167" || *en.DocumentRef != "NF 4410" || *en.Date != "2026-08-05" || en.DocumentID != nil ||
		en.TraceID != nil || en.CreatedBy == nil || *en.CreatedBy != user.AdminID || len(en.Flags) != 0 || en.CompanyID != company {
		t.Fatalf("lançamento = %+v", en)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND actor_id = $1 AND company_id = $2
		AND change::jsonb @> jsonb_build_object('acao', 'inserir_lancamento', 'lancamentos', jsonb_build_array($3::text))`, user.AdminID, company, en.ID); n != 1 {
		t.Fatal("intervenção humana não registrada")
	}
}

// Validação campo a campo: nada é gravado [CLAUDE.md: dinheiro nunca em float].
func TestCC12_CreateValidation(t *testing.T) {
	e := setup(t)
	company := e.company(t, "602", "11222333000181")
	cases := []struct {
		in     entries.CreateInput
		fields []string
	}{
		{entries.CreateInput{CompanyID: company, Date: "05/08/2026", Amount: "abc", Direction: "X"}, []string{"amount", "date", "direction"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "0", Direction: "C"}, []string{"amount"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "-10.00", Direction: "C"}, []string{"amount"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "1.234", Direction: "C"}, []string{"amount"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "1.234,56", Direction: "C"}, []string{"amount"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "1e3", Direction: "C"}, []string{"amount"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "1000000000000.00", Direction: "C"}, []string{"amount"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "10", Direction: "C", CounterpartyDoc: ptr("***.456.789-**")}, []string{"counterparty_doc"}},
		{entries.CreateInput{CompanyID: company, Date: "2026-08-05", Amount: "10", Direction: "C", Counterparty: ptr(strings.Repeat("x", 501))}, []string{"counterparty"}},
		{entries.CreateInput{CompanyID: "x", Date: "2026-08-05", Amount: "10", Direction: "C"}, []string{"company_id"}},
		{entries.CreateInput{CompanyID: "00000000-0000-4000-8000-000000000000", Date: "2026-08-05", Amount: "10", Direction: "C"}, []string{"company_id"}},
	}
	for _, c := range cases {
		var body errorBody
		e.srv.Post("/api/v1/reconciliation/entries", c.in).MustStatus(t, http.StatusBadRequest).JSON(t, &body)
		var got []string
		for k := range body.Details.Fields {
			got = append(got, k)
		}
		if body.Code != "VALIDATION_ERROR" || !sameSet(got, c.fields) {
			t.Errorf("%+v: %s %v", c.in, body.Code, body.Details.Fields)
		}
	}
	if e.count(t, `SELECT count(*) FROM client_entries`) != 0 {
		t.Fatal("lançamento inválido gravado")
	}
}

func sameSet(a, b []string) bool {
	m := map[string]int{}
	for _, x := range a {
		m[x]++
	}
	for _, x := range b {
		m[x]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}

// Contábil fechado na competência da data: 423, nada gravado (AC-025).
func TestCC12_ClosedAccountingLocksManualEntries(t *testing.T) {
	e := setup(t)
	company := e.company(t, "603", "11222333000181")
	e.closeAccounting(t, company, "2026-08")
	in := entries.CreateInput{CompanyID: company, Date: "2026-08-10", Amount: "10.00", Direction: "C"}
	if r := e.srv.Post("/api/v1/reconciliation/entries", in); r.Status != http.StatusLocked || r.ErrorCode(t) != "PERIOD_CLOSED" {
		t.Fatalf("status = %d %s", r.Status, r.Body)
	}
	if r := e.importCSV(t, company, []byte("2026-09-01;5,00;C;Fulano\n2026-08-31;5,00;D;Beltrano\n")); r.Status != http.StatusLocked {
		t.Fatalf("CSV = %d %s", r.Status, r.Body)
	}
	if e.count(t, `SELECT count(*) FROM client_entries`) != 0 {
		t.Fatal("gravou com o Contábil fechado")
	}
	in.Date = "2026-09-01"
	e.srv.Post("/api/v1/reconciliation/entries", in).MustStatus(t, http.StatusCreated)
}

// O CSV da origem: cabeçalho opcional, vírgula decimal, linhas em branco puladas, colunas a mais
// ignoradas; uma intervenção humana com o nome do arquivo.
func TestCC12_ImportCSV(t *testing.T) {
	e := setup(t)
	company := e.company(t, "604", "11222333000181")
	csv := "data;valor;sentido;contraparte\n2026-08-06;20000.00;d;Aco Brasil;coluna a mais\n\n2026-08-05;2000,00;C;Ana Lucia Ferreira\n"
	var out entries.ImportResult
	e.importCSV(t, company, []byte(csv)).MustStatus(t, http.StatusCreated).JSON(t, &out)
	if out.Inserted != 2 || len(out.IDs) != 2 {
		t.Fatalf("resultado = %+v", out)
	}
	var list entries.EntryList
	e.srv.Get("/api/v1/reconciliation/entries?company_id="+company).MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Total != 2 || *list.Items[0].Date != "2026-08-05" || *list.Items[0].Amount != "2000.00" || *list.Items[1].Direction != "D" ||
		*list.Items[1].Counterparty != "Aco Brasil" || list.Items[0].Status != entries.StatusManual {
		t.Fatalf("lista = %+v", list)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano'
		AND change::jsonb @> '{"acao": "importar_lancamentos_csv", "arquivo": "lancamentos.csv"}'`); n != 1 {
		t.Fatal("importação não registrada")
	}
	// CSV do Excel em Windows-1252, com CRLF.
	latin, err := charmap.Windows1252.NewEncoder().String("data;valor;sentido;contraparte\r\n2026-08-07;10,00;C;Cerâmica São José\r\n")
	if err != nil {
		t.Fatal(err)
	}
	e.importCSV(t, company, []byte(latin)).MustStatus(t, http.StatusCreated)
	if e.count(t, `SELECT count(*) FROM client_entries WHERE counterparty = 'Cerâmica São José'`) != 1 {
		t.Fatal("Windows-1252 mal lido")
	}
}

// Tudo ou nada: com linha inválida, nada é gravado e cada linha diz o erro (a origem pulava a linha
// curta e caía na primeira data ou valor ilegível).
func TestCC12_ImportCSVAllOrNothing(t *testing.T) {
	e := setup(t)
	company := e.company(t, "605", "11222333000181")
	csv := "data;valor;sentido;contraparte\n2026-08-05;10,00;C;Ok\n2026-13-01;10;C;X\n2026-08-05;abc;C;X\n2026-08-05;10;Z;X\n2026-08-05;10\n"
	var body errorBody
	e.importCSV(t, company, []byte(csv)).MustStatus(t, http.StatusBadRequest).JSON(t, &body)
	var lines []int
	for _, l := range body.Details.Lines {
		lines = append(lines, l.Line)
	}
	if body.Code != "INVALID_ENTRIES_CSV" || !reflect.DeepEqual(lines, []int{3, 4, 5, 6}) {
		t.Fatalf("erro = %+v", body)
	}
	if e.count(t, `SELECT count(*) FROM client_entries`) != 0 {
		t.Fatal("importação parcial")
	}
	for name, c := range map[string]struct {
		company string
		data    string
		code    string
	}{
		"só cabeçalho":  {company, "data;valor;sentido;contraparte\n", "INVALID_ENTRIES_CSV"},
		"vazio":         {company, "", "INVALID_ENTRIES_CSV"},
		"sem empresa":   {"", "2026-08-05;10,00;C;X\n", "VALIDATION_ERROR"},
		"empresa falsa": {"00000000-0000-4000-8000-000000000000", "2026-08-05;10,00;C;X\n", "VALIDATION_ERROR"},
	} {
		if r := e.importCSV(t, c.company, []byte(c.data)); r.Status != http.StatusBadRequest || r.ErrorCode(t) != c.code {
			t.Errorf("%s: %d %s", name, r.Status, r.Body)
		}
	}
}

// Filtros da lista: empresa, status e período; valores inválidos são 400.
func TestCC12_ListFilters(t *testing.T) {
	e := setup(t)
	a, b := e.company(t, "606", "11222333000181"), e.company(t, "607", "99888777000166")
	e.importCSV(t, a, []byte("2026-08-01;1,00;C;A1\n2026-08-20;2,00;C;A2\n")).MustStatus(t, http.StatusCreated)
	e.importCSV(t, b, []byte("2026-08-10;3,00;D;B1\n")).MustStatus(t, http.StatusCreated)
	if _, err := e.pool.Exec(ctx, `INSERT INTO client_entries (company_id, status, flags) VALUES ($1, 'parcial', '[]')`, a); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]int{
		"":                 4,
		"?company_id=" + a: 3,
		"?status=parcial":  1,
		"?date_from=2026-08-05&date_to=2026-08-31":    2,
		"?company_id=" + a + "&status=manual&limit=1": 2,
	} {
		var list entries.EntryList
		e.srv.Get("/api/v1/reconciliation/entries"+query).MustStatus(t, http.StatusOK).JSON(t, &list)
		if int(list.Total) != want {
			t.Errorf("%q: total %d; esperado %d", query, list.Total, want)
		}
	}
	var list entries.EntryList
	e.srv.Get("/api/v1/reconciliation/entries?company_id="+a).MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Items[len(list.Items)-1].Date != nil {
		t.Fatal("o lançamento sem data devia vir por último")
	}
	raw, _ := json.Marshal(list.Items[0])
	if !strings.Contains(string(raw), `"field_confidence":{}`) || !strings.Contains(string(raw), `"flags":[]`) {
		t.Fatalf("lançamento manual = %s", raw)
	}
	for _, q := range []string{"?status=outro", "?company_id=x", "?date_from=01/08/2026", "?limit=500", "?offset=-1"} {
		e.srv.Get("/api/v1/reconciliation/entries"+q).MustStatus(t, http.StatusBadRequest)
	}
}
