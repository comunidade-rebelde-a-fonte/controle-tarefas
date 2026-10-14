package aliases_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"controle/internal/ai"
	"controle/internal/ai/aliases"
	"controle/internal/platform/config"
	"controle/internal/testutil"
)

// TEST-008: configuração dos modelos de IA contra Postgres real, com um gateway LiteLLM
// simulado no próprio teste (a rota /utils/model_info do LiteLLM real é coberta por
// `make test-litellm`). Nenhuma chamada a provedor.

const gatewayKey = "sk-gateway-do-teste-123"

type modelRow struct {
	mode   string
	schema bool
	vision bool
	maxOut int
	params []string
}

var fullChat = modelRow{mode: "chat", schema: true, vision: true, maxOut: 128000,
	params: []string{"temperature", "max_tokens", "max_completion_tokens", "response_format"}}

// fakeGateway imita GET /utils/model_info do LiteLLM v1.104.0 (mesmos corpos e status).
type fakeGateway struct {
	mu     sync.Mutex
	models map[string]modelRow
	seen   []string
	auths  []string
	fail   int    // status forçado (0 = normal)
	body   string // corpo da falha forçada
}

func newFakeGateway() *fakeGateway {
	return &fakeGateway{models: map[string]modelRow{
		"openai/gpt-6-luna":             fullChat,
		"openai/gpt-6-sol":              fullChat,
		"openai/gpt-6-astra":            fullChat,
		"anthropic/claude-sonnet-5":     fullChat,
		"openai/gpt-6-texto":            {mode: "chat", schema: true, maxOut: 32000, params: []string{"temperature", "max_tokens", "response_format"}},
		"openai/o-sem-temperatura":      {mode: "chat", schema: true, vision: true, maxOut: 100000, params: []string{"max_completion_tokens", "response_format"}},
		"openai/gpt-sem-schema":         {mode: "chat", vision: true, maxOut: 16000, params: []string{"temperature", "max_tokens"}},
		"openai/text-embedding-3-small": {mode: "embedding", params: []string{"temperature", "max_tokens"}},
	}}
}

