package credential

import (
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/httpx"
)

// Handler expõe /credential-controls e /companies/{id}/credential-controls [PRD §41.11].
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/credential-controls", h.list)
	r.Get("/companies/{id}/credential-controls", h.forCompany)
	r.Put("/companies/{id}/credential-controls/{type}", h.update)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	out, err := h.svc.List(r.Context(), ListParams{Filter: q.Get("filter"), Type: q.Get("type")})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func companyID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", companyNotFound, companyMissing)
}

func (h *Handler) forCompany(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.ForCompany(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	typ := chi.URLParam(r, "type")
	if !slices.Contains(Types, typ) {
		httpx.WriteError(w, r, apperr.NotFound("CREDENTIAL_TYPE_NOT_FOUND", "Tipo inválido: use RFB_POWER_OF_ATTORNEY ou DIGITAL_CERTIFICATE."))
		return
	}
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out, err := h.svc.Update(r.Context(), id, typ, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
