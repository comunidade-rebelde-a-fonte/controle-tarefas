package completion

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
)

// Handler expõe a conclusão e a reabertura da conciliação.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/companies/{id}/reconciliation/complete", h.complete)
	r.Post("/companies/{id}/reconciliation/reopen", h.reopen)
}

func competenceParam(w http.ResponseWriter, r *http.Request) (competence.Competence, bool) {
	c, err := competence.Parse(r.URL.Query().Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validation("competence", "Informe a competência (AAAA-MM)."))
		return competence.Competence{}, false
	}
	return c, true
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	c, ok := competenceParam(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Complete(r.Context(), chi.URLParam(r, "id"), c)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type reopenInput struct {
	Reason string `json:"reason"`
}

func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	c, ok := competenceParam(w, r)
	if !ok {
		return
	}
	var in reopenInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Reopen(r.Context(), chi.URLParam(r, "id"), c, in.Reason)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