func (g *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auths = append(g.auths, r.Header.Get("Authorization"))
	if r.URL.Path != "/utils/model_info" {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"detail":"Not Found"}`)
		return
	}
	model := r.URL.Query().Get("model")
	g.seen = append(g.seen, model)
	if g.fail != 0 {
		w.WriteHeader(g.fail)
		_, _ = io.WriteString(w, g.body)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+gatewayKey {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Authentication Error, No api key passed in.","type":"auth_error"}}`)
		return
	}
	m, ok := g.models[model]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"detail":{"error":"model=%s, custom_llm_provider=None is not in the model cost map"}}`, model)
		return
	}
	info := map[string]any{"mode": m.mode, "supports_response_schema": m.schema, "supports_vision": m.vision,
		"max_output_tokens": m.maxOut, "supported_openai_params": m.params, "input_cost_per_token": 1e-07}
	_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "custom_llm_provider": "openai", "model_info": info})
}

func (g *fakeGateway) set(status int, body string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fail, g.body = status, body
}

func (g *fakeGateway) calls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.seen)
}

// Padrões do .env como no .env.example (D-015), sem o classificador: alias sem modelo.
func envDefaults() ai.Defaults {
	return ai.Defaults{
		ai.Triador: {Provider: "openai", Name: "gpt-6-luna"}, ai.Extrator: {Provider: "openai", Name: "gpt-6-sol"},
		ai.ExtratorForte: {Provider: "openai", Name: "gpt-6-astra"}, ai.Resolvedor: {Provider: "openai", Name: "gpt-6-sol"},
		ai.Consultor: {Provider: "openai", Name: "gpt-6-luna"}, ai.Priorizador: {Provider: "openai", Name: "gpt-6-luna"},
	}
}

type env struct {
	srv *testutil.Server
	gw  *fakeGateway
}

func setup(t *testing.T) env {
	t.Helper()
	gw := newFakeGateway()
	gsrv := httptest.NewServer(gw)
	t.Cleanup(gsrv.Close)
	srv := testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"), func(c *config.Config) {
		c.LiteLLMBaseURL = gsrv.URL
		c.LiteLLMAPIKey = gatewayKey
		c.AIDefaults = envDefaults()
	})
	return env{srv: srv, gw: gw}
}

type aliasOut struct {
	Alias        string `json:"alias"`
	Module       string `json:"module"`
	Requirements struct {
		StructuredOutput bool `json:"structured_output"`
		Vision           bool `json:"vision"`
	} `json:"requirements"`
	Provider       *string        `json:"provider"`
	Model          *string        `json:"model"`
	DefaultModel   *string        `json:"default_model"`
	EffectiveModel *string        `json:"effective_model"`
	Source         string         `json:"source"`
	Parameters     map[string]any `json:"parameters"`
	Enabled        bool           `json:"enabled"`
	Version        int32          `json:"version"`
	UpdatedAt      string         `json:"updated_at"`
}

type validationOut struct {
	OK     bool    `json:"ok"`
	Model  *string `json:"model"`
	Source string  `json:"source"`
	Checks []struct {
		Code    string `json:"code"`
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	} `json:"checks"`
	Capabilities *struct {
		Mode            string   `json:"mode"`
		Vision          bool     `json:"vision"`
		MaxOutputTokens *int     `json:"max_output_tokens"`
		Parameters      []string `json:"parameters"`
	} `json:"capabilities"`
}

func (v validationOut) codes() []string {
	out := []string{}
	for _, c := range v.Checks {
		mark := "+"
		if !c.OK {
			mark = "-"
		}
		out = append(out, mark+c.Code)
	}
	return out
}

func body(provider, model any, params map[string]any, enabled bool, version int) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{"provider": provider, "model": model, "parameters": params, "enabled": enabled, "version": version}
}

// putOK grava e decodifica num valor novo (decodificar sobre um mapa usado mistura chaves).
func putOK(t *testing.T, e env, alias string, b map[string]any) aliasOut {
	t.Helper()
	resp := e.srv.Put("/api/v1/settings/ai/models/"+alias, b).MustStatus(t, http.StatusOK)
	assertNoSecret(t, resp.Body)
	var out aliasOut
	resp.JSON(t, &out)
	return out
}

func list(t *testing.T, e env) map[string]aliasOut {
	t.Helper()
	var out struct {
		GatewayConfigured bool       `json:"gateway_configured"`
		Items             []aliasOut `json:"items"`
	}
	e.srv.Get("/api/v1/settings/ai/models").MustStatus(t, http.StatusOK).JSON(t, &out)
	m := map[string]aliasOut{}
	for _, it := range out.Items {
		m[it.Alias] = it
	}
	return m
}

func historyVersions(t *testing.T, e env, alias string) []int32 {
	t.Helper()
	rows, err := e.srv.Pool.Query(context.Background(), `SELECT version FROM ai_model_alias_history WHERE alias = $1 ORDER BY version`, alias)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int32
	for rows.Next() {
		var v int32
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

var unmaskedKey = regexp.MustCompile(`(?i)\b(sk|pk|rk)-[A-Za-z0-9]`)

// Nenhuma resposta da tela traz credencial: nem a chave do gateway, nem chave sem máscara,
// nem campo de chave [REQ-047, TEST-008].
func assertNoSecret(t *testing.T, raw []byte, extra ...string) {
	t.Helper()
	text := string(raw)
	for _, leak := range append([]string{gatewayKey}, extra...) {
		if strings.Contains(text, leak) {
			t.Fatalf("resposta vaza %q:\n%s", leak, text)
		}
	}
	if unmaskedKey.MatchString(text) {
		t.Fatalf("resposta tem chave sem máscara:\n%s", text)
	}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				lk := strings.ToLower(k)
				for _, bad := range []string{"key", "secret", "token", "password", "senha", "credential"} {
					// max_tokens é parâmetro de chamada, não credencial.
					if strings.Contains(lk, bad) && k != "max_tokens" && k != "max_output_tokens" {
						t.Fatalf("resposta tem campo %q com cara de credencial:\n%s", k, text)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	walk(v)
}

func TestTEST008_ListShowsCatalogDefaultsAndNoSecrets(t *testing.T) {
	e := setup(t)
	resp := e.srv.Get("/api/v1/settings/ai/models").MustStatus(t, http.StatusOK)
	assertNoSecret(t, resp.Body)

	var out struct {
		GatewayConfigured bool       `json:"gateway_configured"`
		Items             []aliasOut `json:"items"`
	}
	resp.JSON(t, &out)
	if !out.GatewayConfigured || len(out.Items) != len(ai.Catalog) {
		t.Fatalf("lista = %+v", out)
	}
	for i, info := range ai.Catalog {
		it := out.Items[i]
		if it.Alias != string(info.Alias) || it.Module != info.Module || it.Version != 1 || !it.Enabled || it.Provider != nil || it.Model != nil {
			t.Fatalf("item %d = %+v, want %s na versão 1, ligado e sem escolha na tela", i, it, info.Alias)
		}
		if it.Requirements.Vision != info.NeedsVision || it.Requirements.StructuredOutput != info.NeedsStructuredOutput {
			t.Fatalf("%s: requisitos = %+v", it.Alias, it.Requirements)
		}
	}
	items := list(t, e)
	tri := items["triador"]
	if tri.Source != "env" || *tri.DefaultModel != "openai/gpt-6-luna" || *tri.EffectiveModel != "openai/gpt-6-luna" {
		t.Fatalf("triador = %+v", tri)
	}
	cls := items["classificador"]
	if cls.Source != "none" || cls.DefaultModel != nil || cls.EffectiveModel != nil {
		t.Fatalf("classificador sem .env deveria ficar sem modelo: %+v", cls)
	}
	if len(e.gw.calls()) != 0 {
		t.Fatalf("listar não deve consultar o gateway: %v", e.gw.calls())
	}
}

// AC-027 + F-010: a troca vale para as novas corridas, sem rebuild; a corrida que já resolveu
// mantém o snapshot. Atualização versionada, com histórico.
func TestTEST008_UpdateIsVersionedAndAppliesToNewRuns(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	resolver := aliases.NewResolver(envDefaults())

	running, err := resolver.Resolve(ctx, e.srv.Pool, ai.Triador)
	if err != nil || running.Model.String() != "openai/gpt-6-luna" || running.Source != ai.SourceEnv || running.Version != 1 {
		t.Fatalf("antes da troca: %+v, %v", running, err)
	}

	resp := e.srv.Put("/api/v1/settings/ai/models/triador", body("openai", "gpt-6-sol", map[string]any{"temperature": 0.2, "max_tokens": 4000}, true, 1)).
		MustStatus(t, http.StatusOK)
	assertNoSecret(t, resp.Body)
	var got aliasOut
	resp.JSON(t, &got)
	updatedAt, err := time.Parse(time.RFC3339, got.UpdatedAt)
	if got.Version != 2 || got.Source != "settings" || *got.EffectiveModel != "openai/gpt-6-sol" || *got.DefaultModel != "openai/gpt-6-luna" ||
		got.Parameters["temperature"] != 0.2 || got.Parameters["max_tokens"] != 4000.0 || err != nil ||
		!updatedAt.Equal(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("PUT = %+v", got)
	}
	if calls := e.gw.calls(); !slices.Equal(calls, []string{"openai/gpt-6-sol"}) {
		t.Fatalf("gateway consultado com %v, want só o modelo novo", calls)
	}
	for _, a := range e.gw.auths {
		if a != "Bearer "+gatewayKey {
			t.Fatalf("o gateway recebeu Authorization %q, want só a chave do gateway", a)
		}
	}

	next, err := resolver.Resolve(ctx, e.srv.Pool, ai.Triador)
	if err != nil || next.Model.String() != "openai/gpt-6-sol" || next.Source != ai.SourceSettings || next.Version != 2 ||
		*next.Parameters.Temperature != 0.2 || *next.Parameters.MaxTokens != 4000 {
		t.Fatalf("nova corrida: %+v, %v", next, err)
	}
	if running.Model.String() != "openai/gpt-6-luna" || running.Parameters.Temperature != nil {
		t.Fatalf("a corrida em andamento perdeu o snapshot: %+v", running)
	}

	// Versão velha: conflito, nada gravado.
	resp = e.srv.Put("/api/v1/settings/ai/models/triador", body("openai", "gpt-6-astra", nil, true, 1))
	resp.MustStatus(t, http.StatusConflict)
	var conflict struct {
		Code    string `json:"code"`
		Details struct {
			CurrentVersion int `json:"current_version"`
		} `json:"details"`
	}
	resp.JSON(t, &conflict)
	if conflict.Code != "VERSION_CONFLICT" || conflict.Details.CurrentVersion != 2 {
		t.Fatalf("conflito = %+v", conflict)
	}

	// Volta ao padrão do ambiente: o modelo efetivo muda (sol → luna) e é conferido.
	got = putOK(t, e, "triador", body(nil, nil, nil, true, 2))
	if got.Version != 3 || got.Source != "env" || got.Provider != nil || *got.EffectiveModel != "openai/gpt-6-luna" || len(got.Parameters) != 0 {
		t.Fatalf("restaurar padrão = %+v", got)
	}

	// Desligar não depende do gateway (mesmo fora do ar) e impede novas corridas.
	e.gw.set(http.StatusServiceUnavailable, `bad gateway`)
	got = putOK(t, e, "triador", body(nil, nil, nil, false, 3))
	if got.Enabled || got.Version != 4 {
		t.Fatalf("desligar = %+v", got)
	}
	if _, err := resolver.Resolve(ctx, e.srv.Pool, ai.Triador); !errors.Is(err, ai.ErrAliasDisabled) {
		t.Fatalf("alias desligado: err = %v", err)
	}

	if v := historyVersions(t, e, "triador"); !slices.Equal(v, []int32{1, 2, 3, 4}) {
		t.Fatalf("histórico = %v", v)
	}
	var model, changedBy string
	if err := e.srv.Pool.QueryRow(ctx, `SELECT model, changed_by::text FROM ai_model_alias_history WHERE alias = 'triador' AND version = 2`).Scan(&model, &changedBy); err != nil {
		t.Fatal(err)
	}
	if model != "gpt-6-sol" || changedBy == "" {
		t.Fatalf("histórico v2 = %s por %q", model, changedBy)
	}
}

// Credencial nunca entra: campo de chave é recusado no JSON, nome com cara de chave é recusado
// na validação e no banco, e o erro do gateway chega sanitizado [REQ-047, 10 §9].
func TestTEST008_RejectsAndNeverEchoesSecrets(t *testing.T) {
	e := setup(t)
	const key = "sk-proj-SEGREDO1234567890"

	withKey := body("openai", "gpt-6-sol", nil, true, 1)
	withKey["api_key"] = key
	resp := e.srv.Put("/api/v1/settings/ai/models/triador", withKey).MustStatus(t, http.StatusBadRequest)
	if resp.ErrorCode(t) != "INVALID_JSON" {
		t.Fatalf("api_key: %s", resp.Body)
	}
	assertNoSecret(t, resp.Body, key, "SEGREDO")

	resp = e.srv.Put("/api/v1/settings/ai/models/triador", body("openai", "gpt-6-sol", map[string]any{"api_key": key}, true, 1)).
		MustStatus(t, http.StatusBadRequest)
	assertNoSecret(t, resp.Body, key, "SEGREDO")

	resp = e.srv.Post("/api/v1/settings/ai/models/triador/validate", map[string]any{"provider": "openai", "model": nil, "parameters": map[string]any{}, "api_key": key}).
		MustStatus(t, http.StatusBadRequest)
	assertNoSecret(t, resp.Body, key, "SEGREDO")

	resp = e.srv.Put("/api/v1/settings/ai/models/triador", body("openai", key, nil, true, 1)).MustStatus(t, http.StatusBadRequest)
	var verr struct {
		Code    string `json:"code"`
		Details struct {
			Fields map[string]string `json:"fields"`
		} `json:"details"`
	}
	resp.JSON(t, &verr)
	if verr.Code != "VALIDATION_ERROR" || !strings.Contains(verr.Details.Fields["model"], "chave de API") {
		t.Fatalf("modelo com cara de chave: %s", resp.Body)
	}
	assertNoSecret(t, resp.Body, key, "SEGREDO")
	if len(e.gw.calls()) != 0 {
		t.Fatalf("nada disso deveria chegar ao gateway: %v", e.gw.calls())
	}

	// O gateway ecoa uma chave no erro: a resposta da API chega sanitizada.
	e.gw.set(http.StatusBadRequest, `{"error":{"message":"No connected db. key=`+key+` Bearer `+gatewayKey+`","type":"no_db_connection"}}`)
	resp = e.srv.Post("/api/v1/settings/ai/models/triador/validate", map[string]any{"provider": "openai", "model": "gpt-6-sol", "parameters": map[string]any{}}).
		MustStatus(t, http.StatusServiceUnavailable)
	if resp.ErrorCode(t) != "AI_GATEWAY_UNAVAILABLE" || !strings.Contains(string(resp.Body), "LITELLM_API_KEY") {
		t.Fatalf("gateway recusou: %s", resp.Body)
	}
	assertNoSecret(t, resp.Body, key, "SEGREDO")

	// Nada foi gravado.
	if tri := list(t, e)["triador"]; tri.Version != 1 || tri.Model != nil {
		t.Fatalf("triador alterado: %+v", tri)
	}

	// O banco também recusa, mesmo por SQL direto: só parâmetros da lista e nunca chave.
	ctx := context.Background()
	for _, stmt := range []string{
		`UPDATE ai_model_aliases SET parameters = '{"api_key": "x"}' WHERE alias = 'triador'`,
		`UPDATE ai_model_aliases SET provider = 'openai', model = 'sk-proj-abc' WHERE alias = 'triador'`,
		`UPDATE ai_model_aliases SET provider = 'openai' WHERE alias = 'triador'`,
		`UPDATE ai_model_aliases SET parameters = '{"temperature": 3}' WHERE alias = 'triador'`,
		`UPDATE ai_model_aliases SET parameters = '{"temperature": "0.2"}' WHERE alias = 'triador'`,
		`UPDATE ai_model_aliases SET parameters = '{"max_tokens": 1.5}' WHERE alias = 'triador'`,
		`UPDATE ai_model_aliases SET parameters = '[]' WHERE alias = 'triador'`,
		`INSERT INTO ai_model_alias_history (alias, version, parameters, enabled) VALUES ('triador', 99, '{"token": "x"}', true)`,
	} {
		if _, err := e.srv.Pool.Exec(ctx, stmt); err == nil {
			t.Errorf("o banco aceitou: %s", stmt)
		}
	}
	var cols []string
	rows, err := e.srv.Pool.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_name IN ('ai_model_aliases', 'ai_model_alias_history')`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		cols = append(cols, c)
	}
	rows.Close()
	for _, c := range cols {
		for _, bad := range []string{"key", "secret", "token", "password"} {
			if strings.Contains(c, bad) {
				t.Fatalf("coluna %q com cara de credencial", c)
			}
		}
	}
}

