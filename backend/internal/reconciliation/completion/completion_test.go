package completion_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/competence"
	"controle/internal/reconciliation"
	"controle/internal/store"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

// TEST-022 [REQ-031, REQ-032, AC-023, AC-024]: a conclusão da Conciliação sincroniza a obrigação
// Conciliação do Contábil pelo serviço de obrigações, de forma idempotente, com histórico de
// origem SYSTEM_INTEGRATION, sob concorrência, e trava a competência até a reabertura com motivo.

var ctx = context.Background()

type env struct {
	t       *testing.T
	pool    *pgxpool.Pool
	srv     *testutil.Server
	company string
	seq     int
}

func (e *env) scalar(sql string, args ...any) string {
	e.t.Helper()
	var v string
	if err := e.pool.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(ctx, sql, args...); err != nil {
		e.t.Fatalf("%v\n%s", err, sql)
	}
}

// setup: as 20 empresas da demonstração com a competência 10/2026 gerada; a 101 tem o Contábil.
func setup(t *testing.T) *env {
	t.Helper()
	pool := seeded.NewCompaniesDB(t)
	e := &env{t: t, pool: pool, srv: testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-01T12:00:00Z"))}
	e.srv.Post("/api/v1/competencies/ensure-current", nil).MustStatus(t, http.StatusOK)
	e.company = e.scalar(`SELECT id FROM companies WHERE internal_code = '101'`)
	e.exec(`INSERT INTO chart_accounts (company_id, code, name) VALUES ($1, '1.01', 'Caixa')`, e.company)
	return e
}

// statement importa um extrato da empresa no mês, com as linhas dadas (todas em débito).
func (e *env) statement(month string, days ...int) []string {
	e.t.Helper()
	e.seq++
	doc := e.scalar(`INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type, kind, status)
		VALUES ($1, 'originais', 'originais/' || $2::text, lpad(to_hex($3::int), 64, '0'), 'extrato.pdf', 'application/pdf', 'extrato', 'importado')
		RETURNING id`, e.company, fmt.Sprint(e.seq), e.seq)
	start := month + "-01"
	imp := e.scalar(`INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end, opening_balance,
		closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'beta', '0021', '33412-7', $3::date, ($3::date + interval '1 month' - interval '1 day')::date, 0, 0, true, '{}', 0)
		RETURNING id`, e.company, doc, start)
	lines := make([]string, 0, len(days))
	for _, d := range days {
		e.seq++
		lines = append(lines, e.scalar(`INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description,
			amount, direction, dedupe_key)
			VALUES ($1, $2, $3::int, $4::date, 'LINHA ' || $5::text, 10.00, 'D', lpad(to_hex($3::int), 64, 'f')) RETURNING id`,
			e.company, imp, e.seq, fmt.Sprintf("%s-%02d", month, d), fmt.Sprint(e.seq)))
	}
	return lines
}

// resolve concilia a linha sem comprovante e, com classify, a classifica (o contador).
func (e *env) resolve(line string, classify bool) string {
	e.t.Helper()
	rec := e.scalar(`INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by)
		VALUES ($1, $2, 'sem_comprovante', 'manual', $3) RETURNING id`, e.company, line, user.AdminID)
	if classify {
		e.exec(`UPDATE statement_transactions SET account_code = '1.01', classification_origin = 'manual', classified_by = $2,
			classified_at = now() WHERE id = $1`, line, user.AdminID)
	}
	return rec
}

func (e *env) instance(code string) (id string, version int) {
	e.t.Helper()
	if err := e.pool.QueryRow(ctx, `SELECT i.id, i.version FROM obligation_instances i JOIN obligation_types t ON t.id = i.obligation_type_id
		JOIN competencies k ON k.id = i.competence_id WHERE i.company_id = $1 AND t.code = $2 AND k.competence_key = '2026-10'`,
		e.company, code).Scan(&id, &version); err != nil {
		e.t.Fatalf("instância %s: %v", code, err)
	}
	return id, version
}

// setStatus muda o status de uma obrigação pela API (o contador, no Controle).
func (e *env) setStatus(code, status string) {
	e.t.Helper()
	id, v := e.instance(code)
	e.srv.Patch("/api/v1/obligation-instances/"+id+"/status", map[string]any{"status_code": status, "version": v}).MustStatus(e.t, http.StatusOK)
}

// prerequisites conclui as etapas do Contábil antes da Conciliação (dependência HARD).
func (e *env) prerequisites() {
	e.t.Helper()
	for _, code := range []string{"ACCOUNTING_MOVEMENT_RECEIVED", "ACCOUNTING_PAYROLL_POSTED", "ACCOUNTING_TAXES_POSTED", "ACCOUNTING_PROCESSED"} {
		e.setStatus(code, "OK")
	}
}

type state struct {
	State    string `json:"state"`
	Ready    bool   `json:"ready"`
	Blocking struct {
		MissingStatement bool `json:"missing_statement"`
		Divergent        int  `json:"divergent"`
		WithoutReceipt   int  `json:"without_receipt"`
		Review           int  `json:"review"`
		Classification   int  `json:"classification"`
	} `json:"blocking"`
	Warnings struct {
		WithoutStatement int `json:"without_statement"`
	} `json:"warnings"`
	Obligation *struct {
		StatusCode  string `json:"status_code"`
		StatusLabel string `json:"status_label"`
		IsFinal     bool   `json:"is_final"`
	} `json:"obligation"`
	Current *struct {
		CompletedBy            string  `json:"completed_by"`
		ObligationStatusBefore *string `json:"obligation_status_before"`
		ObligationSync         string  `json:"obligation_sync"`
	} `json:"current"`
	History []struct {
		ObligationSync       string  `json:"obligation_sync"`
		ReopenReason         *string `json:"reopen_reason"`
		ReopenObligationSync *string `json:"reopen_obligation_sync"`
		ReopenedBy           *string `json:"reopened_by"`
	} `json:"history"`
}

func (e *env) summary(comp string) state {
	e.t.Helper()
	var out struct {
		Completion state `json:"completion"`
	}
	e.srv.Get("/api/v1/companies/"+e.company+"/reconciliation?competence="+comp).MustStatus(e.t, http.StatusOK).JSON(e.t, &out)
	return out.Completion
}

func (e *env) complete(comp string) testutil.Response {
	return e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/complete?competence="+comp, nil)
}

func (e *env) reopen(comp, reason string) testutil.Response {
	return e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/reopen?competence="+comp, map[string]string{"reason": reason})
}

type historyRow struct {
	old, new, source, reason string
}

// history devolve o histórico de status da obrigação Conciliação, na ordem.
func (e *env) history() []historyRow {
	e.t.Helper()
	id, _ := e.instance("ACCOUNTING_RECONCILIATION")
	rows, err := e.pool.Query(ctx, `SELECT COALESCE(old_value, ''), COALESCE(new_value, ''), change_source, COALESCE(override_reason, '')
		FROM obligation_status_history WHERE obligation_instance_id = $1 AND field = 'STATUS' ORDER BY created_at, seq`, id)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []historyRow
	for rows.Next() {
		var h historyRow
		if err := rows.Scan(&h.old, &h.new, &h.source, &h.reason); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, h)
	}
	return out
}

