package agents

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type intentGolden struct {
	Empresas []string `json:"empresas"`
	Palavras []struct {
		Mensagem   string  `json:"mensagem"`
		Intencao   string  `json:"intencao"`
		CompanyRef *string `json:"company_ref"`
		PA         *string `json:"pa"`
		Meta       *string `json:"meta"`
	} `json:"palavras"`
	Citada []struct {
		Mensagem string  `json:"mensagem"`
		Empresa  *string `json:"empresa"`
	} `json:"citada"`
	LLM []struct {
		Saida     map[string]any `json:"saida"`
		Resultado struct {
			OK         bool    `json:"ok"`
			Intencao   string  `json:"intencao"`
			CompanyRef *string `json:"company_ref"`
			PA         *string `json:"pa"`
			Meta       *string `json:"meta"`
		} `json:"resultado"`
	} `json:"llm"`
}

func loadIntentGolden(t *testing.T) intentGolden {
	t.Helper()
	data, err := os.ReadFile("testdata/intent_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var g intentGolden
	if err := dec.Decode(&g); err != nil {
		t.Fatal(err)
	}
	return g
}

func opt(p *string) string {
	if p == nil {
		return "<None>"
	}
	return *p
}

func metaText(in Intent) *string {
	if in.Meta == nil {
		return nil
	}
	s := in.Meta.String()
	return &s
}

// O classificador por palavras-chave e a empresa citada dão o mesmo resultado da origem
// (testdata/intent_gen.py roda o próprio intencao.py).
func TestClassifyByKeywords_MatchesOrigin(t *testing.T) {
	g := loadIntentGolden(t)
	for _, c := range g.Palavras {
		got := ClassifyByKeywords(c.Mensagem, g.Empresas)
		if got.Kind != c.Intencao || opt(got.CompanyRef) != opt(c.CompanyRef) || opt(got.PA) != opt(c.PA) || opt(metaText(got)) != opt(c.Meta) {
			t.Errorf("%q: porte = %s/%s/%s/%s; origem = %s/%s/%s/%s", c.Mensagem,
				got.Kind, opt(got.CompanyRef), opt(got.PA), opt(metaText(got)), c.Intencao, opt(c.CompanyRef), opt(c.PA), opt(c.Meta))
		}
	}
	for _, c := range g.Citada {
		if got := CitedCompany(c.Mensagem, g.Empresas); opt(got) != opt(c.Empresa) {
			t.Errorf("empresa citada em %q = %s; origem %s", c.Mensagem, opt(got), opt(c.Empresa))
		}
	}
}

// A saída estruturada do LLM passa pelas mesmas regras do Intencao da origem (pydantic).
func TestParseIntent_MatchesPydanticValidation(t *testing.T) {
	g := loadIntentGolden(t)
	for _, c := range g.LLM {
		raw, _ := json.Marshal(c.Saida)
		got, err := parseIntent(string(raw))
		want := c.Resultado
		if (err == nil) != want.OK {
			t.Errorf("%s: erro = %v; origem ok = %v", raw, err, want.OK)
			continue
		}
		if err == nil && (got.Kind != want.Intencao || opt(got.CompanyRef) != opt(want.CompanyRef) || opt(got.PA) != opt(want.PA) || opt(metaText(got)) != opt(want.Meta)) {
			t.Errorf("%s: porte = %s/%s/%s/%s; origem = %s/%s/%s/%s", raw, got.Kind, opt(got.CompanyRef), opt(got.PA), opt(metaText(got)),
				want.Intencao, opt(want.CompanyRef), opt(want.PA), opt(want.Meta))
		}
	}
	if _, err := parseIntent(`{"intencao":"priorizar"} lixo`); err == nil {
		t.Error("texto depois do JSON deveria recusar")
	}
}
