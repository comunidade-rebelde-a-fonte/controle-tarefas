package reconciliation_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/platform/decimal"
	"controle/internal/platform/migrate"
	"controle/internal/reconciliation"
	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-007: migrations, FKs para companies(id), dedupe, portão no banco e trava pelo
// fechamento Contábil, contra Postgres real.

var ctx = context.Background()

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func scalar[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func company(t *testing.T, pool *pgxpool.Pool, code, cnpj string) string {
	t.Helper()
	return scalar[string](t, pool, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, $2, '2025-01-01', 'PAYROLL', $3, $3) RETURNING id`, code, cnpj, user.AdminID)
}

func document(t *testing.T, pool *pgxpool.Pool, companyID any, sha string) string {
	t.Helper()
	return scalar[string](t, pool, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type)
		VALUES ($1, 'originais', 'originais/' || $2, $2, 'extrato.pdf', 'application/pdf') RETURNING id`, companyID, sha)
}

func statementImport(t *testing.T, pool *pgxpool.Pool, companyID, docID string) string {
	t.Helper()
	return scalar[string](t, pool, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'a_fonte', '0001', '12345-6', '2026-08-01', '2026-08-31', 100.00, 90.00, true, '{"ok": true}', 1) RETURNING id`,
		companyID, docID)
}

const insertTxn = `INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount, direction, dedupe_key)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`

func sha(c byte) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

func TestTEST007_MigrationRevertsAndReapplies(t *testing.T) {
	pool := testutil.NewDB(t)
	tables := []string{"bank_accounts", "reconciliation_documents", "statement_imports", "statement_transactions", "client_entries",
		"reconciliations", "reconciliation_items", "chart_accounts", "classification_rules", "bank_playbooks", "extraction_logs", "resolver_proposals"}
	exists := func(name string) bool {
		return scalar[bool](t, pool, `SELECT to_regclass($1) IS NOT NULL`, name)
	}
	if err := migrate.DownTo(ctx, testutil.URLFor(pool), 19, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, n := range tables {
		if exists(n) {
			t.Fatalf("%s continua existindo depois do down", n)
		}
	}
	if err := migrate.Up(ctx, testutil.URLFor(pool), io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, n := range tables {
		if !exists(n) {
			t.Fatalf("%s não voltou com o up", n)
		}
	}
	// A Conciliação não tem fechamento próprio [REQ-033]: nenhuma tabela de fechamento além da do Controle.
	extra := scalar[int](t, pool, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'
		AND (table_name ILIKE '%fechamento%' OR table_name ILIKE '%closing%' OR table_name ILIKE '%lock%')`)
	if extra != 0 {
		t.Fatalf("%d tabela(s) de fechamento paralela(s)", extra)
	}
}

func TestTEST007_StatementGateDedupeAndCompanyIdentity(t *testing.T) {
	pool := testutil.NewDB(t)
	alfa := company(t, pool, "301", "11222333000181")
	beta := company(t, pool, "302", "11444777000161")
	doc := document(t, pool, alfa, sha('a'))
	imp := statementImport(t, pool, alfa, doc)

	// Portão no banco: importação reprovada não existe [AC-015].
	_, err := pool.Exec(ctx, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'a_fonte', '0001', '12345-6', '2026-08-01', '2026-08-31', 100, 90, false, '{}', 3)`, alfa, document(t, pool, alfa, sha('b')))
	if !db.IsConstraint(err, db.CheckViolation, "") {
		t.Fatalf("importação com validation_ok = false deveria falhar: %v", err)
	}

	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	key := reconciliation.DedupeKey("12345-6", day, decimal.MustParse("10.00"), "D", "PIX João", 1)
	txn := scalar[string](t, pool, insertTxn, alfa, imp, 1, day, "PIX João", "10.00", "D", key)
	if c := scalar[time.Time](t, pool, `SELECT competence FROM statement_transactions WHERE id = $1`, txn); !c.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("competência gerada = %s", c)
	}
	// Dedupe: a mesma transação não entra de novo, nem por outra importação [05 §10].
	imp2 := statementImport(t, pool, alfa, document(t, pool, alfa, sha('c')))
	same := reconciliation.DedupeKey("12345-6", day, decimal.MustParse("10"), "D", "pix JOÃO", 1)
	if _, err := pool.Exec(ctx, insertTxn, alfa, imp2, 1, day, "pix JOÃO", "10.00", "D", same); !db.IsConstraint(err, db.UniqueViolation, "statement_transactions_dedupe_uq") {
		t.Fatalf("transação repetida deveria falhar no dedupe: %v", err)
	}
	cases := []struct {
		name string
		args []any
		code string
	}{
		{"empresa diferente da importação", []any{beta, imp, 2, day, "x", "1.00", "C", sha('1')}, db.ForeignKeyViolation},
		{"valor zero", []any{alfa, imp, 2, day, "x", "0", "C", sha('2')}, db.CheckViolation},
		{"sentido inválido", []any{alfa, imp, 2, day, "x", "1.00", "X", sha('3')}, db.CheckViolation},
		{"chave fora do formato", []any{alfa, imp, 2, day, "x", "1.00", "C", "abc"}, db.CheckViolation},
	}
	for _, c := range cases {
		if _, err := pool.Exec(ctx, insertTxn, c.args...); !db.IsConstraint(err, c.code, "") {
			t.Errorf("%s: esperado %s, veio %v", c.name, c.code, err)
		}
	}
	// Originais e importações não somem: a empresa com dados não pode ser apagada.
	if _, err := pool.Exec(ctx, `DELETE FROM companies WHERE id = $1`, alfa); !db.IsConstraint(err, db.ForeignKeyViolation, "") {
		t.Fatalf("apagar empresa com conciliação deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO reconciliation_documents (bucket, object_key, sha256, file_name, content_type)
		VALUES ('originais', 'outro', $1, 'x.pdf', 'application/pdf')`, sha('a')); !db.IsConstraint(err, db.UniqueViolation, "reconciliation_documents_sha256_uq") {
		t.Fatalf("documento com o mesmo hash deveria ser duplicado: %v", err)
	}
}

