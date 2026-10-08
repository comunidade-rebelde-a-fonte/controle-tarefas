package litellm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"

	"controle/internal/ai"
)

// ErrUnknownModel indica um modelo fora da tabela de modelos do gateway. A tabela é a embutida
// na versão fixada do LiteLLM (LITELLM_LOCAL_MODEL_COST_MAP), e não a do GitHub (D-014).
var ErrUnknownModel = errors.New("modelo fora da tabela de modelos do gateway")

// ModelInfo são as capacidades de um modelo segundo o gateway (GET /utils/model_info). A
// consulta lê só a tabela do LiteLLM: não chama o provedor e não gasta tokens.
type ModelInfo struct {
	Mode             string // chat | embedding | image_generation | …
	StructuredOutput bool   // supports_response_schema
	Vision           bool   // supports_vision
	MaxOutputTokens  int    // 0 = não informado
	// Params são os parâmetros que o gateway repassa ao provedor. Com drop_params (config.yaml),
	// o que não estiver aqui é descartado em silêncio.
	Params []string
}

// Supports indica se o gateway repassa o parâmetro ao provedor.
func (m ModelInfo) Supports(param string) bool { return slices.Contains(m.Params, param) }

// Configured indica se há gateway configurado (LITELLM_BASE_URL).
func (c *Client) Configured() bool { return c.baseURL != "" }

type modelInfoResponse struct {
	ModelInfo *struct {
		Mode                   *string  `json:"mode"`
		SupportsResponseSchema *bool    `json:"supports_response_schema"`
		SupportsVision         *bool    `json:"supports_vision"`
		MaxOutputTokens        *float64 `json:"max_output_tokens"`
		SupportedOpenAIParams  []string `json:"supported_openai_params"`
	} `json:"model_info"`
}

// ModelInfo consulta as capacidades do modelo no gateway, numa tentativa só (a tela espera a
// resposta). Modelo fora da tabela devolve ErrUnknownModel; demais falhas, *Error sanitizado.
func (c *Client) ModelInfo(ctx context.Context, m ai.Model) (ModelInfo, error) {
	if c.baseURL == "" {
		return ModelInfo{}, ErrNotConfigured
	}
	target := c.baseURL + "/utils/model_info?" + url.Values{"model": {m.String()}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ModelInfo{}, &Error{Message: sanitize(err.Error())}
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey.Reveal())

	resp, err := c.http.Do(req)
	if err != nil {
		return ModelInfo{}, &Error{Message: sanitize(err.Error()), Retryable: ctx.Err() == nil}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ModelInfo{}, &Error{Message: sanitize(err.Error()), Retryable: true}
	}

	if resp.StatusCode == http.StatusNotFound && unknownModelBody(raw) {
		return ModelInfo{}, fmt.Errorf("%w: %s", ErrUnknownModel, m)
	}
	if resp.StatusCode != http.StatusOK {
		return ModelInfo{}, &Error{
			Status:    resp.StatusCode,
			Message:   errorMessage(raw, resp.Status),
			Retryable: retryableStatus(resp.StatusCode),
		}
	}
	var parsed modelInfoResponse
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.ModelInfo == nil || parsed.ModelInfo.Mode == nil {
		return ModelInfo{}, &Error{Status: resp.StatusCode, Message: "resposta do gateway sem model_info"}
	}
	info := parsed.ModelInfo
	out := ModelInfo{
		Mode:             *info.Mode,
		StructuredOutput: info.SupportsResponseSchema != nil && *info.SupportsResponseSchema,
		Vision:           info.SupportsVision != nil && *info.SupportsVision,
		Params:           slices.Clone(info.SupportedOpenAIParams),
	}
	if info.MaxOutputTokens != nil && *info.MaxOutputTokens > 0 {
		out.MaxOutputTokens = int(*info.MaxOutputTokens)
	}
	return out, nil
}

// unknownModelBody reconhece o 404 de modelo fora da tabela ({"detail":{"error":"…"}}), para
// não confundir com rota inexistente ({"detail":"Not Found"}), que é falha do gateway.
func unknownModelBody(raw []byte) bool {
	var body struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return false
	}
	var detail struct {
		Error string `json:"error"`
	}
	return json.Unmarshal(body.Detail, &detail) == nil && detail.Error != ""
}
