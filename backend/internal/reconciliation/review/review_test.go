package review_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-021a [REQ-023, REQ-024, REQ-030, AC-025]: a fila de revisão — os baldes, a linha com os
// candidatos da origem e as ações humanas: conciliar à mão (N:1), sem comprovante, desfazer (a
// automática também, com motivo, sem voltar ao automático) e corrigir o comprovante parcial.

var ctx = context.Background()

type env struct {
	srv     *testutil.Server
	pool    *pgxpool.Pool
	company string
	imp     string
	pos     int
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	e := &env{srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00")), pool: pool}
	e.company = e.scalar(t, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('900', 'Empresa 900', '11222333000181', '2025-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID)
	doc := e.scalar(t, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type)
		VALUES ($1, 'originais', 'originais/x', repeat('a', 64), 'extrato.pdf', 'application/pdf') RETURNING id`, e.company)
	e.imp = e.scalar(t, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'a_fonte', '0001', '12345-6', '2026-08-01', '2026-08-31', 0, 0, true, '{"ok": true}', 0) RETURNING id`, e.company, doc)
	e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts", map[string]string{"code": "4.01", "name": "Despesas bancárias"}).MustStatus(t, http.StatusCreated)
	e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules",
		map[string]any{"rule_type": "regex_descritivo", "pattern": "tarifa", "account_code": "4.01"}).MustStatus(t, http.StatusCreated)
	return e
}

func (e *env) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

func (e *env) line(t *testing.T, date, amount, dir, desc string) string {
	t.Helper()
	e.pos++
	return e.scalar(t, `INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount, direction, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, e.company, e.imp, e.pos, date, desc, amount, dir, fmt.Sprintf("%064x", e.pos))
}

func (e *env) entry(t *testing.T, date, amount, dir, counterparty, status string) string {
	t.Helper()
	var d, a, s any = date, amount, dir
	if date == "" {
		d = nil
	}
	if amount == "" {
		a = nil
	}
	if dir == "" {
		s = nil
	}
	return e.scalar(t, `INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, e.company, d, a, s, counterparty, status)
}

type queueOut struct {
	Items []struct {
		Kind   string  `json:"kind"`
		ID     string  `json:"id"`
		Bucket string  `json:"bucket"`
		Reason *string `json:"reason"`
	} `json:"items"`
	Counts map[string]int `json:"counts"`
}

func (e *env) queue(t *testing.T, query string) queueOut {
	t.Helper()
	var out queueOut
	e.srv.Get("/api/v1/reconciliation/review-queue?"+query).MustStatus(t, http.StatusOK).JSON(t, &out)
	return out
}

func (q queueOut) bucketOf(id string) string {
	for _, it := range q.Items {
		if it.ID == id {
			return it.Bucket
		}
	}
	return ""
}

func (e *env) round(t *testing.T) {
	t.Helper()
	e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusOK)
}

