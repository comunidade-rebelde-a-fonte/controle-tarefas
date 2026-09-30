package report

import (
	"context"
	"net/url"
	"slices"

	"controle/internal/company"
	"controle/internal/dashboard"
	"controle/internal/grid"
	comp "controle/internal/platform/competence"
)

// companyStatus reúne, por empresa, o progresso de cada módulo e as contagens do backlog.
type companyStatus struct {
	code, name, regime string
	modules            map[string]grid.Row
	open, overdue      int
}

// general é a situação geral da competência [PRD §36.3]: os KPIs do dashboard e, por
// empresa, o progresso e o estado de cada módulo, as obrigações em aberto e as atrasadas.
func (h *Handler) general(ctx context.Context, q url.Values) (built, error) {
	c, err := requiredCompetence(q)
	if err != nil {
		return built{}, err
	}
	f, err := dashboard.ParseFilters(q)
	if err != nil {
		return built{}, err
	}
	d, err := h.dash.Build(ctx, c, f)
	if err != nil {
		return built{}, err
	}
	companies, err := h.companyStatuses(ctx, c, f)
	if err != nil {
		return built{}, err
	}

	k := d.KPIs
	doc := Document{
		Title:      "Situação geral da competência",
		Competence: &c,
		Filters:    h.dashboardFilters(ctx, f),
		Summary: []Field{
			{Label: "Empresas", Value: itoa(len(companies))},
			{Label: "Obrigações aplicáveis", Value: itoa(k.Applicable.Value)},
			{Label: "Concluídas", Value: itoa(k.Done.Value)},
			{Label: "Em andamento", Value: itoa(k.InProgress.Value)},
			{Label: "Não iniciadas", Value: itoa(k.NotStarted.Value)},
			{Label: "Pendentes", Value: itoa(k.Pending.Value)},
			{Label: "Atrasadas", Value: itoa(k.Overdue.Value)},
			{Label: "Progresso operacional", Value: pct(k.OperationalProgress)},
			{Label: "Conclusão", Value: pct(k.FinalCompletion)},
			{Label: "Empresas prontas para fechar", Value: itoa(k.ReadyToClose.Value)},
			{Label: "Módulos abertos em competências anteriores", Value: itoa(k.PreviousOpen.Value)},
		},
		Columns: []Column{
			{Title: "Código", Weight: 2}, {Title: "Empresa", Weight: 8}, {Title: "Regime", Weight: 4},
		},
		Landscape: true,
	}
	for _, m := range moduleOrder {
		doc.Columns = append(doc.Columns, Column{Title: moduleLabels[m], Weight: 4})
	}
	doc.Columns = append(doc.Columns, Column{Title: "Em aberto", Weight: 3}, Column{Title: "Atrasadas", Weight: 3})

	for _, cs := range companies {
		row := []string{cs.code, cs.name, cs.regime}
		for _, m := range moduleOrder {
			row = append(row, moduleCell(cs.modules, m))
		}
		row = append(row, itoa(cs.open), itoa(cs.overdue))
		doc.Rows = append(doc.Rows, row)
	}
	return built{doc: doc, file: c.String()}, nil
}

// moduleCell resume o módulo da empresa: progresso operacional e estado ("—" sem o módulo).
func moduleCell(rows map[string]grid.Row, module string) string {
	r, ok := rows[module]
	if !ok {
		return "—"
	}
	state := "aberto"
	if r.Period.State == "CLOSED" {
		state = "fechado"
	}
	return pct(r.Progress.OperationalPct) + " · " + state
}

// companyStatuses monta as linhas por empresa: grades dos módulos (progresso e estado) e o
// backlog da competência (em aberto e atrasadas, contadas uma vez pelo módulo principal).
func (h *Handler) companyStatuses(ctx context.Context, c comp.Competence, f dashboard.Filters) ([]*companyStatus, error) {
	specs := map[string][]grid.Spec{
		company.ModuleDeclarations: {grid.DeclarationsSpec()},
		company.ModulePayroll:      {grid.PayrollSpec()},
		company.ModuleAccounting:   {grid.AccountingSpec()},
	}
	for _, g := range []string{grid.GroupParamNormal, grid.GroupParamSimples} {
		spec, err := grid.FiscalSpec(g)
		if err != nil {
			return nil, err
		}
		specs[company.ModuleFiscal] = append(specs[company.ModuleFiscal], spec)
	}

	byID := map[string]*companyStatus{}
	get := func(id, code, name string) *companyStatus {
		cs, ok := byID[id]
		if !ok {
			cs = &companyStatus{code: code, name: name, modules: map[string]grid.Row{}}
			byID[id] = cs
		}
		return cs
	}
	gf := grid.Filters{TaxRegime: f.TaxRegime, ResponsibleID: f.ResponsibleID}
	for _, m := range moduleOrder {
		for _, spec := range specs[m] {
			g, err := h.grids.BuildGrid(ctx, spec, c, gf)
			if err != nil {
				return nil, err
			}
			for _, r := range g.Rows {
				if f.RegimeGroup != nil && r.Snapshot.RegimeGroup != *f.RegimeGroup {
					continue
				}
				cs := get(r.Company.ID, r.Company.InternalCode, clip(r.Company.LegalName, 40))
				cs.regime = company.RegimeLabels[r.Snapshot.TaxRegime]
				cs.modules[m] = r
			}
		}
	}

	for state, add := range map[string]func(*companyStatus){
		"open":                 func(cs *companyStatus) { cs.open++ },
		dashboard.StateOverdue: func(cs *companyStatus) { cs.overdue++ },
	} {
		p := dashboard.BacklogParams{Competence: &c, State: &state, RegimeGroup: f.RegimeGroup, TaxRegime: f.TaxRegime,
			ResponsibleID: f.ResponsibleID, Sort: dashboard.SortCompany}
		items, err := h.allBacklog(ctx, p)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if cs, ok := byID[it.Company.ID]; ok {
				add(cs)
			}
		}
	}

	out := make([]*companyStatus, 0, len(byID))
	for _, cs := range byID {
		out = append(out, cs)
	}
	slices.SortFunc(out, func(a, b *companyStatus) int {
		if a.code != b.code {
			if a.code < b.code {
				return -1
			}
			return 1
		}
		return 0
	})
	return out, nil
}
