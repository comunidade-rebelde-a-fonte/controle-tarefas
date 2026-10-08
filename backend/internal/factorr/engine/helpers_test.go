package engine

import (
	"testing"
	"time"

	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/seed"
)

// Testes portados de fator-r/apps/api/tests/domain (origem, commit 9dd7a74), com os mesmos
// casos e valores esperados. As comparações de valor usam Cmp, como o == do Python; a
// representação (expoente) é conferida no golden (golden_test.go).

var (
	testPA     = competence.MustParse("2026-09")
	testWindow = NewWindow(testPA).Months() // 09/2025 a 08/2026
)

func d(s string) decimal.Decimal { return decimal.MustParse(s) }

func c(s string) competence.Competence { return competence.MustParse(s) }

// tables2018 monta os anexos a partir do CSV do seed, igual a tabelas_fixture.py.
func tables2018(t testing.TB) Tables {
	t.Helper()
	rows, err := seed.SimplesTables()
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	out := Tables{III: AnnexTable{Annex: AnnexIII, ValidFrom: from}, V: AnnexTable{Annex: AnnexV, ValidFrom: from}}
	for _, r := range rows {
		b := Bracket{Number: int(r.Faixa), RBT12UpTo: r.RBT12Ate, NominalRate: r.AliquotaNominal, Deduction: r.ParcelaDeduzir}
		if r.Anexo == "III" {
			out.III.Brackets = append(out.III.Brackets, b)
		} else {
			out.V.Brackets = append(out.V.Brackets, b)
		}
	}
	return out
}

type mv struct{ revenue, proLabore, salaries, cpp, fgts string }

func movement(m competence.Competence, v mv) Movement {
	or := func(s, def string) string {
		if s == "" {
			return def
		}
		return s
	}
	return Movement{
		Competence: m, GrossRevenue: d(or(v.revenue, "50000")), ProLabore: d(or(v.proLabore, "0")),
		Salaries: d(or(v.salaries, "0")), CPP: d(or(v.cpp, "0")), FGTS: d(or(v.fgts, "0")), Origin: "manual",
	}
}

func series(v mv) []Movement {
	out := make([]Movement, len(testWindow))
	for i, m := range testWindow {
		out[i] = movement(m, v)
	}
	return out
}

var (
	policyWithCPP    = Policy{CPPInFS12: true, Target: d("0.30")}
	policyWithoutCPP = Policy{CPPInFS12: false, Target: d("0.30")}
)

func eq(t *testing.T, field string, got *decimal.Decimal, want string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s = nil; esperado %s", field, want)
		return
	}
	if got.Cmp(d(want)) != 0 {
		t.Errorf("%s = %s; esperado %s", field, got, want)
	}
}
