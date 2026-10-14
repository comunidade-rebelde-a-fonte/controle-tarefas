package engine

import "controle/internal/platform/decimal"

// Verdict é o veredito do simulador [REQ-013] (Veredito).
type Verdict string

// Vereditos.
const (
	VerdictOnTarget Verdict = "ja_na_meta"
	VerdictCorrect  Verdict = "corrigir"
	VerdictDontPush Verdict = "nao_forcar"
)

// SimulationParams são os parâmetros da simulação (ParametrosSimulacao). Hipóteses da v1 da
// origem: o reforço de folha vem de pró-labore (fração) e o custo é INSS do sócio + IRRF; a CPP
// não entra no custo porque, nos Anexos III e V, já está no DAS; INSS sem teto e sem múltiplos
// sócios (Fase 2 da origem).
type SimulationParams struct {
	Target            decimal.Decimal `json:"meta"`
	ProLaboreFraction decimal.Decimal `json:"fracao_pro_labore"`
	INSS              decimal.Decimal `json:"inss"`
	IRRF              decimal.Decimal `json:"irrf"`
	HorizonMonths     int             `json:"horizonte_meses"`
	MinAnnualSavings  decimal.Decimal `json:"piso_economia_anual"`
}

// SimulationResult é o resultado da simulação (ResultadoSimulacao).
type SimulationResult struct {
	Status                Status           `json:"status"`
	Reason                *Reason          `json:"motivo"`
	Verdict               *Verdict         `json:"veredito"`
	Params                SimulationParams `json:"parametros"`
	CurrentFactor         *decimal.Decimal `json:"fator_atual"`
	TopUp12m              decimal.Decimal  `json:"reforco_12m"`
	MonthlyTopUp          decimal.Decimal  `json:"reforco_mensal"`
	ExtraProLaboreMonthly decimal.Decimal  `json:"pro_labore_extra_mensal"`
	ExtraProLaboreHorizon decimal.Decimal  `json:"pro_labore_extra_horizonte"`
	INSSCost              decimal.Decimal  `json:"custo_inss"`
	IRRFCost              decimal.Decimal  `json:"custo_irrf"`
	TotalCost             decimal.Decimal  `json:"custo_total"`
	Savings12m            *decimal.Decimal `json:"economia_12m"`
	SavingsHorizon        *decimal.Decimal `json:"economia_horizonte"`
	Net                   *decimal.Decimal `json:"liquido"`
}

// Simulate decide se vale corrigir a folha até a meta (simular). Todo valor é calculado aqui,
// nunca pelo LLM [AC-010]: o consultor só redige a partir deste resultado.
func Simulate(r Result, p SimulationParams) SimulationResult {
	if r.Status != StatusOK || r.FactorR == nil || r.Savings12m == nil {
		return SimulationResult{
			Status: StatusInsufficient, Reason: r.Reason, Params: p, CurrentFactor: r.FactorR,
			TopUp12m: decimal.Zero, MonthlyTopUp: decimal.Zero,
			ExtraProLaboreMonthly: decimal.Zero, ExtraProLaboreHorizon: decimal.Zero,
			INSSCost: decimal.Zero, IRRFCost: decimal.Zero, TotalCost: decimal.Zero,
		}
	}

	horizon := decimal.FromInt(int64(p.HorizonMonths))
	topUp12m := decimal.Max(decimal.Zero, p.Target.Mul(r.RBT12).Sub(r.FS12))
	monthly := topUp12m.Quo(twelve)
	plMonthly := monthly.Mul(p.ProLaboreFraction)
	plHorizon := plMonthly.Mul(horizon)
	inss := plHorizon.Mul(p.INSS)
	irrf := plHorizon.Mul(p.IRRF)
	total := inss.Add(irrf)
	savingsHorizon := r.Savings12m.Mul(horizon).Quo(twelve)
	net := savingsHorizon.Sub(total)

	var v Verdict
	switch {
	case r.FactorR.Cmp(p.Target) >= 0:
		v = VerdictOnTarget
	case r.Savings12m.Cmp(p.MinAnnualSavings) < 0 || net.Sign() <= 0:
		v = VerdictDontPush
	default:
		v = VerdictCorrect
	}

	return SimulationResult{
		Status: StatusOK, Verdict: &v, Params: p, CurrentFactor: r.FactorR,
		TopUp12m: topUp12m, MonthlyTopUp: monthly,
		ExtraProLaboreMonthly: plMonthly, ExtraProLaboreHorizon: plHorizon,
		INSSCost: inss, IRRFCost: irrf, TotalCost: total,
		Savings12m: r.Savings12m, SavingsHorizon: &savingsHorizon, Net: &net,
	}
}
