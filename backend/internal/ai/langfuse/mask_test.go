package langfuse

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// mask_golden.json é gerado por testdata/mask_gen.py rodando o mascarar_cnpj, o _limpar_payload
// e o mascarar_atributo do próprio Fator R de origem (tracing/client.py), com casos de Unicode
// (dígitos não latinos, espaços exóticos, letras acentuadas coladas) e de truncamento.
//
//go:embed testdata/mask_golden.json
var maskGolden []byte

func TestMask_MatchesOrigin(t *testing.T) {
	var g struct {
		Docs []struct {
			In  string `json:"entrada"`
			Out string `json:"saida"`
		} `json:"mascarar_cnpj"`
		Payloads []struct {
			In  json.RawMessage `json:"entrada"`
			Max int             `json:"max"`
			Out json.RawMessage `json:"saida"`
		} `json:"limpar_payload"`
		Attrs []struct {
			In  string `json:"entrada"`
			Max int    `json:"max_bytes"`
			Out string `json:"saida"`
		} `json:"mascarar_atributo"`
	}
	if err := json.Unmarshal(maskGolden, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Docs) < 30 || len(g.Payloads) < 6 || len(g.Attrs) < 7 {
		t.Fatalf("golden incompleto: %d/%d/%d", len(g.Docs), len(g.Payloads), len(g.Attrs))
	}
	for _, c := range g.Docs {
		if got := MaskDocuments(c.In); got != c.Out {
			t.Errorf("MaskDocuments(%q) = %q; a origem dá %q", c.In, got, c.Out)
		}
	}
	for i, c := range g.Payloads {
		in, _ := decodeStructure(string(c.In))
		want, _ := decodeStructure(string(c.Out))
		if got := cleanPayload(in, c.Max); !reflect.DeepEqual(got, want) {
			t.Errorf("payload %d: cleanPayload = %#v; a origem dá %#v", i, got, want)
		}
	}
	for _, c := range g.Attrs {
		if got := MaskAttribute(c.In, c.Max); got != c.Out {
			t.Errorf("MaskAttribute(%.30q…, %d) = %.40q…; a origem dá %.40q…", c.In, c.Max, got, c.Out)
		}
	}
}

// tests/agents/test_tracer.py da origem (test_mascara_remove_texto_do_pdf_e_trunca).
func TestMaskAttribute_OriginUnitCase(t *testing.T) {
	payload := `{"cnpj": "11222333000181", "texto_extraido": "` + strings.Repeat("conteúdo do pdf", 10) + `"}`
	var got map[string]string
	if err := json.Unmarshal([]byte(MaskAttribute(payload, 1024)), &got); err != nil {
		t.Fatal(err)
	}
	if got["texto_extraido"] != "[removido]" || got["cnpj"] != "***0181" {
		t.Fatalf("mascarado = %v", got)
	}
	big := MaskAttribute(strings.Repeat("x", 5000), 1024)
	if utf8.RuneCountInString(big) >= 1100 || !strings.HasSuffix(big, "…[truncado]") {
		t.Fatalf("truncamento: %d caracteres, fim %q", utf8.RuneCountInString(big), big[len(big)-20:])
	}
}

// JSON estruturado também é limitado no total, sem quebrar UTF-8.
func TestMaskAttribute_LimitsJSON(t *testing.T) {
	payload := `{"a": "` + strings.Repeat("ção", 2000) + `", "b": ["` + strings.Repeat("x", 3000) + `"]}`
	got := MaskAttribute(payload, 1000)
	if len(got) > 1000+len(Truncated) || !utf8.ValidString(got) || !strings.HasSuffix(got, Truncated) {
		t.Fatalf("resultado com %d bytes, UTF-8 válido: %v", len(got), utf8.ValidString(got))
	}
}
