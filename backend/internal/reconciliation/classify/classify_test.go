package classify_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	"controle/internal/reconciliation/classify"
	"controle/internal/testutil"
	"controle/internal/user"
)

// TEST-020 [REQ-026, REQ-027, AC-020, AC-025]: a classificação da linha conciliada na precedência
// regra → histórico → agente; a correção propõe a regra, que só vale confirmada; a sugestão do
// agente só muda a conta com a confirmação humana; a trava do Contábil.

var (
	ctx   = context.Background()
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
)

// --- Gateway simulado ---

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
		Usage: litellm.Usage{PromptTokens: 80, CompletionTokens: 40}, CostUSD: "0.0001", Latency: 10 * time.Millisecond}, nil
}

func (g *fakeGateway) calls() []litellm.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]litellm.Request(nil), g.requests...)
}

func (g *fakeGateway) answer(v map[string]any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	g.mu.Lock()
	g.reply = func(litellm.Request) (string, error) { return string(b), nil }
	g.mu.Unlock()
}

func suggestion(account any, rule any, conf float64) map[string]any {
	return map[string]any{"conta_codigo": account, "conta_nome": nil, "justificativa": "Parece tarifa do pacote do banco.",
		"regra_sugerida": rule, "confianca": conf}
}

// --- Ambiente ---

type env struct {
	srv     *testutil.Server
	pool    *pgxpool.Pool
	gw      *fakeGateway
	runner  *jobs.Runner
	company string
	imp     string
	pos     int
}

var models = ai.Defaults{ai.Classificador: {Provider: "openai", Name: "gpt-6-sol"}}