// TEST-022: pronta só com o extrato todo resolvido; a dependência HARD vale; a conclusão leva a
// obrigação para OK com a origem SYSTEM_INTEGRATION; concluir de novo não duplica nada.
func TestTEST022_CompleteSyncsTheObligationOnce(t *testing.T) {
	e := setup(t)
	// Sem extrato: em andamento, e concluir é recusado com o que falta.
	st := e.summary("2026-10")
	if st.State != "em_andamento" || st.Ready || !st.Blocking.MissingStatement || st.Obligation == nil || st.Obligation.StatusCode != "NOT_STARTED" {
		t.Fatalf("sem extrato = %+v", st)
	}
	res := e.complete("2026-10")
	if res.Status != http.StatusConflict || res.ErrorCode(t) != "RECONCILIATION_NOT_READY" || !strings.Contains(string(res.Body), `"missing_statement":true`) {
		t.Fatalf("concluir sem extrato = %d %s", res.Status, res.Body)
	}
	// Com o extrato, as linhas livres e a sem classificação ainda impedem.
	lines := e.statement("2026-10", 3, 15)
	e.resolve(lines[0], false)
	st = e.summary("2026-10")
	if st.Blocking.MissingStatement || st.Blocking.WithoutReceipt != 1 || st.Blocking.Classification != 1 || st.Ready {
		t.Fatalf("parcial = %+v", st.Blocking)
	}
	e.exec(`UPDATE statement_transactions SET account_code = '1.01', classification_origin = 'manual', classified_by = $2, classified_at = now()
		WHERE id = $1`, lines[0], user.AdminID)
	e.resolve(lines[1], true)
	// Comprovante sem extrato só avisa.
	e.exec(`INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status) VALUES ($1, '2026-10-20', 99.00, 'D', 'Outra conta', 'manual')`,
		e.company)
	st = e.summary("2026-10")
	if st.State != "pronta" || !st.Ready || st.Warnings.WithoutStatement != 1 {
		t.Fatalf("pronta = %+v", st)
	}
	// "Contábil digitado/processado" A iniciar: a dependência HARD do Controle bloqueia, e nada é gravado.
	res = e.complete("2026-10")
	if res.Status != http.StatusUnprocessableEntity || res.ErrorCode(t) != "DEPENDENCY_BLOCKED" ||
		!strings.Contains(string(res.Body), "Contábil digitado/processado") {
		t.Fatalf("concluir com a etapa anterior aberta = %d %s", res.Status, res.Body)
	}
	if n := e.scalar(`SELECT count(*) FROM reconciliation_completions`); n != "0" {
		t.Fatalf("conclusões = %s", n)
	}
	e.prerequisites()
	var done state
	e.complete("2026-10").MustStatus(t, http.StatusOK).JSON(t, &done)
	if done.State != "concluida" || done.Current == nil || done.Current.ObligationSync != "concluida" ||
		done.Current.ObligationStatusBefore == nil || *done.Current.ObligationStatusBefore != "NOT_STARTED" || done.Current.CompletedBy == "" ||
		done.Obligation == nil || done.Obligation.StatusCode != "OK" || !done.Obligation.IsFinal {
		t.Fatalf("concluída = %+v (obrigação %+v)", done, done.Obligation)
	}
	h := e.history()
	if len(h) != 1 || h[0].old != "NOT_STARTED" || h[0].new != "OK" || h[0].source != "SYSTEM_INTEGRATION" ||
		h[0].reason != "Conciliação de 10/2026 concluída na Conciliação." {
		t.Fatalf("histórico = %+v", h)
	}
	if by := e.scalar(`SELECT user_id::text FROM obligation_status_history WHERE change_source = 'SYSTEM_INTEGRATION'`); by != user.AdminID {
		t.Errorf("quem disparou = %s", by)
	}
	// Idempotente: concluir de novo devolve o mesmo estado, sem conclusão nem histórico novos.
	e.complete("2026-10").MustStatus(t, http.StatusOK)
	if n := e.scalar(`SELECT count(*) FROM reconciliation_completions`); n != "1" {
		t.Errorf("conclusões = %s", n)
	}
	if h := e.history(); len(h) != 1 {
		t.Errorf("histórico depois de repetir = %+v", h)
	}
	if n := e.scalar(`SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb ->> 'acao' = 'concluir_conciliacao'`); n != "1" {
		t.Errorf("log = %s", n)
	}
}

