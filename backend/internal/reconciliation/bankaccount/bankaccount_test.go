package bankaccount_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-021c [CC-11]: as contas bancárias da empresa (conta_bancaria da origem), cadastradas pelo
// formulário ou a partir das contas que os extratos da empresa leram.

var ctx = context.Background()

func scalar(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func company(t *testing.T, pool *pgxpool.Pool, code string) string {
	t.Helper()
	return scalar(t, pool, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, lpad($1, 14, '1'), '2025-01-01', 'PAYROLL', $2, $2) RETURNING id`, code, user.AdminID)
}

// statement importa um extrato da empresa com a identidade lida da conta.
func statement(t *testing.T, pool *pgxpool.Pool, companyID, sha, bank, branch, account, end string) {
	t.Helper()
	doc := scalar(t, pool, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type, kind, status)
		VALUES ($1, 'originais', 'originais/' || $2::text, $2, 'extrato.pdf', 'application/pdf', 'extrato', 'importado') RETURNING id`,
		companyID, sha)
	scalar(t, pool, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, $3, $4, $5, date_trunc('month', $6::date), $6::date, 0, 0, true, '{"ok": true}', 0) RETURNING id`,
		companyID, doc, bank, branch, account, end)
}

type account struct {
	ID            string  `json:"id"`
	CompanyID     string  `json:"company_id"`
	Bank          string  `json:"bank"`
	Branch        string  `json:"branch"`
	Account       string  `json:"account"`
	Label         string  `json:"label"`
	Active        bool    `json:"active"`
	Statements    int     `json:"statements"`
	LastPeriodEnd *string `json:"last_period_end"`
	CreatedBy     *string `json:"created_by"`
}

type list struct {
	Items []account `json:"items"`
	Read  []struct {
		Bank          string `json:"bank"`
		Branch        string `json:"branch"`
		Account       string `json:"account"`
		Label         string `json:"label"`
		Statements    int    `json:"statements"`
		LastPeriodEnd string `json:"last_period_end"`
	} `json:"read"`
}

func TestTEST021c_BankAccounts(t *testing.T) {
	pool := testutil.NewDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00"))
	a, b := company(t, pool, "900"), company(t, pool, "901")
	statement(t, pool, a, strings.Repeat("a", 64), "a_fonte", "0001", "12345-6", "2026-07-31")
	statement(t, pool, a, strings.Repeat("b", 64), "a_fonte", "0001", "12345-6", "2026-08-31")
	statement(t, pool, a, strings.Repeat("c", 64), "beta", "0021", "33412-7", "2026-08-31")
	base := "/api/v1/companies/" + a + "/bank-accounts"

	// Sem cadastro: as contas lidas nos extratos, exatamente como a leitura gravou.
	var before list
	srv.Get(base).MustStatus(t, http.StatusOK).JSON(t, &before)
	if len(before.Items) != 0 || len(before.Read) != 2 || before.Read[0].Label != "a_fonte 0001/12345-6" || before.Read[0].Statements != 2 ||
		before.Read[0].LastPeriodEnd != "2026-08-31" || before.Read[1].Bank != "beta" || before.Read[1].Statements != 1 {
		t.Fatalf("antes = %+v", before)
	}

	// Cadastro com um clique (os mesmos campos lidos): a conta sai da lista das lidas.
	var created account
	srv.Post(base, map[string]string{"bank": "beta", "branch": "0021", "account": "33412-7"}).MustStatus(t, http.StatusCreated).JSON(t, &created)
	if created.CompanyID != a || created.Label != "beta 0021/33412-7" || !created.Active || created.Statements != 1 ||
		created.LastPeriodEnd == nil || *created.LastPeriodEnd != "2026-08-31" || created.CreatedBy == nil || *created.CreatedBy != user.AdminID {
		t.Fatalf("cadastrada = %+v", created)
	}
	var after list
	srv.Get(base).MustStatus(t, http.StatusOK).JSON(t, &after)
	if len(after.Items) != 1 || after.Items[0].ID != created.ID || len(after.Read) != 1 || after.Read[0].Bank != "a_fonte" {
		t.Fatalf("depois = %+v", after)
	}
	// A importação passa a achar a empresa pela conta (a regra da D-027).
	if owner := scalar(t, pool, `SELECT company_id FROM bank_accounts WHERE bank = 'beta' AND branch = '0021' AND account = '33412-7'`); owner != a {
		t.Errorf("dona = %s", owner)
	}
	// O cadastro fica no log de extração, com quem fez.
	if n := scalar(t, pool, `SELECT count(*) FROM extraction_logs WHERE company_id = $1 AND alias = 'humano' AND actor_id = $2
		AND change::jsonb ->> 'acao' = 'cadastrar_conta_bancaria' AND change::jsonb ->> 'conta_bancaria' = $3`, a, user.AdminID, created.ID); n != "1" {
		t.Errorf("log = %s", n)
	}

	// Manual, com espaços nas pontas; a identidade é única no sistema.
	var manual account
	srv.Post(base, map[string]string{"bank": " itau ", "branch": "1234", "account": " 99999-9"}).MustStatus(t, http.StatusCreated).JSON(t, &manual)
	if manual.Bank != "itau" || manual.Account != "99999-9" || manual.Statements != 0 || manual.LastPeriodEnd != nil {
		t.Errorf("manual = %+v", manual)
	}
	res := srv.Post(base, map[string]string{"bank": "beta", "branch": "0021", "account": "33412-7"})
	if res.Status != http.StatusConflict || res.ErrorCode(t) != "BANK_ACCOUNT_EXISTS" || !strings.Contains(string(res.Body), "nesta empresa") {
		t.Errorf("repetida = %d %s", res.Status, res.Body)
	}
	res = srv.Post("/api/v1/companies/"+b+"/bank-accounts", map[string]string{"bank": "beta", "branch": "0021", "account": "33412-7"})
	if res.Status != http.StatusConflict || res.ErrorCode(t) != "BANK_ACCOUNT_EXISTS" || !strings.Contains(string(res.Body), "900 · Empresa 900") ||
		!strings.Contains(string(res.Body), a) {
		t.Errorf("de outra empresa = %d %s", res.Status, res.Body)
	}

	// Validação campo a campo, no tamanho da origem.
	res = srv.Post(base, map[string]string{"bank": "  ", "branch": strings.Repeat("1", 17), "account": "1\n2"})
	if res.Status != http.StatusBadRequest || res.ErrorCode(t) != "VALIDATION_ERROR" {
		t.Fatalf("inválida = %d %s", res.Status, res.Body)
	}
	var invalid struct {
		Details struct {
			Fields map[string]string `json:"fields"`
		} `json:"details"`
	}
	res.JSON(t, &invalid)
	if f := invalid.Details.Fields; f["bank"] != "Informe o banco." || !strings.Contains(f["branch"], "16") || f["account"] == "" {
		t.Errorf("campos = %v", f)
	}
	for _, id := range []string{"00000000-0000-4000-8000-000000000000", "x"} {
		if res := srv.Get("/api/v1/companies/" + id + "/bank-accounts"); res.Status != http.StatusNotFound || res.ErrorCode(t) != "COMPANY_NOT_FOUND" {
			t.Errorf("GET %s = %d", id, res.Status)
		}
		if res := srv.Post("/api/v1/companies/"+id+"/bank-accounts", map[string]string{"bank": "x", "branch": "1", "account": "2"}); res.Status != http.StatusNotFound {
			t.Errorf("POST %s = %d", id, res.Status)
		}
	}
	if n := scalar(t, pool, `SELECT count(*) FROM bank_accounts`); n != "2" {
		t.Errorf("contas = %s", n)
	}
}
