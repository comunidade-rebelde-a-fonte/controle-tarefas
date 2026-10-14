package receipt

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"controle/internal/platform/decimal"
)

// golden é a saída do próprio código do Conciliador (testdata/receipt_gen.py).
type golden struct {
	Output []struct {
		In     string          `json:"entrada"`
		Result string          `json:"resultado"`
		Out    json.RawMessage `json:"saida"`
		Guard  []string        `json:"guarda"`
		Weak   []string        `json:"abaixo_limiar"`
	} `json:"saida"`
	Reader []struct {
		In     string          `json:"entrada"`
		Result string          `json:"resultado"`
		Out    json.RawMessage `json:"saida"`
	} `json:"leitor"`
}

func loadGolden(t *testing.T) golden {
	t.Helper()
	raw, err := os.ReadFile("testdata/receipt_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.Output) < 70 || len(g.Reader) < 30 {
		t.Fatal("golden incompleto; gere de novo com testdata/receipt_gen.py")
	}
	return g
}

// canonical normaliza um JSON (chaves ordenadas, números como float) para comparar conteúdo.
func canonical(t *testing.T, b []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("JSON inválido %s: %v", b, err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

var originThreshold = decimal.MustParse("0.70")

// A saída do extrator passa pela mesma validação do pydantic da origem: o que ele aceita sai
// igual, com os mesmos campos abaixo do limiar; o que ele recusa (ou derrubava o job) é recusado;
// a guarda do porte (confiança fora de 0 a 1) também recusa.
func TestParse_MatchesPydantic(t *testing.T) {
	accepted := 0
	for _, c := range loadGolden(t).Output {
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
		b, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if canonical(t, b) != canonical(t, c.Out) {
			t.Errorf("saída diverge:\n--- porte\n%s\n--- origem\n%s\n--- entrada\n%s", b, c.Out, c.In)
		}
		if w := Weak(got, originThreshold); !reflect.DeepEqual(w, c.Weak) {
			t.Errorf("campos abaixo do limiar = %v; origem %v\n%s", w, c.Weak, c.In)
		}
	}
	if accepted < 40 {
		t.Fatalf("só %d saídas aceitas: o golden perdeu os casos normais", accepted)
	}
}

// O leitor determinístico lê os 13 comprovantes do cenário, os textos dos testes da origem e as
// variações como o parse_comprovante_texto: mesmo comprovante, mesma recusa (None) e erro onde a
// origem levantava exceção. A empresa é a do cenário, em que o nome e a palavra "horizonte" da
// origem coincidem.
func TestFromText_MatchesOrigin(t *testing.T) {
	client := NewClient("Construtora Horizonte Ltda")
	for _, c := range loadGolden(t).Reader {
		got, ok, err := FromText(c.In, client)
		switch {
		case c.Result == "ok":
			if err != nil || !ok {
				t.Errorf("não leu o que a origem lê (ok=%v, %v):\n%s", ok, err, c.In)
				continue
			}
			b, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if canonical(t, b) != canonical(t, c.Out) {
				t.Errorf("leitura diverge:\n--- porte\n%s\n--- origem\n%s\n--- texto\n%s", b, c.Out, c.In)
			}
		case c.Result == "nenhum":
			if err != nil || ok {
				t.Errorf("leu o que a origem não lê (ok=%v, %v):\n%s", ok, err, c.In)
			}
		case strings.HasPrefix(c.Result, "erro:"):
			if err == nil || !errors.Is(err, ErrUnreadable) {
				t.Errorf("a origem levanta %s; o porte devolveu ok=%v, %v:\n%s", c.Result, ok, err, c.In)
			}
		default:
			t.Fatalf("resultado desconhecido %q", c.Result)
		}
	}
}
