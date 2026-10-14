//go:build litellm

// Integração com o LiteLLM real — a mesma imagem e o mesmo litellm/config.yaml do
// docker-compose.yml — diante de um provedor OpenAI simulado neste processo. Prova a cadeia
// alias → modelo → provedor (TEST-004, AC-028) sem chamada externa e sem custo:
//
//	make test-litellm
package litellm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"controle/internal/ai"
)

// Mesma imagem do serviço litellm do docker-compose.yml.
const gatewayImage = "ghcr.io/berriai/litellm:v1.104.0"

const (
	gatewayKey  = "sk-gateway-integracao"
	providerKey = "sk-provedor-simulado"
)

// fakeProvider imita o endpoint de chat da OpenAI e registra o modelo de cada chamada.
type fakeProvider struct {
	mu     sync.Mutex
	models []string
	paths  []string
	// formats guarda o response_format de cada chamada (saída estruturada dos agentes).
	formats []map[string]any
	// messages guarda as mensagens de cada chamada como chegaram (texto ou partes com imagem).
	messages [][]json.RawMessage
}

func (p *fakeProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.paths = append(p.paths, r.URL.Path)
	p.mu.Unlock()
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+providerKey {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"chave do provedor ausente ou errada","type":"invalid_request_error"}}`)
		return
	}
	var body struct {
		Model          string            `json:"model"`
		Messages       []json.RawMessage `json:"messages"`
		ResponseFormat map[string]any    `json:"response_format"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	p.mu.Lock()
	p.models = append(p.models, body.Model)
	p.formats = append(p.formats, body.ResponseFormat)
	p.messages = append(p.messages, body.Messages)
	p.mu.Unlock()
	var last struct {
		Content string `json:"content"`
	}
	if len(body.Messages) > 0 {
		_ = json.Unmarshal(body.Messages[len(body.Messages)-1], &last) // conteúdo em partes fica vazio
	}
	if last.Content == "provoque-401" {
		// O provedor ecoa a chave no erro; o cliente não pode repassá-la.
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `{"error":{"message":"Incorrect API key provided: %s","type":"invalid_request_error"}}`, providerKey)
		return
	}
	content := "pong"
	if body.ResponseFormat != nil {
		content = `{\"intencao\":\"priorizar\"}`
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":%q,`+
		`"choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"finish_reason":"stop"}],`+
		`"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`, body.Model, content)
}

func (p *fakeProvider) lastFormat() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.formats) == 0 {
		return nil
	}
	return p.formats[len(p.formats)-1]
}

func (p *fakeProvider) lastMessages() []json.RawMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.messages) == 0 {
		return nil
	}
	return p.messages[len(p.messages)-1]
}

func (p *fakeProvider) lastModel() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.models) == 0 {
		return ""
	}
	return p.models[len(p.models)-1]
}

func (p *fakeProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.paths)
}

// envExample lê os <ALIAS>_MODEL do .env.example (os padrões provisórios do Q-003).
func envExample(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "..", ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok && strings.HasSuffix(k, "_MODEL") {
			env[k] = strings.TrimSpace(v)
		}
	}
	return env
}

