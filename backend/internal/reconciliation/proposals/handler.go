package proposals

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
)

// Handler expõe o resolvedor na revisão da Conciliação [06 §4]. O {id} é a linha do extrato.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/reconciliation/review/{id}/proposal", h.request)
	r.Get("/reconciliation/review/{id}/proposals", h.list)
	r.Post("/reconciliation/review/{id}/accept-proposal", h.accept)
	r.Post("/reconciliation/review/{id}/reject-proposal", h.reject)
}

func lineID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", "RECONCILIATION_LINE_NOT_FOUND", "Linha do extrato não encontrada.")
}

// request pede a hipótese (202, com o job).
func (h *Handler) request(w http.ResponseWriter, r *http.Request) {
	id, err := lineID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Request(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	id, err := lineID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.List(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// decision é o corpo do aceite e da rejeição.
type decision struct {
	ProposalID string `json:"proposal_id"`
}

// readDecision lê a linha e a proposta (400 sem a proposta).
func readDecision(w http.ResponseWriter, r *http.Request) (string, decision, error) {
	id, err := lineID(r)
	if err != nil {
		return "", decision{}, err
	}
	var in decision
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return "", decision{}, err
	}
	if strings.TrimSpace(in.ProposalID) == "" {
		return "", decision{}, apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": map[string]any{"proposal_id": "Informe a proposta."}})
	}
	return id, in, nil
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) {
	id, in, err := readDecision(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Accept(r.Context(), id, in.ProposalID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	id, in, err := readDecision(w, r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Reject(r.Context(), id, in.ProposalID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
