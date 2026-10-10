package proposals_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai"
	"controle/internal/ai/aliases"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/config"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/proposals"
	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-019 [REQ-025, AC-019, AC-025, AC-029]: o resolvedor só propõe. O agente grava a proposta
// com a corrida no tracer e nunca cria conciliação; ela nasce do aceite humano, com origem
// manual, o autor e a proposta, e passa pela trava do Contábil.

var (
	ctx   = context.Background()
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
)

// --- Gateway simulado: devolve a resposta do teste e guarda o pedido ---

type fakeGateway struct {
	mu       sync.Mutex
	requests []litellm.Request
	reply    func(req litellm.Request) (string, error)
}

func (g *fakeGateway) Chat(_ context.Context, req litellm.Request) (litellm.Response, error) {
	g.mu.Lock()
	g.requests = append(g.requests, req)
	reply := g.reply
	g.mu.Unlock()
	if reply == nil {
		return litellm.Response{}, errors.New("gateway simulado sem resposta configurada")
	}
	content, err := reply(req)
	if err != nil {
		return litellm.Response{}, err
	}
	return litellm.Response{Content: content, Model: req.Model.Name + "-2026-06-01",
		Usage: litellm.Usage{PromptTokens: 100, CompletionTokens: 50}, CostUSD: "0.0001", Latency: 10 * time.Millisecond}, nil
}

func (g *fakeGateway) calls() []litellm.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]litellm.Request(nil), g.requests...)
}

func (g *fakeGateway) set(reply func(req litellm.Request) (string, error)) {
	g.mu.Lock()
	g.reply = reply
	g.mu.Unlock()
}

// answer faz o gateway responder sempre a hipótese dada.
func (g *fakeGateway) answer(h map[string]any) {
	b, err := json.Marshal(h)
	if err != nil {
		panic(err)
	}
	g.set(func(litellm.Request) (string, error) { return string(b), nil })
}

func pair(line any, kind string, conf float64, entries ...string) map[string]any {
	return map[string]any{"extrato_transacao_id": line, "lancamento_cliente_ids": append([]string{}, entries...), "tipo": kind, "confianca": conf}
}

func hypothesis(conf float64, asksHuman bool, pairs ...map[string]any) map[string]any {
	return map[string]any{"hipotese": "Pagamento ao fornecedor com o comprovante do dia seguinte.",
		"justificativa":   "Mesmo valor e sentido, contraparte compatível com o descritivo.",
		"pares_propostos": append([]map[string]any{}, pairs...), "confianca": conf, "pede_humano": asksHuman}
}

// --- Ambiente: API e worker do resolvedor sobre Postgres real ---

type env struct {
	srv     *testutil.Server
	pool    *pgxpool.Pool
	gw      *fakeGateway
	runner  *jobs.Runner
	company string
	imp     string
	pos     int
}

var models = ai.Defaults{ai.Resolvedor: {Provider: "openai", Name: "gpt-6-sol"}}

func setup(t *testing.T, opts ...func(*config.Config)) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	e := &env{pool: pool, gw: &fakeGateway{}}
	e.srv = testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00"), opts...)
	sys := clock.NewSystem(time.UTC)
	q := jobs.NewQueue(sys, 3)
	agent := proposals.NewAgent(sys, tracer.New(sys, q), e.gw, aliases.NewResolver(models), quiet)
	e.runner = jobs.NewRunner(pool, sys, quiet, jobs.RunnerConfig{WorkerID: "w-resolvedor", Concurrency: 1, PollInterval: time.Millisecond, Lease: time.Minute},
		map[string]jobs.Handler{reconciliation.JobResolve: agent.HandleResolve})
	e.company, e.imp = e.newCompany(t, "900", "11222333000181")
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

