package report

import (
	"context"
	"net/url"

	"controle/internal/dashboard"
)

// overdue lista as obrigações atrasadas da competência, das mais atrasadas para as menos
// [PRD §36.3]. Atraso é calculado pela API (RN-013), nunca um status manual.
func (h *Handler) overdue(ctx context.Context, q url.Values) (built, error) {
	c, err := requiredCompetence(q)
	if err != nil {
		return built{}, err
	}
	p, err := backlogParams(q, c, false, dashboard.StateOverdue)
	if err != nil {
		return built{}, err
	}
	p.Sort = dashboard.SortDaysOverdue
	items, err := h.allBacklog(ctx, p)
	if err != nil {
		return built{}, err
	}

	maxDays := 0
	rows := make([][]string, len(items))
	for i, it := range items {
		maxDays = max(maxDays, it.DaysOverdue)
		rows[i] = []string{
			it.Company.InternalCode, clip(it.Company.LegalName, 34), moduleLabels[it.Module], clip(obligationName(it), 24),
			clip(it.StatusLabel, 22), date(it.DueDate), days(it.DaysOverdue), clip(responsible(it.Responsible), 20),
		}
	}
	summary := append([]Field{
		{"Obrigações atrasadas", itoa(len(items))},
		{"Empresas afetadas", itoa(distinctCompanies(items))},
		{"Maior atraso (dias)", itoa(maxDays)},
	}, moduleCounts(items)...)

	return built{file: c.String(), doc: Document{
		Title:      "Obrigações atrasadas",
		Competence: &c,
		Filters:    h.backlogFilters(ctx, p, false),
		Summary:    summary,
		Columns: []Column{
			{"Código", 2}, {"Empresa", 8}, {"Módulo", 3}, {"Obrigação", 5}, {"Status", 5}, {"Vencimento", 3},
			{"Atraso (dias)", 3}, {"Responsável", 4},
		},
		Rows:      rows,
		Landscape: true,
	}}, nil
}