func TestUpdate_GatewayChecksBlockUnsuitableModels(t *testing.T) {
	e := setup(t)
	cases := []struct {
		name, alias, model string
		params             map[string]any
		failed             string
	}{
		{"triador exige visão", "triador", "gpt-6-texto", nil, "VISION"},
		{"fora da tabela do gateway", "extrator", "gpt-6.1-sol", nil, "MODEL_KNOWN"},
		{"modelo de embedding", "resolvedor", "text-embedding-3-small", nil, "CHAT_MODE"},
		{"sem saída estruturada", "consultor", "gpt-sem-schema", nil, "STRUCTURED_OUTPUT"},
		{"temperature descartada", "priorizador", "o-sem-temperatura", map[string]any{"temperature": 0}, "PARAM_TEMPERATURE"},
		{"max_tokens acima do modelo", "resolvedor", "gpt-6-texto", map[string]any{"max_tokens": 50000}, "PARAM_MAX_TOKENS"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := e.srv.Put("/api/v1/settings/ai/models/"+c.alias, body("openai", c.model, c.params, true, 1)).
				MustStatus(t, http.StatusUnprocessableEntity)
			var out struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Details struct {
					Model  string `json:"model"`
					Checks []struct {
						Code string `json:"code"`
						OK   bool   `json:"ok"`
					} `json:"checks"`
				} `json:"details"`
			}
			resp.JSON(t, &out)
			failed := ""
			for _, ch := range out.Details.Checks {
				if !ch.OK {
					failed = ch.Code
					break
				}
			}
			if out.Code != "AI_MODEL_INVALID" || failed != c.failed || out.Details.Model != "openai/"+c.model {
				t.Fatalf("422 = %s", resp.Body)
			}
			if a := list(t, e)[c.alias]; a.Version != 1 || a.Model != nil {
				t.Fatalf("%s gravado apesar da reprovação: %+v", c.alias, a)
			}
		})
	}

	// O mesmo modelo sem visão serve ao resolvedor, que não lê imagem.
	got := putOK(t, e, "resolvedor", body("openai", "gpt-6-texto", map[string]any{"max_tokens": 32000}, true, 1))
	if *got.EffectiveModel != "openai/gpt-6-texto" {
		t.Fatalf("resolvedor = %+v", got)
	}
	// Outro provedor com rota no gateway (AC-028: troca sem mudar o domínio).
	got = putOK(t, e, "extrator_forte", body("anthropic", "claude-sonnet-5", nil, true, 1))
	if *got.EffectiveModel != "anthropic/claude-sonnet-5" || *got.Provider != "anthropic" {
		t.Fatalf("extrator_forte = %+v", got)
	}

	// Mesmo modelo e parâmetros: ligar/desligar sem consultar o gateway.
	before := len(e.gw.calls())
	e.srv.Put("/api/v1/settings/ai/models/extrator_forte", body("anthropic", "claude-sonnet-5", nil, false, 2)).MustStatus(t, http.StatusOK)
	if len(e.gw.calls()) != before {
		t.Fatal("só ligar/desligar não deveria consultar o gateway")
	}
}

