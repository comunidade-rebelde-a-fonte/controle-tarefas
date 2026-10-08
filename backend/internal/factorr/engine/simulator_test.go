package engine

import (
	"testing"
)

// test_simulador.py — casos tabelados do Plano §5.2 da origem.

func engineResult(t *testing.T, revenue, proLabore string) Result {
	t.Helper()
	return Calculate(testPA, nil, revenueSeries(revenue, proLabore), tables2018(t), policyPortfolio)
}

func simParams(mod ...func(*SimulationParams)) SimulationParams {
	p := SimulationParams{
		Target: d("0.30"), ProLaboreFraction: d("1"), INSS: d("0.11"), IRRF: d("0.275"),
		HorizonMonths: 12, MinAnnualSavings: d("6000"),
	}
	for _, m := range mod {
		m(&p)
	}
	return p
}

func verdict(t *testing.T, r SimulationResult, want Verdict) {
	t.Helper()
	if r.Verdict == nil || *r.Verdict != want {
		t.Fatalf("veredito = %v; esperado %s", r.Verdict, want)
	}
}

func TestAC010_AlreadyOnTarget(t *testing.T) {
	r := Simulate(engineResult(t, "50000", "15000"), simParams())
	verdict(t, r, VerdictOnTarget)
	eq(t, "reforco_12m", &r.TopUp12m, "0")
	eq(t, "custo_total", &r.TotalCost, "0")
}

// AC-010: valores completos, todos calculados por código.
func TestAC010_CorrectFullCalculation(t *testing.T) {
	// RBT12 600 mil, FS12 120 mil (20%): reforço até 30% = 60 mil/12 meses = 5 mil/mês.
	r := Simulate(engineResult(t, "50000", "10000"), simParams())
	if r.Status != StatusOK {
		t.Fatalf("status %s", r.Status)
	}
	eq(t, "reforco_12m", &r.TopUp12m, "60000.00")
	eq(t, "reforco_mensal", &r.MonthlyTopUp, "5000")
	eq(t, "pro_labore_extra_mensal", &r.ExtraProLaboreMonthly, "5000")
	eq(t, "pro_labore_extra_horizonte", &r.ExtraProLaboreHorizon, "60000")
	eq(t, "custo_inss", &r.INSSCost, "6600.00")
	eq(t, "custo_irrf", &r.IRRFCost, "16500.000")
	eq(t, "custo_total", &r.TotalCost, "23100.000")
	eq(t, "economia_12m", r.Savings12m, "43740.00")
	eq(t, "economia_horizonte", r.SavingsHorizon, "43740.00")
	eq(t, "liquido", r.Net, "20640.000")
	verdict(t, r, VerdictCorrect)
}

func TestSimulate_DontPushWhenSavingsBelowFloorEvenWithPositiveNet(t *testing.T) {
	// RBT12 180 mil (faixa 1): economia = (15,5% - 6%) × 180 mil = 17.100; piso 20.000.
	r := Simulate(engineResult(t, "15000", "2000"), simParams(func(p *SimulationParams) {
		p.MinAnnualSavings, p.IRRF = d("20000"), d("0")
	}))
	eq(t, "economia_12m", r.Savings12m, "17100.00")
	if r.Net.Sign() <= 0 {
		t.Fatalf("líquido = %s; esperado positivo", r.Net)
	}
	verdict(t, r, VerdictDontPush)
}

func TestSimulate_DontPushWhenNetNotPositive(t *testing.T) {
	r := Simulate(engineResult(t, "50000", "10000"), simParams(func(p *SimulationParams) { p.INSS, p.IRRF = d("0.30"), d("0.50") }))
	if r.Net.Sign() > 0 {
		t.Fatalf("líquido = %s", r.Net)
	}
	verdict(t, r, VerdictDontPush)
}

func TestSimulate_ProLaboreFraction(t *testing.T) {
	for _, tc := range []struct{ fraction, cost string }{{"0", "0"}, {"1", "23100.000"}, {"0.5", "11550.0000"}} {
		r := Simulate(engineResult(t, "50000", "10000"), simParams(func(p *SimulationParams) { p.ProLaboreFraction = d(tc.fraction) }))
		eq(t, "custo_total fração "+tc.fraction, &r.TotalCost, tc.cost)
	}
}

func TestSimulate_Horizon(t *testing.T) {
	for _, tc := range []struct {
		months          int
		savings, extraH string
	}{{6, "21870.00", "30000"}, {12, "43740.00", "60000"}} {
		r := Simulate(engineResult(t, "50000", "10000"), simParams(func(p *SimulationParams) { p.HorizonMonths = tc.months }))
		eq(t, "economia_horizonte", r.SavingsHorizon, tc.savings)
		eq(t, "pro_labore_extra_horizonte", &r.ExtraProLaboreHorizon, tc.extraH)
	}
}

func TestSimulate_EditableINSSAndIRRF(t *testing.T) {
	a := Simulate(engineResult(t, "50000", "10000"), simParams(func(p *SimulationParams) { p.INSS, p.IRRF = d("0.11"), d("0") }))
	b := Simulate(engineResult(t, "50000", "10000"), simParams(func(p *SimulationParams) { p.INSS, p.IRRF = d("0.20"), d("0.15") }))
	eq(t, "custo_total a", &a.TotalCost, "6600.00")
	eq(t, "custo_total b", &b.TotalCost, "21000.00")
}

func TestSimulate_TargetMayDifferFromPolicy(t *testing.T) {
	r := Simulate(engineResult(t, "50000", "15000"), simParams(func(p *SimulationParams) { p.Target = d("0.32") })) // 30% < 32%
	if r.Verdict != nil && *r.Verdict == VerdictOnTarget {
		t.Fatal("30% não está na meta de 32%")
	}
	eq(t, "reforco_12m", &r.TopUp12m, "12000.00")
}

func TestSimulate_InsufficientDataInventsNoVerdict(t *testing.T) {
	r := Simulate(engineResult(t, "0", "5000"), simParams())
	if r.Status != StatusInsufficient || r.Verdict != nil || r.Reason == nil || *r.Reason != ReasonRBT12Zero {
		t.Fatalf("status %s, veredito %v, motivo %v", r.Status, r.Verdict, r.Reason)
	}
}

func TestSimulate_NegativeSavingsInBracket6DoesNotSayCorrect(t *testing.T) {
	r := Simulate(engineResult(t, "400000", "80000"), simParams(func(p *SimulationParams) { p.IRRF, p.INSS = d("0"), d("0") }))
	if r.Savings12m == nil || r.Savings12m.Sign() >= 0 {
		t.Fatalf("economia = %v; esperado negativa", r.Savings12m)
	}
	verdict(t, r, VerdictDontPush)
}
