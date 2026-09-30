package obligation

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
)

// Handler expõe /obligation-instances.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/obligation-instances/{id}", h.get)
	r.Patch("/obligation-instances/{id}/status", h.changeStatus)
}

func instanceID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", "INSTANCE_NOT_FOUND", "Obrigação não encontrada.")
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := instanceID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	inst, err := h.svc.GetInstance(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, inst)
}

func (h *Handler) changeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := instanceID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in StatusChangeInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := h.svc.ChangeStatus(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