func setup(t *testing.T, opts ...func(*config.Config)) *env {
	t.Helper()
	pool := testutil.NewDB(t)
	e := &env{pool: pool, gw: &fakeGateway{}}
	e.srv = testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-09-15T12:00:00-03:00"), opts...)
	sys := clock.NewSystem(time.UTC)
	q := jobs.NewQueue(sys, 3)
	agent := classify.NewAgent(sys, tracer.New(sys, q), e.gw, aliases.NewResolver(models), quiet)
	e.runner = jobs.NewRunner(pool, sys, quiet, jobs.RunnerConfig{WorkerID: "w-classificador", Concurrency: 1, PollInterval: time.Millisecond, Lease: time.Minute},
		map[string]jobs.Handler{reconciliation.JobClassify: agent.HandleSuggest})
	e.company = e.scalar(t, `INSERT INTO companies (internal_code, legal_name, cnpj, service_start_date, payroll_type, created_by, updated_by)
		VALUES ('900', 'Empresa 900', '11222333000181', '2025-01-01', 'PAYROLL', $1, $1) RETURNING id`, user.AdminID)
	doc := e.scalar(t, `INSERT INTO reconciliation_documents (company_id, bucket, object_key, sha256, file_name, content_type)
		VALUES ($1, 'originais', 'originais/x', repeat('a', 64), 'extrato.pdf', 'application/pdf') RETURNING id`, e.company)
	e.imp = e.scalar(t, `INSERT INTO statement_imports (company_id, document_id, bank, branch, account, period_start, period_end,
		opening_balance, closing_balance, validation_ok, validation, attempts)
		VALUES ($1, $2, 'a_fonte', '0001', '12345-6', '2026-07-01', '2026-09-30', 0, 0, true, '{"ok": true}', 0) RETURNING id`, e.company, doc)
	for code, name := range map[string]string{"1.01": "Receita de serviços", "2.01": "Fornecedores", "3.01": "Material de escritório",
		"4.01": "Despesas bancárias", "5.01": "Consultoria"} {
		e.srv.Post("/api/v1/companies/"+e.company+"/chart-accounts", map[string]string{"code": code, "name": name}).MustStatus(t, http.StatusCreated)
	}
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

// line cria uma linha do extrato.
func (e *env) line(t *testing.T, date, amount, dir, desc string) string {
	t.Helper()
	e.pos++
	return e.scalar(t, `INSERT INTO statement_transactions (company_id, statement_import_id, position, txn_date, description, amount, direction, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, e.company, e.imp, e.pos, date, desc, amount, dir, fmt.Sprintf("%064x", e.pos))
}

// pair cria a linha e o comprovante idêntico (o passe 1 concilia na rodada).
func (e *env) pair(t *testing.T, date, amount, dir, desc, counterparty string, doc *string) (line, entry string) {
	t.Helper()
	line = e.line(t, date, amount, dir, desc)
	entry = e.scalar(t, `INSERT INTO client_entries (company_id, entry_date, amount, direction, counterparty, counterparty_doc, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'manual') RETURNING id`, e.company, date, amount, dir, counterparty, doc)
	return line, entry
}

// noReceipt concilia a linha sem comprovante (o "sem comprovante — confirmado" da fila).
func (e *env) noReceipt(t *testing.T, line string) {
	t.Helper()
	e.scalar(t, `INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, created_by)
		VALUES ($1, $2, 'sem_comprovante', 'manual', $3) RETURNING id`, e.company, line, user.AdminID)
}

type roundOut struct {
	NewMatches     int `json:"new_matches"`
	Classification struct {
		ByRule    int `json:"by_rule"`
		ByHistory int `json:"by_history"`
		Pending   int `json:"pending"`
	} `json:"classification"`
	ClassifierRequested int `json:"classifier_requested"`
}

func (e *env) round(t *testing.T, comp string) roundOut {
	t.Helper()
	var out roundOut
	e.srv.Post("/api/v1/companies/"+e.company+"/reconciliation/run?competence="+comp, nil).MustStatus(t, http.StatusOK).JSON(t, &out)
	return out
}

type classOut struct {
	Account       *string `json:"account_code"`
	Origin        *string `json:"origin"`
	Pending       bool    `json:"pending"`
	Rule          *string `json:"rule"`
	History       *string `json:"history"`
	Suggestion    *string `json:"suggestion"`
	ClassifiedBy  *string `json:"classified_by"`
	ClassifiedAt  *string `json:"classified_at"`
	StatementLine string  `json:"id"`
}

func (e *env) classification(t *testing.T, line string) classOut {
	t.Helper()
	var c classOut
	if err := e.pool.QueryRow(ctx, `SELECT id, account_code, classification_origin, classification_pending, classification_rule_id,
		classification_history_id, classification_suggestion_id, classified_by, classified_at::text FROM statement_transactions WHERE id = $1`, line).
		Scan(&c.StatementLine, &c.Account, &c.Origin, &c.Pending, &c.Rule, &c.History, &c.Suggestion, &c.ClassifiedBy, &c.ClassifiedAt); err != nil {
		t.Fatal(err)
	}
	return c
}

func (e *env) rule(t *testing.T, ruleType, pattern, account string, status int) map[string]any {
	t.Helper()
	var out map[string]any
	r := e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules",
		map[string]any{"rule_type": ruleType, "pattern": pattern, "account_code": account}).MustStatus(t, status)
	if status == http.StatusCreated {
		r.JSON(t, &out)
	}
	return out
}

type correctOut struct {
	Classification struct {
		Account      *string `json:"account_code"`
		Origin       *string `json:"origin"`
		Pending      bool    `json:"pending"`
		ClassifiedBy *string `json:"classified_by"`
	} `json:"classification"`
	ProposedRule *struct {
		RuleType     string `json:"rule_type"`
		Pattern      string `json:"pattern"`
		AccountCode  string `json:"account_code"`
		SourceLineID string `json:"source_statement_transaction_id"`
	} `json:"proposed_rule"`
}

func (e *env) correct(t *testing.T, line, account string, status int) correctOut {
	t.Helper()
	var out correctOut
	r := e.srv.Put("/api/v1/reconciliation/review/"+line+"/classification", map[string]string{"account_code": account}).MustStatus(t, status)
	if status == http.StatusOK {
		r.JSON(t, &out)
	}
	return out
}

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

func ptr(s string) *string { return &s }

func is(p *string, want string) bool { return p != nil && *p == want }

// A precedência é fixa: regra (a mais nova primeiro), histórico dos meses anteriores (o mais
// recente) e, sem os dois, à espera do agente. A rodada classifica o que conciliou; rodar de novo
// não muda nada.
func TestTEST020_PrecedenceRuleHistoryAgent(t *testing.T) {
	e := setup(t)
	// Histórico de julho: Gama classificada à mão em 3.01 e, antes, em 2.01 (vale a mais recente).
	older, _ := e.pair(t, "2026-07-03", "70.00", "D", "PIX ENVIADO GAMA 01", "Gama Papelaria", nil)
	recent, _ := e.pair(t, "2026-07-20", "180.00", "D", "PIX ENVIADO GAMA 02", "GAMA  PAPELARIA", nil)
	e.round(t, "2026-07")
	e.correct(t, older, "2.01", http.StatusOK)
	e.correct(t, recent, "3.01", http.StatusOK)
	// Regras: CNPJ do fornecedor Alfa e a tarifa do banco.
	e.rule(t, "cnpj", "12.345.678/0001-99", "2.01", http.StatusCreated)
	tariffRule := e.rule(t, "regex_descritivo", "tarifa\\s+pacote", "4.01", http.StatusCreated)["rule"].(map[string]any)["id"].(string)

	// Datas e valores espaçados: fora das bordas da TASK-018 (bruto × líquido até R$ 20 em D±3).
	byCNPJ, _ := e.pair(t, "2026-08-05", "500.00", "D", "PIX ENVIADO GAMA 03", "Gama Papelaria", ptr("12345678000199"))
	byHistory, _ := e.pair(t, "2026-08-10", "90.00", "D", "PIX ENVIADO GAMA 04", "Gama Papelaria", nil)
	tariff := e.line(t, "2026-08-15", "35.00", "D", "TARIFA PACOTE SERVICOS")
	e.noReceipt(t, tariff)
	unknown, _ := e.pair(t, "2026-08-25", "250.00", "D", "PIX ENVIADO ZETA", "Zeta Consultoria", nil)

	s := e.round(t, "2026-08")
	if s.NewMatches != 3 || s.Classification.ByRule != 2 || s.Classification.ByHistory != 1 || s.Classification.Pending != 1 {
		t.Fatalf("rodada = %+v", s)
	}
	if c := e.classification(t, byCNPJ); !is(c.Account, "2.01") || !is(c.Origin, "regra") || c.Rule == nil || c.ClassifiedBy != nil {
		t.Fatalf("regra ganha do histórico: %+v", c)
	}
	if c := e.classification(t, byHistory); !is(c.Account, "3.01") || !is(c.Origin, "historico") || !is(c.History, recent) {
		t.Fatalf("histórico (o mais recente) = %+v", c)
	}
	if c := e.classification(t, tariff); !is(c.Account, "4.01") || !is(c.Rule, tariffRule) {
		t.Fatalf("linha sem comprovante pelo descritivo = %+v", c)
	}
	if c := e.classification(t, unknown); c.Account != nil || !is(c.Origin, "agente") || !c.Pending {
		t.Fatalf("sem regra nem histórico, à espera do agente = %+v", c)
	}
	// Sem regra nem histórico em agosto: o agente não roda sem pedido.
	if len(e.gw.calls()) != 0 || e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_classifier'`) != 0 {
		t.Fatal("o classificador rodou sem pedido")
	}
	if again := e.round(t, "2026-08"); again.Classification.ByRule != 0 || again.Classification.ByHistory != 0 || again.Classification.Pending != 1 {
		t.Fatalf("rodada repetida = %+v", again)
	}
}

