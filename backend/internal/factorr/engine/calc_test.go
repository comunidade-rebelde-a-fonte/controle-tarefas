package engine

import (
	"slices"
	"testing"
	"time"

	"controle/internal/platform/competence"
)

// test_fator_r.py — casos tabelados do Plano §5.1 da origem.

func calc(t *testing.T, ms []Movement, opts ...func(*calcArgs)) Result {
	t.Helper()
	a := calcArgs{tables: tables2018(t), policy: policyWithCPP}
	for _, o := range opts {
		o(&a)
	}
	return Calculate(testPA, a.start, ms, a.tables, a.policy)
}

type calcArgs struct {
	start  *competence.Competence
	tables Tables
	policy Policy
}

// AC-007 e TEST-010: fronteiras do corte legal (28%) e da meta (30%).
func TestAC007_BoundariesOfLegalCutAndTarget(t *testing.T) {
	cases := []struct {
		proLabore, factor string
		annex             Annex
		light             Light
	}{
		{"13995", "0.279900", AnnexV, LightRed},
		{"14000", "0.280000", AnnexIII, LightYellow},
		{"14995", "0.299900", AnnexIII, LightYellow},
		{"15000", "0.300000", AnnexIII, LightGreen},
	}
	for _, tc := range cases {
		r := calc(t, series(mv{proLabore: tc.proLabore}))
		if r.Status != StatusOK || r.RBT12.Cmp(d("600000")) != 0 {
			t.Fatalf("pró-labore %s: status %s, RBT12 %s", tc.proLabore, r.Status, r.RBT12)
		}
		if got := r.FactorR.Quantize(-6); got.Cmp(d(tc.factor)) != 0 {
			t.Errorf("pró-labore %s: fator %s; esperado %s", tc.proLabore, got, tc.factor)
		}
		if *r.Annex != tc.annex || *r.Light != tc.light {
			t.Errorf("pró-labore %s: anexo %s / %s; esperado %s / %s", tc.proLabore, *r.Annex, *r.Light, tc.annex, tc.light)
		}
	}
}

// AC-005: valores completos na faixa 3 (RBT12 600 mil, FS12 120 mil, fator 20%).
func TestAC005_FullValuesBracket3(t *testing.T) {
	r := calc(t, series(mv{proLabore: "10000"}))
	eq(t, "fs12", &r.FS12, "120000")
	eq(t, "folha_minima_28", r.MinPayroll28, "168000.00")
	eq(t, "folha_minima_meta", r.MinPayrollTarget, "180000.00")
	eq(t, "gap_12m_28", r.Gap28, "48000.00")
	eq(t, "reforco_mensal_28", r.MonthlyTopUp28, "4000")
	eq(t, "gap_12m_meta", r.GapTarget, "60000.00")
	eq(t, "reforco_mensal_meta", r.MonthlyTopUpTarget, "5000")
	// III: (600000·13,5% - 17.640)/600000 = 10,56%; V: (600000·19,5% - 9.900)/600000 = 17,85%
	eq(t, "aliquota_efetiva_iii", r.EffectiveRateIII, "0.1056")
	eq(t, "aliquota_efetiva_v", r.EffectiveRateV, "0.1785")
	eq(t, "economia_12m", r.Savings12m, "43740.00")
	if !r.TableValidFrom.Equal(time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)) || !r.CPPInFS12 {
		t.Errorf("vigência %s, cpp %v", r.TableValidFrom, r.CPPInFS12)
	}
}

func TestCalc_GapZeroWhenPayrollAboveMinimum(t *testing.T) {
	r := calc(t, series(mv{proLabore: "20000"}))
	eq(t, "gap_12m_28", r.Gap28, "0")
	eq(t, "reforco_mensal_28", r.MonthlyTopUp28, "0")
	eq(t, "gap_12m_meta", r.GapTarget, "0")
}

