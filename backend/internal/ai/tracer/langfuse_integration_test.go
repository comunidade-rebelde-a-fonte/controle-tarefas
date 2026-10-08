//go:build langfuse

// Integração com o Langfuse local real (docker-compose.langfuse.yml): envia uma corrida pelo job
// de sincronização e confere, pela API pública, que o trace chegou com o mesmo trace_id, os spans
// e os scores do contrato da origem, e que o reenvio não duplica nada [eval-plan §6, AC-031].
// Exige o Langfuse no ar (make langfuse-up) e as chaves do .env:
//
//	make test-langfuse
package tracer_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai"
	"controle/internal/ai/langfuse"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/platform/config"
	"controle/internal/platform/decimal"
	"controle/internal/store"
	"controle/internal/testutil"
	"controle/internal/user"
)

type lfAPI struct {
	host, auth string
}

func (a lfAPI) get(t *testing.T, path string, q url.Values) map[string]any {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, a.host+path+"?"+q.Encode(), nil)
	req.Header.Set("Authorization", a.auth)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d: %.200s", path, resp.StatusCode, b)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// wait consulta até cond aceitar a lista (o Langfuse ingere de forma assíncrona).
func (a lfAPI) wait(t *testing.T, path string, q url.Values, cond func([]any) bool) []any {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		data, _ := a.get(t, path, q)["data"].([]any)
		if cond(data) {
			return data
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: o Langfuse não mostrou o esperado em 90 s; último resultado: %v", path, data)
		}
		time.Sleep(2 * time.Second)
	}
}