// A correção grava a conta do contador e propõe a regra derivada, que só vale confirmada. A regra
// confirmada classifica o que espera classificação e nunca troca o que foi confirmado; a mais
// nova vale, e a do mesmo tipo e padrão substitui a antiga [REQ-027].
func TestTEST020_CorrectionProposesRuleOnlyConfirmedRulesApply(t *testing.T) {
	e := setup(t)
	first, _ := e.pair(t, "2026-08-05", "250.00", "D", "PIX ENVIADO ZETA 1", "Zeta Consultoria", nil)
	second, _ := e.pair(t, "2026-08-12", "400.00", "D", "PIX ENVIADO ZETA 2", "ZETA CONSULTORIA", nil)
	e.round(t, "2026-08")

	got := e.correct(t, first, "5.01", http.StatusOK)
	if !is(got.Classification.Account, "5.01") || !is(got.Classification.Origin, "manual") || got.Classification.Pending ||
		!is(got.Classification.ClassifiedBy, user.AdminID) {
		t.Fatalf("correção = %+v", got.Classification)
	}
	p := got.ProposedRule
	if p == nil || p.RuleType != "contraparte" || p.Pattern != "Zeta Consultoria" || p.AccountCode != "5.01" || p.SourceLineID != first {
		t.Fatalf("regra proposta = %+v", p)
	}
	// Proposta não é regra: nada muda até a confirmação.
	if e.count(t, `SELECT count(*) FROM classification_rules`) != 0 {
		t.Fatal("a correção criou regra sem confirmação")
	}
	if c := e.classification(t, second); c.Account != nil {
		t.Fatalf("classificou sem regra confirmada: %+v", c)
	}
	if n := e.count(t, `SELECT count(*) FROM extraction_logs WHERE alias = 'humano' AND change::jsonb->>'acao' = 'classificar'`); n != 1 {
		t.Fatal("correção fora do log de extração")
	}

	// Confirmada, a regra classifica a outra linha de Zeta na próxima classificação.
	var created struct {
		Rule struct {
			ID           string  `json:"id"`
			SourceLineID *string `json:"source_statement_transaction_id"`
		} `json:"rule"`
		Replaced []string `json:"replaced_rule_ids"`
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules", map[string]any{"rule_type": p.RuleType, "pattern": p.Pattern,
		"account_code": p.AccountCode, "source_statement_transaction_id": p.SourceLineID}).MustStatus(t, http.StatusCreated).JSON(t, &created)
	if !is(created.Rule.SourceLineID, first) || len(created.Replaced) != 0 {
		t.Fatalf("regra confirmada = %+v", created)
	}
	var run struct {
		ByRule int `json:"by_rule"`
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/classification/run?competence=2026-08", nil).MustStatus(t, http.StatusOK).JSON(t, &run)
	if c := e.classification(t, second); run.ByRule != 1 || !is(c.Account, "5.01") || !is(c.Rule, created.Rule.ID) {
		t.Fatalf("a regra confirmada não classificou: %+v (%d)", c, run.ByRule)
	}

	// A mais nova vale: a regra do mesmo padrão (normalizado) para outra conta desativa a antiga, e
	// uma regex mais nova passa na frente da contraparte. Nada confirmado muda.
	var replaced struct {
		Replaced []string `json:"replaced_rule_ids"`
	}
	e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules", map[string]any{"rule_type": "contraparte", "pattern": "  ZETA consultoria ",
		"account_code": "3.01"}).MustStatus(t, http.StatusCreated).JSON(t, &replaced)
	if len(replaced.Replaced) != 1 || replaced.Replaced[0] != created.Rule.ID {
		t.Fatalf("substituídas = %v", replaced.Replaced)
	}
	newest := e.rule(t, "regex_descritivo", "zeta", "1.01", http.StatusCreated)["rule"].(map[string]any)["id"].(string)
	third, _ := e.pair(t, "2026-08-25", "600.00", "D", "PIX ENVIADO ZETA 3", "Zeta Consultoria", nil)
	e.round(t, "2026-08")
	if c := e.classification(t, third); !is(c.Account, "1.01") || !is(c.Rule, newest) {
		t.Fatalf("a regra mais nova devia valer: %+v", c)
	}
	if c := e.classification(t, first); !is(c.Account, "5.01") || !is(c.Origin, "manual") {
		t.Fatalf("a regra trocou a classificação do contador: %+v", c)
	}
	if c := e.classification(t, second); !is(c.Account, "5.01") {
		t.Fatalf("a regra nova trocou classificação já feita: %+v", c)
	}
}

