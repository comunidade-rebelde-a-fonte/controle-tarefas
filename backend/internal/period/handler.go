package period

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
)

// Handler expõe /company-module-periods.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas [PRD §41.8].
func (h *Handler) Routes(r chi.Router) {
	r.Get("/company-module-periods/{id}", h.get)
	r.Post("/company-module-periods/{id}/close", h.close)
	r.Post("/company-module-periods/{id}/reopen", h.reopen)
	r.Patch("/company-module-periods/{id}/note", h.note)
	r.Get("/company-module-periods/{id}/events", h.events)
}

func periodID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", notFoundCode, notFoundDetail)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := periodID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) close(w http.ResponseWriter, r *http.Request) {
	id, err := periodID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in CloseInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	p, err := h.svc.Close(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) reopen(w http.ResponseWriter, r *http.Request) {
	id, err := periodID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in ReopenInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	p, err := h.svc.Reopen(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) note(w http.ResponseWriter, r *http.Request) {
	id, err := periodID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in NoteInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	p, err := h.svc.UpdateNote(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	id, err := periodID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	items, err := h.svc.Events(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