// Os baldes da fila e os candidatos da origem: mesmo valor e sentido em D±3 (do mais perto ao
// mais longe), mais os que o matching anotou na divergência.
func TestTEST021_QueueBucketsAndCandidates(t *testing.T) {
	e := setup(t)
	free := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR")
	near := e.entry(t, "2026-08-12", "150.00", "D", "Fornecedor A", "extraido")
	nearer := e.entry(t, "2026-08-11", "150.00", "D", "Fornecedor B", "manual")
	e.entry(t, "2026-08-20", "150.00", "D", "Longe", "manual")
	partial := e.entry(t, "", "", "D", "Parcial Ltda", "parcial")
	tariff := e.line(t, "2026-08-25", "35.00", "D", "TARIFA PACOTE")
	e.round(t)

	q := e.queue(t, "company_id="+e.company+"&competence=2026-08")
	if q.bucketOf(free) != "divergencia" || q.bucketOf(near) != "comprovante_sem" || q.bucketOf(partial) != "revisao" ||
		q.bucketOf(tariff) != "extrato_sem" {
		t.Fatalf("fila = %+v", q.Items)
	}
	if q.Counts["revisao"] != 1 || q.Counts["divergencia"] != 1 || q.Counts["extrato_sem"] != 1 || q.Counts["comprovante_sem"] != 3 ||
		q.Counts["total"] != 6 {
		t.Fatalf("contagens = %v", q.Counts)
	}
	if only := e.queue(t, "bucket=revisao"); len(only.Items) != 1 || only.Items[0].Kind != "comprovante" || *only.Items[0].Reason != "campos_a_revisar" {
		t.Fatalf("balde revisão = %+v", only.Items)
	}
	e.srv.Get("/api/v1/reconciliation/review-queue?bucket=outro").MustStatus(t, http.StatusBadRequest)

	var d struct {
		Line struct {
			ID          string          `json:"id"`
			BankAccount string          `json:"bank_account"`
			Marks       json.RawMessage `json:"marks"`
		} `json:"line"`
		Reconciliation *struct{} `json:"reconciliation"`
		Candidates     []struct {
			ID        string `json:"id"`
			DaysApart *int   `json:"days_apart"`
			Annotated bool   `json:"annotated"`
		} `json:"candidates"`
	}
	e.srv.Get("/api/v1/reconciliation/review/"+free).MustStatus(t, http.StatusOK).JSON(t, &d)
	if d.Line.BankAccount != "a_fonte 0001/12345-6" || d.Reconciliation != nil || len(d.Candidates) != 2 || d.Candidates[0].ID != nearer ||
		*d.Candidates[0].DaysApart != 1 || d.Candidates[1].ID != near || !d.Candidates[0].Annotated {
		t.Fatalf("linha = %+v", d)
	}
	var en struct {
		Candidates []struct {
			ID        string `json:"id"`
			DaysApart int    `json:"days_apart"`
		} `json:"candidates"`
	}
	e.srv.Get("/api/v1/reconciliation/entries/"+near).MustStatus(t, http.StatusOK).JSON(t, &en)
	if len(en.Candidates) != 1 || en.Candidates[0].ID != free || en.Candidates[0].DaysApart != 2 {
		t.Fatalf("candidatos do comprovante = %+v", en.Candidates)
	}
	e.srv.Get("/api/v1/reconciliation/review/00000000-0000-4000-8000-000000000000").MustStatus(t, http.StatusNotFound)
	e.srv.Get("/api/v1/reconciliation/entries/x").MustStatus(t, http.StatusNotFound)
}

type matchOut struct {
	ReconciliationID string `json:"reconciliation_id"`
	Method           string `json:"method"`
	EntriesTotal     string `json:"entries_total"`
	Difference       string `json:"difference"`
	Classification   string `json:"classification"`
}

// Conciliar à mão (N:1, com a diferença na resposta) e "não há comprovante": origem manual, o
// autor e a classificação da linha; linha conciliada e lançamento usado são recusados.
func TestTEST021_ManualMatchAndNoReceipt(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "300.00", "D", "PIX ENVIADO DOIS BOLETOS")
	a := e.entry(t, "2026-08-10", "120.00", "D", "Alfa", "manual")
	b := e.entry(t, "2026-08-09", "170.00", "D", "Alfa", "extraido")
	tariff := e.line(t, "2026-08-25", "35.00", "D", "TARIFA PACOTE")

	var m matchOut
	e.srv.Post("/api/v1/reconciliation/review/"+line+"/manual-match", map[string]any{"entry_ids": []string{a, b, a}}).
		MustStatus(t, http.StatusCreated).JSON(t, &m)
	if m.Method != "manual" || m.EntriesTotal != "290.00" || m.Difference != "10.00" || m.Classification != "agente" {
		t.Fatalf("conciliação = %+v", m)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations r WHERE r.id = $1 AND r.origin = 'manual' AND r.method = 'manual'
		AND r.created_by = $2 AND (SELECT count(*) FROM reconciliation_items i WHERE i.reconciliation_id = r.id) = 2`, m.ReconciliationID, user.AdminID); n != 1 {
		t.Fatal("conciliação manual N:1 não gravada")
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+line+"/manual-match", map[string]any{"entry_ids": []string{a}}).
		MustStatus(t, http.StatusConflict).ErrorCode(t); code != "LINE_RECONCILED" {
		t.Fatalf("código = %s", code)
	}
	other := e.line(t, "2026-08-12", "120.00", "D", "OUTRA")
	if code := e.srv.Post("/api/v1/reconciliation/review/"+other+"/manual-match", map[string]any{"entry_ids": []string{a}}).
		MustStatus(t, http.StatusConflict).ErrorCode(t); code != "ENTRY_NOT_FREE" {
		t.Fatalf("código = %s", code)
	}
	e.srv.Post("/api/v1/reconciliation/review/"+other+"/manual-match", map[string]any{"entry_ids": []string{}}).MustStatus(t, http.StatusBadRequest)

	e.srv.Post("/api/v1/reconciliation/review/"+tariff+"/confirm-no-receipt", nil).MustStatus(t, http.StatusCreated).JSON(t, &m)
	if m.Method != "sem_comprovante" || m.Classification != "regra" {
		t.Fatalf("sem comprovante = %+v", m)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb->>'acao' IN ('conciliar', 'sem_comprovante')`); n != 2 {
		t.Fatalf("%d ações no log", n)
	}
}

