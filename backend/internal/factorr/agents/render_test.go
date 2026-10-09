package agents

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"controle/internal/factorr"
)

type renderGolden struct {
	Casos []struct {
		Nome    string         `json:"nome"`
		Texto   string         `json:"texto"`
		Decisao map[string]any `json:"decisao"`
		Valido  bool           `json:"valido"`
		Orfaos  []string       `json:"orfaos"`
	} `json:"casos"`
	BRL        map[string]string `json:"brl"`
	Percentual map[string]string `json:"percentual"`
	Disclaimer string            `json:"disclaimer"`
}

func loadRenderGolden(t *testing.T) renderGolden {
	t.Helper()
	data, err := os.ReadFile("testdata/render_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber() // inteiros da decisão como na origem (int), nunca float
	var g renderGolden
	if err := dec.Decode(&g); err != nil {
		t.Fatal(err)
	}
	return g
}

// O validador do texto do LLM dá o mesmo resultado que validar_texto da origem, órfão a órfão
// (testdata/render_gen.py roda o próprio render.py da origem).
func TestValidateText_MatchesOrigin(t *testing.T) {
	g := loadRenderGolden(t)
	if len(g.Casos) < 35 || g.Disclaimer != factorr.Disclaimer {
		t.Fatalf("golden com %d casos / disclaimer %q", len(g.Casos), g.Disclaimer)
	}
	for _, c := range g.Casos {
		t.Run(c.Nome, func(t *testing.T) {
			ok, orphans := ValidateText(c.Texto, c.Decisao)
			if ok != c.Valido || strings.Join(orphans, "|") != strings.Join(c.Orfaos, "|") {
				t.Errorf("porte = %v %q; origem = %v %q", ok, orphans, c.Valido, c.Orfaos)
			}
		})
	}
}

func TestFormatters_MatchOrigin(t *testing.T) {
	g := loadRenderGolden(t)
	for in, want := range g.BRL {
		var v *string
		if in != "null" {
			v = &in
		}
		if got := FormatBRL(v); got != want {
			t.Errorf("FormatBRL(%s) = %s; origem %s", in, got, want)
		}
	}
	for in, want := range g.Percentual {
		var v *string
		if in != "null" {
			v = &in
		}
		if got := FormatPercentBR(v); got != want {
			t.Errorf("FormatPercentBR(%s) = %s; origem %s", in, got, want)
		}
	}
}
