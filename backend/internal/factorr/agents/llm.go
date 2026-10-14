package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/prompts"
	"controle/internal/ai/tracer"
)

// O LLM só classifica a intenção (span plan) e redige o texto a partir de uma decisão já
// calculada (span render), como na origem; nenhum cálculo passa por ele (CLAUDE §3.1 da origem).

// ErrLLMUnavailable é o LLM fora do ar, desligado ou com resposta inútil: o agente cai no
// classificador por palavras-chave e no template, como a origem com LLMIndisponivel.
var ErrLLMUnavailable = errors.New("LLM indisponível")

// Chamadas curtas, como na origem: 512 tokens para classificar e 1024 para redigir, salvo
// configuração do alias na tela de IA.
const (
	planMaxTokens   = 512
	renderMaxTokens = 1024
)

var (
	planPrompt   = prompts.Get("fatorr/plan")
	renderPrompt = prompts.Get("fatorr/render")
)

// PromptRef é a referência dos prompts gravada na corrida [prompt-spec §1].
func PromptRef() string { return planPrompt.Ref() + ";" + renderPrompt.Ref() }

// Gateway é o cliente do LiteLLM (litellm.Client).
type Gateway interface {
	Chat(ctx context.Context, req litellm.Request) (litellm.Response, error)
}

// LLM é o modelo resolvido para a corrida (snapshot do alias) com o gateway.
type LLM struct {
	gateway Gateway
	model   ai.Resolved
}

func unavailable(reason string, err error) error {
	if err != nil {
		return fmt.Errorf("%w (%s): %w", ErrLLMUnavailable, reason, err)
	}
	return fmt.Errorf("%w (%s)", ErrLLMUnavailable, reason)
}

func (l *LLM) request(run *tracer.Run, system, user string, maxTokens int, schema *litellm.JSONSchema) litellm.Request {
	if l.model.Parameters.MaxTokens != nil {
		maxTokens = *l.model.Parameters.MaxTokens
	}
	return litellm.Request{
		Model:       l.model.Model,
		Messages:    []litellm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}},
		Temperature: l.model.Parameters.Temperature,
		MaxTokens:   maxTokens,
		JSONSchema:  schema,
		Metadata:    map[string]string{"alias": string(l.model.Alias), "trace_id": run.TraceID()},
	}
}

// Classify pede a intenção do pedido. Só a mensagem do analista vai ao LLM: nenhuma lista de
// empresas, CNPJ ou valor (revisão de segurança M3 da origem).
func (l *LLM) Classify(ctx context.Context, run *tracer.Run, message string) (Intent, error) {
	req := l.request(run, planPrompt.Text, "Pedido: "+message, planMaxTokens,
		&litellm.JSONSchema{Name: "classificar_intencao", Schema: intentSchema})
	resp, err := l.gateway.Chat(ctx, req)
	gen := tracer.Generation{Name: "classificar_intencao", Model: l.model.Model, Err: err, Input: map[string]any{"pedido": message}}
	if err == nil {
		gen.Response = &resp
		gen.Output = map[string]any{"conteudo": resp.Content}
	}
	if gerr := run.Generation(gen); gerr != nil {
		return Intent{}, gerr
	}
	if err != nil {
		return Intent{}, unavailable("gateway", err)
	}
	in, err := parseIntent(resp.Content)
	if err != nil {
		return Intent{}, unavailable("intencao_invalida", nil)
	}
	return in, nil
}

// Write pede o texto da resposta a partir da decisão (só valores necessários; nunca CNPJ).
func (l *LLM) Write(ctx context.Context, run *tracer.Run, decision map[string]any) (string, error) {
	payload, err := json.Marshal(plain(decision))
	if err != nil {
		return "", err
	}
	req := l.request(run, renderPrompt.Text, "DECISÃO:\n"+string(payload), renderMaxTokens, nil)
	resp, err := l.gateway.Chat(ctx, req)
	gen := tracer.Generation{Name: "redigir_resposta", Model: l.model.Model, Err: err}
	if err == nil {
		gen.Response = &resp
		gen.Output = map[string]any{"texto": resp.Content}
	}
	if gerr := run.Generation(gen); gerr != nil {
		return "", gerr
	}
	if err != nil {
		return "", unavailable("gateway", err)
	}
	text := strings.TrimSpace(resp.Content)
	if text == "" {
		return "", unavailable("texto_vazio", nil)
	}
	return text, nil
}