// TEST-010: RBT12 = 0 não inventa anexo nem alíquota.
func TestCalc_RBT12ZeroInventsNothing(t *testing.T) {
	r := calc(t, series(mv{revenue: "0", proLabore: "5000"}))
	if r.Status != StatusInsufficient || r.Reason == nil || *r.Reason != ReasonRBT12Zero {
		t.Fatalf("status %s, motivo %v", r.Status, r.Reason)
	}
	if r.FactorR != nil || r.Annex != nil || r.Light != nil || r.EffectiveRateIII != nil || r.EffectiveRateV != nil || r.Savings12m != nil {
		t.Fatalf("dados insuficientes não pode trazer fator, anexo, semáforo, alíquota nem economia: %+v", r)
	}
}

func TestCalc_NoMovements(t *testing.T) {
	r := calc(t, nil)
	if r.Status != StatusInsufficient || len(r.MissingMonths) != 12 {
		t.Fatalf("status %s, faltantes %d", r.Status, len(r.MissingMonths))
	}
}

func TestCalc_CPPPolicyChangesFS12(t *testing.T) {
	ms := series(mv{proLabore: "12000", cpp: "2000"})
	with := calc(t, ms)
	without := calc(t, ms, func(a *calcArgs) { a.policy = policyWithoutCPP })
	eq(t, "fs12 com CPP", &with.FS12, "168000")
	eq(t, "fs12 sem CPP", &without.FS12, "144000")
	if *with.Annex != AnnexIII || *without.Annex != AnnexV || without.CPPInFS12 {
		t.Fatalf("anexos %s/%s, política %v", *with.Annex, *without.Annex, without.CPPInFS12)
	}
}

func TestCalc_FS12SumsProLaboreSalariesFGTS(t *testing.T) {
	r := calc(t, series(mv{proLabore: "1000", salaries: "2000", fgts: "160", cpp: "0"}))
	eq(t, "fs12", &r.FS12, "37920")
}

func TestCalc_MissingMonthCountsAndSumsZero(t *testing.T) {
	ms := series(mv{proLabore: "14000"})[1:] // falta o primeiro mês da janela
	r := calc(t, ms)
	if r.Status != StatusOK || !slices.Equal(r.MissingMonths, []competence.Competence{c("2025-09")}) || len(r.FilledMonths) != 11 {
		t.Fatalf("status %s, faltantes %v, preenchidos %d", r.Status, r.MissingMonths, len(r.FilledMonths))
	}
	eq(t, "rbt12", &r.RBT12, "550000")
}

func TestCalc_MovementsOutsideWindowIgnored(t *testing.T) {
	ms := append(series(mv{proLabore: "15000"}), movement(testPA, mv{revenue: "999999"}), movement(c("2025-08"), mv{}))
	r := calc(t, ms)
	eq(t, "rbt12", &r.RBT12, "600000")
}

func TestCalc_Company1_5And12Months(t *testing.T) {
	for _, tc := range []struct {
		months int
		status Status
	}{{1, StatusInsufficient}, {5, StatusInsufficient}, {12, StatusOK}} {
		start := testPA.AddMonths(-tc.months)
		var ms []Movement
		for _, m := range testWindow {
			if !m.Before(start) {
				ms = append(ms, movement(m, mv{proLabore: "15000"}))
			}
		}
		r := calc(t, ms, func(a *calcArgs) { a.start = &start })
		if r.Status != tc.status || len(r.MissingMonths) != 0 || len(r.FilledMonths) != tc.months {
			t.Fatalf("%d meses: status %s, faltantes %v, preenchidos %d", tc.months, r.Status, r.MissingMonths, len(r.FilledMonths))
		}
		if tc.status != StatusOK && (r.Reason == nil || *r.Reason != ReasonNewCompany || r.FactorR != nil || r.Annex != nil) {
			t.Fatalf("%d meses: motivo %v, fator %v, anexo %v", tc.months, r.Reason, r.FactorR, r.Annex)
		}
	}
}

