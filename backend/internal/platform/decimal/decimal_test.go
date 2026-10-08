package decimal

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// Os valores esperados foram obtidos com o decimal do Python 3.12 (contexto padrão), o mesmo
// do Fator R de origem: str(Decimal(a) <op> Decimal(b)).
func TestArithmetic_MatchesPythonDecimal(t *testing.T) {
	cases := []struct{ op, a, b, want string }{
		// Divisão exata: expoente ideal (expoente de a menos o de b).
		{"quo", "139950.00", "500000.00", "0.2799"},
		{"quo", "48000.0000", "12", "4000.0000"},
		{"quo", "63360.00000000", "600000.00", "0.105600"},
		{"quo", "168000.00", "12", "14000.00"},
		{"quo", "100", "4", "25"},
		{"quo", "1.0", "4", "0.25"},
		{"quo", "7", "0.50", "14"},
		{"quo", "10", "4", "2.5"},
		{"quo", "5", "2.00", "2.5"},
		// Dividendo zero: expoente ideal, mesmo positivo.
		{"quo", "0.00", "600000.00", "0"},
		{"quo", "0", "0.30", "0E+2"},
		{"quo", "0.0000", "12", "0.0000"},
		// Divisão inexata: 28 dígitos, half-even.
		{"quo", "1", "3", "0.3333333333333333333333333333"},
		{"quo", "2", "3", "0.6666666666666666666666666667"},
		{"quo", "-1", "3", "-0.3333333333333333333333333333"},
		{"mul", "0.300000", "600000.00", "180000.00000000"},
		{"mul", "0.28", "600000.00", "168000.0000"},
		{"mul", "4000.0000", "0.5", "2000.00000"},
		{"mul", "0.0729", "600000.00", "43740.000000"},
		{"mul", "1.234567890123456789012345678", "9.87654321", "12.19326311248285321124828531"},
		{"add", "0", "0.00", "0.00"},
		{"add", "0.10", "0.2", "0.30"},
		{"add", "12345678901234567890.12345678", "0.000000001", "12345678901234567890.12345678"},
		{"sub", "168000.0000", "150000.00", "18000.0000"},
		{"sub", "0.1785", "0.1056", "0.0729"},
	}
	for _, c := range cases {
		a, b := MustParse(c.a), MustParse(c.b)
		var got Decimal
		switch c.op {
		case "quo":
			got = a.Quo(b)
		case "mul":
			got = a.Mul(b)
		case "add":
			got = a.Add(b)
		case "sub":
			got = a.Sub(b)
		}
		if got.String() != c.want {
			t.Errorf("%s %s %s = %s; Python dá %s", c.a, c.op, c.b, got, c.want)
		}
	}
}

func TestNegMaxSumQuantize_MatchPython(t *testing.T) {
	checks := []struct{ got, want string }{
		{MustParse("0.30").Neg().String(), "-0.30"},
		{MustParse("1.2345678901234567890123456789").Neg().String(), "-1.234567890123456789012345679"},
		// max(Decimal("0"), x) devolve o primeiro argumento no empate.
		{Max(Zero, MustParse("0.0000")).String(), "0"},
		{Max(Zero, MustParse("-5.00")).String(), "0"},
		{Max(Zero, MustParse("5.00")).String(), "5.00"},
		{Sum(MustParse("0.00"), MustParse("0.00")).String(), "0.00"},
		{Sum().String(), "0"},
		{MustParse("0.2799499").Quantize(-6).String(), "0.279950"},
		{MustParse("2.675").Quantize(-2).String(), "2.68"},
		{MustParse("2.665").Quantize(-2).String(), "2.66"},
		{MustParse("-2.665").Quantize(-2).String(), "-2.66"},
		{Zero.Quantize(-2).String(), "0.00"},
		{FromInt(12).String(), "12"},
		// Zero de expoente ajustado abaixo de -6 vai em notação exponencial, como no Python.
		{MustParse("0.000000").String(), "0.000000"},
		{MustParse("0.0000000").String(), "0E-7"},
		{MustParse("0.000").Mul(MustParse("0.275000")).String(), "0E-9"},
		{MustParse("-0.0000000").String(), "-0E-7"},
		{MustParse("0.0000001").String(), "1E-7"},
	}
	for i, c := range checks {
		if c.got != c.want {
			t.Errorf("caso %d: %s; Python dá %s", i, c.got, c.want)
		}
	}
}