// newCompany cria a empresa com um extrato importado de agosto.
func (e *env) newCompany(t *testing.T, code, cnpj string) (company, imp string) {
	t.Helper()
	company = e.scalar(t, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ($1, 'Empresa ' || $1, $2, '2025-01-01', 'PAYROLL', $3, $3) RETURNING id`, code, cnpj, user.AdminID)
	doc := e.scalar(t, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type)
		VALUES ($1, 'originais', 'originais/' || $2, repeat($2, 64), 'extrato.pdf', 'application/pdf') RETURNING id`, company, code[len(code)-1:])
	imp = e.scalar(t, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'a_fonte', '0001', '12345-6', '2026-08-01', '2026-08-31', 0, 0, true, '{"ok": true}', 0) RETURNING id`, company, doc)
	return company, imp
}

// lineIn cria uma linha do extrato na empresa e no extrato dados.
func (e *env) lineIn(t *testing.T, company, imp, date, amount, dir, desc string) string {
	t.Helper()
	e.pos++
	return e.scalar(t, `INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount, direction, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, company, imp, e.pos, date, desc, amount, dir, fmt.Sprintf("%064x", e.pos))
}

func (e *env) line(t *testing.T, date, amount, dir, desc string) string {
	t.Helper()
	return e.lineIn(t, e.company, e.imp, date, amount, dir, desc)
}

// entryIn cria um lançamento do cliente na empresa dada.
func (e *env) entryIn(t *testing.T, company, date, amount, dir, counterparty, status string) string {
	t.Helper()
	return e.scalar(t, `INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, status)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, company, date, amount, dir, counterparty, status)
}

func (e *env) entry(t *testing.T, date, amount, dir, counterparty string) string {
	t.Helper()
	return e.entryIn(t, e.company, date, amount, dir, counterparty, "manual")
}

// process executa os jobs prontos (o worker).
func (e *env) process(t *testing.T) {
	t.Helper()
	for range 50 {
		ok, err := e.runner.ProcessOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return
		}
	}
	t.Fatal("fila não esvaziou")
}

// closeAccounting fecha o Contábil da empresa em agosto.
func (e *env) closeAccounting(t *testing.T) {
	t.Helper()
	comp := e.scalar(t, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-08', 2026, 8, '2026-08-01')
		ON CONFLICT (competence_key) DO UPDATE SET competence_key = EXCLUDED.competence_key RETURNING id`)
	if _, err := e.pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', 'CLOSED', now(), $3)`, e.company, comp, user.AdminID); err != nil {
		t.Fatal(err)
	}
}

type requestOut struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	AlreadyQueued bool   `json:"already_queued"`
}

type proposalOut struct {
	ID                     string  `json:"id"`
	CompanyID              string  `json:"company_id"`
	StatementTransactionID string  `json:"statement_transaction_id"`
	Status                 string  `json:"status"`
	TraceID                *string `json:"trace_id"`
	CreatedBy              *string `json:"created_by"`
	DecidedBy              *string `json:"decided_by"`
	DecidedAt              *string `json:"decided_at"`
	Hypothesis             struct {
		Hypothesis string `json:"hipotese"`
		Pairs      []struct {
			StatementID *string  `json:"extrato_transacao_id"`
			EntryIDs    []string `json:"lancamento_cliente_ids"`
			Kind        string   `json:"tipo"`
			Confidence  string   `json:"confianca"`
		} `json:"pares_propostos"`
		Confidence string `json:"confianca"`
		AsksHuman  bool   `json:"pede_humano"`
	} `json:"hypothesis"`
}

type acceptOut struct {
	Proposal        proposalOut `json:"proposal"`
	Reconciliations []string    `json:"reconciliation_ids"`
}

func (e *env) request(t *testing.T, line string, status int) requestOut {
	t.Helper()
	var out requestOut
	r := e.srv.Post("/api/v1/reconciliation/review/"+line+"/proposal", nil).MustStatus(t, status)
	if status == http.StatusAccepted {
		r.JSON(t, &out)
	}
	return out
}

func (e *env) proposals(t *testing.T, line string) []proposalOut {
	t.Helper()
	var out struct {
		Items []proposalOut `json:"items"`
	}
	e.srv.Get("/api/v1/reconciliation/review/"+line+"/proposals").MustStatus(t, http.StatusOK).JSON(t, &out)
	return out.Items
}

