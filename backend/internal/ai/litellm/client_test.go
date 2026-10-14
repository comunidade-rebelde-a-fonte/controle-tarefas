package litellm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"controle/internal/ai"
)

var luna = ai.Model{Provider: "openai", Name: "gpt-6-luna"}

func noWait(context.Context, time.Duration) error { return nil }

func newTestClient(url string, opts ...Option) *Client {
	c := New(url, "sk-gateway-teste", 2*time.Second, opts...)
	c.sleepFunc = noWait
	return c
}

func okBody(model string) string {
	return `{"id":"x","model":"` + model + `","choices":[{"message":{"role":"assistant","content":"{\"tipo\":\"extrato\"}"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}}`
}

func TestChat_SendsConcreteModelAndGatewayKeyOnly(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("chamada = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-gateway-teste" {
			t.Errorf("Authorization = %q, want só a chave do gateway", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.Header().Set("x-litellm-response-cost", "1.25e-05")
		w.Header().Set("x-litellm-call-id", "call-123")
		_, _ = io.WriteString(w, okBody("gpt-6-luna-2026-06-01"))
	}))
	defer srv.Close()

	zero := 0.0
	resp, err := newTestClient(srv.URL+"/").Chat(context.Background(), Request{
		Model:       luna,
		Messages:    []Message{{Role: "system", Content: "Você é o triador."}, {Role: "user", Content: "documento"}},
		Temperature: &zero,
		MaxTokens:   200,
		JSONSchema:  &JSONSchema{Name: "triagem", Schema: json.RawMessage(`{"type":"object"}`)},
		Metadata:    map[string]string{"alias": "triador", "trace_id": "abc"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got["model"] != "openai/gpt-6-luna" {
		t.Fatalf("model enviado = %v, want o modelo concreto resolvido", got["model"])
	}
	format, _ := got["response_format"].(map[string]any)
	schema, _ := format["json_schema"].(map[string]any)
	if format["type"] != "json_schema" || schema["name"] != "triagem" || schema["strict"] != true {
		t.Fatalf("response_format = %v", got["response_format"])
	}
	if got["temperature"] != 0.0 || got["max_tokens"] != 200.0 {
		t.Fatalf("parâmetros = %v %v", got["temperature"], got["max_tokens"])
	}
	if md, _ := got["metadata"].(map[string]any); md["alias"] != "triador" {
		t.Fatalf("metadata = %v", got["metadata"])
	}

	want := Response{
		Content: `{"tipo":"extrato"}`, FinishReason: "stop", Model: "gpt-6-luna-2026-06-01",
		Usage:   Usage{PromptTokens: 12, CompletionTokens: 5, TotalTokens: 17},
		CostUSD: "1.25e-05", CallID: "call-123", Attempts: 1,
	}
	resp.Latency = 0
	if resp != want {
		t.Fatalf("resposta = %+v\nwant %+v", resp, want)
	}
}

func TestChat_RetriesOnlyTransientFailures(t *testing.T) {
	cases := []struct {
		name         string
		statuses     []int
		retries      int
		wantAttempts int32
		wantErr      bool
		retryable    bool
	}{
		{"503 duas vezes e depois ok", []int{503, 503, 200}, 2, 3, false, false},
		{"429 sem novas tentativas", []int{429}, 0, 1, true, true},
		{"400 não repete", []int{400, 200}, 2, 1, true, false},
		{"401 não repete", []int{401, 200}, 2, 1, true, false},
		{"500 esgota as tentativas", []int{500, 502, 504}, 2, 3, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				status := c.statuses[min(int(calls.Add(1))-1, len(c.statuses)-1)]
				if status != http.StatusOK {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"error":{"message":"falha simulada"}}`)
					return
				}
				_, _ = io.WriteString(w, okBody("gpt-6-luna"))
			}))
			defer srv.Close()

			resp, err := newTestClient(srv.URL, WithRetry(c.retries, time.Millisecond)).Chat(context.Background(), Request{Model: luna})
			if calls.Load() != c.wantAttempts {
				t.Fatalf("tentativas = %d, want %d", calls.Load(), c.wantAttempts)
			}
			if !c.wantErr {
				if err != nil || resp.Attempts != int(c.wantAttempts) {
					t.Fatalf("resp = %+v, err = %v", resp, err)
				}
				return
			}
			var callErr *Error
			if !errors.As(err, &callErr) || callErr.Retryable != c.retryable {
				t.Fatalf("err = %v (%#v), want Retryable=%v", err, callErr, c.retryable)
			}
		})
	}
}

// A mensagem de erro do provedor pode citar a chave; nada disso chega a quem chama [spec 06 §10].
func TestChat_SanitizesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key provided: sk-proj-AbCdEf1234567890. Header: Bearer sk-gateway-teste"}}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Chat(context.Background(), Request{Model: luna})
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, leak := range []string{"AbCdEf1234567890", "sk-proj", "sk-gateway-teste"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("erro vaza %q: %v", leak, err)
		}
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Incorrect API key provided") {
		t.Fatalf("erro perdeu o diagnóstico útil: %v", err)
	}
}

func TestChat_TimeoutIsTransient(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	c := New(srv.URL, "sk-gateway-teste", 50*time.Millisecond, WithRetry(0, 0))
	_, err := c.Chat(context.Background(), Request{Model: luna})
	var callErr *Error
	if !errors.As(err, &callErr) || callErr.Status != 0 || !callErr.Retryable {
		t.Fatalf("timeout: err = %v (%#v), want falha de rede transitória", err, callErr)
	}
}

func TestChat_InvalidOrEmptyResponses(t *testing.T) {
	bodies := map[string]string{
		"sem escolhas": `{"model":"m","choices":[]}`,
		"não é JSON":   `<html>proxy</html>`,
		"recusa":       `{"model":"m","choices":[{"message":{"content":null,"refusal":"não posso"},"finish_reason":"stop"}]}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
			defer srv.Close()
			_, err := newTestClient(srv.URL).Chat(context.Background(), Request{Model: luna})
			var callErr *Error
			if !errors.As(err, &callErr) || callErr.Retryable {
				t.Fatalf("err = %v, want erro definitivo", err)
			}
		})
	}
}

func TestChat_NotConfigured(t *testing.T) {
	if _, err := New("", "", time.Second).Chat(context.Background(), Request{Model: luna}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"chave sk-proj-AbC123xyz no texto":        "chave [chave oculta] no texto",
		"Authorization: Bearer abc.def-ghi":       "Authorization: Bearer [oculto]",
		"rk-live-abc e pk-lf-123 e sk-ant-a1b2c3": "[chave oculta] e [chave oculta] e [chave oculta]",
		"linha 1\n\n   linha 2":                   "linha 1 linha 2",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sanitize(strings.Repeat("a", 500)); len([]rune(got)) != 301 {
		t.Errorf("sanitize deve truncar em 300 caracteres + reticências, veio %d", len([]rune(got)))
	}
}

// Com imagens, a mensagem vai em partes (texto e image_url com data URL), o formato da API de chat
// que o LiteLLM repassa aos provedores com visão (TASK-016); sem imagens, continua texto.
func TestChat_SendsImagesAsContentParts(t *testing.T) {
	var got struct {
		Messages []json.RawMessage `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = io.WriteString(w, okBody("gpt-6-sol"))
	}))
	defer srv.Close()
	_, err := newTestClient(srv.URL).Chat(context.Background(), Request{
		Model: luna,
		Messages: []Message{
			{Role: "system", Content: "Você extrai."},
			{Role: "user", Content: "estrategia=render_150", Images: []Image{
				{MIME: "image/png", Data: []byte("\x89PNG"), Detail: "high"},
				{MIME: "image/jpeg", Data: []byte{0xFF, 0xD8}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Messages[0]) != `{"role":"system","content":"Você extrai."}` {
		t.Fatalf("mensagem de texto = %s", got.Messages[0])
	}
	want := `{"role":"user","content":[{"type":"text","text":"estrategia=render_150"},` +
		`{"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw==","detail":"high"}},` +
		`{"type":"image_url","image_url":{"url":"data:image/jpeg;base64,/9g="}}]}`
	if string(got.Messages[1]) != want {
		t.Fatalf("mensagem com imagens =\n%s\nwant\n%s", got.Messages[1], want)
	}
}
