// Package competence gera as competências mensais: snapshot da configuração da empresa,
// períodos de módulo e instâncias de obrigações [PRD §10, §39; plan M2].
package competence

import (
	"slices"

	"controle/internal/company"
)

// Status da empresa "Sem folha" configurados no catálogo [PRD §17.4, plan D7].
const (
	NoPayrollNoMovement    = "NO_MOVEMENT"    // gera aplicável com status Sem movimento
	NoPayrollNotApplicable = "NOT_APPLICABLE" // gera com applicable = false
)

// statusNotApplicable é o código "Não se aplica" nos conjuntos que o possuem.
const statusNotApplicable = "NOT_APPLICABLE"

// Snapshot é a configuração da empresa congelada na competência (D4). Só os campos que
// a resolução usa: regime vigente, tipo de folha e módulos ativos.
type Snapshot struct {
	TaxRegime     string
	PayrollType   string
	ActiveModules []string
}

// RegimeRule é a regra tipo × regime do catálogo (D2, D3).
type RegimeRule struct {
	DefaultApplicable bool
	CountsInProgress  bool
	Active            bool
}

// CatalogType é um tipo de obrigação do catálogo, com o que a resolução precisa.
type CatalogType struct {
	ID                  string
	Code                string
	PrimaryModule       string
	Active              bool
	InitialStatusCode   string
	StatusWhenNoPayroll string   // "" | NO_MOVEMENT | NOT_APPLICABLE
	StatusCodes         []string // códigos do conjunto de status do tipo
	RegimeRules         map[string]RegimeRule
}

// PlannedInstance é a instância a criar para empresa × competência × tipo.
type PlannedInstance struct {
	ObligationTypeID string
	Code             string
	Applicable       bool
	CountsInProgress bool
	StatusCode       string
	RegimeSnapshot   string
}

// Resolve decide quais instâncias gerar para uma empresa na competência, na ordem do
// catálogo [plan M2 passos 5 e 7]:
//   - só tipos ativos, do módulo primário ativo no snapshot e com regra ativa para o
//     regime (D3); os demais não são gerados, mesmo com override;
//   - applicable: override da empresa (APPLICABLE / NOT_APPLICABLE) > "Sem folha"
//     configurado como NOT_APPLICABLE > regra padrão (D2, D6, D7); não aplicável também é
//     gerado, para ficar visível;
//   - counts_in_progress vem da regra tipo × regime (D2);
//   - status inicial: NOT_APPLICABLE quando não aplicável e o conjunto tem esse código;
//     NO_MOVEMENT para "Sem folha" configurado assim (se o conjunto tiver o código);
//     senão initial_status_code.
//
// overrides mapeia obligation_type_id → DEFAULT | APPLICABLE | NOT_APPLICABLE.
func Resolve(snap Snapshot, catalog []CatalogType, overrides map[string]string) []PlannedInstance {
	noPayroll := snap.PayrollType == company.PayrollNone
	var out []PlannedInstance
	for _, t := range catalog {
		if !t.Active || !slices.Contains(snap.ActiveModules, t.PrimaryModule) {
			continue
		}
		rule, ok := t.RegimeRules[snap.TaxRegime]
		if !ok || !rule.Active {
			continue
		}

		override := overrides[t.ID]
		defaultApplicable := rule.DefaultApplicable
		if noPayroll && t.StatusWhenNoPayroll == NoPayrollNotApplicable {
			defaultApplicable = false
		}
		applicable := *company.ResolveApplicable(&defaultApplicable, override)

		status := t.InitialStatusCode
		switch {
		case !applicable:
			if slices.Contains(t.StatusCodes, statusNotApplicable) {
				status = statusNotApplicable
			}
		case noPayroll && t.StatusWhenNoPayroll == NoPayrollNoMovement:
			if slices.Contains(t.StatusCodes, NoPayrollNoMovement) {
				status = NoPayrollNoMovement
			}
		}

		out = append(out, PlannedInstance{
			ObligationTypeID: t.ID,
			Code:             t.Code,
			Applicable:       applicable,
			CountsInProgress: rule.CountsInProgress,
			StatusCode:       status,
			RegimeSnapshot:   snap.TaxRegime,
		})
	}
	return out
}
