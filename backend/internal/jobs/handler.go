package jobs

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
	"controle/internal/user"
)

// HTTPHandler expõe GET /jobs/{id} e POST /jobs/{id}/retry [06 §8].
type HTTPHandler struct{ svc *Service }

// NewHandler cria o handler HTTP.
func NewHandler(svc *Service) *HTTPHandler { return &HTTPHandler{svc: svc} }

// Routes registra as rotas.
func (h *HTTPHandler) Routes(r chi.Router) {
	r.Get("/jobs/{id}", h.get)
	r.Post("/jobs/{id}/retry", h.retry)
}

func (h *HTTPHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", jobNotFound, jobMissing)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *HTTPHandler) retry(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", jobNotFound, jobMissing)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Retry(r.Context(), id, user.FromContext(r.Context()))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
