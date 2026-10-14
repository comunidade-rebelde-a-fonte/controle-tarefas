package ledger_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"controle/internal/platform/decimal"
	"controle/internal/reconciliation/ledger"
	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-028 [REQ-029, AC-022]: o razão filtrado exportado em CSV e em XLSX, comparado aos fixtures
// de testdata/. O cenário tem duas contas bancárias, uma conciliação automática, uma desfeita (fora
// do razão), uma linha sem classificação, aspas e ponto e vírgula no texto e dois meses. A planilha
// tem as abas Razão e Resumo, com números e datas de verdade (decisões do usuário, D-042), e sai
// igual a cada exportação.

const (
	csvFixture  = "testdata/razao_2026-08_2026-09.csv"
	xlsxFixture = "testdata/razao_2026-08_2026-09.xlsx.json"
	betaLabel   = "beta 0021/33412-7"
)

func exportScenario(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	e := &env{srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00")), pool: pool}
	e.company = e.scalar(t, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('901', 'Construtora Exemplo Ltda', '11222333000181', '2025-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID)
	statement := func(sha, bank, branch, account string) string {
		doc := e.scalar(t, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type)
			VALUES ($1, 'originais', 'originais/' || $2, repeat($2, 64), 'extrato-' || $3 || '.pdf', 'application/pdf') RETURNING id`,
			e.company, sha, bank)
		return e.scalar(t, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
			opening_balance, closing_balance, validation_ok, validation, attempts)
			VALUES ($1, $2, $3, $4, $5, '2026-08-01', '2026-09-30', 0, 0, true, '{"ok": true}', 0) RETURNING id`,
			e.company, doc, bank, branch, account)
	}
	entry := func(date, amount, direction, counterparty string) string {
		return e.scalar(t, `INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status)
			VALUES ($1, $2, $3, $4, $5, 'manual') RETURNING id`, e.company, date, amount, direction, counterparty)
	}
	for _, a := range [][2]string{{"1.1.2.01", "Clientes"}, {"2.1.1.01", "Fornecedores; diversos"}, {"4.3.1.01", "Despesas bancárias"}} {
		e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts", map[string]string{"code": a[0], "name": a[1]}).MustStatus(t, http.StatusCreated)
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules",
		map[string]any{"rule_type": "regex_descritivo", "pattern": "tarifa", "account_code": "4.3.1.01"}).MustStatus(t, http.StatusCreated)

	fonte, beta := statement("c", "a_fonte", "1234", "56789-0"), statement("d", "beta", "0021", "33412-7")
	e.imp = fonte
	e.reconcile(t, e.line(t, "2026-08-01", "89.90", "D", "Tarifa Manutencao Conta Pj"))
	ted := e.line(t, "2026-08-05", "120000.00", "C", `Ted Recebido "Vale Verde"`)
	auto := e.scalar(t, `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, pass, score)
		VALUES ($1, $2, 'passe_1', 'auto', 1, 1) RETURNING id`, e.company, ted)
	if _, err := e.pool.Exec(ctx, `INSERT INTO reconciliation_items (reconciliation_id, company_id, client_entry_id) VALUES ($1, $2, $3)`,
		auto, e.company, entry("2026-08-05", "120000.00", "C", "Incorporadora Vale Verde Ltda")); err != nil {
		t.Fatal(err)
	}
	e.reconcile(t, e.line(t, "2026-08-13", "18696.50", "D", "Boleto Ceramica Santa Rita"), entry("2026-08-13", "18696.50", "D", "Cerâmica Santa Rita Ltda"))
	undone := e.reconcile(t, e.line(t, "2026-08-20", "50.00", "D", "PIX ENVIADO DESFEITO"))
	if _, err := e.pool.Exec(ctx, `UPDATE reconciliations SET undone_at = now(), undone_by = $2 WHERE id = $1`, undone, user.AdminID); err != nil {
		t.Fatal(err)
	}
	e.reconcile(t, e.line(t, "2026-09-02", "150.00", "D", "Tarifa Pix"))
	e.imp = beta
	madeireira := e.line(t, "2026-08-10", "12640.00", "D", "Pagamento Boleto Madeireira")
	e.reconcile(t, madeireira, entry("2026-08-10", "12640.00", "D", "Madeireira Pinheiro Ltda"))

	// As tarifas pela regra; o TED e a madeireira pela correção do contador; a cerâmica sem conta.
	for _, c := range []string{"2026-08", "2026-09"} {
		e.srv.Post("/api/v1/companies/"+e.company+"/classification/run?competence="+c, nil).MustStatus(t, http.StatusOK)
	}
	e.srv.Put("/api/v1/reconciliation/review/"+ted+"/classification", map[string]string{"account_code": "1.1.2.01"}).MustStatus(t, http.StatusOK)
	e.srv.Put("/api/v1/reconciliation/review/"+madeireira+"/classification", map[string]string{"account_code": "2.1.1.01"}).MustStatus(t, http.StatusOK)
	return e
}

// attachment confere os cabeçalhos do download: o navegador não adivinha outro tipo e o razão
// não fica em cache.
func attachment(t *testing.T, res testutil.Response) {
	t.Helper()
	if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("cabeçalhos do download = %v", res.Header)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// numeric diz se a célula é número para o Excel (sem tipo ou "n").
func numeric(t *testing.T, f *excelize.File, sheet, cell string) bool {
	t.Helper()
	ct, err := f.GetCellType(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	return ct == excelize.CellTypeUnset || ct == excelize.CellTypeNumber
}

func cellStyle(t *testing.T, f *excelize.File, sheet, cell string) *excelize.Style {
	t.Helper()
	id, err := f.GetCellStyle(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.GetStyle(id)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func openXLSX(t *testing.T, body []byte) *excelize.File {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func rawRows(t *testing.T, f *excelize.File, sheet string) [][]string {
	t.Helper()
	rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestTEST028_ExportsMatchTheFixtures(t *testing.T) {
	e := exportScenario(t)
	base := "/api/v1/companies/" + e.company + "/ledger/export?from=2026-08&to=2026-09"

	// CSV: o fixture byte a byte (as aspas e o ponto e vírgula do texto vão entre aspas), o padrão
	// sem format e a mesma saída a cada exportação.
	wantCSV := readFixture(t, csvFixture)
	res := e.srv.Get(base+"&format=csv").MustStatus(t, http.StatusOK)
	if ct := res.Header.Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("tipo do CSV = %s", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != `attachment; filename="razao-11222333000181-2026-08-2026-09.csv"` {
		t.Fatalf("arquivo CSV = %s", cd)
	}
	if !bytes.Equal(res.Body, wantCSV) {
		t.Fatalf("CSV:\n%s\nfixture:\n%s", res.Body, wantCSV)
	}
	attachment(t, res)
	if again := e.srv.Get(base).MustStatus(t, http.StatusOK); !bytes.Equal(again.Body, wantCSV) {
		t.Fatal("CSV não determinístico (ou o padrão não é CSV)")
	}
	// O filtro de conta dá as linhas do fixture daquela conta.
	lines := strings.SplitAfter(string(wantCSV), "\n")
	betaCSV := lines[0]
	for _, l := range lines[1:] {
		if strings.Contains(l, ";"+betaLabel+";") {
			betaCSV += l
		}
	}
	if got := e.srv.Get(base+"&format=csv&account=beta").MustStatus(t, http.StatusOK); string(got.Body) != betaCSV {
		t.Fatalf("CSV filtrado:\n%s\nquer:\n%s", got.Body, betaCSV)
	}

	// XLSX: as abas, os valores brutos de cada célula (fixture), os tipos e os formatos.
	x := e.srv.Get(base+"&format=xlsx").MustStatus(t, http.StatusOK)
	if ct := x.Header.Get("Content-Type"); ct != ledger.MimeXLSX {
		t.Fatalf("tipo do XLSX = %s", ct)
	}
	if cd := x.Header.Get("Content-Disposition"); cd != `attachment; filename="razao-11222333000181-2026-08-2026-09.xlsx"` {
		t.Fatalf("arquivo XLSX = %s", cd)
	}
	if again := e.srv.Get(base+"&format=xlsx").MustStatus(t, http.StatusOK); !bytes.Equal(again.Body, x.Body) {
		t.Fatal("XLSX não determinístico")
	}
	attachment(t, x)
	var want map[string][][]string
	if err := json.Unmarshal(readFixture(t, xlsxFixture), &want); err != nil {
		t.Fatal(err)
	}
	f := openXLSX(t, x.Body)
	if got := f.GetSheetList(); !slices.Equal(got, []string{"Razão", "Resumo"}) || f.GetActiveSheetIndex() != 0 {
		t.Fatalf("abas = %v (ativa %d)", got, f.GetActiveSheetIndex())
	}
	for _, sheet := range []string{"Razão", "Resumo"} {
		if got := rawRows(t, f, sheet); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", want[sheet]) {
			t.Fatalf("aba %s:\n%q\nfixture:\n%q", sheet, got, want[sheet])
		}
	}
	for row := 2; row <= len(want["Razão"]); row++ {
		for _, col := range []string{"A", "D", "E"} {
			if !numeric(t, f, "Razão", fmt.Sprintf("%s%d", col, row)) {
				t.Fatalf("%s%d deveria ser número", col, row)
			}
		}
		if ct, _ := f.GetCellType("Razão", fmt.Sprintf("B%d", row)); ct != excelize.CellTypeSharedString {
			t.Fatalf("B%d deveria ser texto: %v", row, ct)
		}
		if st := cellStyle(t, f, "Razão", fmt.Sprintf("A%d", row)); st.CustomNumFmt == nil || *st.CustomNumFmt != "dd/mm/yyyy" {
			t.Fatalf("formato da data na linha %d = %+v", row, st)
		}
		for _, col := range []string{"D", "E"} {
			if st := cellStyle(t, f, "Razão", fmt.Sprintf("%s%d", col, row)); st.NumFmt != 4 {
				t.Fatalf("formato de %s%d = %d", col, row, st.NumFmt)
			}
		}
	}
	if st := cellStyle(t, f, "Razão", "A1"); st.Font == nil || !st.Font.Bold {
		t.Fatal("cabeçalho sem negrito")
	}
	if p, err := f.GetPanes("Razão"); err != nil || !p.Freeze || p.YSplit != 1 {
		t.Fatalf("cabeçalho congelado = %+v (%v)", p, err)
	}
	filtered := false
	for _, dn := range f.GetDefinedName() {
		filtered = filtered || (dn.Name == "_xlnm._FilterDatabase" && strings.Contains(dn.RefersTo, "$A$1:$K$6"))
	}
	if !filtered {
		t.Fatalf("autofiltro ausente: %+v", f.GetDefinedName())
	}
	for _, cell := range []string{"B5", "B6", "B7"} {
		if !numeric(t, f, "Resumo", cell) {
			t.Fatalf("Resumo %s deveria ser número", cell)
		}
	}
	for _, cell := range []string{"B6", "B7"} {
		if st := cellStyle(t, f, "Resumo", cell); st.NumFmt != 4 {
			t.Fatalf("formato do total %s = %d", cell, st.NumFmt)
		}
	}

	// XLSX filtrado: a linha da conta e o filtro no Resumo.
	fb := openXLSX(t, e.srv.Get(base+"&format=xlsx&account=beta").MustStatus(t, http.StatusOK).Body)
	if got := rawRows(t, fb, "Razão"); len(got) != 2 || fmt.Sprintf("%q", got[1]) != fmt.Sprintf("%q", want["Razão"][3]) {
		t.Fatalf("XLSX filtrado = %q", got)
	}
	if got := rawRows(t, fb, "Resumo"); got[3][1] != "beta" || got[4][1] != "1" || got[5][1] != "12640.00" || got[6][1] != "0.00" {
		t.Fatalf("Resumo filtrado = %q", got)
	}

	// Formato desconhecido.
	if code := e.srv.Get(base+"&format=pdf").MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "VALIDATION_ERROR" {
		t.Fatalf("formato desconhecido = %s", code)
	}
}

// Sem nenhuma linha, a planilha tem o cabeçalho, o resumo zerado e o autofiltro.
func TestTEST028_EmptyLedgerSpreadsheet(t *testing.T) {
	zero := ledger.Ledger{CompanyID: "x", From: "2026-08", To: "2026-08"}
	zero.Debit, zero.Credit = decimal.MustParse("0.00"), decimal.MustParse("0.00")
	body, err := ledger.XLSX(zero, ledger.Info{CompanyCode: "901", CompanyName: "Construtora Exemplo Ltda", Cnpj: "11222333000181"})
	if err != nil {
		t.Fatal(err)
	}
	f := openXLSX(t, body)
	if got := rawRows(t, f, "Razão"); len(got) != 1 || len(got[0]) != 11 {
		t.Fatalf("razão vazio = %q", got)
	}
	if got := rawRows(t, f, "Resumo"); got[2][1] != "08/2026 a 08/2026" || got[4][1] != "0" || got[5][1] != "0.00" {
		t.Fatalf("resumo vazio = %q", got)
	}
}