// propose pede e processa a hipótese da linha e devolve a proposta gravada.
func (e *env) propose(t *testing.T, line string) proposalOut {
	t.Helper()
	e.request(t, line, http.StatusAccepted)
	e.process(t)
	items := e.proposals(t, line)
	if len(items) == 0 {
		t.Fatal("nenhuma proposta gravada")
	}
	return items[0]
}

func (e *env) decide(t *testing.T, line, action, proposal string, status int) testutil.Response {
	t.Helper()
	return e.srv.Post("/api/v1/reconciliation/review/"+line+"/"+action, map[string]string{"proposal_id": proposal}).MustStatus(t, status)
}

// O agente recebe o contexto da linha, grava só a proposta pendente, com a corrida (needs_review)
// e a decisão no tracer, e não concilia nada, nem com confiança alta e sem pedir humano. O aceite
// humano cria a conciliação manual com o método resolvedor, o autor e a proposta [AC-019].
func TestTEST019_AgentOnlyProposesAndHumanAcceptReconciles(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	candidate := e.entry(t, "2026-08-11", "150.00", "D", "Fornecedor X Y Z Ltda")
	e.entry(t, "2026-08-28", "150.00", "D", "Longe demais")
	e.gw.answer(hypothesis(0.97, false, pair(line, "1:1", 0.97, candidate)))

	first := e.request(t, line, http.StatusAccepted)
	if first.JobID == "" || first.Status != "pending" || first.AlreadyQueued {
		t.Fatalf("pedido = %+v", first)
	}
	// Pedido repetido com o job na fila: o mesmo job, sem novo registro no log.
	if again := e.request(t, line, http.StatusAccepted); again.JobID != first.JobID || !again.AlreadyQueued {
		t.Fatalf("pedido repetido = %+v", again)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb->>'acao' = 'pedir_hipotese'
		AND actor_id = $1 AND company_id = $2`, user.AdminID, e.company); n != 1 {
		t.Fatalf("%d pedidos no log de extração", n)
	}
	e.process(t)

	calls := e.gw.calls()
	if len(calls) != 1 {
		t.Fatalf("%d chamadas ao gateway", len(calls))
	}
	req := calls[0]
	if req.Model.Name != "gpt-6-sol" || req.JSONSchema == nil || req.JSONSchema.Name != "hipotese_resolvedor" || len(req.Messages) != 2 {
		t.Fatalf("pedido ao gateway = %+v", req)
	}
	var payload struct {
		Line struct {
			ID string `json:"id"`
		} `json:"transacao_extrato"`
		Candidates []struct {
			ID string `json:"id"`
		} `json:"candidatos_mesmo_valor"`
	}
	if err := json.Unmarshal([]byte(req.Messages[1].Content), &payload); err != nil {
		t.Fatalf("contexto não é JSON: %v", err)
	}
	if payload.Line.ID != line || len(payload.Candidates) != 1 || payload.Candidates[0].ID != candidate {
		t.Fatalf("contexto = %s", req.Messages[1].Content)
	}

	items := e.proposals(t, line)
	if len(items) != 1 {
		t.Fatalf("%d propostas", len(items))
	}
	p := items[0]
	if p.Status != proposals.StatusPending || p.CompanyID != e.company || p.TraceID == nil || p.CreatedBy == nil || *p.CreatedBy != user.AdminID ||
		p.DecidedBy != nil || len(p.Hypothesis.Pairs) != 1 || p.Hypothesis.Pairs[0].EntryIDs[0] != candidate || p.Hypothesis.Confidence != "0.97" ||
		p.Hypothesis.AsksHuman {
		t.Fatalf("proposta = %+v", p)
	}
	// A corrida (AC-029): o agente, o gatilho, quem pediu, a empresa e a decisão com a proposta.
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND agent = 'resolvedor' AND trigger = 'pedido'
		AND status = 'needs_review' AND user_id = $2 AND company_id = $3 AND prompt_version IS NOT NULL`, *p.TraceID, user.AdminID, e.company); n != 1 {
		t.Fatal("corrida do resolvedor não gravada")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'proposta_resolvedor' AND data->>'proposta_id' = $2
		AND data->>'status' = 'pendente' AND data->'ids_fora_do_contexto' = '[]'::jsonb AND data->'ids_invalidos' = '0'::jsonb`, *p.TraceID, p.ID); n != 1 {
		t.Fatal("decisão do resolvedor não gravada")
	}
	// AC-030: a corrida vai ao Langfuse pelo job de sincronização, na mesma transação.
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'langfuse_sync' AND payload->>'trace_id' = $1`, *p.TraceID); n != 1 {
		t.Fatal("sincronização com o Langfuse não enfileirada")
	}
	// AC-019: nada conciliado pelo agente.
	if n := e.count(t, `SELECT count(*) FROM reconciliations`); n != 0 {
		t.Fatalf("o agente criou %d conciliações", n)
	}

	var acc acceptOut
	e.decide(t, line, "accept-proposal", p.ID, http.StatusOK).JSON(t, &acc)
	if acc.Proposal.Status != proposals.StatusAccepted || acc.Proposal.DecidedBy == nil || *acc.Proposal.DecidedBy != user.AdminID ||
		acc.Proposal.DecidedAt == nil || len(acc.Reconciliations) != 1 {
		t.Fatalf("aceite = %+v", acc)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations r JOIN reconciliation_items i ON i.reconciliation_id = r.id
		WHERE r.id = $1 AND r.statement_transaction_id = $2 AND i.client_entry_id = $3 AND r.origin = 'manual' AND r.method = 'resolvedor'
		AND r.created_by = $4 AND r.proposal_id = $5 AND r.pass IS NULL AND r.score IS NULL`,
		acc.Reconciliations[0], line, candidate, user.AdminID, p.ID); n != 1 {
		t.Fatal("conciliação do aceite não gravada como manual, com o autor e a proposta")
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb->>'acao' = 'aceitar_proposta'
		AND change::jsonb->>'proposta' = $1`, p.ID); n != 1 {
		t.Fatal("aceite fora do log de extração")
	}

	// Decidida, a proposta não volta: aceitar ou rejeitar de novo é 409; a linha conciliada não
	// pede outra hipótese.
	if code := e.decide(t, line, "accept-proposal", p.ID, http.StatusConflict).ErrorCode(t); code != "PROPOSAL_DECIDED" {
		t.Fatalf("código = %s", code)
	}
	if code := e.decide(t, line, "reject-proposal", p.ID, http.StatusConflict).ErrorCode(t); code != "PROPOSAL_DECIDED" {
		t.Fatalf("código = %s", code)
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+line+"/proposal", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "LINE_RECONCILED" {
		t.Fatalf("código = %s", code)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations`); n != 1 {
		t.Fatalf("%d conciliações", n)
	}
}

// O aceite ignora o que não pode conciliar, como a origem: id inventado, fora do formato, de
// outra empresa ou repetido. O par de extrato sem comprovante confirma a linha sem lançamento.
func TestTEST019_AcceptIgnoresInventedAndForeignIDs(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	fee := e.line(t, "2026-08-12", "35.00", "D", "TARIFA PACOTE SERVICOS")
	mine := e.entry(t, "2026-08-11", "150.00", "D", "Fornecedor X Y Z Ltda")
	otherCompany, otherImp := e.newCompany(t, "901", "11444777000161")
	foreignLine := e.lineIn(t, otherCompany, otherImp, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	foreignEntry := e.entryIn(t, otherCompany, "2026-08-10", "150.00", "D", "Fornecedor X Y Z Ltda", "manual")
	invented := "0b5e2c1a-1111-4222-8333-944455566677"
	e.gw.answer(hypothesis(0.8, true,
		pair(nil, "1:1", 0.8, invented, "lancamento-7", strings.ToUpper(mine), mine, foreignEntry),
		pair(fee, "extrato_sem_comprovante", 0.9),
		pair(foreignLine, "1:1", 0.7, foreignEntry)))
	p := e.propose(t, line)

	// A decisão registra os ids que o modelo citou fora do contexto (a linha da tarifa não estava
	// nele) e só conta o que nem é UUID: o texto do modelo não vai ao Langfuse.
	var unknown []string
	var raw []byte
	var malformed int
	if err := e.pool.QueryRow(ctx, `SELECT data->'ids_fora_do_contexto', (data->>'ids_invalidos')::int FROM ai_decisions WHERE trace_id = $1`,
		*p.TraceID).Scan(&raw, &malformed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &unknown); err != nil {
		t.Fatal(err)
	}
	slices.Sort(unknown)
	want := []string{invented, fee, foreignLine, foreignEntry}
	slices.Sort(want)
	if !slices.Equal(unknown, want) || malformed != 1 {
		t.Fatalf("ids fora do contexto = %v, inválidos = %d; esperado %v e 1", unknown, malformed, want)
	}

	var acc acceptOut
	e.decide(t, line, "accept-proposal", p.ID, http.StatusOK).JSON(t, &acc)
	if len(acc.Reconciliations) != 2 {
		t.Fatalf("aceite = %+v", acc)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations r JOIN reconciliation_items i ON i.reconciliation_id = r.id
		WHERE r.statement_transaction_id = $1 AND i.client_entry_id = $2 AND r.method = 'resolvedor'`, line, mine); n != 1 {
		t.Fatal("o par válido não foi conciliado")
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
		WHERE r.statement_transaction_id = $1`, line); n != 1 {
		t.Fatalf("%d lançamentos na conciliação (inventado, repetido ou de outra empresa entrou)", n)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations r WHERE r.statement_transaction_id = $1 AND r.method = 'sem_comprovante'
		AND r.origin = 'manual' AND NOT EXISTS (SELECT 1 FROM reconciliation_items i WHERE i.reconciliation_id = r.id)`, fee); n != 1 {
		t.Fatal("a linha sem comprovante não foi confirmada")
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations WHERE company_id = $1`, otherCompany); n != 0 {
		t.Fatal("o aceite conciliou na outra empresa")
	}
}

// Sem nenhum par conciliável, o aceite não muda nada (409) e a proposta segue pendente; a
// rejeição a encerra sem conciliar.
func TestTEST019_NothingToAcceptThenReject(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	otherCompany, _ := e.newCompany(t, "901", "11444777000161")
	foreignEntry := e.entryIn(t, otherCompany, "2026-08-10", "150.00", "D", "Fornecedor", "manual")
	e.gw.answer(hypothesis(0.4, true, pair(line, "1:1", 0.4, foreignEntry), pair(line, "outro", 0.2)))
	p := e.propose(t, line)

	if code := e.decide(t, line, "accept-proposal", p.ID, http.StatusConflict).ErrorCode(t); code != "PROPOSAL_NOTHING_TO_ACCEPT" {
		t.Fatalf("código = %s", code)
	}
	if got := e.proposals(t, line)[0]; got.Status != proposals.StatusPending || got.DecidedBy != nil {
		t.Fatalf("proposta = %+v", got)
	}
	var rej proposalOut
	e.decide(t, line, "reject-proposal", p.ID, http.StatusOK).JSON(t, &rej)
	if rej.Status != proposals.StatusRejected || rej.DecidedBy == nil || *rej.DecidedBy != user.AdminID {
		t.Fatalf("rejeição = %+v", rej)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations`); n != 0 {
		t.Fatalf("%d conciliações", n)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb->>'acao' = 'rejeitar_proposta'`); n != 1 {
		t.Fatal("rejeição fora do log de extração")
	}
	// A proposta de outra linha não é encontrada por esta.
	other := e.line(t, "2026-08-20", "99.00", "C", "PIX RECEBIDO")
	if code := e.decide(t, other, "reject-proposal", p.ID, http.StatusNotFound).ErrorCode(t); code != "PROPOSAL_NOT_FOUND" {
		t.Fatalf("código = %s", code)
	}
}

// Saída fora do schema: a corrida fica com erro, o job morre sem nova tentativa (cada chamada tem
// custo) e nenhuma proposta é gravada.
func TestTEST019_InvalidOutputRecordsNoProposal(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	e.gw.set(func(litellm.Request) (string, error) {
		return `{"hipotese": "x", "justificativa": "y", "pares_propostos": [], "confianca": 1.7, "pede_humano": false}`, nil
	})
	e.request(t, line, http.StatusAccepted)
	e.process(t)
	if len(e.gw.calls()) != 1 {
		t.Fatalf("%d chamadas ao gateway", len(e.gw.calls()))
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_resolver' AND status = 'dead' AND last_error_code = 'RESOLVER_INVALID_OUTPUT'`); n != 1 {
		t.Fatal("o job devia morrer com RESOLVER_INVALID_OUTPUT")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE agent = 'resolvedor' AND status = 'error' AND error_code = 'RESOLVER_INVALID_OUTPUT'`); n != 1 {
		t.Fatal("corrida de erro não gravada")
	}
	if len(e.proposals(t, line)) != 0 || e.count(t, `SELECT count(*) FROM reconciliations`) != 0 {
		t.Fatal("saída inválida gravou proposta ou conciliação")
	}
}

// Alias desligado na tela de IA: o job morre sem chamar o modelo.
func TestTEST019_DisabledAlias(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	if _, err := e.pool.Exec(ctx, `UPDATE ai_model_aliases SET enabled = false WHERE alias = 'resolvedor'`); err != nil {
		t.Fatal(err)
	}
	e.request(t, line, http.StatusAccepted)
	e.process(t)
	if len(e.gw.calls()) != 0 {
		t.Fatal("chamou o modelo com o alias desligado")
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_resolver' AND status = 'dead' AND last_error_code = 'AGENT_DISABLED'`); n != 1 {
		t.Fatal("o job devia morrer com AGENT_DISABLED")
	}
}

// Contábil fechado (AC-025): o pedido, o aceite e a rejeição respondem 423; o job que já estava na
// fila termina sem chamar o modelo.
func TestTEST019_AccountingLock(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	candidate := e.entry(t, "2026-08-11", "150.00", "D", "Fornecedor X Y Z Ltda")
	e.gw.answer(hypothesis(0.9, false, pair(line, "1:1", 0.9, candidate)))
	p := e.propose(t, line)
	e.request(t, line, http.StatusAccepted)
	e.closeAccounting(t)
	e.process(t)
	if len(e.gw.calls()) != 1 || len(e.proposals(t, line)) != 1 {
		t.Fatal("o job rodou o modelo com o Contábil fechado")
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_resolver' AND status = 'done'`); n != 2 {
		t.Fatalf("%d jobs concluídos", n)
	}
	for _, action := range []string{"accept-proposal", "reject-proposal"} {
		if code := e.decide(t, line, action, p.ID, http.StatusLocked).ErrorCode(t); code != "PERIOD_CLOSED" {
			t.Fatalf("%s: código = %s", action, code)
		}
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+line+"/proposal", nil).MustStatus(t, http.StatusLocked).ErrorCode(t); code != "PERIOD_CLOSED" {
		t.Fatalf("código = %s", code)
	}
	if got := e.proposals(t, line)[0]; got.Status != proposals.StatusPending || e.count(t, `SELECT count(*) FROM reconciliations`) != 0 {
		t.Fatal("o Contábil fechado não travou")
	}
}

// A linha conciliada pela rodada enquanto o pedido esperava não chega ao modelo.
func TestTEST019_LineReconciledWhileQueued(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO ALFA")
	e.entry(t, "2026-08-10", "150.00", "D", "Alfa")
	e.request(t, line, http.StatusAccepted)
	e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusOK)
	e.process(t)
	if len(e.gw.calls()) != 0 || len(e.proposals(t, line)) != 0 {
		t.Fatal("pediu hipótese para linha conciliada")
	}
}

