package pytext

import (
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// As classes traduzidas valem para o mesmo conjunto que IsSpace e IsDigit, em todo o Unicode.
func TestClasses_SameSetsAsPredicates(t *testing.T) {
	space := regexp.MustCompile(`^` + SpaceClass + `$`)
	notSpace := regexp.MustCompile(`^` + NotSpaceClass + `$`)
	digit := regexp.MustCompile(`^` + DigitClass + `$`)
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // surrogates não existem em texto válido
		}
		s := string(r)
		if space.MatchString(s) != IsSpace(r) || notSpace.MatchString(s) == IsSpace(r) {
			t.Fatalf("classe de espaço diverge de IsSpace em %U", r)
		}
		if digit.MatchString(s) != IsDigit(r) {
			t.Fatalf("classe de dígito diverge de IsDigit em %U", r)
		}
	}
}

func TestDigitValue(t *testing.T) {
	cases := map[rune]int{'0': 0, '7': 7, 0x0660: 0, 0x0663: 3, 0x096F: 9, 0xFF15: 5, 0x1D7CE: 0, 0x1D7D8: 0, 0x1D7DB: 3}
	for r, want := range cases {
		if v, ok := DigitValue(r); !ok || v != want {
			t.Errorf("DigitValue(%U) = %d, %v; quer %d", r, v, ok, want)
		}
	}
	for _, r := range []rune{'a', 0x00B2, 0x2155, '.'} {
		if _, ok := DigitValue(r); ok {
			t.Errorf("DigitValue(%U) aceitou um não-dígito", r)
		}
	}
	// Todo dígito Nd tem valor (blocos contíguos de 10, sem passo).
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if IsDigit(r) {
			if _, ok := DigitValue(r); !ok {
				t.Fatalf("DigitValue(%U) sem valor", r)
			}
		}
	}
	if got := ASCIIDigits("٢٠٢٦-٠٨ e 12"); got != "2026-08 e 12" {
		t.Errorf("ASCIIDigits = %q", got)
	}
	if n, ok := Atoi("٠٨"); !ok || n != 8 {
		t.Errorf("Atoi = %d, %v", n, ok)
	}
}

func TestWordBoundary_Unicode(t *testing.T) {
	cases := []struct {
		s    string
		pos  int
		want bool
	}{
		{"CNPJ", 0, true},
		{"éCNPJ", len("é"), false}, // é é letra: sem fronteira (o \b do Go veria uma)
		{"-CNPJ", 1, true},
		{"12a", 2, false},
		{"12 ", 2, true},
		{"", 0, false},
	}
	for _, c := range cases {
		if got := IsWordBoundary(c.s, c.pos); got != c.want {
			t.Errorf("IsWordBoundary(%q, %d) = %v", c.s, c.pos, got)
		}
	}
}

func group(t *testing.T, m Match, n int) string {
	t.Helper()
	g, ok := m.Group(n)
	if !ok {
		return "<None>"
	}
	return g
}

func TestRegexp_PythonSemantics(t *testing.T) {
	// \b Unicode: "éPA: 08/2026" não casa no Python (é é letra), mas "x PA: 08/2026" sim.
	pa := MustCompile(`\bPA\s*[:\-]\s*(\d{2})/(\d{4})\b`, false)
	if _, ok := pa.Search("éPA: 08/2026"); ok {
		t.Error(`\b casou depois de letra acentuada`)
	}
	if m, ok := pa.Search("éPA: 01/2025 e PA: 08/2026"); !ok || group(t, m, 1) != "08" || group(t, m, 2) != "2026" {
		t.Errorf("busca seguinte = %v %v", m, ok)
	}
	if _, ok := pa.Search("PA: 08/20261"); ok {
		t.Error(`\b final casou antes de dígito`)
	}
	// \s Unicode (espaço fino) e \d Unicode (dígitos árabe-índicos).
	if m, ok := pa.Search("PA: ٠٨/٢٠٢٦"); !ok || group(t, m, 1) != "٠٨" {
		t.Errorf(`\s ou \d Unicode = %v %v`, m, ok)
	}
	// IGNORECASE: o Python iguala i, I, ı e İ.
	nome := MustCompile(`^Nome\s+empresarial\b\s*:?\s*(.*)$`, true)
	for _, s := range []string{"NOME EMPRESARIAL: X", "nome empresarıal: X", "Nome EmpresarİAL X"} {
		if m, ok := nome.Search(s); !ok || group(t, m, 1) != "X" {
			t.Errorf("IGNORECASE em %q = %v", s, ok)
		}
	}
	if _, ok := nome.Search("x Nome empresarial: X"); ok {
		t.Error("^ casou fora do início")
	}
	// Lookahead (?!\d) no último ramo: o início é descartado e a busca segue no caractere
	// seguinte (resultados conferidos no Python).
	num := MustCompile(`(\d{1,3}(?:\.\d{3}){1,6},\d{2}|\d{1,15},\d{2}|\d{1,15}\.\d{2}(?!\d))`, false)
	for s, want := range map[string]string{
		"1234.567 1.234,56 99.99": "1.234,56|99.99",
		"12345678901234567.89":    "345678901234567.89",
	} {
		var got []string
		for _, m := range num.FindAll(s) {
			got = append(got, group(t, m, 1))
		}
		if strings.Join(got, "|") != want {
			t.Errorf("findall(%q) = %q; quer %q", s, got, want)
		}
	}
	// Lookahead com classe e IGNORECASE: [IVX] também recusa i, v, x, ı e İ.
	anexo := MustCompile(`\bAnexo\s+(III|V)(?![IVX])`, true)
	for s, want := range map[string]string{"Anexo III": "III", "anexo v.": "v", "Anexo IIIi": "<None>", "Anexo Vı": "<None>", "Anexo VI": "<None>"} {
		m, ok := anexo.Search(s)
		g := "<None>"
		if ok {
			g = group(t, m, 1)
		}
		if g != want {
			t.Errorf("anexo em %q = %q; quer %q", s, g, want)
		}
	}
	// Grupo opcional que não participou.
	fator := MustCompile(`Fator\s*r\s*[:=]?\s*(\d{1,6}(?:[.,]\d{1,6})?)\s*(%)?`, true)
	if m, ok := fator.Search("Fator r: 0,28"); !ok || group(t, m, 2) != "<None>" {
		t.Errorf("grupo opcional = %v", m)
	}
}

func TestCompile_RejectsUnsupported(t *testing.T) {
	for _, p := range []string{`\w+`, `(?<=a)b`, `(?P<x>a)`, `[\s]`, `(?!(a))`, `[^a]`, `(?i)a`, `[h-j]`} {
		if _, err := Compile(p, true); err == nil {
			t.Errorf("Compile(%q) deveria recusar", p)
		}
	}
}
