package grid

import (
	"controle/internal/company"
	"controle/internal/platform/validate"
)

// Agrupamentos da grade Fiscal na URL [PRD §28.1, §41.6].
const (
	GroupParamNormal  = "normal"
	GroupParamSimples = "simples"
)

// FiscalSpec devolve a grade Fiscal do agrupamento: Normais (Presumido, Real, Arbitrado,
// Imune/Isenta) ou Simples Nacional, pelo regime do snapshot da competência [PRD §13.1, §14].
func FiscalSpec(group string) (Spec, error) {
	spec := Spec{Module: "FISCAL", PeriodModule: "FISCAL", Group: group, Closable: true}
	switch group {
	case GroupParamNormal:
		spec.RegimeGroup = company.GroupNormal
	case GroupParamSimples:
		spec.RegimeGroup = company.GroupSimples
	default:
		return Spec{}, validate.Field("group", "Informe normal ou simples.")
	}
	for _, r := range company.Regimes {
		if company.RegimeGroup(r) == spec.RegimeGroup {
			spec.Regimes = append(spec.Regimes, r)
		}
	}
	return spec, nil
}
