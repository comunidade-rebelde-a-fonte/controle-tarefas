package ledger_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/reconciliation/ledger"
	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-020 [REQ-028, REQ-029, AC-021, AC-022]: o razão como consulta sobre as conciliações ativas,
// rastreável até a classificação, a conciliação, a transação e o original; a exportação CSV.

var ctx = context.Background()

// O rótulo da conta bancária é o da origem (test_razao.py).
func TestBankLabel_LikeTheOrigin(t *testing.T) {
	for _, c := range []struct{ bank, branch, account, want string }{
		{"a_fonte", "1234", "56789-0", "a_fonte 1234/56789-0"},
		{"beta", "0021", "33412-7", "beta 0021/33412-7"},
		{"a_fonte", "", "", "a_fonte"},
		{" a_fonte ", "  ", "", "a_fonte"},
		{"", "1234", "56789-0", "1234/56789-0"},
		{"", "", "", ""},
	} {
		if got := ledger.BankLabel(c.bank, c.branch, c.account); got != c.want {
			t.Errorf("BankLabel(%q, %q, %q) = %q; origem %q", c.bank, c.branch, c.account, got, c.want)
		}
	}
}

type env struct {
	srv     *testutil.Server
	pool    *pgxpool.Pool
	company string
	imp     string
	pos     int
}

