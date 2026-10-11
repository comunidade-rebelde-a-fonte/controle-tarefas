package metrics

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
)

// Handler expõe o painel de métricas, os playbooks e o resumo da ficha.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/reconciliation/metrics", h.metrics)
	r.Get("/reconciliation/playbooks", h.playbooks)
	r.Get("/companies/{id}/reconciliation", h.summary)
}

func validation(fields map[string]any) error {
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").WithDetails(map[string]any{"fields": fields})
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	var f Filter
	fields := map[string]any{}
	if v := r.URL.Query().Get("company_id"); v != "" {
		if !httpx.IsUUID(v) {
			fields["company_id"] = "Empresa inválida."
		}
		f.CompanyID = &v
	}
	if v := r.URL.Query().Get("competence"); v != "" {
		c, err := competence.Parse(v)
		if err != nil {
			fields["competence"] = "Use AAAA-MM."
		}
		f.Competence = &c
	}
	if len(fields) > 0 {
		httpx.WriteError(w, r, validation(fields))
		return
	}
	out, err := h.svc.Metrics(r.Context(), f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) playbooks(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Playbooks(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	c, err := competence.Parse(r.URL.Query().Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validation(map[string]any{"competence": "Informe a competência (AAAA-MM)."}))
		return
	}
	out, err := h.svc.Summary(r.Context(), chi.URLParam(r, "id"), c)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