// Concluída, a competência trava toda mutação da Conciliação (423 RECONCILIATION_COMPLETED); a
// reabertura exige motivo, volta a obrigação para Pendente e destrava; a outra competência segue.
func TestTEST022_LockedUntilReopenedWithReason(t *testing.T) {
	e := setup(t)
	lines := e.statement("2026-10", 5)
	rec := e.resolve(lines[0], true)
	e.prerequisites()
	e.complete("2026-10").MustStatus(t, http.StatusOK)

	entry := map[string]any{"company_id": e.company, "date": "2026-10-10", "amount": "10,00", "direction": "D"}
	for name, res := range map[string]testutil.Response{
		"lançamento":    e.srv.Post("/api/v1/reconciliation/entries", entry),
		"rodada":        e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence=2026-10", nil),
		"classificação": e.srv.Post("/api/v1/companies/"+e.company+"/classification/run?competence=2026-10", nil),
		"desfazer":      e.srv.Post("/api/v1/reconciliation/reconciliations/"+rec+"/undo", map[string]string{"reason": "teste"}),
	} {
		if res.Status != http.StatusLocked || res.ErrorCode(t) != "RECONCILIATION_COMPLETED" {
			t.Errorf("%s com a conciliação concluída = %d %s", name, res.Status, res.Body)
		}
	}
	// O documento tardio fica como pendência, com o motivo da conclusão.
	c := competence.MustParse("2026-10")
	if reason, err := reconciliation.LockReason(ctx, e.pool, e.company, c); err != nil || reason != reconciliation.LockCompleted {
		t.Errorf("motivo da trava = %q, %v", reason, err)
	}
	// Outra competência não trava.
	e.srv.Post("/api/v1/reconciliation/entries", map[string]any{"company_id": e.company, "date": "2026-09-10", "amount": "10,00", "direction": "D"}).
		MustStatus(t, http.StatusCreated)

	// Reabrir exige motivo; reaberta, a obrigação volta para Pendente com o motivo no histórico.
	if res := e.reopen("2026-10", "  "); res.Status != http.StatusBadRequest || res.ErrorCode(t) != "VALIDATION_ERROR" {
		t.Fatalf("reabrir sem motivo = %d %s", res.Status, res.Body)
	}
	var reopened state
	e.reopen("2026-10", "Faltou o extrato da conta poupança").MustStatus(t, http.StatusOK).JSON(t, &reopened)
	if reopened.State != "pronta" || reopened.Current != nil || reopened.Obligation == nil || reopened.Obligation.StatusCode != "PENDING" ||
		len(reopened.History) != 1 || reopened.History[0].ReopenObligationSync == nil || *reopened.History[0].ReopenObligationSync != "pendente" ||
		reopened.History[0].ReopenReason == nil || *reopened.History[0].ReopenReason != "Faltou o extrato da conta poupança" ||
		reopened.History[0].ReopenedBy == nil {
		t.Fatalf("reaberta = %+v", reopened)
	}
	h := e.history()
	if len(h) != 2 || h[1].old != "OK" || h[1].new != "PENDING" || h[1].source != "SYSTEM_INTEGRATION" ||
		h[1].reason != "Conciliação de 10/2026 reaberta na Conciliação: Faltou o extrato da conta poupança" {
		t.Fatalf("histórico = %+v", h)
	}
	e.srv.Post("/api/v1/reconciliation/entries", entry).MustStatus(t, http.StatusCreated)
	if res := e.reopen("2026-10", "de novo"); res.Status != http.StatusConflict || res.ErrorCode(t) != "RECONCILIATION_NOT_COMPLETED" {
		t.Errorf("reabrir a reaberta = %d %s", res.Status, res.Body)
	}
	// Concluir de novo: Pendente → OK, e as duas conclusões ficam no histórico.
	var again state
	e.complete("2026-10").MustStatus(t, http.StatusOK).JSON(t, &again)
	if again.State != "concluida" || again.Obligation.StatusCode != "OK" || len(again.History) != 2 || again.Current == nil ||
		*again.Current.ObligationStatusBefore != "PENDING" {
		t.Errorf("concluída de novo = %+v", again)
	}
}

