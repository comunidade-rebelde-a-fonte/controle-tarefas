package reconcile_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/matching"
	"controle/internal/reconciliation/reconcile"
	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-018 [REQ-024, REQ-030, AC-018, AC-025]: a rodada do matching gravando as conciliações
// automáticas e as marcas da fila, com a trava do Contábil.

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
		t.Fatal(err)
	}
	return n
}

// line cria uma linha do extrato.
func (e *env) line(t *testing.T, date, amount, dir, desc string) string {
	t.Helper()
	e.pos++
	key := fmt.Sprintf("%064x", e.pos)
	return e.scalar(t, `INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount, direction, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, e.company, e.imp, e.pos, date, desc, amount, dir, key)
}

// entry cria um lançamento do cliente com o status dado.
func (e *env) entry(t *testing.T, date, amount, dir, counterparty, status string, flags ...string) string {
	t.Helper()
	b, _ := json.Marshal(append([]string{}, flags...))
	return e.scalar(t, `INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status, flags)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, e.company, date, amount, dir, counterparty, status, b)
}

type summary struct {
	NewMatches       int            `json:"new_matches"`
	ByPass           map[string]int `json:"by_pass"`
	Reconciled       int            `json:"reconciled"`
	Divergent        int            `json:"divergent"`
	WithoutReceipt   int            `json:"without_receipt"`
	WithoutStatement int            `json:"without_statement"`
	AwaitingReview   int            `json:"awaiting_review"`
}

func (e *env) run(t *testing.T, comp string, status int) summary {
	t.Helper()
	var s summary
	r := e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence="+comp, nil).MustStatus(t, status)
	if status == http.StatusOK {
		r.JSON(t, &s)
	}
	return s
}

