package report

import (
	"context"
	"net/url"
	"sort"

	"controle/internal/dashboard"
)

func obligationName(it dashboard.BacklogItem) string {
	if it.ObligationType == nil {
		return "Fechamento do módulo"
	}
	return it.ObligationType.Name
}

// pending lista as pendências da competência agrupadas por módulo [PRD §36.3]. Por padrão,
// toda obrigação aplicável ainda não final; state=pending|not_started|in_progress|overdue
// restringe à classe dos indicadores do dashboard.
func (h *Handler) pending(ctx context.Context, q url.Values) (built, error) {
	c, err := requiredCompetence(q)
	if err != nil {
		return built{}, err
	}
	p, err := backlogParams(q, c, false, "")
	if err != nil {
		return built{}, err
	}
	p.Sort = dashboard.SortCompany
	items, err := h.allBacklog(ctx, p)
	if err != nil {
		return built{}, err
	}
	sort.SliceStable(items, func(i, j int) bool { return moduleRank(items[i].Module) < moduleRank(items[j].Module) })

	overdue := 0
	rows := make([][]string, len(items))
	for i, it := range items {
		if it.DaysOverdue > 0 {
			overdue++
		}
		rows[i] = []string{
			moduleLabels[it.Module], it.Company.InternalCode, clip(it.Company.LegalName, 34), clip(obligationName(it), 24),
			clip(it.StatusLabel, 22), date(it.DueDate), days(it.DaysOverdue), clip(responsible(it.Responsible), 20),
		}
	}
	summary := append([]Field{
		{"Pendências", itoa(len(items))},
		{"Empresas com pendências", itoa(distinctCompanies(items))},
		{"Atrasadas", itoa(overdue)},
	}, moduleCounts(items)...)

	return built{file: c.String(), doc: Document{
		Title:      "Pendências por módulo",
		Competence: &c,
		Filters:    h.backlogFilters(ctx, p, true),
		Summary:    summary,
		Columns: []Column{
			{"Módulo", 3}, {"Código", 2}, {"Empresa", 8}, {"Obrigação", 5}, {"Status", 5}, {"Vencimento", 3},
			{"Atraso (dias)", 3}, {"Responsável", 4},
		},
		Rows:      rows,
		Landscape: true,
	}}, nil
}
