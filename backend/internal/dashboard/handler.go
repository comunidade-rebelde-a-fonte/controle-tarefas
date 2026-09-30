package dashboard

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/company"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe /dashboard e /backlog.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas [PRD §41.9].
func (h *Handler) Routes(r chi.Router) {
	r.Get("/dashboard", h.dashboard)
	r.Get("/dashboard/attention", h.attention)
	r.Get("/dashboard/history", h.history)
	r.Get("/backlog", h.backlog)
}

func (h *Handler) backlog(w http.ResponseWriter, r *http.Request) {
	p, err := ParseBacklog(r.URL.Query())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	b, err := h.svc.Backlog(r.Context(), p)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, b)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := parseCompetence(q, "competence")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	f, err := parseFilters(q)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	hist, err := h.svc.History(r.Context(), c, f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, hist)
}

func (h *Handler) attention(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := parseCompetence(q, "competence")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	f, err := parseFilters(q)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	a, err := h.svc.Attention(r.Context(), c, f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a)
}

// parseCompetence lê a competência obrigatória da query.
func parseCompetence(q url.Values, name string) (comp.Competence, error) {
	c, err := comp.Parse(q.Get(name))
	if err != nil {
		return c, validate.Field(name, "Informe a competência no formato AAAA-MM.")
	}
	return c, nil
}

// parseFilters lê regime_group, tax_regime e responsible_id.
func parseFilters(q url.Values) (Filters, error) {
	var f Filters
	if v := strings.ToUpper(strings.TrimSpace(q.Get("regime_group"))); v != "" {
		if v != company.GroupNormal && v != company.GroupSimples {
			return f, validate.Field("regime_group", "Use NORMAL ou SIMPLES.")
		}
		f.RegimeGroup = &v
	}
	if v := strings.TrimSpace(q.Get("tax_regime")); v != "" {
		if company.RegimeGroup(v) == "" {
			return f, validate.Field("tax_regime", "Regime inválido.")
		}
		f.TaxRegime = &v
	}
	if v := strings.TrimSpace(q.Get("responsible_id")); v != "" {
		if !httpx.IsUUID(v) {
			return f, validate.Field("responsible_id", "Identificador inválido.")
		}
		f.ResponsibleID = &v
	}
	return f, nil
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, err := parseCompetence(q, "competence")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	f, err := parseFilters(q)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	d, err := h.svc.Build(r.Context(), c, f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}
