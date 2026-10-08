package langfuse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testPK = "pk-lf-teste"
	testSK = "sk-lf-segredo-do-teste-123"
)

type capture struct {
	path, auth string
	body       map[string]any
}

func server(t *testing.T, status int, reply string) (*httptest.Server, *[]capture) {
	t.Helper()
	var got []capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		got = append(got, capture{r.URL.Path, r.Header.Get("Authorization"), body})
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func client(url string) *Client {
	return New(Config{Host: url + "/", PublicKey: testPK, SecretKey: testSK, Environment: "development", Timeout: 2 * time.Second, MaxPayloadBytes: 1024})
}

func ptr(i int) *int { return &i }

var t0 = time.Date(2026, 10, 8, 15, 0, 0, 0, time.UTC)

func sampleTrace() Trace {
	return Trace{
		ID: "0123456789abcdef0123456789abcdef", RootID: "aaaaaaaaaaaaaaaa", Name: "consultor",
		Start: t0, End: t0.Add(900 * time.Millisecond), Level: LevelWarning, StatusMessage: "needs_review",
		Input:    map[string]any{"pergunta": "Qual o Fator R da empresa 11.222.333/0001-81?", "texto_extraido": "PDF inteiro"},
		Output:   map[string]any{"texto": "resposta"},
		Tags:     []string{"consultor", "pergunta", "fator_r"},
		Metadata: map[string]any{"company_id": "c1", "pa": "2026-09"},
		UserID:   "u1", SessionID: "c1",
		Spans: []Span{
			{ID: "bbbbbbbbbbbbbbbb", Name: "plan", Type: TypeChain, Start: t0, End: t0.Add(300 * time.Millisecond)},
			{ID: "cccccccccccccccc", ParentID: "bbbbbbbbbbbbbbbb", Name: "classificar", Type: TypeGeneration,
				Start: t0.Add(10 * time.Millisecond), End: t0.Add(290 * time.Millisecond),
				Model: "openai/gpt-6-luna", ResponseModel: "gpt-6-luna-2026-06-01", TokensIn: ptr(12), TokensOut: ptr(5), CostUSD: "1.25e-05"},
			{ID: "dddddddddddddddd", Name: "render", Type: TypeChain, Start: t0.Add(300 * time.Millisecond), End: t0.Add(800 * time.Millisecond),
				Level: LevelError, StatusMessage: "LLM_UNAVAILABLE"},
		},
	}
}

func attrMap(t *testing.T, span map[string]any) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, a := range span["attributes"].([]any) {
		kv := a.(map[string]any)
		v := kv["value"].(map[string]any)
		if s, ok := v["stringValue"]; ok {
			out[kv["key"].(string)] = s
		} else if arr, ok := v["arrayValue"]; ok {
			var list []string
			for _, x := range arr.(map[string]any)["values"].([]any) {
				list = append(list, x.(map[string]any)["stringValue"].(string))
			}
			out[kv["key"].(string)] = list
		}
	}
	return out
}