func TestUpdate_GatewayDownOrMissing(t *testing.T) {
	e := setup(t)
	e.gw.set(http.StatusBadGateway, `<html>bad gateway</html>`)
	resp := e.srv.Put("/api/v1/settings/ai/models/triador", body("openai", "gpt-6-sol", nil, true, 1)).MustStatus(t, http.StatusServiceUnavailable)
	if resp.ErrorCode(t) != "AI_GATEWAY_UNAVAILABLE" {
		t.Fatalf("gateway fora do ar: %s", resp.Body)
	}

	srv := testutil.NewServer(t, testutil.NewDB(t), testutil.FixedClock(t, "2026-10-01T12:00:00Z"), func(c *config.Config) {
		c.AIDefaults = envDefaults()
	})
	var lst struct {
		GatewayConfigured bool `json:"gateway_configured"`
	}
	srv.Get("/api/v1/settings/ai/models").MustStatus(t, http.StatusOK).JSON(t, &lst)
	if lst.GatewayConfigured {
		t.Fatal("sem LITELLM_BASE_URL, gateway_configured deve ser false")
	}
	resp = srv.Put("/api/v1/settings/ai/models/triador", body("openai", "gpt-6-sol", nil, true, 1)).MustStatus(t, http.StatusServiceUnavailable)
	if resp.ErrorCode(t) != "AI_GATEWAY_NOT_CONFIGURED" {
		t.Fatalf("sem gateway: %s", resp.Body)
	}
	resp = srv.Post("/api/v1/settings/ai/models/triador/validate", map[string]any{"provider": nil, "model": nil, "parameters": map[string]any{}}).
		MustStatus(t, http.StatusServiceUnavailable)
	if resp.ErrorCode(t) != "AI_GATEWAY_NOT_CONFIGURED" {
		t.Fatalf("validar sem gateway: %s", resp.Body)
	}
	// Desligar continua possível sem gateway.
	srv.Put("/api/v1/settings/ai/models/triador", body(nil, nil, nil, false, 1)).MustStatus(t, http.StatusOK)
}

