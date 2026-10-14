package aliases

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
)

// Handler expõe a configuração de IA em /settings/ai/models [06 §7].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/settings/ai/models", h.list)
	r.Put("/settings/ai/models/{alias}", h.update)
	r.Post("/settings/ai/models/{alias}/validate", h.validate)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Update(r.Context(), chi.URLParam(r, "alias"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) validate(w http.ResponseWriter, r *http.Request) {
	var in ValidateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Validate(r.Context(), chi.URLParam(r, "alias"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
