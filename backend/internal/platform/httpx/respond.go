// Package httpx reúne utilitários HTTP compartilhados: envelope de erro,
// serialização JSON, middlewares e helpers de parâmetros.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"controle/internal/platform/apperr"
)

// MaxJSONBody limita o tamanho dos corpos JSON aceitos.
const MaxJSONBody = 1 << 20 // 1 MiB

// ErrorBody é o envelope padrão de erro da API.
type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// WriteJSON serializa v com o status informado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("falha ao serializar resposta", "error", err)
	}
}

// NoContent responde 204.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// WriteError converte err no envelope padrão. Erros de domínio (*apperr.Error)
// mantêm status, código e mensagem; qualquer outro erro vira 500 genérico,
// registrado no log sem expor detalhes ao cliente.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if ae, ok := apperr.As(err); ok {
		if ae.Status >= http.StatusInternalServerError {
			logger(r).Error("erro interno", "code", ae.Code, "error", err)
		}
		WriteJSON(w, ae.Status, ErrorBody{Code: ae.Code, Message: ae.Message, Details: ae.Details})
		return
	}
	logger(r).Error("erro não tratado", "error", err)
	WriteJSON(w, http.StatusInternalServerError, ErrorBody{
		Code:    "INTERNAL_ERROR",
		Message: "Erro interno. Tente novamente em instantes.",
	})
}

// DecodeJSON lê o corpo JSON em dst, com limite de tamanho. Campos
// desconhecidos são rejeitados para evitar erros silenciosos de contrato.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		return apperr.BadRequest("INVALID_CONTENT_TYPE", "O corpo da requisição deve ser JSON.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return apperr.BadRequest("BODY_TOO_LARGE", "O corpo da requisição excede o tamanho máximo.")
		case errors.Is(err, io.EOF):
			return apperr.BadRequest("EMPTY_BODY", "O corpo da requisição está vazio.")
		default:
			return apperr.BadRequest("INVALID_JSON", "O corpo da requisição não é um JSON válido para esta operação.").
				WithDetails(map[string]any{"reason": err.Error()})
		}
	}
	if dec.More() {
		return apperr.BadRequest("INVALID_JSON", "O corpo da requisição deve conter um único objeto JSON.")
	}
	return nil
}