func TestExport_OTLPJSONWithLangfuseAttributes(t *testing.T) {
	srv, got := server(t, http.StatusOK, `{"name":"otel-ingestion-job"}`)
	if err := client(srv.URL).Export(context.Background(), sampleTrace()); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 {
		t.Fatalf("%d chamadas", len(*got))
	}
	c := (*got)[0]
	if c.path != "/api/public/otel/v1/traces" {
		t.Fatalf("rota = %s", c.path)
	}
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte(testPK+":"+testSK)); c.auth != want {
		t.Fatal("autenticação diferente de Basic pk:sk")
	}
	rs := c.body["resourceSpans"].([]any)[0].(map[string]any)
	spans := rs["scopeSpans"].([]any)[0].(map[string]any)["spans"].([]any)
	if len(spans) != 4 {
		t.Fatalf("%d spans", len(spans))
	}
	root := spans[0].(map[string]any)
	ra := attrMap(t, root)
	if root["traceId"] != "0123456789abcdef0123456789abcdef" || root["spanId"] != "aaaaaaaaaaaaaaaa" || root["parentSpanId"] != nil ||
		root["startTimeUnixNano"] != "1791471600000000000" || root["endTimeUnixNano"] != "1791471600900000000" {
		t.Fatalf("raiz = %v", root)
	}
	if ra["langfuse.observation.type"] != "agent" || ra["langfuse.trace.name"] != "consultor" || ra["user.id"] != "u1" ||
		ra["session.id"] != "c1" || ra["langfuse.environment"] != "development" || ra["langfuse.observation.level"] != "WARNING" {
		t.Fatalf("atributos da raiz = %v", ra)
	}
	if tags := ra["langfuse.trace.tags"].([]string); strings.Join(tags, ",") != "consultor,pergunta,fator_r" {
		t.Fatalf("tags = %v", tags)
	}
	// Mascaramento na saída: texto do PDF removido e CNPJ mascarado [09 §PII].
	input := ra["langfuse.trace.input"].(string)
	if strings.Contains(input, "PDF inteiro") || strings.Contains(input, "11.222.333") ||
		!strings.Contains(input, `"texto_extraido":"[removido]"`) || !strings.Contains(input, "***0181") {
		t.Fatalf("entrada enviada = %s", input)
	}
	plan, gen, render := spans[1].(map[string]any), spans[2].(map[string]any), spans[3].(map[string]any)
	if plan["parentSpanId"] != "aaaaaaaaaaaaaaaa" || gen["parentSpanId"] != "bbbbbbbbbbbbbbbb" || attrMap(t, plan)["langfuse.observation.type"] != "chain" {
		t.Fatalf("hierarquia: plan %v, geração %v", plan["parentSpanId"], gen["parentSpanId"])
	}
	ga := attrMap(t, gen)
	if ga["langfuse.observation.type"] != "generation" || ga["gen_ai.request.model"] != "gpt-6-luna" || ga["gen_ai.system"] != "openai" ||
		ga["gen_ai.response.model"] != "gpt-6-luna-2026-06-01" || ga["langfuse.observation.usage_details"] != `{"input":12,"output":5}` ||
		ga["langfuse.observation.cost_details"] != `{"total":1.25e-05}` {
		t.Fatalf("geração = %v", ga)
	}
	if render["status"].(map[string]any)["code"] != 2.0 || attrMap(t, render)["langfuse.observation.level"] != "ERROR" {
		t.Fatalf("span com erro = %v", render)
	}
}

func TestSendScore_IdempotentIDAndMaskedComment(t *testing.T) {
	srv, got := server(t, http.StatusOK, `{"id":"s1"}`)
	err := client(srv.URL).SendScore(context.Background(), Score{
		ID: "11111111-2222-3333-4444-555555555555", TraceID: "0123456789abcdef0123456789abcdef", Name: "human_eval",
		Type: ScoreCategorical, Value: "erro", Comment: "errou o CNPJ 11222333000181",
	})
	if err != nil {
		t.Fatal(err)
	}
	b := (*got)[0]
	if b.path != "/api/public/scores" || b.body["id"] != "11111111-2222-3333-4444-555555555555" || b.body["name"] != "human_eval" ||
		b.body["dataType"] != "CATEGORICAL" || b.body["value"] != "erro" || b.body["environment"] != "development" ||
		b.body["comment"] != "errou o CNPJ ***0181" {
		t.Fatalf("score = %v", b.body)
	}
}

func TestPost_ErrorsAreSanitized(t *testing.T) {
	srv, _ := server(t, http.StatusUnauthorized, `{"message":"Invalid credentials. Confirm that you've configured the correct host. Key sk-lf-segredo-do-teste-123 pk-lf-teste"}`)
	err := client(srv.URL).Export(context.Background(), sampleTrace())
	var lfErr *Error
	if !errors.As(err, &lfErr) || lfErr.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "segredo") || strings.Contains(err.Error(), testPK) {
		t.Fatalf("erro vaza a chave: %v", err)
	}

	srv.Close()
	if err := client(srv.URL).SendScore(context.Background(), Score{ID: "x", TraceID: "y", Name: "z", Type: ScoreNumeric, Value: 1}); !errors.As(err, &lfErr) || lfErr.Status != 0 {
		t.Fatalf("fora do ar: err = %v", err)
	}
	if err := New(Config{}).Export(context.Background(), sampleTrace()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("sem LANGFUSE_HOST: err = %v", err)
	}
}