func TestUpdate_LocalValidation(t *testing.T) {
	e := setup(t)
	cases := map[string]struct {
		body  map[string]any
		field string
	}{
		"provedor sem modelo":  {body("openai", nil, nil, true, 1), "model"},
		"modelo sem provedor":  {body(nil, "gpt-6-sol", nil, true, 1), "provider"},
		"provedor sem rota":    {body("azure", "gpt-6-sol", nil, true, 1), "provider"},
		"modelo com barra":     {body("openai", "openai/gpt-6-sol", nil, true, 1), "model"},
		"modelo com espaço":    {body("openai", "gpt 6", nil, true, 1), "model"},
		"temperatura alta":     {body("openai", "gpt-6-sol", map[string]any{"temperature": 2.5}, true, 1), "parameters.temperature"},
		"max_tokens zero":      {body("openai", "gpt-6-sol", map[string]any{"max_tokens": 0}, true, 1), "parameters.max_tokens"},
		"sem version":          {map[string]any{"provider": nil, "model": nil, "parameters": map[string]any{}, "enabled": true}, "version"},
		"sem enabled":          {map[string]any{"provider": nil, "model": nil, "parameters": map[string]any{}, "version": 1}, "enabled"},
		"sem provider e model": {map[string]any{"parameters": map[string]any{}, "enabled": true, "version": 1}, "provider"},
		"sem parameters":       {map[string]any{"provider": nil, "model": nil, "enabled": true, "version": 1}, "parameters"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resp := e.srv.Put("/api/v1/settings/ai/models/triador", c.body).MustStatus(t, http.StatusBadRequest)
			var out struct {
				Code    string `json:"code"`
				Details struct {
					Fields map[string]string `json:"fields"`
				} `json:"details"`
			}
			resp.JSON(t, &out)
			if out.Code != "VALIDATION_ERROR" || out.Details.Fields[c.field] == "" {
				t.Fatalf("want erro em %s: %s", c.field, resp.Body)
			}
		})
	}
	resp := e.srv.Put("/api/v1/settings/ai/models/triador", body("openai", "gpt-6-sol", map[string]any{"max_tokens": 1.5}, true, 1)).
		MustStatus(t, http.StatusBadRequest)
	if resp.ErrorCode(t) != "INVALID_JSON" {
		t.Fatalf("max_tokens fracionário: %s", resp.Body)
	}
	for _, a := range []string{"parser_pgdas", "nao_existe", "TRIADOR"} {
		resp := e.srv.Put("/api/v1/settings/ai/models/"+a, body(nil, nil, nil, true, 1)).MustStatus(t, http.StatusNotFound)
		if resp.ErrorCode(t) != "AI_ALIAS_NOT_FOUND" {
			t.Fatalf("%s: %s", a, resp.Body)
		}
	}
	if len(e.gw.calls()) != 0 {
		t.Fatalf("validação local não consulta o gateway: %v", e.gw.calls())
	}
}

