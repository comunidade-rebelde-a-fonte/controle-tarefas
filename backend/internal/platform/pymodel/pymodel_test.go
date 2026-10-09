package pymodel

import (
	"encoding/json"
	"errors"
	"testing"
)

// Os goldens do extrato e do comprovante (gerados pelo Python da origem) cobrem o uso; aqui ficam
// os limites de cada regra do pydantic que o pacote reproduz.

func TestParseDate_PydanticRules(t *testing.T) {
	ok := map[string]string{
		"2026-08-05": "2026-08-05", "2026-08-05T00:00:00": "2026-08-05", "2026-08-05 00:00": "2026-08-05",
		"2026-08-05T00:00:00.000Z": "2026-08-05", "2024-02-29": "2024-02-29",
	}
	for in, want := range ok {
		d, valid := ParseDate(in)
		if !valid || d.Format("2006-01-02") != want {
			t.Errorf("ParseDate(%q) = %v, %v", in, d, valid)
		}
	}
	for _, in := range []string{"05/08/2026", "2026-02-30", "2025-02-29", "2026-08-05T10:00:00", "0000-01-01", "2026-8-5", ""} {
		if _, valid := ParseDate(in); valid {
			t.Errorf("ParseDate(%q) aceito", in)
		}
	}
}

func TestMoneyValue_ParseMoney(t *testing.T) {
	for in, want := range map[any]string{"10": "10.00", " 7.5 ": "7.50", "1e3": "1000.00", json.Number("10"): "10.00", "-5": "-5.00"} {
		d, ok := MoneyValue(in)
		if !ok || d.Text() != want {
			t.Errorf("MoneyValue(%v) = %s, %v", in, d.Text(), ok)
		}
	}
	// Float, booleano, texto não decimal e especiais são recusados, como na origem.
	for _, in := range []any{json.Number("10.5"), json.Number("1e3"), true, "abc", "NaN", "Infinity", "", nil} {
		if _, ok := MoneyValue(in); ok {
			t.Errorf("MoneyValue(%v) aceito", in)
		}
	}
}

func TestIntValueAndLoads(t *testing.T) {
	if i, ok := IntValue(json.Number("2.0")); !ok || i != 2 {
		t.Errorf("IntValue(2.0) = %d, %v", i, ok)
	}
	for _, in := range []any{json.Number("2.5"), "2", true, json.Number("1e400")} {
		if _, ok := IntValue(in); ok {
			t.Errorf("IntValue(%v) aceito", in)
		}
	}
	if _, err := LoadsObject(`{"a": 1} lixo`); !errors.Is(err, ErrInvalidOutput) {
		t.Error("texto depois do JSON aceito")
	}
	if _, err := LoadsObject(`[1]`); !errors.Is(err, ErrInvalidOutput) {
		t.Error("lista aceita como objeto")
	}
	if got := ExtractJSON("```json\n{\"a\": 1}\n```"); got != `{"a": 1}` {
		t.Errorf("ExtractJSON = %q", got)
	}
	if b, _ := Marshal("<a & b>"); string(b) != `"<a & b>"` {
		t.Errorf("Marshal escapou HTML: %s", b)
	}
}
