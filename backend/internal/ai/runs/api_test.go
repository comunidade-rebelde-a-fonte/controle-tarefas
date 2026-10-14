package runs_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/config"
	"controle/internal/platform/decimal"
	"controle/internal/testutil"
	"controle/internal/testutil/seeded"
	"controle/internal/user"
)

// TEST-009 (API): lista, detalhe, nota humana e painel da auditoria local de IA [06 §9, FR-18],
// contra Postgres real. As corridas são criadas pelo próprio tracer, com o relógio real.

const secret = "sk-lf-segredo-da-api-123"

type fixture struct {
	srv     *testutil.Server
	clk     *clock.Fixed
	tr      *tracer.Tracer
	company string
}

func setup(t *testing.T) fixture {
	t.Helper()
	srv := testutil.NewServer(t, seeded.NewCompaniesDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"), func(c *config.Config) {
		c.LangfusePublicURL = "http://localhost:3100"
		c.LangfuseProjectID = "controle-integrado-dev"
		c.LangfuseSecretKey = secret
	})
	clk := clock.NewFixed(time.Now().UTC().Add(-time.Hour), time.UTC)
	var company string
	if err := srv.Pool.QueryRow(context.Background(), `SELECT id FROM companies WHERE internal_code = '202'`).Scan(&company); err != nil {
		t.Fatal(err)
	}
	return fixture{srv: srv, clk: clk, tr: tracer.New(clk, jobs.NewQueue(clk, 3)), company: company}
}