// Linha inexistente, id fora do formato e corpo sem a proposta.
func TestTEST019_Validation(t *testing.T) {
	e := setup(t)
	line := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO ALFA")
	for _, id := range []string{"00000000-0000-4000-8000-000000000000", "x"} {
		if code := e.srv.Post("/api/v1/reconciliation/review/"+id+"/proposal", nil).MustStatus(t, http.StatusNotFound).ErrorCode(t); code != "RECONCILIATION_LINE_NOT_FOUND" {
			t.Fatalf("código = %s", code)
		}
		e.srv.Get("/api/v1/reconciliation/review/"+id+"/proposals").MustStatus(t, http.StatusNotFound)
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+line+"/accept-proposal", map[string]string{}).MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "VALIDATION_ERROR" {
		t.Fatalf("código = %s", code)
	}
	if code := e.decide(t, line, "accept-proposal", "00000000-0000-4000-8000-000000000000", http.StatusNotFound).ErrorCode(t); code != "PROPOSAL_NOT_FOUND" {
		t.Fatalf("código = %s", code)
	}
}

// RESOLVEDOR_AUTOMATICO: a rodada pede hipótese uma só vez para cada linha que sobrou; o job com
// falha e a proposta rejeitada não voltam sozinhos. Desligado (padrão), a rodada não pede nada.
func TestTEST019_AutoResolveAsksOncePerLine(t *testing.T) {
	off := setup(t)
	off.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	var s struct {
		ResolverRequested int `json:"resolver_requested"`
	}
	off.srv.Post("/api/v1/companies/"+off.company+"/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusOK).JSON(t, &s)
	if s.ResolverRequested != 0 || off.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_resolver'`) != 0 {
		t.Fatal("a rodada pediu hipótese com o resolvedor automático desligado")
	}

	e := setup(t, func(c *config.Config) { c.ResolverAuto = true })
	a := e.line(t, "2026-08-10", "150.00", "D", "PIX ENVIADO FORNECEDOR XYZ")
	b := e.line(t, "2026-08-12", "35.00", "D", "TARIFA PACOTE SERVICOS")
	matched := e.line(t, "2026-08-15", "80.00", "C", "PIX RECEBIDO ALFA")
	e.entry(t, "2026-08-15", "80.00", "C", "Alfa")
	run := func() int {
		t.Helper()
		var out struct {
			NewMatches        int `json:"new_matches"`
			ResolverRequested int `json:"resolver_requested"`
		}
		e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence=2026-08", nil).MustStatus(t, http.StatusOK).JSON(t, &out)
		return out.ResolverRequested
	}
	if n := run(); n != 2 {
		t.Fatalf("primeira rodada pediu %d hipóteses", n)
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_resolver' AND dedupe_key = $1`, matched); n != 0 {
		t.Fatal("pediu hipótese para a linha conciliada")
	}
	if n := run(); n != 0 {
		t.Fatalf("rodada repetida pediu %d hipóteses", n)
	}
	// A hipótese de a é válida; a de b sai fora do schema (job morto).
	valid, err := json.Marshal(hypothesis(0.5, true, pair(a, "outro", 0.5)))
	if err != nil {
		t.Fatal(err)
	}
	e.gw.set(func(req litellm.Request) (string, error) {
		if strings.Contains(req.Messages[1].Content, a) {
			return string(valid), nil
		}
		return `{"hipotese": ""}`, nil
	})
	e.process(t)
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE agent = 'resolvedor' AND trigger = 'rodada' AND user_id IS NULL`); n != 2 {
		t.Fatalf("%d corridas da rodada", n)
	}
	if n := e.count(t, `SELECT count(*) FROM resolver_proposals WHERE created_by IS NULL AND statement_transaction_id = $1`, a); n != 1 {
		t.Fatalf("%d propostas do automático", n)
	}
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_resolver' AND dedupe_key = $1 AND status = 'dead'`, b); n != 1 {
		t.Fatal("o job da linha com saída inválida devia morrer")
	}
	if n := run(); n != 0 {
		t.Fatalf("depois das corridas, a rodada pediu %d hipóteses", n)
	}
	if n := e.count(t, `SELECT count(*) FROM reconciliations WHERE origin = 'manual'`); n != 0 {
		t.Fatal("o resolvedor automático conciliou")
	}
}