func (e *env) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func (e *env) line(t *testing.T, date, amount, dir, desc string) string {
	t.Helper()
	e.pos++
	return e.scalar(t, `INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount, direction, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, e.company, e.imp, e.pos, date, desc, amount, dir, fmt.Sprintf("%064x", e.pos))
}

// reconcile concilia a linha à mão com os lançamentos dados (nenhum: sem comprovante).
func (e *env) reconcile(t *testing.T, line string, entries ...string) string {
	t.Helper()
	method := "manual"
	if len(entries) == 0 {
		method = "sem_comprovante"
	}
	id := e.scalar(t, `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by)
		VALUES ($1, $2, $3, 'manual', $4) RETURNING id`, e.company, line, method, user.AdminID)
	for _, en := range entries {
		if _, err := e.pool.Exec(ctx, `INSERT INTO reconciliation_items (reconciliation_id, company_id, client_entry_id) VALUES ($1, $2, $3)`,
			id, e.company, en); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

type ledgerOut struct {
	Items []struct {
		Date                   string   `json:"date"`
		Competence             string   `json:"competence"`
		BankAccount            string   `json:"bank_account"`
		AccountCode            *string  `json:"account_code"`
		AccountName            *string  `json:"account_name"`
		ClassificationOrigin   *string  `json:"classification_origin"`
		ClassificationPending  bool     `json:"classification_pending"`
		History                string   `json:"history"`
		Counterparty           *string  `json:"counterparty"`
		Debit                  string   `json:"debit"`
		Credit                 string   `json:"credit"`
		ReconciliationID       string   `json:"reconciliation_id"`
		StatementTransactionID string   `json:"statement_transaction_id"`
		StatementDocumentID    string   `json:"statement_document_id"`
		EntryIDs               []string `json:"entry_ids"`
		ReceiptDocumentIDs     []string `json:"receipt_document_ids"`
	} `json:"items"`
	Debit  string `json:"total_debit"`
	Credit string `json:"total_credit"`
}

func (e *env) ledger(t *testing.T, query string) ledgerOut {
	t.Helper()
	var out ledgerOut
	e.srv.Get("/api/v1/companies/"+e.company+"/ledger?"+query).MustStatus(t, http.StatusOK).JSON(t, &out)
	return out
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestTEST020_LedgerIsAProjectionWithDrillDown(t *testing.T) {
	pool := testutil.NewDB(t)
	e := &env{srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00")), pool: pool}
	e.company = e.scalar(t, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('900', 'Empresa 900', '11222333000181', '2025-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID)
	statementDoc := e.scalar(t, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type)
		VALUES ($1, 'originais', 'originais/a', repeat('a', 64), 'extrato.pdf', 'application/pdf') RETURNING id`, e.company)
	receiptDoc := e.scalar(t, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type)
		VALUES ($1, 'originais', 'originais/b', repeat('b', 64), 'comprovante.pdf', 'application/pdf') RETURNING id`, e.company)
	e.imp = e.scalar(t, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'a_fonte', '1234', '56789-0', '2026-08-01', '2026-09-30', 0, 0, true, '{"ok": true}', 0) RETURNING id`, e.company, statementDoc)
	for code, name := range map[string]string{"1.01": "Receita de mensalidades", "4.01": "Despesas bancárias"} {
		e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts", map[string]string{"code": code, "name": name}).MustStatus(t, http.StatusCreated)
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules",
		map[string]any{"rule_type": "regex_descritivo", "pattern": "tarifa", "account_code": "4.01"}).MustStatus(t, http.StatusCreated)

	tariff := e.line(t, "2026-08-01", "89.90", "D", "Tarifa Manutencao Conta Pj")
	tariffRec := e.reconcile(t, tariff)
	school := e.line(t, "2026-08-26", "20000.00", "C", "Pix Recebido Escola Crescer")
	entry := e.scalar(t, `INSERT INTO client_entries (company_id, document_id, entry_date, amount, direction, counterparty, counterparty_doc, status)
		VALUES ($1, $2, '2026-08-26', 20000.00, 'C', 'Escola Crescer', '98765432000110', 'extraido') RETURNING id`, e.company, receiptDoc)
	schoolRec := e.reconcile(t, school, entry)
	undone := e.line(t, "2026-08-20", "50.00", "D", "PIX ENVIADO DESFEITO")
	undoneRec := e.reconcile(t, undone)
	if _, err := pool.Exec(ctx, `UPDATE reconciliations SET undone_at = now(), undone_by = $2 WHERE id = $1`, undoneRec, user.AdminID); err != nil {
		t.Fatal(err)
	}
	september := e.line(t, "2026-09-02", "150.00", "D", "Tarifa Pix")
	e.reconcile(t, september)
	e.srv.Post("/api/v1/companies/"+e.company+"/classification/run?competence=2026-08", nil).MustStatus(t, http.StatusOK)

	got := e.ledger(t, "from=2026-08&to=2026-08")
	if len(got.Items) != 2 || got.Debit != "89.90" || got.Credit != "20000.00" {
		t.Fatalf("razão de agosto = %+v", got)
	}
	l1, l2 := got.Items[0], got.Items[1]
	if l1.Date != "2026-08-01" || l1.Competence != "2026-08" || l1.BankAccount != "a_fonte 1234/56789-0" || str(l1.AccountCode) != "4.01" ||
		str(l1.AccountName) != "Despesas bancárias" || str(l1.ClassificationOrigin) != "regra" || l1.Debit != "89.90" || l1.Credit != "0.00" ||
		l1.ReconciliationID != tariffRec || l1.StatementTransactionID != tariff || l1.StatementDocumentID != statementDoc || len(l1.EntryIDs) != 0 {
		t.Fatalf("linha da tarifa = %+v", l1)
	}
	if l2.AccountCode != nil || !l2.ClassificationPending || str(l2.Counterparty) != "Escola Crescer" || l2.Credit != "20000.00" ||
		l2.Debit != "0.00" || !slices.Equal(l2.EntryIDs, []string{entry}) || !slices.Equal(l2.ReceiptDocumentIDs, []string{receiptDoc}) {
		t.Fatalf("linha da escola = %+v", l2)
	}

	// Filtro da origem: a conta bancária (sem caixa) ou a conta contábil.
	if n := len(e.ledger(t, "from=2026-08&to=2026-08&account=4.01").Items); n != 1 {
		t.Fatalf("filtro pela conta contábil = %d", n)
	}
	if n := len(e.ledger(t, "from=2026-08&to=2026-08&account=A_FONTE%201234").Items); n != 2 {
		t.Fatalf("filtro pela conta bancária = %d", n)
	}
	if n := len(e.ledger(t, "from=2026-08&to=2026-09").Items); n != 3 {
		t.Fatalf("razão de agosto a setembro = %d", n)
	}

	// Reclassificar muda o razão sem escrever nele: ele é consulta (M5.6).
	e.srv.Put("/api/v1/reconciliation/review/"+school+"/classification", map[string]string{"account_code": "1.01"}).MustStatus(t, http.StatusOK)
	if l := e.ledger(t, "from=2026-08&to=2026-08").Items[1]; str(l.AccountCode) != "1.01" || str(l.ClassificationOrigin) != "manual" || l.ClassificationPending {
		t.Fatalf("razão depois da correção = %+v", l)
	}

	// Exportação CSV: as colunas da origem e as do AC-022, determinística.
	res := e.srv.Get("/api/v1/companies/"+e.company+"/ledger/export?format=csv&from=2026-08&to=2026-08").MustStatus(t, http.StatusOK)
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("tipo = %s", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, `filename="razao-11222333000181-2026-08-2026-08.csv"`) {
		t.Fatalf("arquivo = %s", cd)
	}
	want := "data;conta;historico;debito;credito;competencia;conta_contabil;nome_conta;contraparte;origem_classificacao;origem_conciliacao\n" +
		"2026-08-01;a_fonte 1234/56789-0;Tarifa Manutencao Conta Pj;89.90;0.00;2026-08;4.01;Despesas bancárias;;regra;manual\n" +
		"2026-08-26;a_fonte 1234/56789-0;Pix Recebido Escola Crescer;0.00;20000.00;2026-08;1.01;Receita de mensalidades;Escola Crescer;manual;manual\n"
	if string(res.Body) != want {
		t.Fatalf("CSV:\n%s\nquer:\n%s", res.Body, want)
	}
	again := e.srv.Get("/api/v1/companies/"+e.company+"/ledger/export?from=2026-08&to=2026-08").MustStatus(t, http.StatusOK)
	if string(again.Body) != want {
		t.Fatal("exportação não determinística")
	}
	e.srv.Get("/api/v1/companies/"+e.company+"/ledger/export?format=xlsx&from=2026-08&to=2026-08").MustStatus(t, http.StatusBadRequest)

	// Drill-down (AC-021): a conciliação leva à classificação (com a regra), à transação com o
	// original do extrato e aos comprovantes com os originais deles.
	var d struct {
		Method      string `json:"method"`
		Transaction struct {
			ID                  string `json:"id"`
			StatementDocumentID string `json:"statement_document_id"`
			BankAccount         string `json:"bank_account"`
		} `json:"statement_transaction"`
		Entries []struct {
			ID         string  `json:"id"`
			DocumentID *string `json:"document_id"`
		} `json:"entries"`
		Classification struct {
			AccountCode *string `json:"account_code"`
			AccountName *string `json:"account_name"`
			Origin      *string `json:"origin"`
			Rule        *struct {
				RuleType string `json:"rule_type"`
				Pattern  string `json:"pattern"`
			} `json:"rule"`
			ClassifiedBy *string `json:"classified_by"`
		} `json:"classification"`
	}
	e.srv.Get("/api/v1/reconciliation/reconciliations/"+tariffRec).MustStatus(t, http.StatusOK).JSON(t, &d)
	if d.Method != "sem_comprovante" || d.Transaction.ID != tariff || d.Transaction.StatementDocumentID != statementDoc ||
		d.Transaction.BankAccount != "a_fonte 1234/56789-0" || len(d.Entries) != 0 || str(d.Classification.Origin) != "regra" ||
		d.Classification.Rule == nil || d.Classification.Rule.Pattern != "tarifa" || str(d.Classification.AccountName) != "Despesas bancárias" {
		t.Fatalf("drill-down da tarifa = %+v", d)
	}
	e.srv.Get("/api/v1/reconciliation/reconciliations/"+schoolRec).MustStatus(t, http.StatusOK).JSON(t, &d)
	if len(d.Entries) != 1 || d.Entries[0].ID != entry || str(d.Entries[0].DocumentID) != receiptDoc || str(d.Classification.Origin) != "manual" ||
		str(d.Classification.ClassifiedBy) != user.AdminID || d.Classification.Rule != nil {
		t.Fatalf("drill-down da escola = %+v", d)
	}
	for _, id := range []string{"00000000-0000-4000-8000-000000000000", "x"} {
		if code := e.srv.Get("/api/v1/reconciliation/reconciliations/"+id).MustStatus(t, http.StatusNotFound).ErrorCode(t); code != "RECONCILIATION_NOT_FOUND" {
			t.Fatalf("código = %s", code)
		}
	}

	// Período: obrigatório, AAAA-MM e em ordem; empresa inexistente, 404.
	for _, q := range []string{"", "from=2026-08", "from=2026-13&to=2026-08", "from=2026-09&to=2026-08"} {
		e.srv.Get("/api/v1/companies/"+e.company+"/ledger?"+q).MustStatus(t, http.StatusBadRequest)
	}
	e.srv.Get("/api/v1/companies/00000000-0000-4000-8000-000000000000/ledger?from=2026-08&to=2026-08").MustStatus(t, http.StatusNotFound)
}