func TestTEST007_ReconciliationsNOneActiveAndUndo(t *testing.T) {
	pool := testutil.NewDB(t)
	alfa := company(t, pool, "301", "11222333000181")
	beta := company(t, pool, "302", "11444777000161")
	imp := statementImport(t, pool, alfa, document(t, pool, alfa, sha('a')))
	day := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	txn := scalar[string](t, pool, insertTxn, alfa, imp, 1, day, "PIX", "30.00", "C", sha('1'))
	entry := func(company, amount string) string {
		return scalar[string](t, pool, `INSERT INTO client_entries (company_id, entry_date, amount, direction, status) VALUES ($1, $2, $3, 'C', 'extraido') RETURNING id`,
			company, day, amount)
	}
	e1, e2, other := entry(alfa, "10.00"), entry(alfa, "20.00"), entry(beta, "30.00")

	// Passe 4 (N:1): uma conciliação, dois lançamentos.
	rec := scalar[string](t, pool, `INSERT INTO reconciliations (company_id, statement_transaction_id, score, pass, method, origin)
		VALUES ($1, $2, 0.8, 4, 'passe_4', 'auto') RETURNING id`, alfa, txn)
	exec(t, pool, `INSERT INTO reconciliation_items (reconciliation_id, company_id, client_entry_id) VALUES ($1, $2, $3), ($1, $2, $4)`, rec, alfa, e1, e2)

	_, err := pool.Exec(ctx, `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by)
		VALUES ($1, $2, 'manual', 'manual', $3)`, alfa, txn, user.AdminID)
	if !db.IsConstraint(err, db.UniqueViolation, "reconciliations_active_txn_uq") {
		t.Fatalf("segunda conciliação ativa da transação deveria falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO reconciliation_items (reconciliation_id, company_id, client_entry_id) VALUES ($1, $2, $3)`, rec, alfa, other); !db.IsConstraint(err, db.ForeignKeyViolation, "") {
		t.Fatalf("lançamento de outra empresa deveria falhar: %v", err)
	}
	for name, sql := range map[string]string{
		"auto sem passe":   `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin) VALUES ($1, $2, 'passe_1', 'auto')`,
		"manual sem autor": `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin) VALUES ($1, $2, 'manual', 'manual')`,
		"origem inválida":  `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, pass) VALUES ($1, $2, 'x', 'agente', 1)`,
	} {
		if _, err := pool.Exec(ctx, sql, alfa, txn); !db.IsConstraint(err, db.CheckViolation, "") {
			t.Errorf("%s: esperado CHECK, veio %v", name, err)
		}
	}

	// Desfazer não apaga: marca a conciliação e libera os lançamentos, que podem ser usados de novo.
	exec(t, pool, `UPDATE reconciliations SET undone_at = now(), undone_by = $2, undo_reason = 'teste' WHERE id = $1`, rec, user.AdminID)
	exec(t, pool, `UPDATE reconciliation_items SET released_at = now() WHERE reconciliation_id = $1`, rec)
	rec2 := scalar[string](t, pool, `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by, score)
		VALUES ($1, $2, 'manual', 'manual', $3, 1) RETURNING id`, alfa, txn, user.AdminID)
	exec(t, pool, `INSERT INTO reconciliation_items (reconciliation_id, company_id, client_entry_id) VALUES ($1, $2, $3)`, rec2, alfa, e1)
	txn2 := scalar[string](t, pool, insertTxn, alfa, imp, 2, day, "PIX 2", "10.00", "C", sha('2'))
	rec3 := scalar[string](t, pool, `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by)
		VALUES ($1, $2, 'manual', 'manual', $3) RETURNING id`, alfa, txn2, user.AdminID)
	if _, err := pool.Exec(ctx, `INSERT INTO reconciliation_items (reconciliation_id, company_id, client_entry_id) VALUES ($1, $2, $3)`,
		rec3, alfa, e1); !db.IsConstraint(err, db.UniqueViolation, "reconciliation_items_active_entry_uq") {
		t.Fatalf("lançamento em duas conciliações ativas deveria falhar: %v", err)
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM reconciliations WHERE statement_transaction_id = $1`, txn); n != 2 {
		t.Fatalf("a conciliação desfeita deveria continuar registrada: %d", n)
	}
}

func TestTEST007_PlaybooksBankAccountsAndProposals(t *testing.T) {
	pool := testutil.NewDB(t)
	alfa := company(t, pool, "301", "11222333000181")
	beta := company(t, pool, "302", "11444777000161")
	ins := `INSERT INTO bank_playbooks (bank, version, table_start_page, date_format, decimal_separator, sign_representation,
		balance_location, winning_strategy, valid) VALUES ('a_fonte', $1, 1, 'DD/MM/YYYY', ',', 'D/C', 'rodapé', 'texto_nativo', $2)`
	exec(t, pool, ins, 1, false)
	exec(t, pool, ins, 2, true)
	if _, err := pool.Exec(ctx, ins, 3, true); !db.IsConstraint(err, db.UniqueViolation, "bank_playbooks_valid_uq") {
		t.Fatalf("dois playbooks válidos para o mesmo banco deveriam falhar: %v", err)
	}
	if _, err := pool.Exec(ctx, ins, 2, false); !db.IsConstraint(err, db.UniqueViolation, "bank_playbooks_version_uq") {
		t.Fatalf("versão repetida deveria falhar: %v", err)
	}
	acc := `INSERT INTO bank_accounts (company_id, bank, branch, account) VALUES ($1, 'a_fonte', '0001', '12345-6')`
	exec(t, pool, acc, alfa)
	if _, err := pool.Exec(ctx, acc, beta); !db.IsConstraint(err, db.UniqueViolation, "bank_accounts_identity_uq") {
		t.Fatalf("a mesma conta bancária em duas empresas deveria falhar: %v", err)
	}
	imp := statementImport(t, pool, alfa, document(t, pool, alfa, sha('a')))
	txn := scalar[string](t, pool, insertTxn, alfa, imp, 1, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "TED", "5.00", "D", sha('1'))
	// O resolvedor só propõe; decidir exige autor e data [AC-019].
	p := scalar[string](t, pool, `INSERT INTO resolver_proposals (company_id, statement_transaction_id, payload) VALUES ($1, $2, '{"hipotese": "x"}') RETURNING id`, alfa, txn)
	if _, err := pool.Exec(ctx, `UPDATE resolver_proposals SET status = 'aceita' WHERE id = $1`, p); !db.IsConstraint(err, db.CheckViolation, "resolver_proposals_decision_ck") {
		t.Fatalf("aceite sem data de decisão deveria falhar: %v", err)
	}
	if n := scalar[int](t, pool, `SELECT count(*) FROM reconciliations`); n != 0 {
		t.Fatalf("proposta não pode criar conciliação: %d", n)
	}
}

// Trava pelo fechamento Contábil, a única autoridade [REQ-033, AC-025, AC-026].
func TestTEST007_AccountingClosingLocksReconciliation(t *testing.T) {
	pool := testutil.NewDB(t)
	alfa := company(t, pool, "301", "11222333000181")
	aug := competence.MustParse("2026-08")
	comp := scalar[string](t, pool, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-08', 2026, 8, '2026-08-01') RETURNING id`)

	check := func(want bool) {
		t.Helper()
		closed, err := reconciliation.AccountingClosed(ctx, pool, alfa, aug)
		if err != nil || closed != want {
			t.Fatalf("AccountingClosed = %v, %v; esperado %v", closed, err, want)
		}
	}
	check(false) // sem período gerado, sem trava
	exec(t, pool, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'FISCAL', 'CLOSED', now(), $3)`, alfa, comp, user.AdminID)
	check(false) // Fiscal fechado não trava a Conciliação
	period := scalar[string](t, pool, `INSERT INTO company_module_periods (company_id, competence_id, module_code) VALUES ($1, $2, 'ACCOUNTING') RETURNING id`, alfa, comp)
	check(false)
	exec(t, pool, `UPDATE company_module_periods SET state = 'CLOSED', closed_at = now(), closed_by = $2 WHERE id = $1`, period, user.AdminID)
	check(true)
	err := reconciliation.RequireOpen(ctx, pool, alfa, aug)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Status != http.StatusLocked || ae.Code != "PERIOD_CLOSED" {
		t.Fatalf("RequireOpen = %v; esperado 423 PERIOD_CLOSED", err)
	}
	if err := reconciliation.RequireOpen(ctx, pool, alfa, competence.MustParse("2026-09")); err != nil {
		t.Fatalf("outra competência não está travada: %v", err)
	}
}