func TestGateway_RoutesEveryAliasToTheProviderWithItsKey(t *testing.T) {
	ctx := context.Background()

	provider := &fakeProvider{}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: provider, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	port := ln.Addr().(*net.TCPAddr).Port

	config, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "litellm", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	models := envExample(t)
	env := map[string]string{
		"LITELLM_MASTER_KEY":           gatewayKey,
		"OPENAI_API_KEY":               providerKey,
		"OPENAI_API_BASE":              fmt.Sprintf("http://%s:%d/v1", testcontainers.HostInternal, port),
		"LITELLM_LOCAL_MODEL_COST_MAP": "True",
	}
	for k, v := range models {
		env[k] = v
	}

	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:           gatewayImage,
			Cmd:             []string{"--config", "/app/config.yaml", "--port", "4000"},
			Env:             env,
			Files:           []testcontainers.ContainerFile{{HostFilePath: config, ContainerFilePath: "/app/config.yaml", FileMode: 0o644}},
			ExposedPorts:    []string{"4000/tcp"},
			HostAccessPorts: []int{port},
			WaitingFor:      wait.ForHTTP("/health/liveliness").WithPort("4000/tcp").WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("subir o LiteLLM: %v", err)
	}
	endpoint, err := ctr.PortEndpoint(ctx, "4000/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}

	defaults, err := ai.DefaultsFromEnv(func(k string) string { return models[k] })
	if err != nil {
		t.Fatal(err)
	}
	resolver := ai.NewResolver(defaults)
	client := New(endpoint, gatewayKey, 60*time.Second, WithRetry(0, 0))
	ping := []Message{{Role: "user", Content: "ping"}}

	// Caminho da aplicação: alias resolvido no app, modelo concreto pela rota openai/*.
	for _, info := range ai.Catalog {
		t.Run(string(info.Alias), func(t *testing.T) {
			resolved, err := resolver.Resolve(info.Alias)
			if err != nil {
				t.Fatal(err)
			}
			if resolved.Model.Provider != "openai" {
				t.Fatalf("provedor inicial = %s, want openai (AC-028)", resolved.Model.Provider)
			}
			resp, err := client.Chat(ctx, Request{Model: resolved.Model, Messages: ping, Metadata: map[string]string{"alias": string(info.Alias)}})
			if err != nil {
				t.Fatalf("chat: %v (caminhos vistos pelo provedor: %v)", err, provider.paths)
			}
			if got := provider.lastModel(); got != resolved.Model.Name {
				t.Fatalf("o provedor recebeu %q, want %q", got, resolved.Model.Name)
			}
			if resp.Content != "pong" || resp.Usage.TotalTokens != 8 {
				t.Fatalf("resposta = %+v", resp)
			}
		})
	}

	// Saída estruturada (TASK-013): o plan do consultor pede json_schema estrito; o LiteLLM
	// repassa o response_format ao provedor e devolve o JSON como conteúdo.
	t.Run("saída estruturada json_schema", func(t *testing.T) {
		resolved, err := resolver.Resolve(ai.Consultor)
		if err != nil {
			t.Fatal(err)
		}
		schema := json.RawMessage(`{"type":"object","properties":{"intencao":{"type":"string","enum":["priorizar"]}},"required":["intencao"],"additionalProperties":false}`)
		resp, err := client.Chat(ctx, Request{Model: resolved.Model, Messages: ping, JSONSchema: &JSONSchema{Name: "classificar_intencao", Schema: schema}})
		if err != nil {
			t.Fatal(err)
		}
		rf := provider.lastFormat()
		js, _ := rf["json_schema"].(map[string]any)
		if rf["type"] != "json_schema" || js["name"] != "classificar_intencao" || js["strict"] != true || js["schema"] == nil {
			t.Fatalf("response_format no provedor = %v", rf)
		}
		if resp.Content != `{"intencao":"priorizar"}` {
			t.Fatalf("conteúdo = %q", resp.Content)
		}
	})

	// Imagens (TASK-016): o extrator manda as páginas renderizadas em partes (texto e image_url com
	// data URL); o LiteLLM repassa as partes ao provedor OpenAI como vieram.
	t.Run("mensagem com imagens", func(t *testing.T) {
		resolved, err := resolver.Resolve(ai.Extrator)
		if err != nil {
			t.Fatal(err)
		}
		png := []byte("\x89PNG\r\n\x1a\npagina")
		msgs := []Message{{Role: "system", Content: "extraia"}, {Role: "user", Content: "estrategia=render_150 dpi=150", Images: []Image{{MIME: "image/png", Data: png, Detail: "high"}}}}
		if _, err := client.Chat(ctx, Request{Model: resolved.Model, Messages: msgs}); err != nil {
			t.Fatal(err)
		}
		got := provider.lastMessages()
		if len(got) != 2 {
			t.Fatalf("mensagens no provedor = %d", len(got))
		}
		var user struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL struct {
					URL    string `json:"url"`
					Detail string `json:"detail"`
				} `json:"image_url"`
			} `json:"content"`
		}
		if err := json.Unmarshal(got[1], &user); err != nil {
			t.Fatalf("mensagem do usuário no provedor = %s (%v)", got[1], err)
		}
		if len(user.Content) != 2 || user.Content[0].Type != "text" || user.Content[1].Type != "image_url" ||
			user.Content[1].ImageURL.URL != "data:image/png;base64,iVBORw0KGgpwYWdpbmE=" || user.Content[1].ImageURL.Detail != "high" {
			t.Fatalf("partes no provedor = %s", got[1])
		}
	})

	// Rota do próprio alias no gateway (compatibilidade com o Conciliador): o LiteLLM resolve pelo .env.
	t.Run("alias direto no gateway", func(t *testing.T) {
		if _, err := client.Chat(ctx, Request{Model: ai.Model{Provider: "openai", Name: "x"}, Messages: ping}); err != nil {
			t.Fatal(err)
		}
		raw := New(endpoint, gatewayKey, 60*time.Second, WithRetry(0, 0))
		if _, err := raw.chatRaw(ctx, "triador", ping); err != nil {
			t.Fatal(err)
		}
		if got, want := provider.lastModel(), strings.TrimPrefix(models["TRIADOR_MODEL"], "openai/"); got != want {
			t.Fatalf("alias triador chegou ao provedor como %q, want %q", got, want)
		}
	})

	t.Run("chave errada do gateway", func(t *testing.T) {
		// Sem banco, o LiteLLM não consulta chaves virtuais e responde 400 ("No connected db.");
		// sem cabeçalho nenhum, 401. Em ambos: recusa definitiva, sem nova tentativa.
		_, err := New(endpoint, "sk-chave-errada", 30*time.Second, WithRetry(0, 0)).Chat(ctx, Request{Model: defaults[ai.Triador], Messages: ping})
		var callErr *Error
		if !errors.As(err, &callErr) || callErr.Status < 400 || callErr.Status >= 500 || callErr.Retryable {
			t.Fatalf("err = %v, want recusa 4xx definitiva", err)
		}
		if strings.Contains(err.Error(), "chave-errada") {
			t.Fatalf("erro vaza a chave: %v", err)
		}
	})

	// Validação da tela Configurações → IA (TASK-008): capacidades lidas da tabela de modelos do
	// gateway real, sem nenhuma chamada ao provedor.
	t.Run("model_info", func(t *testing.T) {
		before := provider.calls()
		for _, info := range ai.Catalog {
			m := defaults[info.Alias]
			mi, err := client.ModelInfo(ctx, m)
			if err != nil {
				t.Fatalf("%s (%s): %v", info.Alias, m, err)
			}
			if mi.Mode != "chat" || !mi.StructuredOutput || (info.NeedsVision && !mi.Vision) ||
				!mi.Supports("temperature") || !mi.Supports("max_tokens") || mi.MaxOutputTokens == 0 {
				t.Fatalf("%s (%s): o padrão do .env não atende ao agente: %+v", info.Alias, m, mi)
			}
		}
		if _, err := client.ModelInfo(ctx, ai.Model{Provider: "openai", Name: "gpt-6.1-sol"}); !errors.Is(err, ErrUnknownModel) {
			t.Fatalf("modelo fora da tabela fixada: err = %v, want ErrUnknownModel", err)
		}
		emb, err := client.ModelInfo(ctx, ai.Model{Provider: "openai", Name: "text-embedding-3-small"})
		if err != nil || emb.Mode != "embedding" {
			t.Fatalf("embedding: %+v, %v", emb, err)
		}
		_, err = New(endpoint, "sk-chave-errada", 30*time.Second).ModelInfo(ctx, defaults[ai.Triador])
		var callErr *Error
		if !errors.As(err, &callErr) || callErr.Status < 400 || callErr.Status >= 500 || strings.Contains(err.Error(), "chave-errada") {
			t.Fatalf("chave errada: err = %v, want recusa 4xx sem ecoar a chave", err)
		}
		if provider.calls() != before {
			t.Fatal("model_info não pode chamar o provedor")
		}
	})

	t.Run("erro do provedor com a chave no texto", func(t *testing.T) {
		_, err := client.Chat(ctx, Request{Model: defaults[ai.Triador], Messages: []Message{{Role: "user", Content: "provoque-401"}}})
		if err == nil {
			t.Fatal("esperava erro do provedor")
		}
		if strings.Contains(err.Error(), "provedor-simulado") {
			t.Fatalf("erro vaza a chave do provedor: %v", err)
		}
	})
}

// chatRaw chama o gateway com um nome de modelo sem provedor (rota de alias do config.yaml).
func (c *Client) chatRaw(ctx context.Context, model string, msgs []Message) (Response, error) {
	payload, err := json.Marshal(chatRequest{Model: model, Messages: msgs})
	if err != nil {
		return Response{}, err
	}
	resp, callErr := c.do(ctx, payload)
	if callErr != nil {
		return Response{}, callErr
	}
	return resp, nil
}