// run cria uma corrida com status e confiança dados, 100 ms de latência e uma geração.
func (f fixture) run(t *testing.T, agent string, status tracer.Status, conf string, company *string) string {
	t.Helper()
	pa := competence.MustParse("2026-09")
	model := &ai.Resolved{Alias: ai.Consultor, Model: ai.Model{Provider: "openai", Name: "gpt-6-luna"}, Version: 2}
	id, err := f.tr.Do(context.Background(), f.srv.Pool, tracer.Spec{
		Agent: agent, Trigger: "pergunta", CompanyID: company, Competence: &pa, Model: model,
		Input: map[string]any{"pergunta": "qual o Fator R?"}, Metadata: map[string]string{"tabela_vigencia": "2018-01-01"},
	}, func(ctx context.Context, tx pgx.Tx, r *tracer.Run) (tracer.Result, error) {
		err := r.Span(tracer.SpanPlan, nil, func(tracer.SpanHandle) error {
			f.clk.Advance(100 * time.Millisecond)
			return r.Generation(tracer.Generation{Name: "classificar", Model: model.Model, Response: &litellm.Response{
				Usage: litellm.Usage{PromptTokens: 10, CompletionTokens: 4}, CostUSD: "0.0001", Latency: 90 * time.Millisecond,
			}})
		})
		if err != nil {
			return tracer.Result{}, err
		}
		if err := r.RecordDecision(ctx, "resposta", map[string]any{"fator_r": "0.28"}); err != nil {
			return tracer.Result{}, err
		}
		res := tracer.Result{Status: status, Text: "texto"}
		if conf != "" {
			c := decimal.MustParse(conf)
			res.Confidence = &c
		}
		return res, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.clk.Advance(time.Second)
	return id
}

type runOut struct {
	TraceID            string  `json:"trace_id"`
	Agent              string  `json:"agent"`
	Status             string  `json:"status"`
	Confidence         *string `json:"confidence"`
	LatencyMs          *int    `json:"latency_ms"`
	CompanyID          *string `json:"company_id"`
	Competence         *string `json:"competence"`
	Model              *string `json:"model"`
	TokensIn           *int    `json:"tokens_in"`
	CostUSD            *string `json:"cost_usd"`
	LangfuseSyncStatus string  `json:"langfuse_sync_status"`
	HasHumanEval       bool    `json:"has_human_eval"`
}

func TestTEST009_ListFiltersAndPaginates(t *testing.T) {
	f := setup(t)
	a := f.run(t, "consultor", tracer.StatusOK, "0.9", &f.company)
	b := f.run(t, "priorizador", tracer.StatusNeedsReview, "0.4", nil)
	c := f.run(t, "consultor", tracer.StatusOK, "", nil)

	var list struct {
		Items []runOut `json:"items"`
		Total int      `json:"total"`
	}
	f.srv.Get("/api/v1/ai/runs").MustStatus(t, http.StatusOK).JSON(t, &list)
	if list.Total != 3 || len(list.Items) != 3 || list.Items[0].TraceID != c || list.Items[2].TraceID != a {
		t.Fatalf("lista = %+v", list)
	}
	first := list.Items[2]
	if first.Agent != "consultor" || *first.Confidence != "0.9000" || *first.LatencyMs != 100 || *first.CompanyID != f.company ||
		*first.Competence != "2026-09" || *first.Model != "gpt-6-luna" || *first.TokensIn != 10 || *first.CostUSD != "0.00010000" ||
		first.LangfuseSyncStatus != "pending" || first.HasHumanEval {
		t.Fatalf("corrida = %+v", first)
	}
	for q, want := range map[string][]string{
		"?agent=consultor":                           {c, a},
		"?status=needs_review":                       {b},
		"?company_id=" + f.company:                   {a},
		"?agent=consultor&limit=1&offset=1":          {a},
		"?without_human_eval=true&agent=priorizador": {b},
	} {
		var l struct {
			Items []runOut `json:"items"`
		}
		f.srv.Get("/api/v1/ai/runs"+q).MustStatus(t, http.StatusOK).JSON(t, &l)
		var got []string
		for _, it := range l.Items {
			got = append(got, it.TraceID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s = %v, want %v", q, got, want)
		}
	}
	for _, q := range []string{"?status=ruim", "?limit=0", "?limit=500", "?offset=-1", "?company_id=x", "?agent=Consultor", "?without_human_eval=talvez"} {
		if code := f.srv.Get("/api/v1/ai/runs"+q).MustStatus(t, http.StatusBadRequest).ErrorCode(t); code != "VALIDATION_ERROR" {
			t.Errorf("%s: %s", q, code)
		}
	}
}

func TestTEST009_DetailAndHumanEvaluation(t *testing.T) {
	f := setup(t)
	id := f.run(t, "consultor", tracer.StatusNeedsReview, "0.35", &f.company)

	resp := f.srv.Get("/api/v1/ai/runs/"+id).MustStatus(t, http.StatusOK)
	if strings.Contains(string(resp.Body), secret) {
		t.Fatal("o detalhe vaza a chave do Langfuse")
	}
	var d struct {
		runOut
		ErrorCode   *string           `json:"error_code"`
		Metadata    map[string]string `json:"metadata"`
		Input       map[string]any    `json:"input"`
		Output      map[string]any    `json:"output"`
		Spans       []map[string]any  `json:"spans"`
		Decisions   []map[string]any  `json:"decisions"`
		GoldEvals   []map[string]any  `json:"gold_evals"`
		HumanEvals  []map[string]any  `json:"human_evals"`
		LangfuseURL *string           `json:"langfuse_url"`
	}
	resp.JSON(t, &d)
	if d.TraceID != id || d.Status != "needs_review" || d.Metadata["tabela_vigencia"] != "2018-01-01" || d.Input["pergunta"] != "qual o Fator R?" ||
		d.Output["texto"] != "texto" || len(d.Spans) != 2 || d.Spans[1]["type"] != "generation" || len(d.Decisions) != 1 ||
		d.Decisions[0]["kind"] != "resposta" || len(d.GoldEvals) != 0 || len(d.HumanEvals) != 0 {
		t.Fatalf("detalhe = %+v", d)
	}
	if d.LangfuseURL == nil || *d.LangfuseURL != "http://localhost:3100/project/controle-integrado-dev/traces/"+id {
		t.Fatalf("link do Langfuse = %v", d.LangfuseURL)
	}
	for _, bad := range []string{"xyz", strings.Repeat("A", 32), strings.Repeat("0", 32)} {
		if code := f.srv.Get("/api/v1/ai/runs/"+bad).MustStatus(t, http.StatusNotFound).ErrorCode(t); code != "AI_RUN_NOT_FOUND" {
			t.Errorf("%s: %s", bad, code)
		}
	}

	// Nota humana: erro exige comentário (422); fora do vocabulário, 400; campo extra, 400.
	evalPath := "/api/v1/ai/runs/" + id + "/evaluation"
	if code := f.srv.Post(evalPath, map[string]any{"rating": "erro", "comment": "  "}).MustStatus(t, http.StatusUnprocessableEntity).ErrorCode(t); code != "EVAL_COMMENT_REQUIRED" {
		t.Fatalf("erro sem comentário: %s", code)
	}
	f.srv.Post(evalPath, map[string]any{"rating": "otimo"}).MustStatus(t, http.StatusBadRequest)
	f.srv.Post(evalPath, map[string]any{"rating": "acerto", "nota": "x"}).MustStatus(t, http.StatusBadRequest)
	f.srv.Post("/api/v1/ai/runs/"+strings.Repeat("0", 32)+"/evaluation", map[string]any{"rating": "acerto"}).MustStatus(t, http.StatusNotFound)

	var ev struct {
		Rating  string  `json:"rating"`
		Comment *string `json:"comment"`
		UserID  string  `json:"user_id"`
	}
	f.srv.Post(evalPath, map[string]any{"rating": "erro", "comment": " confundiu o PA "}).MustStatus(t, http.StatusCreated).JSON(t, &ev)
	if ev.Rating != "erro" || *ev.Comment != "confundiu o PA" || ev.UserID != user.AdminID {
		t.Fatalf("nota = %+v", ev)
	}
	f.srv.Get("/api/v1/ai/runs/"+id).MustStatus(t, http.StatusOK).JSON(t, &d)
	if !d.HasHumanEval || len(d.HumanEvals) != 1 {
		t.Fatalf("detalhe após a nota = %+v", d.HumanEvals)
	}
	var jobsN int
	if err := f.srv.Pool.QueryRow(context.Background(), `SELECT count(*) FROM jobs WHERE kind = 'langfuse_sync' AND dedupe_key = $1 AND status = 'pending'`, id).Scan(&jobsN); err != nil || jobsN != 1 {
		t.Fatalf("a nota deve entrar na fila do Langfuse (job pendente único): %d, %v", jobsN, err)
	}
}

func TestTEST009_SummaryPanel(t *testing.T) {
	f := setup(t)
	var empty struct {
		RunsTotal     int     `json:"runs_total"`
		GoldAccuracy  *string `json:"gold_accuracy"`
		HumanAccuracy *string `json:"human_accuracy"`
		LLMCost30d    *string `json:"llm_cost_30d"`
		LLMTokens30d  *int    `json:"llm_tokens_30d"`
		ByAgent       []any   `json:"by_agent"`
	}
	f.srv.Get("/api/v1/ai/summary").MustStatus(t, http.StatusOK).JSON(t, &empty)
	if empty.RunsTotal != 0 || empty.GoldAccuracy != nil || empty.HumanAccuracy != nil || empty.LLMCost30d != nil || empty.LLMTokens30d != nil || len(empty.ByAgent) != 0 {
		t.Fatalf("painel vazio = %+v", empty)
	}

	a := f.run(t, "consultor", tracer.StatusOK, "0.9", nil)
	f.run(t, "consultor", tracer.StatusNeedsReview, "0.5", nil)
	p := f.run(t, "parser_pgdas", tracer.StatusOK, "", nil)
	ctx := context.Background()
	s := func(v string) *string { return &v }
	if err := pgx.BeginFunc(ctx, f.srv.Pool, func(tx pgx.Tx) error {
		if _, err := f.tr.RecordGold(ctx, tx, p, []tracer.GoldResult{
			{Field: "rbt12", Status: tracer.GoldOK}, {Field: "fs12", Status: tracer.GoldOK},
			{Field: "anexo", Status: tracer.GoldError}, {Field: "fator_r", Status: tracer.GoldUnavailable},
		}); err != nil {
			return err
		}
		if _, err := f.tr.RecordHuman(ctx, tx, a, user.AdminID, tracer.RatingCorrect, nil); err != nil {
			return err
		}
		_, err := f.tr.RecordHuman(ctx, tx, p, user.AdminID, tracer.RatingPartial, s("faltou um mês"))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var sum struct {
		Runs24h          int     `json:"runs_24h"`
		RunsTotal        int     `json:"runs_total"`
		PendingHumanEval int     `json:"pending_human_eval"`
		GoldAccuracy     *string `json:"gold_accuracy"`
		HumanAccuracy    *string `json:"human_accuracy"`
		LLMCost30d       *string `json:"llm_cost_30d"`
		LLMTokens30d     *int    `json:"llm_tokens_30d"`
		ByAgent          []struct {
			Agent         string  `json:"agent"`
			Runs          int     `json:"runs"`
			AvgLatencyMs  *int    `json:"avg_latency_ms"`
			AvgConfidence *string `json:"avg_confidence"`
			Errors        int     `json:"errors"`
			Reviews       int     `json:"reviews"`
		} `json:"by_agent"`
	}
	f.srv.Get("/api/v1/ai/summary").MustStatus(t, http.StatusOK).JSON(t, &sum)
	if sum.Runs24h != 3 || sum.RunsTotal != 3 || sum.PendingHumanEval != 1 {
		t.Fatalf("contagens = %+v", sum)
	}
	if *sum.GoldAccuracy != "0.6666666666666666666666666667" || *sum.HumanAccuracy != "0.5" {
		t.Fatalf("acertos = %v / %v", *sum.GoldAccuracy, *sum.HumanAccuracy)
	}
	if *sum.LLMCost30d != "0.00030000" || *sum.LLMTokens30d != 42 {
		t.Fatalf("custo e tokens = %v / %v", *sum.LLMCost30d, *sum.LLMTokens30d)
	}
	if len(sum.ByAgent) != 2 {
		t.Fatalf("por agente = %+v", sum.ByAgent)
	}
	cons, pars := sum.ByAgent[0], sum.ByAgent[1]
	if cons.Agent != "consultor" || cons.Runs != 2 || *cons.AvgLatencyMs != 100 || *cons.AvgConfidence != "0.7000" || cons.Reviews != 1 || cons.Errors != 0 {
		t.Fatalf("consultor = %+v", cons)
	}
	if pars.Agent != "parser_pgdas" || pars.AvgConfidence != nil {
		t.Fatalf("parser_pgdas = %+v", pars)
	}
}
