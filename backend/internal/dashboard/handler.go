package dashboard

import (
	"net/http"
	"net/url"

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

// ParseFilters lê os filtros do dashboard (usados também pelos relatórios PDF).
func ParseFilters(q url.Values) (Filters, error) { return parseFilters(q) }

// parseFilters lê regime_group, tax_regime e responsible_id (os filtros de empresa, D-038).
func parseFilters(q url.Values) (Filters, error) {
	f, err := company.ParseFilter(q)
	return Filters(f), err
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
