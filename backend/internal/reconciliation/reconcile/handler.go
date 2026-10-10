package reconcile

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe a rodada do matching [06 §4].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/companies/{id}/reconciliation/run", h.run)
}

// run roda o matching determinístico da competência e devolve os baldes.
func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "COMPANY_NOT_FOUND", "Empresa não encontrada.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	comp, err := competence.Parse(r.URL.Query().Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validate.Field("competence", "Competência inválida: use AAAA-MM."))
		return
	}
	out, err := h.svc.Run(r.Context(), id, comp)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