// Regras: CNPJ só com os dígitos (11 ou 14), regex no subconjunto do re que o porte traduz, conta
// do plano ativo; desativar não apaga.
func TestTEST020_RuleValidation(t *testing.T) {
	e := setup(t)
	for _, c := range []struct{ ruleType, pattern, account, field string }{
		{"cnpj", "123", "2.01", "pattern"},
		{"regex_descritivo", "(?P<x>a)", "2.01", "pattern"},
		{"regex_descritivo", "(", "2.01", "pattern"},
		{"contraparte", "   ", "2.01", "pattern"},
		{"outro", "x", "2.01", "rule_type"},
		{"contraparte", "Alfa", "9.99", "account_code"},
	} {
		var errOut struct {
			Code    string `json:"code"`
			Details struct {
				Fields map[string]string `json:"fields"`
			} `json:"details"`
		}
		e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules",
			map[string]any{"rule_type": c.ruleType, "pattern": c.pattern, "account_code": c.account}).MustStatus(t, http.StatusBadRequest).JSON(t, &errOut)
		if errOut.Code != "VALIDATION_ERROR" || errOut.Details.Fields[c.field] == "" {
			t.Errorf("%+v: %+v", c, errOut)
		}
	}
	r := e.rule(t, "cnpj", "12.345.678/0001-99", "2.01", http.StatusCreated)["rule"].(map[string]any)
	if r["pattern"] != "12345678000199" {
		t.Fatalf("CNPJ não normalizado: %v", r["pattern"])
	}
	id := r["id"].(string)
	e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules/"+id+"/deactivate", nil).MustStatus(t, http.StatusOK)
	if code := e.srv.Post("/api/v1/companies/"+e.company+"/classification-rules/"+id+"/deactivate", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "RULE_INACTIVE" {
		t.Fatalf("código = %s", code)
	}
	var list struct {
		Items []map[string]any `json:"items"`
	}
	e.srv.Get("/api/v1/companies/"+e.company+"/classification-rules").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 1 || list.Items[0]["active"] != false || list.Items[0]["deactivated_by"] != user.AdminID {
		t.Fatalf("regras = %+v", list.Items)
	}
	e.srv.Get("/api/v1/companies/"+e.company+"/classification-rules?active=true").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 0 {
		t.Fatal("regra desativada listada como ativa")
	}
}

