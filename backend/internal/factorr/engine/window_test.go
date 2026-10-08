package engine

import (
	"slices"
	"testing"

	"controle/internal/platform/competence"
)

// test_janela.py

func TestWindow_PRDExamplePASeptember2026(t *testing.T) {
	w := NewWindow(c("2026-09"))
	if w.Start != c("2025-09") || w.End != c("2026-08") {
		t.Fatalf("janela = %s a %s", w.Start, w.End)
	}
	m := w.Months()
	if len(m) != 12 || m[0] != c("2025-09") || m[11] != c("2026-08") {
		t.Fatalf("meses = %v", m)
	}
}

func TestWindow_JanuaryPACrossesYear(t *testing.T) {
	w := NewWindow(c("2026-01"))
	if w.Start != c("2025-01") || w.End != c("2025-12") {
		t.Fatalf("janela = %s a %s", w.Start, w.End)
	}
}

// AC-006: dois PAs consecutivos — exatamente um mês sai e o novo entra, sem edição manual.
func TestAC006_ChangingPAMovesOneMonthInOneOut(t *testing.T) {
	before, after := NewWindow(c("2026-09")).Months(), NewWindow(c("2026-10")).Months()
	diff := func(a, b []competence.Competence) []competence.Competence {
		var out []competence.Competence
		for _, m := range a {
			if !slices.Contains(b, m) {
				out = append(out, m)
			}
		}
		return out
	}
	if in := diff(after, before); len(in) != 1 || in[0] != c("2026-09") {
		t.Fatalf("entrou %v; esperado só 09/2026", in)
	}
	if out := diff(before, after); len(out) != 1 || out[0] != c("2025-09") {
		t.Fatalf("saiu %v; esperado só 09/2025", out)
	}
}

func TestWindow_ValidMonthsByActivityStart(t *testing.T) {
	cases := []struct {
		start    *competence.Competence
		valid    int
		isNewCom bool
	}{
		{nil, 12, false},
		{ptr(c("2020-01")), 12, false}, // antes da janela
		{ptr(c("2025-09")), 12, false}, // exatamente no início da janela
		{ptr(c("2026-03")), 6, true},   // dentro da janela
		{ptr(c("2026-09")), 0, true},   // igual ao PA
	}
	w := NewWindow(c("2026-09"))
	for _, tc := range cases {
		if n := len(ValidMonths(w, tc.start)); n != tc.valid {
			t.Errorf("início %v: %d meses válidos; esperado %d", tc.start, n, tc.valid)
		}
		if got := NewCompanyInWindow(w, tc.start); got != tc.isNewCom {
			t.Errorf("início %v: empresa nova = %v; esperado %v", tc.start, got, tc.isNewCom)
		}
	}
}

func TestWindow_MissingOnlyCountsValidMonths(t *testing.T) {
	w := NewWindow(c("2026-09"))
	valid := ValidMonths(w, ptr(c("2026-03")))
	filled, missing := Classify(valid, map[competence.Competence]bool{c("2026-03"): true, c("2026-05"): true})
	if !slices.Equal(filled, []competence.Competence{c("2026-03"), c("2026-05")}) {
		t.Fatalf("preenchidos = %v", filled)
	}
	want := []competence.Competence{c("2026-04"), c("2026-06"), c("2026-07"), c("2026-08")}
	if !slices.Equal(missing, want) {
		t.Fatalf("faltantes = %v; esperado %v", missing, want)
	}
	if slices.Contains(missing, c("2025-12")) {
		t.Fatal("mês antes do início de atividade não é faltante")
	}
}
