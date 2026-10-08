package pgdas

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

type goldenCase struct {
	Nome      string `json:"nome"`
	Origem    string `json:"origem"`
	Texto     string `json:"texto"`
	Resultado *struct {
		Campos        map[string]any `json:"campos"`
		Confianca     string         `json:"confianca"`
		ParserVersion string         `json:"parser_version"`
	} `json:"resultado"`
	Excecao *string `json:"excecao"`
}

// O parser portado dá o mesmo resultado que o da origem em cada caso do golden gerado pelo
// próprio Python (testdata/golden_gen.py): as fixtures da origem e as variações de borda.
func TestParse_MatchesOriginGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []goldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 100 {
		t.Fatalf("golden com %d casos; gere de novo com testdata/golden_gen.py", len(cases))
	}
	fixtures := 0
	for _, c := range cases {
		if c.Origem == "fixture" {
			fixtures++
		}
		t.Run(c.Nome, func(t *testing.T) {
			res, err := Parse(c.Texto)
			if c.Excecao != nil {
				if !errors.Is(err, ErrParse) {
					t.Fatalf("a origem levanta %s; o porte devolveu %v", *c.Excecao, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			b, err := json.Marshal(res.Stored())
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.Resultado.Campos) {
				for k, want := range c.Resultado.Campos {
					if !reflect.DeepEqual(got[k], want) {
						t.Errorf("%s: porte = %#v; origem = %#v", k, got[k], want)
					}
				}
			}
			if res.Confidence.String() != c.Resultado.Confianca || res.ParserVersion != c.Resultado.ParserVersion {
				t.Errorf("confiança/versão = %s/%s; origem = %s/%s", res.Confidence, res.ParserVersion, c.Resultado.Confianca, c.Resultado.ParserVersion)
			}
		})
	}
	if fixtures != 13 {
		t.Errorf("golden com %d fixtures com texto; a origem tem 13", fixtures)
	}
}
