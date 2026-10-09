package agents

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
)

// Handler expõe o simulador e os agentes do Fator R [06 §3].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/companies/{id}/factor-r/simulations", h.simulate)
	r.Get("/companies/{id}/factor-r/simulations", h.simulations)
	r.Post("/factor-r/agents/consult", h.consult)
	r.Post("/factor-r/agents/prioritize", h.prioritize)
}

func companyID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", "COMPANY_NOT_FOUND", "Empresa não encontrada.")
}

func (h *Handler) simulate(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in SimulationInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Simulate(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handler) simulations(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var limit int32
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			httpx.WriteError(w, r, apperr.New(http.StatusBadRequest, "VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
				WithDetails(map[string]any{"fields": map[string]any{"limit": "Use um número inteiro."}}))
			return
		}
		limit = int32(n)
	}
	out, err := h.svc.ListSimulations(r.Context(), id, limit)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) consult(w http.ResponseWriter, r *http.Request) {
	var in ConsultInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Consult(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) prioritize(w http.ResponseWriter, r *http.Request) {
	var in PrioritizeInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Prioritize(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
