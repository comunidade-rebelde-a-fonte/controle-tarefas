// Package litellm é o cliente do gateway LiteLLM, que expõe a API de chat compatível com a OpenAI
// [ADR-004, spec 04 §8].
//
// O cliente recebe sempre o modelo concreto já resolvido (ai.Resolver) e só conhece a chave do
// gateway: as chaves dos provedores ficam no container do LiteLLM [spec 09 §4]. Não registra log
// de conteúdo; quem chama grava os metadados da corrida (TASK-009).
package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"controle/internal/ai"
	"controle/internal/platform/config"
	"controle/internal/platform/redact"
)

// ErrNotConfigured indica que LITELLM_BASE_URL não foi definida.
var ErrNotConfigured = errors.New("gateway de IA não configurado (LITELLM_BASE_URL)")

// Client chama o endpoint /v1/chat/completions do gateway.
type Client struct {
	baseURL   string
	apiKey    config.Secret
	http      *http.Client
	retries   int
	backoff   time.Duration
	sleepFunc func(context.Context, time.Duration) error
}

// Option ajusta o cliente.
type Option func(*Client)

// WithRetry define quantas novas tentativas fazer em falhas transitórias (rede, 429, 5xx) e o
// intervalo inicial, que dobra a cada tentativa. Padrão: 2 novas tentativas, a partir de 500 ms.
func WithRetry(retries int, backoff time.Duration) Option {
	return func(c *Client) { c.retries, c.backoff = retries, backoff }
}

// New cria o cliente. timeout limita cada tentativa.
func New(baseURL string, apiKey config.Secret, timeout time.Duration, opts ...Option) *Client {
	c := &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		http:      &http.Client{Timeout: timeout},
		retries:   2,
		backoff:   500 * time.Millisecond,
		sleepFunc: sleep,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Message é uma mensagem do chat.
type Message struct {
	Role    string `json:"role"` // system | user | assistant
	Content string `json:"content"`
}

// JSONSchema pede saída estruturada validada pelo provedor (response_format json_schema, strict).
// A saída continua sendo validada pelo código antes de qualquer uso [spec 09 §6].
type JSONSchema struct {
	Name   string
	Schema json.RawMessage
}

// Request é uma chamada de chat.
type Request struct {
	Model       ai.Model
	Messages    []Message
	Temperature *float64
	MaxTokens   int
	JSONSchema  *JSONSchema
	// Metadata segue para o gateway (ex.: alias, trace_id). Nunca dados do documento.
	Metadata map[string]string
}

// Usage é o consumo de tokens informado pelo provedor.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response é o resultado de uma chamada.
type Response struct {
	Content      string
	FinishReason string
	// Model é o modelo informado na resposta (o provedor pode devolver uma versão datada).
	Model string
	Usage Usage
	// CostUSD é o custo informado pelo gateway, em decimal textual ("" se ausente). Nunca float.
	CostUSD  string
	CallID   string
	Latency  time.Duration
	Attempts int
}

// Error é a falha de uma chamada, com mensagem sanitizada (sem chaves nem cabeçalhos).
type Error struct {
	Status    int // 0 = falha de rede ou timeout
	Message   string
	Retryable bool
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return "gateway de IA: " + e.Message
	}
	return fmt.Sprintf("gateway de IA: HTTP %d: %s", e.Status, e.Message)
}

type chatRequest struct {
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	Temperature    *float64          `json:"temperature,omitempty"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat   `json:"response_format,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type responseFormat struct {
	Type       string          `json:"type"`
	JSONSchema *jsonSchemaSpec `json:"json_schema,omitempty"`
}

type jsonSchemaSpec struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
			Refusal *string `json:"refusal"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

// Chat envia a chamada ao gateway e devolve a resposta da primeira tentativa bem-sucedida.
func (c *Client) Chat(ctx context.Context, req Request) (Response, error) {
	if c.baseURL == "" {
		return Response{}, ErrNotConfigured
	}
	body := chatRequest{
		Model:       req.Model.String(),
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Metadata:    req.Metadata,
	}
	if req.JSONSchema != nil {
		body.ResponseFormat = &responseFormat{
			Type:       "json_schema",
			JSONSchema: &jsonSchemaSpec{Name: req.JSONSchema.Name, Schema: req.JSONSchema.Schema, Strict: true},
		}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, fmt.Errorf("montar a chamada ao gateway: %w", err)
	}

	var lastErr *Error
	for attempt := 1; attempt <= c.retries+1; attempt++ {
		if attempt > 1 {
			wait := c.backoff << (attempt - 2)
			if err := c.sleepFunc(ctx, wait); err != nil {
				return Response{}, lastErr
			}
		}
		resp, callErr := c.do(ctx, payload)
		if callErr == nil {
			resp.Attempts = attempt
			return resp, nil
		}
		lastErr = callErr
		if !callErr.Retryable {
			break
		}
	}
	return Response{}, lastErr
}

func (c *Client) do(ctx context.Context, payload []byte) (Response, *Error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Response{}, &Error{Message: sanitize(err.Error())}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey.Reveal())

	start := time.Now()
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		// Rede, DNS, timeout: transitório, salvo cancelamento pelo chamador.
		return Response{}, &Error{Message: sanitize(err.Error()), Retryable: ctx.Err() == nil}
	}
	defer func() { _ = httpResp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 8<<20))
	latency := time.Since(start)
	if err != nil {
		return Response{}, &Error{Message: sanitize(err.Error()), Retryable: true}
	}

	if httpResp.StatusCode != http.StatusOK {
		return Response{}, &Error{
			Status:    httpResp.StatusCode,
			Message:   errorMessage(raw, httpResp.Status),
			Retryable: retryableStatus(httpResp.StatusCode),
		}
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		return Response{}, &Error{Status: httpResp.StatusCode, Message: "resposta do gateway sem escolhas válidas"}
	}
	choice := parsed.Choices[0]
	if choice.Message.Refusal != nil && *choice.Message.Refusal != "" {
		return Response{}, &Error{Status: httpResp.StatusCode, Message: "o modelo recusou a solicitação"}
	}
	content := ""
	if choice.Message.Content != nil {
		content = *choice.Message.Content
	}
	return Response{
		Content:      content,
		FinishReason: choice.FinishReason,
		Model:        parsed.Model,
		Usage:        parsed.Usage,
		CostUSD:      decimalHeader(httpResp.Header.Get("x-litellm-response-cost")),
		CallID:       httpResp.Header.Get("x-litellm-call-id"),
		Latency:      latency,
	}, nil
}

func retryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// errorMessage extrai a mensagem do corpo, já sanitizada: error.message (formato OpenAI/LiteLLM)
// ou detail (rotas utilitárias do LiteLLM, em FastAPI: texto ou {"error": "…"}).
func errorMessage(raw []byte, status string) string {
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return sanitize(status)
	}
	if body.Error.Message != "" {
		return sanitize(body.Error.Message)
	}
	var text string
	if json.Unmarshal(body.Detail, &text) == nil && text != "" {
		return sanitize(text)
	}
	var detail struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body.Detail, &detail) == nil && detail.Error != "" {
		return sanitize(detail.Error)
	}
	return sanitize(status)
}

var decimalCost = regexp.MustCompile(`^\d+(\.\d+)?([eE]-?\d+)?$`)

// sanitize remove chaves e credenciais da mensagem e limita o tamanho [spec 06 §10].
func sanitize(msg string) string { return redact.Message(msg) }

func decimalHeader(v string) string {
	v = strings.TrimSpace(v)
	if !decimalCost.MatchString(v) {
		return ""
	}
	return v
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