// Desfazer (decisão do usuário): a automática também, com motivo obrigatório. Nada é apagado; os
// lançamentos ficam livres; a linha perde a classificação, ganha a marca "desfeita", volta à
// fila como divergência e a rodada não a casa de novo sozinha.
func TestTEST021_UndoKeepsHistoryAndLeavesTheAutomatic(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "35.00", "D", "TARIFA PACOTE")
	entry := e.entry(t, "2026-08-10", "35.00", "D", "Banco", "manual")
	e.round(t)
	rec := e.scalar(t, `SELECT id FROM reconciliations WHERE statement_transaction_id = $1 AND origin = 'auto'`, line)
	if n := e.count(t, `SELECT count(*) FROM statement_transactions WHERE id = $1 AND classification_origin = 'regra'`, line); n != 1 {
		t.Fatal("a rodada devia classificar a linha")
	}
	var recent struct {
		Items []struct {
			ID     string `json:"id"`
			Origin string `json:"origin"`
		} `json:"items"`
	}
	e.srv.Get("/api/v1/reconciliation/reconciliations?company_id="+e.company).MustStatus(t, http.StatusOK).JSON(t, &recent)
	if len(recent.Items) != 1 || recent.Items[0].ID != rec || recent.Items[0].Origin != "auto" {
		t.Fatalf("recentes = %+v", recent.Items)
	}

	path := "/api/v1/reconciliation/reconciliations/" + rec + "/undo"
	e.srv.Post(path, map[string]string{"reason": "  "}).MustStatus(t, http.StatusBadRequest)
	e.srv.Post(path, map[string]string{"reason": "O comprovante é de outro mês."}).MustStatus(t, http.StatusOK)
	if n := e.count(t, `SELECT count(*) FROM reconciliations WHERE id = $1 AND undone_at IS NOT NULL AND undone_by = $2
		AND undo_reason = 'O comprovante é de outro mês.'`, rec, user.AdminID); n != 1 {
		t.Fatal("desfazer não guardou quem e por quê")
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliation_items WHERE client_entry_id = $1 AND released_at IS NOT NULL`, entry); n != 1 {
		t.Fatal("lançamento não liberado")
	}
	if n := e.count(t, `SELECT count(*) FROM statement_transactions WHERE id = $1 AND classification_origin IS NULL AND account_code IS NULL
		AND flags @> '[{"tipo": "desfeita", "motivo": "conciliacao_desfeita"}]'`, line); n != 1 {
		t.Fatal("linha sem a marca ou com classificação")
	}
	if code := e.srv.Post(path, map[string]string{"reason": "de novo"}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "RECONCILIATION_UNDONE" {
		t.Fatalf("código = %s", code)
	}

	// A rodada não casa de novo o que o contador desfez: a linha fica na fila como divergência.
	var sum struct {
		NewMatches int `json:"new_matches"`
		Divergent  int `json:"divergent"`
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusOK).JSON(t, &sum)
	if sum.NewMatches != 0 || sum.Divergent != 1 {
		t.Fatalf("rodada depois do desfazer = %+v", sum)
	}
	q := e.queue(t, "company_id="+e.company)
	if q.bucketOf(line) != "divergencia" || q.bucketOf(entry) != "comprovante_sem" {
		t.Fatalf("fila = %+v", q.Items)
	}
	for _, it := range q.Items {
		if it.ID == line && (it.Reason == nil || *it.Reason != "conciliacao_desfeita") {
			t.Fatalf("motivo = %v", it.Reason)
		}
	}
	// À mão, o contador concilia de novo.
	e.srv.Post("/api/v1/reconciliation/review/"+line+"/manual-match", map[string]any{"entry_ids": []string{entry}}).MustStatus(t, http.StatusCreated)
}

// A correção campo a campo do comprovante parcial: o campo corrigido ganha confiança 1 e origem
// humana; completo, o parcial sai da revisão e a competência roda de novo (e concilia).
func TestTEST021_CorrectPartialReceiptFields(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO ALFA")
	partial := e.scalar(t, `INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status, field_confidence, field_origin)
		VALUES ($1, '2026-08-10', NULL, 'D', 'Alfa', 'parcial', '{"valor": "0.40", "data": "0.95"}',
		'{"valor": {"pagina": 1, "trecho": "R$ 15O,00", "bbox": [1, 2, 3, 4]}}') RETURNING id`, e.company)
	path := "/api/v1/reconciliation/entries/" + partial
	var bad struct {
		Details struct {
			Fields map[string]string `json:"fields"`
		} `json:"details"`
	}
	e.srv.Patch(path, map[string]any{"amount": "-1", "direction": "X"}).MustStatus(t, http.StatusBadRequest).JSON(t, &bad)
	if bad.Details.Fields["amount"] == "" || bad.Details.Fields["direction"] == "" {
		t.Fatalf("erros = %+v", bad)
	}
	e.srv.Patch(path, map[string]any{}).MustStatus(t, http.StatusBadRequest)

	var out struct {
		Entry struct {
			Status          string            `json:"status"`
			Amount          *string           `json:"amount"`
			FieldConfidence map[string]string `json:"field_confidence"`
			FieldOrigin     map[string]any    `json:"field_origin"`
		} `json:"entry"`
		Round *struct {
			NewMatches int `json:"new_matches"`
		} `json:"round"`
	}
	e.srv.Patch(path, map[string]any{"amount": "150,00"}).MustStatus(t, http.StatusOK).JSON(t, &out)
	if out.Entry.Status != "extraido" || out.Entry.Amount == nil || *out.Entry.Amount != "150.00" || out.Entry.FieldConfidence["valor"] != "1.00" ||
		out.Entry.FieldConfidence["data"] != "0.95" || out.Round == nil || out.Round.NewMatches != 1 {
		t.Fatalf("correção = %+v", out)
	}
	if o, _ := out.Entry.FieldOrigin["valor"].(map[string]any); o["origem"] != "humano" {
		t.Fatalf("origem do campo = %v", out.Entry.FieldOrigin)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations WHERE statement_transaction_id = $1 AND origin = 'auto'`, line); n != 1 {
		t.Fatal("a rodada devia conciliar o comprovante corrigido")
	}
	if code := e.srv.Patch(path, map[string]any{"counterparty": "Alfa Ltda"}).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "ENTRY_RECONCILED" {
		t.Fatalf("código = %s", code)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb->>'acao' = 'corrigir_campos'`); n != 1 {
		t.Fatal("correção fora do log")
	}
}

// Contábil fechado (AC-025): conciliar, sem comprovante, desfazer e corrigir respondem 423.
func TestTEST021_AccountingLock(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO ALFA")
	other := e.line(t, "2026-08-20", "35.00", "D", "TARIFA")
	entry := e.entry(t, "2026-08-10", "150.00", "D", "Alfa", "manual")
	partial := e.entry(t, "2026-08-12", "", "D", "Beta", "parcial")
	e.srv.Post("/api/v1/reconciliation/review/"+other+"/confirm-no-receipt", nil).MustStatus(t, http.StatusCreated)
	rec := e.scalar(t, `SELECT id FROM reconciliations WHERE statement_transaction_id = $1`, other)
	comp := e.scalar(t, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-08', 2026, 8, '2026-08-01')
		ON CONFLICT (competence_key) DO UPDATE SET competence_key = EXCLUDED.competence_key RETURNING id`)
	if _, err := e.pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', 'CLOSED', now(), $3)`, e.company, comp, user.AdminID); err != nil {
		t.Fatal(err)
	}
	for _, r := range []testutil.Response{
		e.srv.Post("/api/v1/reconciliation/review/"+line+"/manual-match", map[string]any{"entry_ids": []string{entry}}),
		e.srv.Post("/api/v1/reconciliation/review/"+line+"/confirm-no-receipt", nil),
		e.srv.Post("/api/v1/reconciliation/reconciliations/"+rec+"/undo", map[string]string{"reason": "teste"}),
		e.srv.Patch("/api/v1/reconciliation/entries/"+partial, map[string]any{"amount": "10.00"}),
	} {
		if code := r.MustStatus(t, http.StatusLocked).ErrorCode(t); code != "PERIOD_CLOSED" {
			t.Fatalf("código = %s", code)
		}
	}
}
