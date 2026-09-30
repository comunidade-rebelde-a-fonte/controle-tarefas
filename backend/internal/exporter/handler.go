package exporter

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/grid"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
)

// Handler expõe GET /exports/{module}.xlsx [PRD §41.13].
type Handler struct {
	builder *grid.Builder
	q       *store.Queries
	clock   clock.Clock
}

// NewHandler cria o handler.
func NewHandler(builder *grid.Builder, pool *pgxpool.Pool, clk clock.Clock) *Handler {
	return &Handler{builder: builder, q: store.New(pool), clock: clk}
}

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/exports/{file}", h.export)
}

var fixedModules = map[string]struct {
	label string
	spec  func() grid.Spec
}{
	"declarations": {"Declarações", grid.DeclarationsSpec},
	"payroll":      {"Folha", grid.PayrollSpec},
	"accounting":   {"Contábil", grid.AccountingSpec},
	"dctfweb":      {"DCTFWeb", grid.DctfwebSpec},
}

// resolve devolve a grade do módulo (Fiscal exige group=normal|simples).
func resolve(module, group string) (grid.Spec, string, error) {
	if module == "fiscal" {
		spec, err := grid.FiscalSpec(group)
		if err != nil {
			return grid.Spec{}, "", err
		}
		label := "Fiscal — Normais"
		if group == grid.GroupParamSimples {
			label = "Fiscal — Simples Nacional"
		}
		return spec, label, nil
	}
	m, ok := fixedModules[module]
	if !ok {
		return grid.Spec{}, "", apperr.NotFound("EXPORT_NOT_FOUND", "Exportação não encontrada: use fiscal, declarations, payroll, accounting ou dctfweb.")
	}
	return m.spec(), m.label, nil
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	file := chi.URLParam(r, "file")
	module, ok := strings.CutSuffix(strings.ToLower(file), ".xlsx")
	if !ok {
		httpx.WriteError(w, r, apperr.NotFound("EXPORT_NOT_FOUND", "Exportação não encontrada."))
		return
	}
	q := r.URL.Query()
	group := strings.ToLower(q.Get("group"))
	spec, label, err := resolve(module, group)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	c, err := comp.Parse(q.Get("competence"))
	if err != nil {
		httpx.WriteError(w, r, validate.Field("competence", "Informe a competência no formato AAAA-MM."))
		return
	}
	f, err := grid.ParseFilters(q)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	g, err := h.builder.BuildGrid(r.Context(), spec, c, f)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	search := strings.TrimSpace(q.Get("q"))
	rows := make([]grid.Row, 0, len(g.Rows))
	for _, row := range g.Rows {
		if MatchSearch(row.Company, search) {
			rows = append(rows, row)
		}
	}
	meta := Meta{
		ModuleLabel: label,
		Competence:  c.Display(),
		Filters:     h.describeFilters(r.Context(), q, g),
		GeneratedAt: h.clock.Now().In(h.clock.Location()),
	}
	data, err := XLSX(g, rows, meta)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	name := module
	if module == "fiscal" {
		name += "-" + group
	}
	name += "-" + c.String() + ".xlsx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// describeFilters traduz os filtros aplicados para o cabeçalho da planilha.
func (h *Handler) describeFilters(ctx context.Context, q url.Values, g grid.Grid) []string {
	var out []string
	if v := q.Get("tax_regime"); v != "" {
		out = append(out, "Regime: "+regimeLabels[strings.ToUpper(v)])
	}
	if v := q.Get("responsible_id"); v != "" {
		name := v
		if u, err := h.q.GetUser(ctx, v); err == nil {
			name = u.Name
		}
		out = append(out, "Responsável: "+name)
	}
	statusLabel := func(code string, closing bool) string {
		for _, c := range g.Columns {
			if c.IsClosingStage != closing {
				continue
			}
			for _, s := range c.Statuses {
				if strings.EqualFold(s.Code, code) {
					return s.Label
				}
			}
		}
		return code
	}
	if v := q.Get("status"); v != "" {
		text := "Status: " + statusLabel(v, false)
		if t := q.Get("obligation_type_id"); t != "" {
			for _, c := range g.Columns {
				if c.ObligationType.ID == t {
					text += " em " + c.ObligationType.Name
				}
			}
		}
		out = append(out, text)
	}
	if v := q.Get("closing_status"); v != "" {
		out = append(out, "Fechamento: "+statusLabel(v, true))
	}
	if v := q.Get("period_state"); v != "" {
		out = append(out, "Módulo: "+periodLabels[strings.ToUpper(v)])
	}
	switch strings.ToLower(q.Get("note")) {
	case "with":
		out = append(out, "Com observação")
	case "without":
		out = append(out, "Sem observação")
	}
	for _, flag := range []struct{ key, label string }{
		{"overdue", "Somente atrasadas"}, {"pending", "Somente pendentes"},
		{"not_started", "Somente não iniciadas"}, {"ready_to_close", "Somente prontas para fechar"},
	} {
		if q.Get(flag.key) == "true" {
			out = append(out, flag.label)
		}
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		out = append(out, "Busca: "+v)
	}
	return out
}