func TestCalc_RBT12BracketChange(t *testing.T) {
	bracket1 := calc(t, series(mv{revenue: "15000", proLabore: "5000"})) // RBT12 = 180.000,00
	ms := series(mv{revenue: "15000", proLabore: "5000"})
	ms[0] = movement(testWindow[0], mv{revenue: "15000.01", proLabore: "5000"})
	bracket2 := calc(t, ms)
	eq(t, "aliquota_efetiva_iii faixa 1", bracket1.EffectiveRateIII, "0.06")
	eq(t, "aliquota_efetiva_v faixa 1", bracket1.EffectiveRateV, "0.155")
	if bracket2.EffectiveRateIII.Cmp(*bracket1.EffectiveRateIII) <= 0 {
		t.Fatalf("faixa 2 deveria ter alíquota efetiva maior: %s", bracket2.EffectiveRateIII)
	}
}

func TestCalc_NegativeSavingsInBracket6(t *testing.T) {
	// RBT12 = 4,8 mi: III efetiva 19,50% > V efetiva 19,25% (tabela legal)
	r := calc(t, series(mv{revenue: "400000", proLabore: "150000"}))
	eq(t, "aliquota_efetiva_iii", r.EffectiveRateIII, "0.195")
	eq(t, "aliquota_efetiva_v", r.EffectiveRateV, "0.1925")
	eq(t, "economia_12m", r.Savings12m, "-12000.0000")
}

func TestCalc_RBT12AboveSimplesCeiling(t *testing.T) {
	r := calc(t, series(mv{revenue: "400001", proLabore: "150000"}))
	if r.Status != StatusInsufficient || *r.Reason != ReasonAboveSimplesCeil || r.Annex != nil {
		t.Fatalf("status %s, motivo %v, anexo %v", r.Status, r.Reason, r.Annex)
	}
}

func TestCalc_UsesTheTablesReceived(t *testing.T) {
	from := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := Tables{
		III: AnnexTable{Annex: AnnexIII, ValidFrom: from, Brackets: []Bracket{{1, d("4800000"), d("0.10"), d("0")}}},
		V:   AnnexTable{Annex: AnnexV, ValidFrom: from, Brackets: []Bracket{{1, d("4800000"), d("0.20"), d("0")}}},
	}
	old := calc(t, series(mv{proLabore: "15000"}))
	current := calc(t, series(mv{proLabore: "15000"}), func(a *calcArgs) { a.tables = newer })
	if old.TableValidFrom.Year() != 2018 || current.TableValidFrom.Year() != 2027 {
		t.Fatalf("vigências %s / %s", old.TableValidFrom, current.TableValidFrom)
	}
	eq(t, "aliquota_efetiva_iii", current.EffectiveRateIII, "0.10")
	eq(t, "economia_12m", current.Savings12m, "60000.00")
}

func TestCalc_TargetOnlyChangesLightAndTargetPayrollNeverAnnex(t *testing.T) {
	r := calc(t, series(mv{proLabore: "15000"}), func(a *calcArgs) { a.policy = Policy{CPPInFS12: true, Target: d("0.32")} })
	if *r.Annex != AnnexIII || *r.Light != LightYellow {
		t.Fatalf("anexo %s, semáforo %s", *r.Annex, *r.Light)
	}
	eq(t, "folha_minima_meta", r.MinPayrollTarget, "192000.00")
}

// Na origem, a folha do PGDAS-D ia inteira em salarios; aqui tem campo próprio. O resultado
// tem de ser o mesmo, com qualquer política de CPP.
func TestCalc_DeclaredPGDASPayrollEqualsOriginSalariesConvention(t *testing.T) {
	for _, p := range []Policy{policyWithCPP, policyWithoutCPP} {
		origin := calc(t, series(mv{salaries: "14000"}), func(a *calcArgs) { a.policy = p })
		ms := series(mv{})
		for i := range ms {
			total := d("14000")
			ms[i].DeclaredPayroll, ms[i].Origin = &total, "pgdas"
		}
		ported := calc(t, ms, func(a *calcArgs) { a.policy = p })
		if origin.FS12.String() != ported.FS12.String() || origin.FactorR.String() != ported.FactorR.String() {
			t.Fatalf("FS12 %s × %s, fator %s × %s", origin.FS12, ported.FS12, origin.FactorR, ported.FactorR)
		}
	}
}