func TestLangfuse_SyncRealTraceAndScores(t *testing.T) {
	host := os.Getenv("LANGFUSE_IT_HOST")
	if host == "" {
		host = "http://127.0.0.1:" + valueOr(os.Getenv("LANGFUSE_WEB_PORT"), "3100")
	}
	pk, sk := os.Getenv("LANGFUSE_PUBLIC_KEY"), os.Getenv("LANGFUSE_SECRET_KEY")
	if pk == "" || sk == "" {
		t.Fatal("LANGFUSE_PUBLIC_KEY e LANGFUSE_SECRET_KEY são obrigatórias (o make test-langfuse as lê do .env)")
	}
	api := lfAPI{host: host, auth: "Basic " + base64.StdEncoding.EncodeToString([]byte(pk+":"+sk))}
	api.get(t, "/api/public/health", nil)

	e := setup(t, testutil.NewDB(t))
	ctx := context.Background()
	client := langfuse.New(langfuse.Config{Host: host, PublicKey: pk, SecretKey: config.Secret(sk), Environment: "test", Timeout: 10 * time.Second})
	syncer := tracer.NewSyncer(client, e.clk, e.queue, quiet)
	runner := syncRunner(e, syncer)

	model := &ai.Resolved{Alias: ai.Consultor, Model: ai.Model{Provider: "openai", Name: "gpt-6-luna"}, Version: 1}
	conf := decimal.MustParse("0.42")
	id, err := e.tr.Do(ctx, e.pool, tracer.Spec{
		Agent: "consultor", Trigger: "pergunta", Model: model, Metadata: map[string]string{"politica_cpp": "integra_fs12"},
		Input: map[string]any{"pergunta": "teste de integração com CNPJ 11.222.333/0001-81", "texto_extraido": "não pode sair"},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		if err := run.Span(tracer.SpanPlan, map[string]any{"intencao": "status_empresa"}, func(s tracer.SpanHandle) error {
			e.clk.Advance(200 * time.Millisecond)
			return run.Generation(tracer.Generation{Name: "classificar", Model: model.Model, Response: &litellm.Response{
				Model: "gpt-6-luna", Usage: litellm.Usage{PromptTokens: 21, CompletionTokens: 7}, CostUSD: "0.0000125", Latency: 180 * time.Millisecond,
			}})
		}); err != nil {
			return tracer.Result{}, err
		}
		if err := run.Span(tracer.SpanRender, nil, func(s tracer.SpanHandle) error {
			s.Warn("baixa confiança")
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		return tracer.Result{Status: tracer.StatusNeedsReview, Text: "revise", Confidence: &conf}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		c := "comentário com CPF 123.456.789-09"
		if _, err := e.tr.RecordHuman(ctx, tx, id, user.AdminID, tracer.RatingWrong, &c); err != nil {
			return err
		}
		_, err := e.tr.RecordGold(ctx, tx, id, []tracer.GoldResult{{Field: "rbt12", Status: tracer.GoldOK}, {Field: "fs12", Status: tracer.GoldError}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	drain(t, runner)
	if r := getRun(t, e, id); r.LangfuseSyncStatus != "ok" {
		t.Fatalf("sincronização = %s (%v)", r.LangfuseSyncStatus, r.LangfuseLastError)
	}

	obs := api.wait(t, "/api/public/v2/observations", url.Values{
		"traceId": {id}, "fields": {"basic,io,usage,metrics,trace_context,metadata,model"}, "limit": {"50"},
	}, func(d []any) bool { return len(d) >= 4 })
	byName := map[string]map[string]any{}
	for _, o := range obs {
		m := o.(map[string]any)
		byName[m["name"].(string)] = m
	}
	root := byName["consultor"]
	if root == nil || root["type"] != "AGENT" || root["level"] != "WARNING" || root["environment"] != "test" || root["traceName"] != "consultor" {
		t.Fatalf("raiz no Langfuse = %v", root)
	}
	if in := fmt.Sprint(root["input"]); !strings.Contains(in, "***0181") || strings.Contains(in, "11.222.333") || strings.Contains(in, "não pode sair") {
		t.Fatalf("entrada no Langfuse sem mascaramento: %s", in)
	}
	if byName["plan"]["type"] != "CHAIN" || byName["render"]["level"] != "WARNING" {
		t.Fatalf("spans = %v / %v", byName["plan"], byName["render"])
	}
	gen := byName["classificar"]
	usage, _ := gen["usageDetails"].(map[string]any)
	if gen["type"] != "GENERATION" || gen["model"] != "gpt-6-luna" || usage["input"] != 21.0 || usage["output"] != 7.0 {
		t.Fatalf("geração = %v", gen)
	}

	scores := api.wait(t, "/api/public/v3/scores", url.Values{"traceId": {id}, "fields": {"details,subject"}, "limit": {"20"}},
		func(d []any) bool { return len(d) >= 4 })
	got := map[string]map[string]any{}
	for _, s := range scores {
		m := s.(map[string]any)
		got[m["name"].(string)] = m
	}
	if got["human_eval"]["value"] != "erro" || got["human_eval"]["dataType"] != "CATEGORICAL" || strings.Contains(fmt.Sprint(got["human_eval"]["comment"]), "123.456.789") ||
		// A leitura (API v3) devolve o BOOLEAN como true/false; o envio é 1/0, como na origem.
		got["gold_rbt12"]["value"] != true || got["gold_fs12"]["value"] != false || got["gold_acerto"]["value"] != 0.5 {
		t.Fatalf("scores no Langfuse = %v", got)
	}

	// Reenvio (corrida e notas de novo como pendentes): os mesmos IDs, nada duplicado.
	if _, err := e.pool.Exec(ctx, `UPDATE ai_runs SET langfuse_sync_status = 'failed', langfuse_synced_at = NULL WHERE trace_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE ai_human_evals SET langfuse_sync_status = 'failed' WHERE trace_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(time.Hour)
	if n, err := syncer.Requeue(ctx, e.pool, time.Minute, 10); err != nil || n != 1 {
		t.Fatalf("Requeue = %d, %v", n, err)
	}
	drain(t, runner)
	time.Sleep(10 * time.Second) // dá tempo de uma eventual duplicata aparecer
	// O Langfuse v4 grava cada envio e consolida as versões da mesma observação depois (ClickHouse):
	// até lá, a API pode listar a mesma observação duas vezes, com o mesmo ID. O que não pode
	// acontecer é surgir ID novo.
	again := api.wait(t, "/api/public/v2/observations", url.Values{"traceId": {id}, "fields": {"basic"}, "limit": {"50"}},
		func(d []any) bool { return len(d) >= 4 })
	if got, want := distinctIDs(again), distinctIDs(obs); got != want {
		t.Fatalf("o reenvio criou observações novas: %d → %d IDs distintos", want, got)
	}
	againScores := api.wait(t, "/api/public/v3/scores", url.Values{"traceId": {id}, "limit": {"20"}}, func(d []any) bool { return len(d) >= 4 })
	if got, want := distinctIDs(againScores), distinctIDs(scores); got != want {
		t.Fatalf("o reenvio criou scores novos: %d → %d IDs distintos", want, got)
	}
	run, _ := store.New(e.pool).GetAIRun(ctx, id)
	t.Logf("trace %s no Langfuse (ambiente test), %d observações e %d scores; tentativas de envio: %d", id, len(obs), len(scores), run.LangfuseAttempts)
}

func distinctIDs(list []any) int {
	seen := map[string]bool{}
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			seen[fmt.Sprint(m["id"])] = true
		}
	}
	return len(seen)
}

func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
