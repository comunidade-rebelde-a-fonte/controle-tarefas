package report

import (
	"context"
	"net/url"

	"controle/internal/dashboard"
	comp "controle/internal/platform/competence"
)

func displayOrDash(c *comp.Competence) string {
	if c == nil {
		return "—"
	}
	return c.Display()
}

// backlog lista o que ficou em aberto em competências anteriores à informada (plan R8):
// obrigações aplicáveis não finais e módulos abertos sem pendências (falta fechar)
// [PRD §25, §36.3].
func (h *Handler) backlog(ctx context.Context, q url.Values) (built, error) {
	c, err := requiredCompetence(q)
	if err != nil {
		return built{}, err
	}
	p, err := backlogParams(q, c, true, "")
	if err != nil {
		return built{}, err
	}
	if q.Get("sort") == "" {
		p.Sort = dashboard.SortDaysOverdue
	}
	items, err := h.allBacklog(ctx, p)
	if err != nil {
		return built{}, err
	}

	obligations, modules, maxDays := 0, 0, 0
	var oldest *comp.Competence
	rows := make([][]string, len(items))
	for i, it := range items {
		if it.Kind == dashboard.BacklogModule {
			modules++
		} else {
			obligations++
		}
		maxDays = max(maxDays, it.DaysOverdue)
		if oldest == nil || it.Competence.String() < oldest.String() {
			oldest = &it.Competence
		}
		rows[i] = []string{
			it.Competence.Display(), it.Company.InternalCode, clip(it.Company.LegalName, 32), moduleLabels[it.Module],
			clip(obligationName(it), 22), clip(it.StatusLabel, 20), date(it.DueDate), days(it.DaysOverdue),
			clip(responsible(it.Responsible), 18),
		}
	}
	filters := append([]Field{{"Competências", "anteriores a " + c.Display()}}, h.backlogFilters(ctx, p, true)...)

	return built{file: c.String(), doc: Document{
		Title:      "Backlog",
		Competence: &c,
		Filters:    filters,
		Summary: []Field{
			{"Itens no backlog", itoa(len(items))},
			{"Obrigações em aberto", itoa(obligations)},
			{"Módulos a fechar", itoa(modules)},
			{"Empresas", itoa(distinctCompanies(items))},
			{"Competência mais antiga", displayOrDash(oldest)},
			{"Maior atraso (dias)", itoa(maxDays)},
		},
		Columns: []Column{
			{"Competência", 3}, {"Código", 2}, {"Empresa", 8}, {"Módulo", 3}, {"Obrigação", 5}, {"Status", 5},
			{"Vencimento", 3}, {"Atraso (dias)", 3}, {"Responsável", 4},
		},
		Rows:      rows,
		Landscape: true,
	}}, nil
}
