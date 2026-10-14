package resolver

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// A saída do resolvedor passa pela mesma validação do pydantic da origem
// (testdata/resolver_gen.py): o que ele aceita sai igual (é o payload da proposta); o que ele
// recusa é recusado; e a guarda do porte (confiança fora de 0 a 1) também recusa.
func TestParse_MatchesPydantic(t *testing.T) {
	raw, err := os.ReadFile("testdata/resolver_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		In     string          `json:"entrada"`
		Result string          `json:"resultado"`
		Out    json.RawMessage `json:"saida"`
		Guard  []string        `json:"guarda"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, c := range cases {
		got, err := Parse(c.In)
		if c.Result != "ok" || len(c.Guard) > 0 {
			if err == nil || !errors.Is(err, ErrInvalidOutput) {
				t.Errorf("aceitou o que a origem recusa (%s, guarda %v):\n%s", c.Result, c.Guard, c.In)
			}
			continue
		}
		if err != nil {
			t.Errorf("recusou o que a origem aceita: %v\n%s", err, c.In)
			continue
		}
		accepted++
		if canonical(t, got.Payload()) != canonical(t, c.Out) {
			t.Errorf("saída diverge:\n--- porte\n%s\n--- origem\n%s", got.Payload(), c.Out)
		}
	}
	if len(cases) < 50 || accepted < 20 {
		t.Fatalf("golden incompleto (%d casos, %d aceitos); gere de novo com testdata/resolver_gen.py", len(cases), accepted)
	}
}

func canonical(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("JSON inválido %s: %v", b, err)
	}
	out, _ := json.Marshal(v)
	return string(out)
}
