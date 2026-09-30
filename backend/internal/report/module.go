package report

import (
	"context"
	"net/url"
	"strings"

	"controle/internal/exporter"
	"controle/internal/grid"
	"controle/internal/platform/validate"
)

func periodSituation(r grid.Row) string {
	switch {
	case r.Period.State == "CLOSED":
		return "Fechado"
	case r.ReadyToClose:
		return "Pronto para fechar"
	}
	return "Aberto"
}

// module é o controle completo de um módulo na competência [PRD §36.3]: uma linha por
// empresa e obrigação, com status, vencimento, atraso e o estado do módulo, sobre a mesma
// grade da tela e com os mesmos filtros (inclusive a busca q). No Fiscal, sem group, o
// relatório traz Normais e Simples Nacional.
func (h *Handler) module(ctx context.Context, q url.Values) (built, error) {
	c, err := requiredCompetence(q)
	if err != nil {
		return built{}, err
	}
	module := strings.ToLower(strings.TrimSpace(q.Get("module")))
	if module == "" {
		return built{}, validate.Field("module", "Informe o módulo: fiscal, declarations, payroll, accounting ou dctfweb.")
	}
	group := strings.ToLower(strings.TrimSpace(q.Get("group")))
	groups := []string{group}
	if module == "fiscal" && group == "" {
		groups = []string{grid.GroupParamNormal, grid.GroupParamSimples}
	}
	f, err := grid.ParseFilters(q)
	if err != nil {
		return built{}, err
	}
	search := strings.TrimSpace(q.Get("q"))

	var (
		label                                          string
		grids                                          []grid.Grid
		companies, applicable, done, blocking, overdue int
		operational, closed, ready                     int
	)
	for _, gp := range groups {
		spec, l, err := exporter.Resolve(module, gp)
		if err != nil {
			return built{}, err
		}
		label = l
		g, err := h.grids.BuildGrid(ctx, spec, c, f)
		if err != nil {
			return built{}, err
		}
		grids = append(grids, g)
	}
	if len(groups) > 1 {
		label = "Fiscal"
	}

	var rows [][]string
	for _, g := range grids {
		for _, r := range g.Rows {
			if !exporter.MatchSearch(r.Company, search) {
				continue
			}
			companies++
			applicable += r.Progress.Denominator
			done += r.Progress.FinalDone
			operational += r.Progress.OperationalDone
			blocking += r.Progress.Blocking
			overdue += r.Overdue
			if r.Period.State == "CLOSED" {
				closed++
			} else if r.ReadyToClose {
				ready++
			}
			state := periodSituation(r)
			if !g.Closable {
				state = "—"
			}
			resp := "—"
			if r.Snapshot.Responsible != nil {
				resp = r.Snapshot.Responsible.Name
			}
			for _, col := range g.Columns {
				cell, ok := r.Cells[col.ObligationType.Code]
				status, due, late := "—", "—", "—"
				switch {
				case !ok:
				case !cell.Applicable:
					status = "Não se aplica"
				default:
					status, due = cell.StatusLabel, date(cell.DueDateEffective)
					if cell.IsOverdue && cell.DueDateEffective != nil {
						late = days(int(g.Today.Sub(cell.DueDateEffective.Time).Hours() / 24))
					}
				}
				rows = append(rows, []string{
					r.Company.InternalCode, clip(r.Company.LegalName, 30), clip(col.ObligationType.Name, 32),
					clip(status, 22), due, late, state, clip(resp, 18),
				})
			}
		}
	}

	ratio := func(n int) string {
		if applicable == 0 {
			return pct(1)
		}
		return pct(float64(n) / float64(applicable))
	}
	var filters []Field
	for _, d := range exporter.DescribeFilters(ctx, h.q, q, grids[0]) {
		filters = append(filters, Field{Label: d})
	}
	file := module
	if len(groups) == 1 && group != "" {
		file += "-" + group
	}
	return built{file: file + "-" + c.String(), doc: Document{
		Title:      "Controle completo — " + label,
		Competence: &c,
		Filters:    filters,
		Summary: []Field{
			{"Empresas", itoa(companies)},
			{"Obrigações aplicáveis", itoa(applicable)},
			{"Concluídas", itoa(done)},
			{"Pendentes", itoa(blocking)},
			{"Atrasadas", itoa(overdue)},
			{"Progresso operacional", ratio(operational)},
			{"Conclusão", ratio(done)},
			{"Módulos fechados", itoa(closed)},
			{"Prontos para fechar", itoa(ready)},
		},
		Columns: []Column{
			{"Código", 2}, {"Empresa", 7}, {"Obrigação", 7}, {"Status", 5}, {"Vencimento", 3}, {"Atraso (dias)", 3},
			{"Situação do módulo", 5}, {"Responsável", 4},
		},
		Rows:      rows,
		Landscape: true,
	}}, nil
}