// A reabertura respeita o Controle: se alguém mudou a obrigação depois da conclusão, ou se ela já
// era final antes, nada muda (mantida). Obrigação não aplicável e competência sem a obrigação
// também concluem, sem tocar no Controle.
func TestTEST022_HumanChangesAreKept(t *testing.T) {
	e := setup(t)
	lines := e.statement("2026-10", 5)
	e.resolve(lines[0], true)
	e.prerequisites()
	e.complete("2026-10").MustStatus(t, http.StatusOK)
	e.setStatus("ACCOUNTING_RECONCILIATION", "PENDING") // o contador, no Controle
	e.setStatus("ACCOUNTING_RECONCILIATION", "OK")
	var kept state
	e.reopen("2026-10", "revisão").MustStatus(t, http.StatusOK).JSON(t, &kept)
	if kept.Obligation.StatusCode != "OK" || *kept.History[0].ReopenObligationSync != "mantida" {
		t.Fatalf("mudança humana desfeita = %+v", kept)
	}
	// Já final antes da conclusão (o contador marcou OK): ja_final, e a reabertura mantém.
	var final state
	e.complete("2026-10").MustStatus(t, http.StatusOK).JSON(t, &final)
	if final.Current.ObligationSync != "ja_final" {
		t.Fatalf("já final = %+v", final.Current)
	}
	e.reopen("2026-10", "revisão 2").MustStatus(t, http.StatusOK)
	if st := e.summary("2026-10"); st.Obligation.StatusCode != "OK" {
		t.Errorf("obrigação depois de reabrir = %s", st.Obligation.StatusCode)
	}
	if n := e.scalar(`SELECT count(*) FROM obligation_status_history WHERE change_source = 'SYSTEM_INTEGRATION'`); n != "1" {
		t.Errorf("mudanças da integração = %s", n)
	}
	// Não aplicável: conclui sem tocar na obrigação.
	id, _ := e.instance("ACCOUNTING_RECONCILIATION")
	e.exec(`UPDATE obligation_instances SET applicable = false WHERE id = $1`, id)
	var na state
	e.complete("2026-10").MustStatus(t, http.StatusOK).JSON(t, &na)
	if na.Current.ObligationSync != "nao_aplicavel" {
		t.Errorf("não aplicável = %+v", na.Current)
	}
	// Competência sem a obrigação gerada (11/2026): conclui sem obrigação.
	e.resolve(e.statement("2026-11", 4)[0], true)
	var none state
	e.complete("2026-11").MustStatus(t, http.StatusOK).JSON(t, &none)
	if none.State != "concluida" || none.Obligation != nil || none.Current.ObligationSync != "sem_obrigacao" {
		t.Errorf("sem obrigação = %+v", none)
	}
}