func (e *env) flags(t *testing.T, id string) []map[string]any {
	t.Helper()
	var raw []byte
	if err := e.pool.QueryRow(ctx, `SELECT flags FROM statement_transactions WHERE id = $1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// A rodada concilia o que não é ambíguo (origem auto, passe, método e score, sem autor), marca a
// disputa com as linhas concorrentes, deixa o parcial esperando a revisão e conta os baldes. De
// novo, não muda nada; com o parcial corrigido, concilia.
func TestTEST018_RunReconcilesMarksAndIsIdempotent(t *testing.T) {
	e := setup(t)
	exact := e.line(t, "2026-08-10", "100.00", "D", "PIX ENVIADO ALFA")
	partialLine := e.line(t, "2026-08-12", "200.00", "C", "PIX RECEBIDO BETA")
	fee := e.line(t, "2026-08-15", "50.00", "D", "TARIFA BANCARIA")
	d1 := e.line(t, "2026-08-20", "300.00", "C", "PIX RECEBIDO GAMA")
	d2 := e.line(t, "2026-08-20", "300.00", "C", "TED RECEBIDA DELTA")
	alfa := e.entry(t, "2026-08-10", "100.00", "D", "Alfa", "manual")
	partial := e.entry(t, "2026-08-12", "200.00", "C", "Beta", "parcial")
	disputed := e.entry(t, "2026-08-20", "300.00", "C", "Epsilon", "extraido")

	s := e.run(t, "2026-08", http.StatusOK)
	want := summary{NewMatches: 1, ByPass: map[string]int{"passe_1": 1, "passe_2": 0, "passe_3": 0, "passe_4": 0}, Reconciled: 1,
		Divergent: 2, WithoutReceipt: 2, WithoutStatement: 1, AwaitingReview: 1}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("baldes = %+v", s)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations r JOIN reconciliation_items i ON i.reconciliation_id = r.id
		WHERE r.statement_transaction_id = $1 AND i.client_entry_id = $2 AND r.origin = 'auto' AND r.pass = 1 AND r.method = 'passe_1'
		AND r.score = 1 AND r.created_by IS NULL`, exact, alfa); n != 1 {
		t.Fatal("conciliação automática não gravada")
	}
	for line, rival := range map[string]string{d1: d2, d2: d1} {
		f := e.flags(t, line)
		if len(f) != 2 || f[0]["tipo"] != "divergencia" || f[0]["motivo"] != "ambiguo_passe_1" ||
			!reflect.DeepEqual(f[0]["candidatos"], []any{disputed}) || !reflect.DeepEqual(f[0]["concorrentes"], []any{rival}) {
			t.Fatalf("marcas da disputa = %v", f)
		}
	}
	if len(e.flags(t, fee)) != 0 || len(e.flags(t, partialLine)) != 0 || len(e.flags(t, exact)) != 0 {
		t.Fatal("marca em linha sem disputa")
	}

	// Idempotente: nada novo, as mesmas marcas.
	if again := e.run(t, "2026-08", http.StatusOK); again.NewMatches != 0 || again.Reconciled != 1 || again.Divergent != 2 {
		t.Fatalf("segunda rodada = %+v", again)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations`); n != 1 {
		t.Fatalf("%d conciliações", n)
	}
	if f := e.flags(t, d1); len(f) != 2 {
		t.Fatalf("marcas duplicadas: %v", f)
	}

	// O parcial corrigido entra no automático.
	if _, err := e.pool.Exec(ctx, `UPDATE client_entries SET status = 'manual' WHERE id = $1`, partial); err != nil {
		t.Fatal(err)
	}
	if third := e.run(t, "2026-08", http.StatusOK); third.NewMatches != 1 || third.AwaitingReview != 0 || third.WithoutReceipt != 1 {
		t.Fatalf("terceira rodada = %+v", third)
	}
}

// A linha que vira conciliada perde as marcas da rodada anterior; as outras marcas ficam.
func TestTEST018_RunReplacesOnlyDerivedMarks(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "100.00", "D", "PIX ENVIADO ALFA")
	if _, err := e.pool.Exec(ctx, `UPDATE statement_transactions SET flags = '[{"tipo": "divergencia", "motivo": "velha"}, "nota do contador"]' WHERE id = $1`, line); err != nil {
		t.Fatal(err)
	}
	e.entry(t, "2026-08-10", "100.00", "D", "Alfa", "manual")
	e.run(t, "2026-08", http.StatusOK)
	var raw string
	if err := e.pool.QueryRow(ctx, `SELECT flags::text FROM statement_transactions WHERE id = $1`, line).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != `["nota do contador"]` {
		t.Fatalf("marcas = %s", raw)
	}
}

// Caso de borda: o estorno sai do automático mesmo com comprovante idêntico, e a marca aponta a
// linha do par [REQ-030].
func TestTEST018_EdgeCasesStayOutOfTheAutomatic(t *testing.T) {
	e := setup(t)
	payment := e.line(t, "2026-08-10", "500.00", "D", "PIX ENVIADO ZETA")
	refund := e.line(t, "2026-08-13", "500.00", "C", "DEVOLUCAO PIX ZETA")
	e.entry(t, "2026-08-10", "500.00", "D", "Zeta", "manual")
	s := e.run(t, "2026-08", http.StatusOK)
	if s.NewMatches != 0 || s.Divergent != 2 {
		t.Fatalf("baldes = %+v", s)
	}
	f := e.flags(t, payment)
	if len(f) != 1 || f[0]["tipo"] != "borda" || f[0]["motivo"] != matching.EdgeReversal || !reflect.DeepEqual(f[0]["relacionados"], []any{refund}) {
		t.Fatalf("marcas = %v", f)
	}
}

// Contábil fechado: a API responde 423 e nada muda; o job da rodada não faz nada (AC-025).
func TestTEST018_RunRespectsTheAccountingLock(t *testing.T) {
	e := setup(t)
	e.line(t, "2026-08-10", "100.00", "D", "PIX ENVIADO ALFA")
	e.entry(t, "2026-08-10", "100.00", "D", "Alfa", "manual")
	comp := e.scalar(t, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-08', 2026, 8, '2026-08-01')
		ON CONFLICT (competence_key) DO UPDATE SET competence_key = EXCLUDED.competence_key RETURNING id`)
	if _, err := e.pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', 'CLOSED', now(), $3)`, e.company, comp, user.AdminID); err != nil {
		t.Fatal(err)
	}
	if code := e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusLocked).ErrorCode(t); code != "PERIOD_CLOSED" {
		t.Fatalf("código = %s", code)
	}
	svc := reconcile.NewService(e.pool, clock.NewSystem(time.UTC), matching.DefaultOptions(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := jobs.NewQueue(clock.NewSystem(time.UTC), 3)
	if err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		return reconciliation.EnqueueRun(ctx, q, tx, e.company, competence.MustParse("2026-08"))
	}); err != nil {
		t.Fatal(err)
	}
	runner := jobs.NewRunner(e.pool, clock.NewSystem(time.UTC), slog.New(slog.NewTextHandler(io.Discard, nil)),
		jobs.RunnerConfig{WorkerID: "w", Concurrency: 1, PollInterval: time.Millisecond, Lease: time.Minute},
		map[string]jobs.Handler{reconciliation.JobRun: svc.HandleRun})
	if ok, err := runner.ProcessOne(ctx); !ok || err != nil {
		t.Fatalf("job = %v, %v", ok, err)
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'reconciliation_run' AND status = 'done'`); n != 1 {
		t.Fatal("o job devia terminar sem erro")
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations`); n != 0 {
		t.Fatal("conciliou com o Contábil fechado")
	}
}

// Empresa inexistente: 404; competência inválida: 400.
func TestTEST018_RunValidation(t *testing.T) {
	e := setup(t)
	e.srv.Post("/api/v1/companies/00000000-0000-4000-8000-000000000000/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusNotFound)
	e.srv.Post("/api/v1/companies/x/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusNotFound)
	for _, c := range []string{"", "2026-13", "08/2026"} {
		e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence="+c, nil).MustStatus(t, http.StatusBadRequest)
	}
}
