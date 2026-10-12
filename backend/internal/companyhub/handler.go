package companyhub

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe a Visão geral da ficha e os indicadores da listagem.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/companies/{id}/overview", h.overview)
	r.Get("/company-indicators", h.indicators)
}

func competenceParam(w http.ResponseWriter, r *http.Request) (competence.Competence, bool) {
	c, err := competence.Parse(r.URL.Query().Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validate.Field("competence", "Informe a competência (AAAA-MM)."))
		return competence.Competence{}, false
	}
	return c, true
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	c, ok := competenceParam(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Overview(r.Context(), chi.URLParam(r, "id"), c)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) indicators(w http.ResponseWriter, r *http.Request) {
	c, ok := competenceParam(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Indicators(r.Context(), c)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
