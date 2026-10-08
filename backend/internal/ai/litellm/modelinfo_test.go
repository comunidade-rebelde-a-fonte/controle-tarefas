package litellm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Corpo real do LiteLLM v1.104.0 para openai/gpt-6-luna, reduzido aos campos usados.
const lunaInfo = `{"model":"openai/gpt-6-luna","custom_llm_provider":"openai","model_info":{"key":"gpt-6-luna",` +
	`"max_tokens":128000,"max_output_tokens":128000,"mode":"chat","supports_response_schema":true,"supports_vision":true,` +
	`"supported_openai_params":["max_tokens","max_completion_tokens","temperature","response_format","reasoning_effort"]}}`

func TestModelInfo_ReadsCapabilitiesWithGatewayKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/utils/model_info" || r.URL.Query().Get("model") != "openai/gpt-6-luna" {
			t.Errorf("chamada = %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer sk-gateway-teste" {
			t.Errorf("Authorization = %q, want só a chave do gateway", r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, lunaInfo)
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL+"/").ModelInfo(context.Background(), luna)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "chat" || !got.StructuredOutput || !got.Vision || got.MaxOutputTokens != 128000 {
		t.Fatalf("capacidades = %+v", got)
	}
	if !got.Supports("temperature") || !got.Supports("max_tokens") || got.Supports("top_k") {
		t.Fatalf("parâmetros = %v", got.Params)
	}
}

func TestModelInfo_MissingCapabilitiesAreFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"model_info":{"mode":"embedding","supports_response_schema":null,"max_output_tokens":null,"supported_openai_params":null}}`)
	}))
	defer srv.Close()
	got, err := newTestClient(srv.URL).ModelInfo(context.Background(), luna)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != "embedding" || got.StructuredOutput || got.Vision || got.MaxOutputTokens != 0 || len(got.Params) != 0 {
		t.Fatalf("capacidades = %+v", got)
	}
}

func TestModelInfo_Errors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		unknown bool
	}{
		// Modelo fora da tabela: o 404 do LiteLLM traz detail.error.
		{"fora da tabela", 404, `{"detail":{"error":"model=openai/gpt-6.1-sol, custom_llm_provider=None is not in the model cost map"}}`, true},
		// Rota inexistente (LiteLLM antigo): falha do gateway, não "modelo desconhecido".
		{"rota inexistente", 404, `{"detail":"Not Found"}`, false},
		{"sem chave", 401, `{"error":{"message":"Authentication Error, No api key passed in.","type":"auth_error"}}`, false},
		// Chave errada sem banco no LiteLLM: 400 "No connected db." (TEST-004).
		{"chave errada", 400, `{"error":{"message":"No connected db.","type":"no_db_connection"}}`, false},
		{"fora do ar", 503, `<html>bad gateway</html>`, false},
		{"sem model_info", 200, `{"model":"x"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			}))
			defer srv.Close()
			_, err := newTestClient(srv.URL).ModelInfo(context.Background(), luna)
			if got := errors.Is(err, ErrUnknownModel); got != c.unknown {
				t.Fatalf("err = %v; ErrUnknownModel = %v, want %v", err, got, c.unknown)
			}
			var callErr *Error
			if !c.unknown && !errors.As(err, &callErr) {
				t.Fatalf("err = %v, want *Error", err)
			}
		})
	}
}

func TestModelInfo_SanitizesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"detail":{"error":"chave inválida: sk-proj-AbCdEf1234567890 (Bearer sk-gateway-teste)"}}`)
	}))
	defer srv.Close()
	_, err := newTestClient(srv.URL).ModelInfo(context.Background(), luna)
	if err == nil {
		t.Fatal("esperava erro")
	}
	for _, leak := range []string{"AbCdEf1234567890", "sk-proj", "sk-gateway-teste"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("erro vaza %q: %v", leak, err)
		}
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "chave inválida") {
		t.Fatalf("erro perdeu o diagnóstico útil: %v", err)
	}
}

func TestModelInfo_NetworkAndConfig(t *testing.T) {
	if _, err := New("", "", time.Second).ModelInfo(context.Background(), luna); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("sem LITELLM_BASE_URL: err = %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	_, err := newTestClient(url).ModelInfo(context.Background(), luna)
	var callErr *Error
	if !errors.As(err, &callErr) || callErr.Status != 0 {
		t.Fatalf("gateway fora do ar: err = %v", err)
	}
}

func TestErrorMessage_FastAPIDetail(t *testing.T) {
	cases := map[string]string{
		`{"error":{"message":"m1"}}`:      "m1",
		`{"detail":"Not Found"}`:          "Not Found",
		`{"detail":{"error":"m2"}}`:       "m2",
		`{"detail":[{"msg":"required"}]}`: "422 Unprocessable Entity",
		`<html>`:                          "422 Unprocessable Entity",
	}
	for body, want := range cases {
		if got := errorMessage([]byte(body), "422 Unprocessable Entity"); got != want {
			t.Errorf("errorMessage(%s) = %q, want %q", body, got, want)
		}
	}
}
