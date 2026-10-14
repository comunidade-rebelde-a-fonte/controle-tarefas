package pgdas

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pdftext"
)

// Porte de tests/parsing/test_fixtures.py: extração (pdftext) + parser contra as 14 fixtures
// da origem (testdata/fixtures, copiadas de apps/api/fixtures/pgdas), campo a campo e com a
// faixa de confiança [eval-plan §2].

type fixtureExpected struct {
	Sintetica *bool              `json:"sintetica"`
	Campos    map[string]*string `json:"campos"`
	// Lacuna conhecida: o documento tem o valor, o parser ainda não lê. Exige-se nil, nunca um
	// valor errado.
	Lacunas       map[string]string `json:"lacunas_conhecidas"`
	Identificacao *struct {
		NomeEmpresarial *string `json:"nome_empresarial"`
		SujeitaFatorR   *bool   `json:"sujeita_fator_r"`
		InicioAtividade *string `json:"inicio_atividade"`
	} `json:"identificacao"`
	Series *struct {
		Meses            int  `json:"meses"`
		ReceitasConferem bool `json:"receitas_conferem"`
		FolhasConferem   bool `json:"folhas_conferem"`
	} `json:"series_anteriores"`
	ConfiancaMin json.Number `json:"confianca_min"`
	ConfiancaMax json.Number `json:"confianca_max"`
	Motivo       *string     `json:"motivo"`
}

var extractor = pdftext.New(0)

type fixture struct {
	name     string
	result   Result
	expected fixtureExpected
}

func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	dirs, err := filepath.Glob("testdata/fixtures/*/expected.json")
	if err != nil || len(dirs) != 14 {
		t.Fatalf("fixtures: %d (%v); a origem tem 14", len(dirs), err)
	}
	var out []fixture
	for _, exp := range dirs {
		dir := filepath.Dir(exp)
		docs, _ := filepath.Glob(filepath.Join(dir, "documento.*"))
		if len(docs) != 1 {
			t.Fatalf("%s: documento ausente", dir)
		}
		data, err := os.ReadFile(docs[0])
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(exp)
		if err != nil {
			t.Fatal(err)
		}
		var e fixtureExpected
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("%s: %v", exp, err)
		}
		mime, err := pdftext.DetectMIME(data)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		text, err := extractor.Extract(context.Background(), data, mime)
		if err != nil {
			t.Fatalf("%s: %v", dir, err)
		}
		res := Empty(text.Reason)
		if text.Reason == "" {
			if res, err = Parse(text.Text); err != nil {
				t.Fatalf("%s: %v", dir, err)
			}
		}
		out = append(out, fixture{name: filepath.Base(dir), result: res, expected: e})
	}
	return out
}

// igual compara como _igual da origem: numericamente quando os dois são números.
func igual(got, want *string) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	g, err1 := decimal.Parse(*got)
	w, err2 := decimal.Parse(*want)
	if err1 != nil || err2 != nil {
		return *got == *want
	}
	return g.Cmp(w) == 0
}

func TestParse_OriginFixtures(t *testing.T) {
	for _, f := range loadFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			r, e := f.result, f.expected
			for _, campo := range Fields {
				if lacuna, ok := e.Lacunas[campo]; ok {
					if r.Field(campo) != nil {
						t.Errorf("%s: lacuna (%s) passou a ser lida como %s; confira e retire a lacuna", campo, lacuna, *r.Field(campo))
					}
					continue
				}
				if !igual(r.Field(campo), e.Campos[campo]) {
					t.Errorf("%s: obtido=%s esperado=%s", campo, val(r.Field(campo)), val(e.Campos[campo]))
				}
			}
			if id := e.Identificacao; id != nil {
				got := r.Identification
				if val(got.NomeEmpresarial) != val(id.NomeEmpresarial) || !sameBool(got.SujeitaFatorR, id.SujeitaFatorR) ||
					val(got.InicioAtividade) != val(id.InicioAtividade) {
					t.Errorf("identificação = %s/%v/%s", val(got.NomeEmpresarial), got.SujeitaFatorR, val(got.InicioAtividade))
				}
			}
			if s := e.Series; s != nil {
				if len(r.Series.Receitas) != s.Meses || r.Series.ReceitasConferem != s.ReceitasConferem || r.Series.FolhasConferem != s.FolhasConferem {
					t.Errorf("séries = %+v", r.Series)
				}
			}
			lo := decimal.MustParse(e.ConfiancaMin.String())
			hi := decimal.MustParse(e.ConfiancaMax.String())
			if r.Confidence.Cmp(lo) < 0 || r.Confidence.Cmp(hi) > 0 {
				t.Errorf("confiança %s fora de [%s, %s]", r.Confidence, lo, hi)
			}
			if e.Motivo != nil && *e.Motivo != "" && !contains(r.Reasons, *e.Motivo) {
				t.Errorf("motivo %q ausente em %v", *e.Motivo, r.Reasons)
			}
		})
	}
}

// Meta da origem: acerto >= 80% nas fixtures sintéticas (lacunas contam como erro).
func TestParse_FixtureAccuracyGoal(t *testing.T) {
	ok, total := 0, 0
	byField := map[string][2]int{}
	for _, f := range loadFixtures(t) {
		if f.expected.Sintetica != nil && !*f.expected.Sintetica {
			continue
		}
		for _, campo := range Fields {
			hit := igual(f.result.Field(campo), f.expected.Campos[campo])
			c := byField[campo]
			if hit {
				ok++
				c[0]++
			}
			total++
			c[1]++
			byField[campo] = c
		}
	}
	var report []string
	for _, campo := range Fields {
		report = append(report, fmt.Sprintf("%s %d/%d", campo, byField[campo][0], byField[campo][1]))
	}
	t.Logf("acerto do parser por campo: %s; sintéticas: %d/%d", strings.Join(report, ", "), ok, total)
	if ok*100 < total*80 {
		t.Fatalf("acerto %d/%d abaixo de 80%%", ok, total)
	}
}

// O parser é determinístico: nenhum arquivo do pacote fala com LLM (test_parsing_nao_usa_llm).
func TestParse_NoLLM(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "controle/internal/ai") {
			t.Errorf("%s importa o pacote de IA", f)
		}
	}
}

func sameBool(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
