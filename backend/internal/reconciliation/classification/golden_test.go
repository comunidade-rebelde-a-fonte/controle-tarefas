package classification

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"

	"controle/internal/platform/pytext"
)

// Golden do Python da origem (testdata/classification_gen.py): classificar, a regra derivada da
// correção, o re.escape e a SugestaoClassificacao.

type goldenRule struct {
	Type    string `json:"tipo"`
	Pattern string `json:"padrao"`
	Account string `json:"conta_codigo"`
}

type golden struct {
	RuleLists [][]goldenRule `json:"listas_regras"`
	Histories [][]struct {
		Key     string `json:"chave"`
		Account string `json:"conta_codigo"`
	} `json:"historicos"`
	Classify []struct {
		CNPJ         *string `json:"cnpj"`
		Counterparty *string `json:"contraparte"`
		Description  *string `json:"descritivo"`
		Rules        int     `json:"regras"`
		History      int     `json:"historico"`
		Account      *string `json:"conta_codigo"`
		Origin       string  `json:"origem"`
		Pending      bool    `json:"pendente"`
	} `json:"classificar"`
	Derived []struct {
		CNPJ         *string    `json:"cnpj"`
		Counterparty *string    `json:"contraparte"`
		Description  *string    `json:"descritivo"`
		Rule         goldenRule `json:"regra"`
	} `json:"derivada"`
	Escape []struct {
		Text    string   `json:"texto"`
		Escaped string   `json:"escapado"`
		Matches []string `json:"casa"`
	} `json:"escape"`
	Suggestion []struct {
		In     string          `json:"entrada"`
		Result string          `json:"resultado"`
		Out    json.RawMessage `json:"saida"`
		Guard  []string        `json:"guarda"`
	} `json:"sugestao"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	raw, err := os.ReadFile("testdata/classification_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Classify) < 1000 || len(g.Derived) < 400 || len(g.Escape) < 10 || len(g.Suggestion) < 40 {
		t.Fatal("golden incompleto; gere de novo com testdata/classification_gen.py")
	}
	return g
}

func rulesOf(t *testing.T, list []goldenRule) []Rule {
	t.Helper()
	rules := make([]Rule, len(list))
	for i, r := range list {
		rules[i] = Rule{Type: r.Type, Pattern: r.Pattern, AccountCode: r.Account}
	}
	ready, skipped := Prepare(rules)
	if len(skipped) > 0 {
		t.Fatalf("regex do golden fora do subconjunto do porte: %+v", skipped)
	}
	return ready
}

// A precedência e o resultado de cada classificação são os da origem.
func TestClassify_MatchesOrigin(t *testing.T) {
	g := loadGolden(t)
	lists := make([][]Rule, len(g.RuleLists))
	for i, l := range g.RuleLists {
		lists[i] = rulesOf(t, l)
	}
	histories := make([][]HistoryItem, len(g.Histories))
	for i, h := range g.Histories {
		for _, it := range h {
			histories[i] = append(histories[i], HistoryItem{Key: it.Key, AccountCode: it.Account})
		}
	}
	origins := map[string]int{}
	for _, c := range g.Classify {
		got := Classify(Input{CNPJ: c.CNPJ, Counterparty: c.Counterparty, Description: c.Description}, lists[c.Rules], histories[c.History])
		origins[got.Origin]++
		if got.Origin != c.Origin || got.Pending != c.Pending || !equalPtr(got.AccountCode, c.Account) {
			t.Errorf("classificar(%v, %v, %v; regras %d, histórico %d) = %v %s %v; origem %v %s %v", str(c.CNPJ), str(c.Counterparty),
				str(c.Description), c.Rules, c.History, str(got.AccountCode), got.Origin, got.Pending, str(c.Account), c.Origin, c.Pending)
		}
		if (got.Origin == OriginRule) != (got.Rule != nil) || (got.Origin == OriginHistory) != (got.History != nil) {
			t.Errorf("rastreio da classificação incoerente: %+v", got)
		}
	}
	if origins[OriginRule] < 100 || origins[OriginHistory] < 100 || origins[OriginAgent] < 100 {
		t.Fatalf("cobertura fraca: %v", origins)
	}
}

// A regra derivada da correção é a da origem; sem padrão (descritivo vazio), não há regra.
func TestDerivedRule_MatchesOrigin(t *testing.T) {
	for _, c := range loadGolden(t).Derived {
		got, ok := DerivedRule(Input{CNPJ: c.CNPJ, Counterparty: c.Counterparty, Description: c.Description}, "5.01")
		if c.Rule.Pattern == "" {
			if ok {
				t.Errorf("propôs regra sem padrão: %+v", got)
			}
			continue
		}
		if !ok || got.Type != c.Rule.Type || got.Pattern != c.Rule.Pattern || got.AccountCode != c.Rule.Account {
			t.Errorf("derivada(%v, %v, %q) = %+v; origem %+v", str(c.CNPJ), str(c.Counterparty), str(c.Description), got, c.Rule)
		}
	}
}

// O re.escape e o padrão escapado casam como no Python (IGNORECASE).
func TestEscape_MatchesPython(t *testing.T) {
	g := loadGolden(t)
	var all []string
	for _, c := range g.Escape {
		all = append(all, c.Text)
	}
	for _, c := range g.Escape {
		if got := Escape(c.Text); got != c.Escaped {
			t.Errorf("escape(%q) = %q; Python %q", c.Text, got, c.Escaped)
			continue
		}
		re, err := pytext.Compile(c.Escaped, true)
		if err != nil {
			t.Errorf("padrão escapado não compila: %q: %v", c.Escaped, err)
			continue
		}
		var matches []string
		for _, o := range all {
			if _, ok := re.Search(o); ok {
				matches = append(matches, o)
			}
		}
		if !slices.Equal(matches, c.Matches) {
			t.Errorf("%q casa %q; Python %q", c.Escaped, matches, c.Matches)
		}
	}
}

// A sugestão do agente passa pela validação do pydantic da origem; a guarda do porte recusa a
// confiança fora de 0 a 1.
func TestParseSuggestion_MatchesPydantic(t *testing.T) {
	accepted := 0
	for _, c := range loadGolden(t).Suggestion {
		got, err := ParseSuggestion(c.In)
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
	if accepted < 15 {
		t.Fatalf("só %d sugestões aceitas no golden", accepted)
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

func equalPtr(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func str(s *string) string {
	if s == nil {
		return "None"
	}
	return *s
}
