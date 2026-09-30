package grid

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"controle/internal/company"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
)

// Handler expõe /modules/{module}/grid.
type Handler struct{ b *Builder }

// NewHandler cria o handler.
func NewHandler(b *Builder) *Handler { return &Handler{b: b} }

// Routes registra as rotas [PRD §41.6].
func (h *Handler) Routes(r chi.Router) {
	r.Get("/modules/fiscal/grid", h.fiscal)
}

func (h *Handler) fiscal(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	spec, err := FiscalSpec(strings.ToLower(q.Get("group")))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	h.serve(w, r, spec)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request, spec Spec) {
	q := r.URL.Query()
	c, err := comp.Parse(q.Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validate.Field("competence", "Informe a competência no formato AAAA-MM."))
		return
	}
	f, err := ParseFilters(q)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	g, err := h.b.BuildGrid(r.Context(), spec, c, f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, g)
}

var statusCodeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)

// ParseFilters lê os filtros da query string (nomes iguais aos da URL da tela).
func ParseFilters(q url.Values) (Filters, error) {
	var f Filters
	opt := func(name string) *string {
		if v := strings.TrimSpace(q.Get(name)); v != "" {
			return &v
		}
		return nil
	}
	if v := opt("tax_regime"); v != nil {
		if company.RegimeGroup(*v) == "" {
			return f, validate.Field("tax_regime", "Regime inválido.")
		}
		f.TaxRegime = v
	}
	for name, dst := range map[string]**string{"responsible_id": &f.ResponsibleID, "obligation_type_id": &f.ObligationTypeID} {
		if v := opt(name); v != nil {
			if !httpx.IsUUID(*v) {
				return f, validate.Field(name, "Identificador inválido.")
			}
			*dst = v
		}
	}
	for name, dst := range map[string]**string{"status": &f.Status, "closing_status": &f.ClosingStatus} {
		if v := opt(name); v != nil {
			if !statusCodeRe.MatchString(*v) {
				return f, validate.Field(name, "Status inválido.")
			}
			up := strings.ToUpper(*v)
			*dst = &up
		}
	}
	if f.ObligationTypeID != nil && f.Status == nil {
		return f, validate.Field("status", "Informe o status da coluna escolhida.")
	}
	if v := opt("period_state"); v != nil {
		up := strings.ToUpper(*v)
		if up != "OPEN" && up != "CLOSED" {
			return f, validate.Field("period_state", "Use OPEN ou CLOSED.")
		}
		f.PeriodState = &up
	}
	if v := opt("note"); v != nil {
		switch strings.ToLower(*v) {
		case "with":
			t := true
			f.HasNote = &t
		case "without":
			n := false
			f.HasNote = &n
		default:
			return f, validate.Field("note", "Use with ou without.")
		}
	}
	for name, dst := range map[string]*bool{"overdue": &f.Overdue, "pending": &f.Pending, "not_started": &f.NotStarted, "ready_to_close": &f.ReadyToClose} {
		if v := opt(name); v != nil {
			b, err := strconv.ParseBool(*v)
			if err != nil {
				return f, validate.Field(name, "Use true ou false.")
			}
			*dst = b
		}
	}
	return f, nil
}
