package classification

import (
	"errors"
	"testing"
)

// O cadastro normaliza o padrão: o CNPJ só com os dígitos (11 ou 14), a contraparte aparada, a
// regex no subconjunto do re que o porte traduz.
func TestNormalizePattern(t *testing.T) {
	ok := []struct{ ruleType, in, want string }{
		{RuleCNPJ, "12.345.678/0001-99", "12345678000199"},
		{RuleCNPJ, "123.456.789-09", "12345678909"},
		{RuleCounterparty, "  Alfa Ltda ", "Alfa Ltda"},
		{RuleRegex, `^pix\s+recebido`, `^pix\s+recebido`},
		{RuleRegex, `nf [0-9]+$`, `nf [0-9]+$`},
	}
	for _, c := range ok {
		got, err := NormalizePattern(c.ruleType, c.in)
		if err != nil || got != c.want {
			t.Errorf("NormalizePattern(%s, %q) = %q, %v; quer %q", c.ruleType, c.in, got, err, c.want)
		}
	}
	for _, c := range []struct{ ruleType, in string }{
		{RuleCNPJ, "1234"}, {RuleCounterparty, " \t "}, {RuleRegex, ""}, {RuleRegex, "(?P<x>a)"}, {RuleRegex, "(?<=a)b"},
		{RuleRegex, "[a-z]+"}, {"outro", "x"},
	} {
		if _, err := NormalizePattern(c.ruleType, c.in); !errors.Is(err, ErrInvalidPattern) {
			t.Errorf("NormalizePattern(%s, %q) aceitou: %v", c.ruleType, c.in, err)
		}
	}
}

func TestSamePattern(t *testing.T) {
	if !SamePattern(RuleCounterparty, "Zeta  Consultoria", "zeta consultória") || SamePattern(RuleRegex, "zeta", "ZETA") ||
		!SamePattern(RuleCNPJ, "12345678000199", "12345678000199") {
		t.Fatal("comparação de padrão")
	}
}

// A regra sugerida pelo agente só é proposta com tipo e padrão válidos.
func TestProposedRule(t *testing.T) {
	s := Suggestion{SuggestedRule: map[string]any{"tipo": "cnpj", "padrao": "12.345.678/0001-99"}}
	if r, ok := s.ProposedRule("2.01"); !ok || r.Type != RuleCNPJ || r.Pattern != "12345678000199" || r.AccountCode != "2.01" {
		t.Fatalf("regra = %+v %v", r, ok)
	}
	for _, rule := range []map[string]any{nil, {}, {"tipo": nil, "padrao": nil}, {"tipo": "regex_descritivo", "padrao": "(?P<x>a)"},
		{"tipo": "outro", "padrao": "x"}, {"tipo": "contraparte", "padrao": 5}} {
		if r, ok := (Suggestion{SuggestedRule: rule}).ProposedRule("2.01"); ok {
			t.Errorf("propôs %+v a partir de %v", r, rule)
		}
	}
}

// Sem regra que case, a origem é o agente; regra com conta vazia (dado antigo) não classifica e
// cede ao histórico, como o "if conta" da origem.
func TestClassify_EmptyAccountFallsToHistory(t *testing.T) {
	d := "PIX ENVIADO ALFA"
	rules, _ := Prepare([]Rule{{Type: RuleRegex, Pattern: "alfa", AccountCode: ""}, {Type: RuleRegex, Pattern: "pix", AccountCode: "9"}})
	got := Classify(Input{Description: &d}, rules, []HistoryItem{{Key: "pix enviado alfa", AccountCode: "3"}})
	if got.Origin != OriginHistory || *got.AccountCode != "3" {
		t.Fatalf("classificação = %+v", got)
	}
}
