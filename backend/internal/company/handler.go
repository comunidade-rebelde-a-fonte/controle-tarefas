package company

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe /companies. Não há DELETE: empresas são inativadas [RN-008].
type Handler struct {
	svc *Service
}

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/companies", h.list)
	r.Post("/companies", h.create)
	r.Get("/companies/{id}", h.get)
	r.Patch("/companies/{id}", h.update)
	r.Post("/companies/{id}/deactivate", h.deactivate)
	r.Get("/companies/{id}/tax-regimes", h.listRegimes)
	r.Post("/companies/{id}/tax-regimes", h.changeRegime)
	r.Get("/companies/{id}/modules", h.getModules)
	r.Put("/companies/{id}/modules/{module}", h.setModule)
	r.Get("/companies/{id}/obligation-overrides", h.getOverrides)
	r.Put("/companies/{id}/obligation-overrides/{obligationTypeId}", h.setOverride)
	r.Get("/companies/{id}/history", h.history)
}

func companyID(r *http.Request) (string, error) {
	return httpx.PathUUID(r, "id", "COMPANY_NOT_FOUND", "Empresa não encontrada.")
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := ListFilter{Q: q.Get("q"), Cnpj: q.Get("cnpj"), Code: q.Get("code"), TaxRegime: q.Get("tax_regime"), Module: q.Get("module")}
	if f.TaxRegime != "" && RegimeGroup(f.TaxRegime) == "" {
		httpx.WriteError(w, r, validate.Field("tax_regime", "Regime inválido."))
		return
	}
	if f.Module != "" && !IsModule(f.Module) {
		httpx.WriteError(w, r, validate.Field("module", "Módulo inválido."))
		return
	}
	if raw := q.Get("active"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			httpx.WriteError(w, r, validate.Field("active", "Use true ou false."))
			return
		}
		f.Active = &b
	}
	items, err := h.svc.List(r.Context(), f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

type deactivateRequest struct {
	Version *int32 `json:"version"`
}

func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in deactivateRequest
	if r.ContentLength > 0 {
		if err := httpx.DecodeJSON(w, r, &in); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	c, err := h.svc.Deactivate(r.Context(), id, in.Version)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}
