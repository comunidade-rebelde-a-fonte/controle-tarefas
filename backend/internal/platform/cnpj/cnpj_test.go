package cnpj

import "testing"

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"00.000.001/0001-01":   "00000001000101",
		" 00.000.001/0001-01 ": "00000001000101",
		"00 000 001 0001 01":   "00000001000101",
		"00000001000101":       "00000001000101",
		"11.222.333/0001-81\n": "11222333000181",
		"":                     "",
		"abc":                  "",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValid(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"11.222.333/0001-81", true},
		{"11222333000181", true},
		{"45.723.174/0001-10", true},
		{"11.222.333/0001-82", false}, // DV errado
		{"11222333000191", false},     // DV errado
		{"00000000000000", false},     // repetido
		{"11111111111111", false},     // repetido
		{"1122233300018", false},      // 13 dígitos
		{"112223330001810", false},    // 15 dígitos
		{"", false},
	}
	for _, tc := range cases {
		if got := Valid(tc.in); got != tc.ok {
			t.Errorf("Valid(%q) = %v, want %v", tc.in, got, tc.ok)
		}
	}
}

func TestFormat(t *testing.T) {
	if got := Format("00000001000101"); got != "00.000.001/0001-01" {
		t.Fatal(got)
	}
	if got := Format("123"); got != "123" {
		t.Fatalf("valor inválido deve voltar inalterado: %q", got)
	}
}
