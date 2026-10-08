package tracer_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai"
	"controle/internal/ai/langfuse"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/store"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

// TEST-009: trilha local das corridas de agente e sincronização com o Langfuse, contra Postgres
// real e um Langfuse simulado. Porta os casos de tests/agents/test_tracer.py do Fator R.

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type env struct {
	pool  *pgxpool.Pool
	clk   *clock.Fixed
	queue *jobs.Queue
	tr    *tracer.Tracer
}

func setup(t *testing.T, pool *pgxpool.Pool) env {
	t.Helper()
	clk := clock.NewFixed(time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC), time.UTC)
	q := jobs.NewQueue(clk, 3)
	return env{pool: pool, clk: clk, queue: q, tr: tracer.New(clk, q)}
}

func getRun(t *testing.T, e env, id string) store.AiRun {
	t.Helper()
	r, err := store.New(e.pool).GetAIRun(context.Background(), id)
	if err != nil {
		t.Fatalf("corrida %s: %v", id, err)
	}
	return r
}

func count(t *testing.T, e env, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func syncJobs(t *testing.T, e env, traceID string) int {
	return count(t, e, `SELECT count(*) FROM jobs WHERE kind = 'langfuse_sync' AND dedupe_key = $1`, traceID)
}

func jsonOf(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// A corrida e a decisão nascem na mesma transação; o resultado fica gravado e a sincronização
// com o Langfuse entra na fila junto [AC-029].
func TestTEST009_RunRecordsTraceAndDecisionInOneTransaction(t *testing.T) {
	e := setup(t, testutil.NewDB(t))
	ctx := context.Background()
	admin := user.AdminID
	pa := competence.MustParse("2026-09")
	resolved := &ai.Resolved{Alias: ai.Consultor, Model: ai.Model{Provider: "openai", Name: "gpt-6-luna"}, Source: ai.SourceSettings, Version: 3}
	conf := decimal.MustParse("0.875")

	id, err := e.tr.Do(ctx, e.pool, tracer.Spec{
		Agent: "consultor", Trigger: "pergunta", UserID: &admin, Competence: &pa, Model: resolved, PromptVersion: "v2",
		Input:    map[string]any{"pergunta": "Qual o Fator R?", "texto_extraido": "PDF inteiro", "chave": "sk-proj-abcdef123456"},
		Metadata: map[string]string{"politica_cpp": "integra_fs12"},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		err := run.Span(tracer.SpanPlan, map[string]any{"etapa": 1}, func(s tracer.SpanHandle) error {
			e.clk.Advance(120 * time.Millisecond)
			return run.Generation(tracer.Generation{Name: "classificar", Model: resolved.Model, Response: &litellm.Response{
				Model: "gpt-6-luna-2026-06-01", Usage: litellm.Usage{PromptTokens: 12, CompletionTokens: 5}, CostUSD: "1.25e-05", Latency: 100 * time.Millisecond,
			}})
		})
		if err != nil {
			return tracer.Result{}, err
		}
		if err := run.Span(tracer.SpanTool, nil, func(s tracer.SpanHandle) error {
			s.Output(map[string]any{"fator_r": decimal.MustParse("0.2800")})
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		if err := run.RecordDecision(ctx, "resposta", map[string]any{"valor": decimal.MustParse("1.50")}); err != nil {
			return tracer.Result{}, err
		}
		e.clk.Advance(30 * time.Millisecond)
		return tracer.Result{Status: tracer.StatusOK, Text: "feito", Decision: map[string]any{"ok": true}, Confidence: &conf}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tracer.ValidTraceID(id) {
		t.Fatalf("trace_id = %q", id)
	}
	r := getRun(t, e, id)
	if r.Status != "ok" || r.ErrorCode != nil || r.LangfuseSyncStatus != "pending" || r.LatencyMs == nil || *r.LatencyMs != 150 {
		t.Fatalf("corrida = %+v", r)
	}
	if *r.Alias != "consultor" || *r.Provider != "openai" || *r.Model != "gpt-6-luna" || *r.ModelConfigVersion != 3 || *r.PromptVersion != "v2" ||
		r.Competence.Format("2006-01") != "2026-09" || *r.UserID != admin {
		t.Fatalf("snapshot do modelo e vínculos = %+v", r)
	}
	if r.Confidence.Text() != "0.8750" || *r.TokensIn != 12 || *r.TokensOut != 5 || r.CostUsd.Text() != "0.00001250" {
		t.Fatalf("confiança e consumo = %v %v %v %v", r.Confidence, *r.TokensIn, *r.TokensOut, r.CostUsd)
	}
	in := jsonOf(t, r.Input)
	if in["texto_extraido"] != "[removido]" || strings.Contains(string(r.Input), "sk-proj") || in["pergunta"] != "Qual o Fator R?" {
		t.Fatalf("entrada não sanitizada: %s", r.Input)
	}
	if out := jsonOf(t, r.Output); out["texto"] != "feito" {
		t.Fatalf("saída = %s", r.Output)
	}
	var spans []tracer.SpanRecord
	if err := json.Unmarshal(r.Spans, &spans); err != nil || len(spans) != 3 {
		t.Fatalf("spans = %s (%v)", r.Spans, err)
	}
	plan, gen, tool := spans[0], spans[1], spans[2]
	if plan.Name != "plan" || plan.Type != "chain" || plan.ParentID != "" || gen.ParentID != plan.ID || gen.Type != "generation" ||
		gen.Model != "openai/gpt-6-luna" || *gen.TokensIn != 12 || gen.CostUSD != "1.25e-05" || tool.Type != "tool" || tool.ParentID != "" {
		t.Fatalf("spans = %+v", spans)
	}
	if n := count(t, e, `SELECT count(*) FROM ai_decisions WHERE trace_id = $1 AND kind = 'resposta' AND data->>'valor' = '1.50'`, id); n != 1 {
		t.Fatalf("decisões = %d", n)
	}
	if syncJobs(t, e, id) != 1 {
		t.Fatal("a sincronização com o Langfuse deveria entrar na fila junto com a corrida")
	}
}

// Erro no agente: decisões e efeitos de domínio desfeitos; a corrida de erro fica, com o código
// do erro e não a mensagem.
func TestTEST009_ErrorRollsBackDecisionsAndKeepsErrorTrace(t *testing.T) {
	e := setup(t, testutil.NewDB(t))
	ctx := context.Background()
	id, err := e.tr.Do(ctx, e.pool, tracer.Spec{Agent: "consultor", Trigger: "pergunta"}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('efeito_do_agente', '{}')`); err != nil {
			return tracer.Result{}, err
		}
		if err := run.RecordDecision(ctx, "t", map[string]any{}); err != nil {
			return tracer.Result{}, err
		}
		return tracer.Result{}, run.Span(tracer.SpanRender, nil, func(tracer.SpanHandle) error {
			return &litellm.Error{Status: 503, Message: "provedor fora: CNPJ 11.222.333/0001-81"}
		})
	})
	var gw *litellm.Error
	if !errors.As(err, &gw) || id == "" {
		t.Fatalf("err = %v, id = %q", err, id)
	}
	r := getRun(t, e, id)
	if r.Status != "error" || *r.ErrorCode != "LLM_UNAVAILABLE" || strings.Contains(string(r.Output), "CNPJ") {
		t.Fatalf("corrida de erro = %+v / %s", r, r.Output)
	}
	if out := jsonOf(t, r.Output); out["erro"] != "LLM_UNAVAILABLE" {
		t.Fatalf("saída = %s", r.Output)
	}
	var spans []tracer.SpanRecord
	_ = json.Unmarshal(r.Spans, &spans)
	if len(spans) != 1 || spans[0].Level != "ERROR" || spans[0].StatusMessage != "LLM_UNAVAILABLE" {
		t.Fatalf("o span que falhou fica na trilha: %+v", spans)
	}
	if count(t, e, `SELECT count(*) FROM ai_decisions`) != 0 || count(t, e, `SELECT count(*) FROM settings WHERE key = 'efeito_do_agente'`) != 0 {
		t.Fatal("decisão ou efeito de corrida com erro ficou gravado")
	}
	if syncJobs(t, e, id) != 1 {
		t.Fatal("a corrida de erro também vai para o Langfuse")
	}
	// Código do domínio vira o código da corrida.
	id2, _ := e.tr.Do(ctx, e.pool, tracer.Spec{Agent: "consultor", Trigger: "pergunta"}, func(context.Context, pgx.Tx, *tracer.Run) (tracer.Result, error) {
		return tracer.Result{}, apperr.Unprocessable("SIMPLES_TABLE_NOT_IN_FORCE", "sem tabela")
	})
	if r := getRun(t, e, id2); *r.ErrorCode != "SIMPLES_TABLE_NOT_IN_FORCE" {
		t.Fatalf("código = %v", *r.ErrorCode)
	}
}

// Corrida aberta sem Finish fica como erro (NOT_FINISHED), como "tracer.run terminou sem
// finish()" na origem; spans só com os nomes do contrato; decisão só dentro de corrida aberta.
func TestTEST009_ContractGuards(t *testing.T) {
	e := setup(t, testutil.NewDB(t))
	ctx := context.Background()
	var id string
	err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		run, err := e.tr.Begin(ctx, tx, tracer.Spec{Agent: "parser_pgdas", Trigger: "upload"})
		if err != nil {
			return err
		}
		id = run.TraceID()
		if err := run.Span("qualquer", nil, func(tracer.SpanHandle) error { return nil }); !errors.Is(err, tracer.ErrSpanNotAllowed) {
			t.Errorf("span fora do contrato: err = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if r := getRun(t, e, id); r.Status != "error" || *r.ErrorCode != "NOT_FINISHED" {
		t.Fatalf("corrida sem Finish = %s/%v", r.Status, r.ErrorCode)
	}
	var nilRun *tracer.Run
	if err := nilRun.RecordDecision(ctx, "t", nil); !errors.Is(err, tracer.ErrDecisionWithoutRun) {
		t.Fatalf("decisão sem corrida: err = %v", err)
	}
	_, err = e.tr.Do(ctx, e.pool, tracer.Spec{Agent: "consultor", Trigger: "pergunta"}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		if err := run.Finish(ctx, tracer.Result{Status: tracer.StatusOK}); err != nil {
			return tracer.Result{}, err
		}
		if err := run.RecordDecision(ctx, "depois", nil); !errors.Is(err, tracer.ErrDecisionWithoutRun) {
			t.Errorf("decisão depois do fim: err = %v", err)
		}
		return tracer.Result{}, errors.New("fim")
	})
	if err == nil {
		t.Fatal("esperava o erro devolvido por fn")
	}
	// O banco também recusa decisão sem corrida e nota "erro" sem comentário.
	for _, stmt := range []string{
		`INSERT INTO ai_decisions (trace_id, agent, kind, data, created_at) VALUES (NULL, 'x', 'y', '{}', now())`,
		`INSERT INTO ai_decisions (trace_id, agent, kind, data, created_at) VALUES ('` + strings.Repeat("0", 32) + `', 'x', 'y', '{}', now())`,
		`INSERT INTO ai_human_evals (trace_id, user_id, rating, comment, created_at) VALUES ('` + id + `', '` + user.AdminID + `', 'erro', '  ', now())`,
		`INSERT INTO ai_runs (trace_id, agent, trigger, status, created_at, updated_at) VALUES ('XYZ', 'a1', 'b1', 'ok', now(), now())`,
	} {
		if _, err := e.pool.Exec(ctx, stmt); err == nil {
			t.Errorf("o banco aceitou: %s", stmt)
		}
	}
}

// Num job do worker: a tentativa que falha é desfeita, e a corrida de erro é gravada depois do
// rollback junto com a falha do job (jobs.AfterRollback).
func TestTEST009_JobAttemptKeepsErrorTraceAfterRollback(t *testing.T) {
	e := setup(t, testutil.NewDB(t))
	ctx := context.Background()
	var traceID string
	runner := jobs.NewRunner(e.pool, e.clk, quiet, jobs.RunnerConfig{WorkerID: "w1", Concurrency: 1, PollInterval: time.Millisecond, Lease: time.Minute},
		map[string]jobs.Handler{"agente_teste": func(ctx context.Context, tx pgx.Tx, j jobs.Claimed) error {
			run, err := e.tr.Begin(ctx, tx, tracer.Spec{Agent: "triador", Trigger: "upload"})
			if err != nil {
				return err
			}
			traceID = run.TraceID()
			if err := run.RecordDecision(ctx, "triagem", map[string]any{"tipo": "extrato"}); err != nil {
				return err
			}
			return run.Fail(jobs.Coded("LLM_UNAVAILABLE", errors.New("gateway fora")))
		}})
	j, _, err := e.queue.Enqueue(ctx, e.pool, jobs.Spec{Kind: "agente_teste"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	r := getRun(t, e, traceID)
	if r.Status != "error" || *r.ErrorCode != "LLM_UNAVAILABLE" || count(t, e, `SELECT count(*) FROM ai_decisions`) != 0 {
		t.Fatalf("corrida = %s/%v", r.Status, r.ErrorCode)
	}
	job, _ := store.New(e.pool).GetJob(ctx, j.ID)
	if job.Status != jobs.StatusRetry || syncJobs(t, e, traceID) != 1 {
		t.Fatalf("job = %s, sync jobs = %d", job.Status, syncJobs(t, e, traceID))
	}
}

// Notas: a ouro devolve o acerto como gold() da origem; a humana exige comentário no erro.
func TestTEST009_GoldAndHumanEvals(t *testing.T) {
	e := setup(t, testutil.NewDB(t))
	ctx := context.Background()
	id, err := e.tr.Do(ctx, e.pool, tracer.Spec{Agent: "parser_pgdas", Trigger: "upload"}, func(context.Context, pgx.Tx, *tracer.Run) (tracer.Result, error) {
		return tracer.Result{Status: tracer.StatusOK}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s := func(v string) *string { return &v }
	yes, no := true, false
	var acerto *decimal.Decimal
	err = pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		acerto, err = e.tr.RecordGold(ctx, tx, id, []tracer.GoldResult{
			{Field: "rbt12", Expected: s("600000.00"), Obtained: s("600000.00"), WithinTolerance: &yes, Status: tracer.GoldOK},
			{Field: "fs12", Expected: s("120000.00"), Obtained: s("100000.00"), WithinTolerance: &no, Status: tracer.GoldError},
			{Field: "anexo", Expected: s("III"), Obtained: s("III"), WithinTolerance: &yes, Status: tracer.GoldOK},
			{Field: "fator_r", Status: tracer.GoldUnavailable},
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if acerto == nil || acerto.Text() != "0.6666666666666666666666666667" {
		t.Fatalf("acerto = %v (Decimal(2)/Decimal(3) do Python)", acerto)
	}
	err = pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		none, err := e.tr.RecordGold(ctx, tx, id, []tracer.GoldResult{{Field: "x", Status: tracer.GoldUnavailable}})
		if none != nil {
			t.Errorf("sem campo comparável, o acerto é nulo: %v", none)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	human := func(rating string, comment *string) error {
		return pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
			_, err := e.tr.RecordHuman(ctx, tx, id, user.AdminID, rating, comment)
			return err
		})
	}
	var ae *apperr.Error
	if err := human("erro", s("   ")); !errors.As(err, &ae) || ae.Code != "EVAL_COMMENT_REQUIRED" || ae.Status != http.StatusUnprocessableEntity {
		t.Fatalf("erro sem comentário: %v", err)
	}
	if err := human("otimo", nil); !errors.As(err, &ae) || ae.Code != "VALIDATION_ERROR" {
		t.Fatalf("nota inválida: %v", err)
	}
	if err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		_, err := e.tr.RecordHuman(ctx, tx, strings.Repeat("a", 32), user.AdminID, "acerto", nil)
		return err
	}); !errors.As(err, &ae) || ae.Code != "AI_RUN_NOT_FOUND" {
		t.Fatalf("trace inexistente: %v", err)
	}
	if err := human("erro", s("faltou o mês de agosto")); err != nil {
		t.Fatal(err)
	}
	if n := count(t, e, `SELECT count(*) FROM ai_human_evals WHERE trace_id = $1 AND rating = 'erro' AND comment = 'faltou o mês de agosto'`, id); n != 1 {
		t.Fatalf("nota humana = %d", n)
	}
}

// fakeLangfuse imita as rotas do Langfuse usadas no envio (OTLP e scores).
type fakeLangfuse struct {
	mu       sync.Mutex
	down     bool
	traces   []map[string]any
	scores   []map[string]any
	requests int
}

func (f *fakeLangfuse) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"message":"indisponível; chave sk-lf-segredo-do-teste"}`)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	switch r.URL.Path {
	case "/api/public/otel/v1/traces":
		f.traces = append(f.traces, body)
	case "/api/public/scores":
		f.scores = append(f.scores, body)
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_, _ = io.WriteString(w, `{}`)
}

func (f *fakeLangfuse) set(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

func (f *fakeLangfuse) snapshot() (traces, scores []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.traces...), append([]map[string]any(nil), f.scores...)
}

func syncRunner(e env, s *tracer.Syncer) *jobs.Runner {
	return jobs.NewRunner(e.pool, e.clk, quiet, jobs.RunnerConfig{WorkerID: "w-sync", Concurrency: 1, PollInterval: time.Millisecond, Lease: time.Minute},
		map[string]jobs.Handler{tracer.KindLangfuseSync: s.Handle})
}

func drain(t *testing.T, r *jobs.Runner) {
	t.Helper()
	for {
		ok, err := r.ProcessOne(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return
		}
	}
}

// AC-030: com o Langfuse fora, o domínio não falha; a corrida fica failed (erro sanitizado) e o
// reenvio periódico manda tudo quando ele volta — com os mesmos IDs, sem duplicar.
func TestTEST009_LangfuseOutageIsRecordedAndRetried(t *testing.T) {
	e := setup(t, seeded.NewCompaniesDB(t))
	ctx := context.Background()
	fake := &fakeLangfuse{down: true}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	client := langfuse.New(langfuse.Config{Host: srv.URL, PublicKey: "pk-lf-teste", SecretKey: "sk-lf-segredo-do-teste", Environment: "test"})
	syncer := tracer.NewSyncer(client, e.clk, e.queue, quiet)
	runner := syncRunner(e, syncer)

	var companyID string
	if err := e.pool.QueryRow(ctx, `SELECT id FROM companies WHERE internal_code = '202'`).Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	id, err := e.tr.Do(ctx, e.pool, tracer.Spec{Agent: "consultor", Trigger: "pergunta", CompanyID: &companyID,
		Input: map[string]any{"pergunta": "Fator R da 11.222.333/0001-81?"}},
		func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
			return tracer.Result{Status: tracer.StatusNeedsReview, Text: "revise"}, nil
		})
	if err != nil {
		t.Fatalf("o domínio não pode falhar por causa do Langfuse: %v", err)
	}
	if err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		c := "faltou"
		if _, err := e.tr.RecordHuman(ctx, tx, id, user.AdminID, "erro", &c); err != nil {
			return err
		}
		_, err := e.tr.RecordGold(ctx, tx, id, []tracer.GoldResult{{Field: "rbt12", Status: tracer.GoldOK}, {Field: "fs12", Status: tracer.GoldError}})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	drain(t, runner)
	r := getRun(t, e, id)
	if r.LangfuseSyncStatus != "failed" || r.LangfuseAttempts != 1 || r.LangfuseLastError == nil ||
		!strings.Contains(*r.LangfuseLastError, "503") || strings.Contains(*r.LangfuseLastError, "segredo") {
		t.Fatalf("corrida após falha = %s/%d/%v", r.LangfuseSyncStatus, r.LangfuseAttempts, r.LangfuseLastError)
	}
	if n := count(t, e, `SELECT count(*) FROM jobs WHERE kind = 'langfuse_sync' AND status = 'done'`); n != 1 {
		t.Fatalf("o job termina (a falha fica na corrida, não na fila): %d", n)
	}

	// Reenvio periódico: corrida e notas com falha (ou pendentes há tempo) voltam para a fila.
	fake.set(false)
	e.clk.Advance(10 * time.Minute)
	if n, err := syncer.Requeue(ctx, e.pool, 5*time.Minute, 50); err != nil || n != 1 {
		t.Fatalf("Requeue = %d, %v", n, err)
	}
	drain(t, runner)
	r = getRun(t, e, id)
	if r.LangfuseSyncStatus != "ok" || r.LangfuseSyncedAt == nil || r.LangfuseLastError != nil || r.LangfuseAttempts != 2 {
		t.Fatalf("corrida após o reenvio = %s/%v/%d", r.LangfuseSyncStatus, r.LangfuseLastError, r.LangfuseAttempts)
	}
	if n := count(t, e, `SELECT count(*) FROM ai_gold_evals WHERE langfuse_sync_status <> 'ok'`) + count(t, e, `SELECT count(*) FROM ai_human_evals WHERE langfuse_sync_status <> 'ok'`); n != 0 {
		t.Fatalf("%d notas não sincronizadas", n)
	}
	traces, scores := fake.snapshot()
	if len(traces) != 1 {
		t.Fatalf("%d envios de trace", len(traces))
	}
	spans := traces[0]["resourceSpans"].([]any)[0].(map[string]any)["scopeSpans"].([]any)[0].(map[string]any)["spans"].([]any)
	root := spans[0].(map[string]any)
	if root["traceId"] != id || root["spanId"] != tracer.RootSpanID(id) {
		t.Fatalf("raiz = %v", root)
	}
	rootJSON, _ := json.Marshal(root)
	if !strings.Contains(string(rootJSON), "***0181") || strings.Contains(string(rootJSON), "11.222.333") || !strings.Contains(string(rootJSON), companyID) {
		t.Fatalf("o CNPJ da pergunta deveria ir mascarado (e a empresa pelo ID): %s", rootJSON)
	}
	names := map[string]map[string]any{}
	for _, s := range scores {
		names[s["name"].(string)] = s
	}
	if len(scores) != 4 || names["human_eval"]["value"] != "erro" || names["human_eval"]["dataType"] != "CATEGORICAL" ||
		names["gold_rbt12"]["value"] != 1.0 || names["gold_fs12"]["value"] != 0.0 || names["gold_rbt12"]["dataType"] != "BOOLEAN" ||
		names["gold_acerto"]["value"] != 0.5 || names["gold_acerto"]["dataType"] != "NUMERIC" {
		t.Fatalf("scores = %v", scores)
	}

	// Nada pendente: o reenvio periódico não enfileira nada.
	e.clk.Advance(10 * time.Minute)
	if n, _ := syncer.Requeue(ctx, e.pool, 5*time.Minute, 50); n != 0 {
		t.Fatalf("reenvio sem pendência: %d", n)
	}
}

// Sem Langfuse configurado (ex.: testes, ambiente sem a stack), o job não faz nada e a corrida
// fica pending; o reenvio periódico não roda.
func TestTEST009_WithoutLangfuseRunsStayPending(t *testing.T) {
	e := setup(t, testutil.NewDB(t))
	ctx := context.Background()
	syncer := tracer.NewSyncer(langfuse.New(langfuse.Config{}), e.clk, e.queue, quiet)
	id, err := e.tr.Do(ctx, e.pool, tracer.Spec{Agent: "consultor", Trigger: "pergunta"}, func(context.Context, pgx.Tx, *tracer.Run) (tracer.Result, error) {
		return tracer.Result{Status: tracer.StatusOK}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, syncRunner(e, syncer))
	if r := getRun(t, e, id); r.LangfuseSyncStatus != "pending" {
		t.Fatalf("status = %s", r.LangfuseSyncStatus)
	}
	e.clk.Advance(time.Hour)
	if n, err := syncer.Requeue(ctx, e.pool, time.Minute, 50); err != nil || n != 0 {
		t.Fatalf("Requeue sem Langfuse = %d, %v", n, err)
	}
}