// O agente só sugere [AC-020]: recebe o plano e o histórico parecido, grava a sugestão pendente
// com a corrida, e a conta da linha só muda com o aceite humano, que propõe a regra sugerida.
func TestTEST020_AgentSuggestsHumanConfirms(t *testing.T) {
	e := setup(t)
	// Histórico de julho parecido com a linha de agosto (Gama Papelaria × Gama Papelarias Ltda).
	hist, _ := e.pair(t, "2026-07-03", "70.00", "D", "PIX ENVIADO GAMA", "Gama Papelaria", nil)
	e.round(t, "2026-07")
	e.correct(t, hist, "3.01", http.StatusOK)
	line, _ := e.pair(t, "2026-08-10", "90.00", "D", "PIX ENVIADO GAMA LTDA", "Gama Papelarias Ltda", ptr("12345678909"))
	e.round(t, "2026-08")
	e.gw.answer(suggestion("3.01", map[string]any{"tipo": "contraparte", "padrao": "Gama Papelarias Ltda"}, 0.82))

	var req struct {
		JobID         string `json:"job_id"`
		AlreadyQueued bool   `json:"already_queued"`
	}
	e.srv.Post("/api/v1/reconciliation/review/"+line+"/classification-suggestion", nil).MustStatus(t, http.StatusAccepted).JSON(t, &req)
	if req.JobID == "" || req.AlreadyQueued {
		t.Fatalf("pedido = %+v", req)
	}
	e.process(t)
	calls := e.gw.calls()
	if len(calls) != 1 || calls[0].Model.Name != "gpt-6-sol" || calls[0].JSONSchema == nil || calls[0].JSONSchema.Name != "sugestao_classificacao" {
		t.Fatalf("chamadas = %+v", calls)
	}
	var payload struct {
		Entry struct {
			ID           string  `json:"id"`
			Counterparty *string `json:"contraparte"`
			CNPJ         *string `json:"cnpj_contraparte"`
		} `json:"lancamento"`
		Chart   []struct{ Codigo string } `json:"plano_contas"`
		Similar []struct {
			Key     string `json:"contraparte_ou_descritivo"`
			Account string `json:"conta_codigo"`
		} `json:"historico_classificacao"`
	}
	if err := json.Unmarshal([]byte(calls[0].Messages[1].Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Entry.ID != line || !is(payload.Entry.Counterparty, "Gama Papelarias Ltda") || payload.Entry.CNPJ != nil || len(payload.Chart) != 5 ||
		len(payload.Similar) != 1 || payload.Similar[0].Key != "gama papelaria" || payload.Similar[0].Account != "3.01" {
		t.Fatalf("contexto (CPF fora, plano e histórico parecido) = %s", calls[0].Messages[1].Content)
	}

	var list struct {
		Items []struct {
			ID      string  `json:"id"`
			Status  string  `json:"status"`
			Account *string `json:"account_code"`
			TraceID *string `json:"trace_id"`
		} `json:"items"`
	}
	e.srv.Get("/api/v1/reconciliation/review/"+line+"/classification-suggestions").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 1 || list.Items[0].Status != "pendente" || !is(list.Items[0].Account, "3.01") || list.Items[0].TraceID == nil {
		t.Fatalf("sugestões = %+v", list.Items)
	}
	sg := list.Items[0]
	// Só a sugestão: a linha continua à espera.
	if c := e.classification(t, line); c.Account != nil || !c.Pending {
		t.Fatalf("o agente classificou: %+v", c)
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE trace_id = $1 AND agent = 'classificador' AND trigger = 'pedido' AND status = 'needs_review'
		AND user_id = $2`, *sg.TraceID, user.AdminID); n != 1 {
		t.Fatal("corrida do classificador não gravada")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'sugestao_classificacao' AND data->>'sugestao_id' = $2
		AND data->>'conta' = '3.01' AND (data->>'conta_no_plano')::boolean`, *sg.TraceID, sg.ID); n != 1 {
		t.Fatal("decisão do classificador não gravada")
	}

	var acc struct {
		Classification struct {
			Account      *string `json:"account_code"`
			Origin       *string `json:"origin"`
			Pending      bool    `json:"pending"`
			SuggestionID *string `json:"suggestion_id"`
			ClassifiedBy *string `json:"classified_by"`
		} `json:"classification"`
		Suggestion struct {
			Status string `json:"status"`
		} `json:"suggestion"`
		ProposedRule *struct {
			RuleType string `json:"rule_type"`
			Pattern  string `json:"pattern"`
		} `json:"proposed_rule"`
	}
	e.srv.Post("/api/v1/reconciliation/review/"+line+"/accept-classification-suggestion", map[string]string{"suggestion_id": sg.ID}).
		MustStatus(t, http.StatusOK).JSON(t, &acc)
	c := acc.Classification
	if !is(c.Account, "3.01") || !is(c.Origin, "agente") || c.Pending || !is(c.SuggestionID, sg.ID) || !is(c.ClassifiedBy, user.AdminID) ||
		acc.Suggestion.Status != "aceita" || acc.ProposedRule == nil || acc.ProposedRule.RuleType != "contraparte" || acc.ProposedRule.Pattern != "Gama Papelarias Ltda" {
		t.Fatalf("aceite = %+v", acc)
	}
	if e.count(t, `SELECT count(*) FROM classification_rules`) != 0 {
		t.Fatal("o aceite criou regra sem confirmação")
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+line+"/classification-suggestion", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "LINE_ALREADY_CLASSIFIED" {
		t.Fatalf("código = %s", code)
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+line+"/reject-classification-suggestion", map[string]string{"suggestion_id": sg.ID}).
		MustStatus(t, http.StatusConflict).ErrorCode(t); code != "SUGGESTION_DECIDED" {
		t.Fatalf("código = %s", code)
	}
}

// Conta fora do plano não vira sugestão aceitável; saída inválida não vira sugestão; regra
// confirmada enquanto o pedido esperava classifica sem chamar o modelo; sem plano, não há pedido.
func TestTEST020_AgentGuards(t *testing.T) {
	e := setup(t)
	outside, _ := e.pair(t, "2026-08-02", "90.00", "D", "PIX ENVIADO OMEGA", "Omega Ltda", nil)
	invalid, _ := e.pair(t, "2026-08-09", "300.00", "D", "PIX ENVIADO SIGMA", "Sigma Ltda", nil)
	ruled, _ := e.pair(t, "2026-08-16", "600.00", "D", "PIX ENVIADO TAU", "Tau Ltda", nil)
	e.round(t, "2026-08")

	e.gw.answer(suggestion("9.99", nil, 0.9))
	e.srv.Post("/api/v1/reconciliation/review/"+outside+"/classification-suggestion", nil).MustStatus(t, http.StatusAccepted)
	e.process(t)
	var list struct {
		Items []struct {
			ID      string  `json:"id"`
			Account *string `json:"account_code"`
		} `json:"items"`
	}
	e.srv.Get("/api/v1/reconciliation/review/"+outside+"/classification-suggestions").MustStatus(t, http.StatusOK).JSON(t, &list)
	if len(list.Items) != 1 || list.Items[0].Account != nil {
		t.Fatalf("conta fora do plano gravada: %+v", list.Items)
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+outside+"/accept-classification-suggestion", map[string]string{"suggestion_id": list.Items[0].ID}).
		MustStatus(t, http.StatusConflict).ErrorCode(t); code != "SUGGESTION_WITHOUT_ACCOUNT" {
		t.Fatalf("código = %s", code)
	}
	e.srv.Post("/api/v1/reconciliation/review/"+outside+"/reject-classification-suggestion", map[string]string{"suggestion_id": list.Items[0].ID}).
		MustStatus(t, http.StatusOK)
	if c := e.classification(t, outside); c.Account != nil || !c.Pending {
		t.Fatalf("a rejeição mudou a linha: %+v", c)
	}

	e.gw.answer(map[string]any{"justificativa": "x", "confianca": 1.5})
	e.srv.Post("/api/v1/reconciliation/review/"+invalid+"/classification-suggestion", nil).MustStatus(t, http.StatusAccepted)
	e.process(t)
	if n := e.count(t, `SELECT count(*) FROM jobs WHERE kind = 'agent_classifier' AND status = 'dead' AND last_error_code = 'CLASSIFIER_INVALID_OUTPUT'`); n != 1 {
		t.Fatal("saída inválida devia encerrar o job")
	}
	if e.count(t, `SELECT count(*) FROM classification_suggestions WHERE statement_transaction_id = $1`, invalid) != 0 {
		t.Fatal("saída inválida virou sugestão")
	}

	calls := len(e.gw.calls())
	e.srv.Post("/api/v1/reconciliation/review/"+ruled+"/classification-suggestion", nil).MustStatus(t, http.StatusAccepted)
	e.rule(t, "contraparte", "tau ltda", "2.01", http.StatusCreated)
	e.process(t)
	if c := e.classification(t, ruled); len(e.gw.calls()) != calls || !is(c.Account, "2.01") || !is(c.Origin, "regra") {
		t.Fatalf("a regra confirmada devia classificar sem o modelo: %+v", c)
	}

	// Sem plano de contas ativo: 409 no pedido.
	if _, err := e.pool.Exec(ctx, `UPDATE chart_accounts SET active = false WHERE company_id = $1`, e.company); err != nil {
		t.Fatal(err)
	}
	if code := e.srv.Post("/api/v1/reconciliation/review/"+invalid+"/classification-suggestion", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "CHART_EMPTY" {
		t.Fatalf("código = %s", code)
	}
	// Linha não conciliada não é classificada.
	free := e.line(t, "2026-08-28", "13.00", "D", "LINHA LIVRE")
	if code := e.correct(t, free, "2.01", http.StatusConflict); code.Classification.Account != nil {
		t.Fatal("classificou linha livre")
	}
}

// CLASSIFICADOR_AUTOMATICO: a rodada pede sugestão uma só vez para cada linha à espera do agente.
func TestTEST020_AutoClassifierAsksOncePerLine(t *testing.T) {
	e := setup(t, func(c *config.Config) { c.ClassifierAuto = true })
	line, _ := e.pair(t, "2026-08-10", "90.00", "D", "PIX ENVIADO OMEGA", "Omega Ltda", nil)
	if s := e.round(t, "2026-08"); s.ClassifierRequested != 1 {
		t.Fatalf("rodada = %+v", s)
	}
	if s := e.round(t, "2026-08"); s.ClassifierRequested != 0 {
		t.Fatalf("rodada repetida = %+v", s)
	}
	e.gw.answer(suggestion("5.01", nil, 0.5))
	e.process(t)
	if n := e.count(t, `SELECT count(*) FROM classification_suggestions WHERE statement_transaction_id = $1 AND created_by IS NULL`, line); n != 1 {
		t.Fatal("sugestão do automático não gravada")
	}
	if n := e.count(t, `SELECT count(*) FROM ai_runs WHERE agent = 'classificador' AND trigger = 'rodada' AND user_id IS NULL`); n != 1 {
		t.Fatal("corrida do automático não gravada")
	}
	if s := e.round(t, "2026-08"); s.ClassifierRequested != 0 {
		t.Fatalf("depois da sugestão, a rodada pediu de novo: %+v", s)
	}
	if c := e.classification(t, line); c.Account != nil {
		t.Fatal("o automático classificou")
	}
}

// Contábil fechado (AC-025): correção, pedido, aceite e a rodada da classificação respondem 423.
func TestTEST020_AccountingLock(t *testing.T) {
	e := setup(t)
	line, _ := e.pair(t, "2026-08-10", "90.00", "D", "PIX ENVIADO OMEGA", "Omega Ltda", nil)
	e.round(t, "2026-08")
	e.gw.answer(suggestion("5.01", nil, 0.5))
	e.srv.Post("/api/v1/reconciliation/review/"+line+"/classification-suggestion", nil).MustStatus(t, http.StatusAccepted)
	e.process(t)
	sid := e.scalar(t, `SELECT id FROM classification_suggestions WHERE statement_transaction_id = $1`, line)
	comp := e.scalar(t, `INSERT INTO competencies (competence_key, year, month, first_day) VALUES ('2026-08', 2026, 8, '2026-08-01')
		ON CONFLICT (competence_key) DO UPDATE SET competence_key = EXCLUDED.competence_key RETURNING id`)
	if _, err := e.pool.Exec(ctx, `INSERT INTO company_module_periods (company_id, competence_id, module_code, state, closed_at, closed_by)
		VALUES ($1, $2, 'ACCOUNTING', 'CLOSED', now(), $3)`, e.company, comp, user.AdminID); err != nil {
		t.Fatal(err)
	}
	for _, r := range []testutil.Response{
		e.srv.Put("/api/v1/reconciliation/review/"+line+"/classification", map[string]string{"account_code": "5.01"}),
		e.srv.Post("/api/v1/reconciliation/review/"+line+"/classification-suggestion", nil),
		e.srv.Post("/api/v1/reconciliation/review/"+line+"/accept-classification-suggestion", map[string]string{"suggestion_id": sid}),
		e.srv.Post("/api/v1/reconciliation/review/"+line+"/reject-classification-suggestion", map[string]string{"suggestion_id": sid}),
		e.srv.Post("/api/v1/companies/"+e.company+"/classification/run?competence=2026-08", nil),
	} {
		if code := r.MustStatus(t, http.StatusLocked).ErrorCode(t); code != "PERIOD_CLOSED" {
			t.Fatalf("código = %s", code)
		}
	}
	if c := e.classification(t, line); c.Account != nil || !strings.EqualFold(*c.Origin, "agente") {
		t.Fatalf("o Contábil fechado não travou: %+v", c)
	}
}

// O banco garante a coerência da classificação (migration 0029): a origem com o que a produziu, a
// conta do plano da empresa e o autor da classificação humana.
func TestTEST020_SchemaKeepsClassificationCoherent(t *testing.T) {
	e := setup(t)
	line, _ := e.pair(t, "2026-08-10", "90.00", "D", "PIX ENVIADO OMEGA", "Omega Ltda", nil)
	e.round(t, "2026-08")
	for name, sql := range map[string]string{
		"regra sem a regra":       `UPDATE statement_transactions SET account_code = '2.01', classification_origin = 'regra', classification_pending = false WHERE id = $1`,
		"histórico sem a linha":   `UPDATE statement_transactions SET account_code = '2.01', classification_origin = 'historico', classification_pending = false WHERE id = $1`,
		"manual sem autor":        `UPDATE statement_transactions SET account_code = '2.01', classification_origin = 'manual', classification_pending = false WHERE id = $1`,
		"agente pendente c/conta": `UPDATE statement_transactions SET account_code = '2.01', classification_origin = 'agente', classification_pending = true WHERE id = $1`,
		"conta fora do plano": `UPDATE statement_transactions SET account_code = '9.99', classification_origin = 'manual', classification_pending = false,
			classified_by = '` + user.AdminID + `' WHERE id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, line); err == nil {
			t.Errorf("%s: o banco aceitou", name)
		}
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO classification_rules (company_id, rule_type, pattern, account_code) VALUES ($1, 'contraparte', 'x', '9.99')`,
		e.company); err == nil {
		t.Error("regra com conta fora do plano aceita")
	}
}