// Com o Contábil fechado (a autoridade, ADR-003), não se conclui nem se reabre.
func TestTEST022_AccountingClosedBlocks(t *testing.T) {
	e := setup(t)
	e.resolve(e.statement("2026-10", 5)[0], true)
	e.prerequisites()
	closeAccounting := func(state string) {
		e.exec(`UPDATE company_module_periods p SET state = $2, closed_at = CASE WHEN $2 = 'CLOSED' THEN now() END,
			closed_by = CASE WHEN $2 = 'CLOSED' THEN $3::uuid END
			FROM competencies k WHERE k.id = p.competence_id AND p.company_id = $1 AND p.module_code = 'ACCOUNTING' AND k.competence_key = '2026-10'`,
			e.company, state, user.AdminID)
	}
	closeAccounting("CLOSED")
	if res := e.complete("2026-10"); res.Status != http.StatusLocked || res.ErrorCode(t) != "PERIOD_CLOSED" {
		t.Fatalf("concluir com o Contábil fechado = %d %s", res.Status, res.Body)
	}
	closeAccounting("OPEN")
	e.complete("2026-10").MustStatus(t, http.StatusOK)
	closeAccounting("CLOSED")
	if res := e.reopen("2026-10", "motivo"); res.Status != http.StatusLocked || res.ErrorCode(t) != "PERIOD_CLOSED" {
		t.Fatalf("reabrir com o Contábil fechado = %d %s", res.Status, res.Body)
	}
}

// Concorrência: conclusões simultâneas gravam uma conclusão e um histórico só; a gravação que
// checa a trava espera a conclusão em andamento e, depois dela, é recusada.
func TestTEST022_Concurrency(t *testing.T) {
	e := setup(t)
	e.resolve(e.statement("2026-10", 5)[0], true)
	e.prerequisites()
	var wg sync.WaitGroup
	codes := make([]int, 8)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = e.complete("2026-10").Status
		}(i)
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("conclusão %d = %d", i, code)
		}
	}
	if n := e.scalar(`SELECT count(*) FROM reconciliation_completions`); n != "1" {
		t.Errorf("conclusões = %s", n)
	}
	if h := e.history(); len(h) != 1 {
		t.Errorf("histórico = %+v", h)
	}
	e.reopen("2026-10", "teste de concorrência").MustStatus(t, http.StatusOK)

	// Uma conclusão em andamento (trava exclusiva, ainda sem commit) segura a checagem da trava de
	// uma gravação, que depois vê a conciliação concluída.
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := store.New(tx)
	if err := q.LockReconciliationCompany(ctx, e.company); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertCompletion(ctx, store.InsertCompletionParams{CompanyID: e.company, Competence: competence.MustParse("2026-10").FirstDay(),
		CompletedBy: user.AdminID, Now: time.Now(), ObligationSync: "sem_obrigacao"}); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		wtx, err := e.pool.Begin(ctx)
		if err != nil {
			result <- err
			return
		}
		defer func() { _ = wtx.Rollback(ctx) }()
		result <- reconciliation.RequireOpen(ctx, wtx, e.company, competence.MustParse("2026-10"))
	}()
	select {
	case err := <-result:
		t.Fatalf("a checagem não esperou a conclusão em andamento: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err == nil || !reconciliation.IsLocked(err) || !strings.Contains(err.Error(), "concluída") {
		t.Fatalf("depois da conclusão = %v", err)
	}
	var codesJSON []byte
	if codesJSON, err = json.Marshal(codes); err != nil || len(codesJSON) == 0 {
		t.Fatal(err)
	}
}
