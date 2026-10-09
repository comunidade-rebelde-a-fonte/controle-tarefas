package pytext

import (
	"reflect"
	"testing"
)

// Classes do Python 3 em texto (str.isspace, \d e \w do re). Os goldens da chave de dedupe da
// Conciliação e do mascaramento do Langfuse cobrem o uso; aqui ficam os casos-limite.
func TestClasses_MatchPython(t *testing.T) {
	spaces := []rune{' ', '\t', '\n', '\v', '\f', '\r', 0x1C, 0x1D, 0x1E, 0x1F, 0x85, 0xA0, 0x1680, 0x2009, 0x2028, 0x202F, 0x3000}
	for _, r := range spaces {
		if !IsSpace(r) {
			t.Errorf("IsSpace(%U) = false; o Python considera espaço", r)
		}
	}
	for _, r := range []rune{'a', '0', '_', 0x200B, 0x1B} {
		if IsSpace(r) {
			t.Errorf("IsSpace(%U) = true; o Python não considera espaço", r)
		}
	}
	for _, r := range []rune{'0', '9', 0x0661, 0x0966, 0xFF15} { // ASCII, árabe-índico, devanágari, largura total
		if !IsDigit(r) {
			t.Errorf("IsDigit(%U) = false", r)
		}
	}
	for _, r := range []rune{'a', 0x00B2, 0x2155, '.'} { // ² e ⅕ são números, mas não dígitos decimais
		if IsDigit(r) {
			t.Errorf("IsDigit(%U) = true", r)
		}
	}
	for _, r := range []rune{'a', 'Z', 'é', 'ß', '_', '7', 0x0661, 0x00B2, 0x4E00} {
		if !IsWord(r) {
			t.Errorf("IsWord(%U) = false", r)
		}
	}
	for _, r := range []rune{'-', '.', ' ', '/', 0x0301} { // marca combinante sozinha não é \w
		if IsWord(r) {
			t.Errorf("IsWord(%U) = true", r)
		}
	}
}

// Saídas do str.splitlines() do Python 3.13 para os mesmos textos.
func TestSplitLines_MatchesPython(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a\nb", []string{"a", "b"}},
		{"a\r\nb\rc\vd\fe\x1cf\x1dg\x1eh\u0085i\u2028j\u2029k", []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}},
		{"a\n", []string{"a"}},
		{"\n", []string{""}},
		{"", []string{}},
		{"a\n\nb", []string{"a", "", "b"}},
		{"a\x1fb", []string{"a\x1fb"}},
		{"a\r\r\nb", []string{"a", "", "b"}},
	}
	for _, c := range cases {
		if got := SplitLines(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitLines(%q) = %q; Python %q", c.in, got, c.want)
		}
	}
}
