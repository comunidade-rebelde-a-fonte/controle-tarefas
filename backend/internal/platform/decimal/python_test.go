package decimal

import (
	"math"
	"testing"
)

// Decimal(str) do Python (valores conferidos no Python 3.14).
func TestParsePython_MatchesPythonDecimal(t *testing.T) {
	cases := map[string]string{
		"1_000": "1000", "1__0": "10", "_1": "1", "1_": "1", "1._5": "1.5", "1_.5": "1.5",
		" 12 ": "12", " 12　": "12", "+5": "5", "1.": "1", ".5": "0.5", "1e5": "1E+5",
		"1E+5": "1E+5", "-0": "-0", "١٢": "12", "-.5e-2": "-0.005", "1_e5": "1E+5", "1e_5": "1E+5",
		"\x1c12\x1f": "12", "600000.00": "600000.00",
	}
	for in, want := range cases {
		d, special, ok := ParsePython(in)
		if !ok || special || d.String() != want {
			t.Errorf("ParsePython(%q) = %s, %v, %v; Python: %s", in, d, special, ok, want)
		}
	}
	for _, in := range []string{"inf", "INFINITY", "nan", "sNaN12", "NaN", "Inf"} {
		if _, special, ok := ParsePython(in); !ok || !special {
			t.Errorf("ParsePython(%q) deveria ser especial", in)
		}
	}
	for _, in := range []string{"1 2", "0x10", "1e", "e5", ".", "", "abc", "Beta Engenharia", "2026-09"} {
		if _, _, ok := ParsePython(in); ok {
			t.Errorf("ParsePython(%q) deveria falhar (InvalidOperation)", in)
		}
	}
}

func TestNormalizeKeyAbsTryQuantize(t *testing.T) {
	if got := MustParse("600000.00").Normalize().String(); got != "6E+5" {
		t.Errorf("normalize = %s", got)
	}
	for _, s := range []string{"0", "-0", "0.00"} {
		d, _, _ := ParsePython(s)
		if d.Key() != "0" {
			t.Errorf("Key(%s) = %s", s, d.Key())
		}
	}
	if MustParse("12.50").Key() != MustParse("12.5").Key() || MustParse("12.50").Key() == MustParse("12.51").Key() {
		t.Error("Key deveria comparar pelo valor")
	}
	if MustParse("-3.20").Abs().String() != "3.20" {
		t.Error("Abs")
	}
	big, _, _ := ParsePython("1E+30")
	if _, ok := big.TryQuantize(-2); ok {
		t.Error("quantize de 1E+30 em centavos deveria falhar, como no Python")
	}
	if q, ok := MustParse("2.345").TryQuantize(-2); !ok || q.String() != "2.34" {
		t.Errorf("TryQuantize = %s", q)
	}
}

// repr() do float do Python e Decimal(repr(f)), conferidos no Python 3.13 (TASK-016).
func TestPythonFloatRepr(t *testing.T) {
	cases := []struct {
		f          float64
		repr, decs string
	}{
		{0.1, "0.1", "0.1"}, {0.3, "0.3", "0.3"}, {1.0, "1.0", "1.0"}, {math.Copysign(0, -1), "-0.0", "-0.0"},
		{100000.0, "100000.0", "100000.0"}, {1e15, "1000000000000000.0", "1000000000000000.0"},
		{1e16, "1e+16", "1E+16"}, {1.5e16, "1.5e+16", "1.5E+16"}, {1e-4, "0.0001", "0.0001"},
		{1e-5, "1e-05", "0.00001"}, {1.5e-7, "1.5e-07", "1.5E-7"}, {123.456, "123.456", "123.456"},
		{1.7976931348623157e308, "1.7976931348623157e+308", "1.7976931348623157E+308"},
		{5e-324, "5e-324", "5E-324"}, {12345678901234567890.5, "1.2345678901234567e+19", "1.2345678901234567E+19"},
		{0.95, "0.95", "0.95"}, {2.0, "2.0", "2.0"}, {-1234.5, "-1234.5", "-1234.5"},
	}
	for _, c := range cases {
		if got := PythonFloatRepr(c.f); got != c.repr {
			t.Errorf("repr(%v) = %q; quer %q", c.f, got, c.repr)
		}
		d, ok := FromPythonFloat(c.f)
		if !ok || d.String() != c.decs {
			t.Errorf("Decimal(repr(%v)) = %q, %v; quer %q", c.f, d.String(), ok, c.decs)
		}
	}
	for _, f := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
		if _, ok := FromPythonFloat(f); ok {
			t.Errorf("FromPythonFloat(%v) aceitou", f)
		}
	}
}
