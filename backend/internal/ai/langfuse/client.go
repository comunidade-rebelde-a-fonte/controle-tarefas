// Package langfuse envia traces e notas ao Langfuse self-hosted [REQ-041, 11 §4]. Sem SDK:
// OTLP/HTTP em JSON (/api/public/otel/v1/traces), que o Langfuse ingere como o SDK Python da
// origem, e a API pública de scores. Tudo que sai passa pelo mascaramento do Fator R de origem
// (CNPJ/CPF, texto bruto de PDF, payload limitado) [09 §PII no Langfuse].
//
// O pacote só envia. Quem decide o que reenviar e grava o status de sincronização é o tracer
// (TASK-009): a trilha local é a fonte de verdade e o Langfuse, um espelho [REQ-042].
package langfuse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"controle/internal/platform/config"
	"controle/internal/platform/redact"
)

// ErrNotConfigured indica que LANGFUSE_HOST não foi definida.
var ErrNotConfigured = errors.New("sem Langfuse configurado (LANGFUSE_HOST)")

// Config é o acesso ao Langfuse. As chaves são do projeto no Langfuse local; a secreta nunca
// aparece em log nem em resposta.
type Config struct {
	Host        string // URL interna da API (ex.: http://langfuse:3000)
	PublicKey   string
	SecretKey   config.Secret
	Environment string // ambiente do Langfuse (ex.: development)
	Timeout     time.Duration
	// MaxPayloadBytes limita cada atributo enviado (TRACE_PAYLOAD_MAX_KB da origem).
	MaxPayloadBytes int
}

// Client envia ao Langfuse.
type Client struct {
	cfg  Config
	http *http.Client
}

// New cria o cliente.
func New(cfg Config) *Client {
	cfg.Host = strings.TrimRight(cfg.Host, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxPayloadBytes <= 0 {
		cfg.MaxPayloadBytes = 32 * 1024
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Configured indica se há Langfuse configurado.
func (c *Client) Configured() bool { return c.cfg.Host != "" }

// Error é uma falha de envio, com mensagem sanitizada (sem chaves).
type Error struct {
	Status  int // 0 = rede ou timeout
	Message string
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return "Langfuse: " + e.Message
	}
	return fmt.Sprintf("Langfuse: HTTP %d: %s", e.Status, e.Message)
}

// Export envia o trace (span raiz e filhos) por OTLP/HTTP JSON. O mesmo trace com os mesmos IDs
// pode ser reenviado: o Langfuse atualiza as observações em vez de duplicar.
func (c *Client) Export(ctx context.Context, t Trace) error {
	body, err := c.otlp(t)
	if err != nil {
		return fmt.Errorf("montar o trace para o Langfuse: %w", err)
	}
	return c.post(ctx, "/api/public/otel/v1/traces", body)
}

// ScoreType é o tipo do score no Langfuse.
type ScoreType string

const (
	ScoreBoolean     ScoreType = "BOOLEAN"
	ScoreNumeric     ScoreType = "NUMERIC"
	ScoreCategorical ScoreType = "CATEGORICAL"
)

// Score é uma nota do trace. O ID fixo torna o reenvio idempotente: o Langfuse atualiza a mesma
// nota em vez de criar outra.
type Score struct {
	ID      string
	TraceID string
	Name    string
	Type    ScoreType
	// Value: número (BOOLEAN e NUMERIC) ou texto (CATEGORICAL).
	Value   any
	Comment string
}

// SendScore grava a nota no Langfuse. O comentário tem CNPJ e CPF mascarados.
func (c *Client) SendScore(ctx context.Context, s Score) error {
	payload := map[string]any{
		"id": s.ID, "traceId": s.TraceID, "name": s.Name, "dataType": string(s.Type), "value": s.Value,
	}
	if c.cfg.Environment != "" {
		payload["environment"] = c.cfg.Environment
	}
	if s.Comment != "" {
		payload["comment"] = MaskAttribute(s.Comment, c.cfg.MaxPayloadBytes)
	}
	body, err := marshal(payload)
	if err != nil {
		return fmt.Errorf("montar o score para o Langfuse: %w", err)
	}
	return c.post(ctx, "/api/public/scores", body)
}

func (c *Client) post(ctx context.Context, path string, body []byte) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Host+path, bytes.NewReader(body))
	if err != nil {
		return &Error{Message: redact.Message(err.Error())}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.cfg.PublicKey+":"+c.cfg.SecretKey.Reveal())))
	resp, err := c.http.Do(req)
	if err != nil {
		return &Error{Message: redact.Message(err.Error())}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 != 2 {
		return &Error{Status: resp.StatusCode, Message: errorMessage(raw, resp.Status)}
	}
	return nil
}

// errorMessage extrai a mensagem do corpo de erro do Langfuse, sanitizada.
func errorMessage(raw []byte, status string) string {
	var body struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &body) == nil {
		if body.Message != "" {
			return redact.Message(body.Message)
		}
		if body.Error != "" {
			return redact.Message(body.Error)
		}
	}
	return redact.Message(status)
}