// A validação não grava nada e não chama o provedor: só a tabela de modelos do gateway.
func TestValidate_DryRun(t *testing.T) {
	e := setup(t)
	validate := func(alias string, b map[string]any) validationOut {
		t.Helper()
		resp := e.srv.Post("/api/v1/settings/ai/models/"+alias+"/validate", b).MustStatus(t, http.StatusOK)
		assertNoSecret(t, resp.Body)
		var v validationOut
		resp.JSON(t, &v)
		return v
	}

	v := validate("triador", map[string]any{"provider": "openai", "model": "gpt-6-sol", "parameters": map[string]any{"temperature": 0}})
	if !v.OK || *v.Model != "openai/gpt-6-sol" || v.Source != "settings" ||
		!slices.Equal(v.codes(), []string{"+MODEL_KNOWN", "+CHAT_MODE", "+STRUCTURED_OUTPUT", "+VISION", "+PARAM_TEMPERATURE"}) {
		t.Fatalf("validar sol = %+v", v)
	}
	if v.Capabilities == nil || !slices.Equal(v.Capabilities.Parameters, []string{"temperature", "max_tokens"}) || *v.Capabilities.MaxOutputTokens != 128000 {
		t.Fatalf("capacidades = %+v", v.Capabilities)
	}

	v = validate("triador", map[string]any{"provider": nil, "model": nil, "parameters": map[string]any{}})
	if !v.OK || *v.Model != "openai/gpt-6-luna" || v.Source != "env" {
		t.Fatalf("validar padrão do ambiente = %+v", v)
	}

	v = validate("priorizador", map[string]any{"provider": "openai", "model": "o-sem-temperatura", "parameters": map[string]any{"temperature": 1}})
	if v.OK || !slices.Equal(v.codes(), []string{"+MODEL_KNOWN", "+CHAT_MODE", "+STRUCTURED_OUTPUT", "-PARAM_TEMPERATURE"}) ||
		!slices.Equal(v.Capabilities.Parameters, []string{"max_tokens"}) {
		t.Fatalf("validar sem temperature = %+v", v)
	}

	v = validate("extrator", map[string]any{"provider": "openai", "model": "gpt-6.1-sol", "parameters": map[string]any{}})
	if v.OK || !slices.Equal(v.codes(), []string{"-MODEL_KNOWN"}) || v.Capabilities != nil {
		t.Fatalf("validar desconhecido = %+v", v)
	}

	v = validate("classificador", map[string]any{"provider": nil, "model": nil, "parameters": map[string]any{}})
	if v.OK || v.Source != "none" || v.Model != nil || !slices.Equal(v.codes(), []string{"-MODEL_CONFIGURED"}) ||
		!strings.Contains(v.Checks[0].Message, "CLASSIFICADOR_MODEL") {
		t.Fatalf("validar sem modelo = %+v", v)
	}

	for alias, it := range list(t, e) {
		if it.Version != 1 {
			t.Fatalf("validar gravou %s: %+v", alias, it)
		}
	}
	if v := historyVersions(t, e, "triador"); !slices.Equal(v, []int32{1}) {
		t.Fatalf("histórico após validar = %v", v)
	}
	if e.srv.Post("/api/v1/settings/ai/models/parser_pgdas/validate", map[string]any{"provider": nil, "model": nil, "parameters": map[string]any{}}).
		MustStatus(t, http.StatusNotFound).ErrorCode(t) != "AI_ALIAS_NOT_FOUND" {
		t.Fatal("parser_pgdas não é alias (ADR-006)")
	}
}

// A tabela tem exatamente os aliases do catálogo (o seed da migração acompanha o código).
func TestCatalog_MatchesTable(t *testing.T) {
	pool := testutil.NewDB(t)
	rows, err := pool.Query(context.Background(), `SELECT alias FROM ai_model_aliases ORDER BY alias`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		got = append(got, a)
	}
	want := []string{}
	for _, info := range ai.Catalog {
		want = append(want, string(info.Alias))
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("ai_model_aliases = %v; ai.Catalog = %v", got, want)
	}
}