func TestQuo_ByZeroPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("divisão por zero deveria entrar em pânico, como o ZeroDivisionError do Python")
		}
	}()
	FromInt(1).Quo(Zero)
}

func TestCompareByValue(t *testing.T) {
	if MustParse("0.30").Cmp(MustParse("0.300000")) != 0 {
		t.Error("0.30 e 0.300000 têm o mesmo valor")
	}
	if MustParse("0.2799").Cmp(MustParse("0.28")) >= 0 {
		t.Error("0.2799 < 0.28")
	}
	if !MustParse("0.000").IsZero() || Zero.Sign() != 0 || MustParse("-1").Sign() != -1 {
		t.Error("IsZero/Sign")
	}
}

func TestParse_OnlyPlainNotation(t *testing.T) {
	for _, s := range []string{"", " 1", "1 ", "+1", "1e3", "1E+2", "NaN", "Infinity", "1,50", "1.", ".5", "--1", "0x10"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) deveria falhar", s)
		}
	}
	for s, want := range map[string]int32{"1234.50": -2, "0.300000": -6, "-0.3": -1, "12": 0, "007": 0} {
		d, err := Parse(s)
		if err != nil || d.Exponent() != want {
			t.Errorf("Parse(%q) = %v (exp %d), %v; expoente esperado %d", s, d, d.Exponent(), err, want)
		}
	}
}

func TestText_PlainNotation(t *testing.T) {
	cases := map[string]string{
		"0.2799":    MustParse("139950.00").Quo(MustParse("500000.00")).Text(),
		"0":         MustParse("0").Quo(MustParse("0.30")).Text(),
		"0.0000001": MustParse("1").Quo(MustParse("10000000")).Text(),
		"-12.50":    MustParse("-12.50").Text(),
	}
	for want, got := range cases {
		if got != want {
			t.Errorf("Text = %q; esperado %q", got, want)
		}
	}
}

func TestFits_PydanticNumericRule(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"0", true}, {"0.00", true}, {"1234.5", true}, {"10.000", true}, {"999999999999.99", true},
		{"1234.567", false}, {"1000000000000", false}, {"1000000000000.00", false},
	}
	for _, c := range cases {
		if got := MustParse(c.s).Fits(12, 2); got != c.want {
			t.Errorf("Fits(%s, 12, 2) = %v; esperado %v", c.s, got, c.want)
		}
	}
}

func TestJSON_StringOnly(t *testing.T) {
	b, err := json.Marshal(struct {
		V Decimal  `json:"v"`
		N *Decimal `json:"n"`
	}{V: MustParse("0").Quo(MustParse("0.30"))})
	if err != nil || string(b) != `{"v":"0","n":null}` {
		t.Fatalf("Marshal = %s, %v", b, err)
	}
	var v Decimal
	if err := json.Unmarshal([]byte(`"1234.50"`), &v); err != nil || v.String() != "1234.50" {
		t.Fatalf("Unmarshal string = %v, %v", v, err)
	}
	for _, raw := range []string{`1234.5`, `"1e3"`, `null`, `true`} {
		if err := json.Unmarshal([]byte(raw), &v); err == nil {
			t.Errorf("Unmarshal(%s) deveria falhar", raw)
		}
	}
}

func TestNumeric_RoundTrip(t *testing.T) {
	for _, s := range []string{"1234.50", "-0.300000", "0", "12345678901234567890.123456789"} {
		n, err := MustParse(s).NumericValue()
		if err != nil {
			t.Fatal(err)
		}
		var back Decimal
		if err := back.ScanNumeric(n); err != nil || back.String() != s {
			t.Errorf("ida e volta de %s = %v, %v", s, back, err)
		}
	}
	var d Decimal
	if err := d.ScanNumeric(pgtype.Numeric{}); err == nil {
		t.Error("NULL deveria falhar em Decimal")
	}
	if err := d.ScanNumeric(pgtype.Numeric{NaN: true, Valid: true}); err == nil {
		t.Error("NaN deveria falhar")
	}
	if err := d.ScanNumeric(pgtype.Numeric{Int: big.NewInt(-5), Exp: -1, Valid: true}); err != nil || d.String() != "-0.5" {
		t.Errorf("negativo = %v, %v", d, err)
	}
}

func TestImmutability(t *testing.T) {
	a := MustParse("1.50")
	_ = a.Add(FromInt(1))
	_ = a.Mul(FromInt(3))
	_ = a.Quo(FromInt(4))
	if a.String() != "1.50" {
		t.Fatalf("operação alterou o operando: %s", a)
	}
}
