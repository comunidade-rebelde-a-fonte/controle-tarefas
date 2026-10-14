package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"controle/internal/factorr/pgdas"
	"controle/internal/platform/decimal"
	"controle/internal/platform/pytext"
)

// Intenções do pedido (TipoIntencao da origem).
const (
	IntentStatus     = "status_empresa"
	IntentSimulate   = "simular"
	IntentExplain    = "explicar"
	IntentPrioritize = "priorizar"
)

var intents = []string{IntentStatus, IntentSimulate, IntentExplain, IntentPrioritize}

// Intent é a intenção do pedido do analista (Intencao da origem): só classificação e
// parâmetros citados; nenhum valor calculado.
type Intent struct {
	Kind       string           `json:"intencao"`
	CompanyRef *string          `json:"company_ref"`
	PA         *string          `json:"pa"`
	Meta       *decimal.Decimal `json:"meta"`
}

var (
	minMeta = decimal.MustParse("0.28")
	// Padrão do campo pa (pydantic com o regex do Rust: \d Unicode e $ só no fim do texto).
	rePAPattern = regexp.MustCompile(`^\p{Nd}{4}-(0[1-9]|1[0-2])$`)
	// ErrInvalidIntent é a saída do LLM que não passa na validação: o plan cai no fallback.
	ErrInvalidIntent = errors.New("intencao_invalida")
)

// validPA aplica o validador do campo pa: competência válida entre 2000-01 e 2100-12 (comparação
// de texto, como na origem); fora disso, nula.
func validPA(pa string) *string {
	if _, _, err := pgdas.ParsePA(pa); err != nil {
		return nil
	}
	if pa < "2000-01" || pa > "2100-12" {
		return nil
	}
	return &pa
}

// metaInRange é a regra do campo meta: de 0,28 a menos de 1.
func metaInRange(d decimal.Decimal) bool { return d.Cmp(minMeta) >= 0 && d.Cmp(one) < 0 }

// parseIntent valida a saída estruturada do LLM como Intencao.model_validate da origem: intenção
// conhecida; company_ref e pa texto ou nulo; pa no padrão AAAA-MM (fora da faixa vira nulo);
// meta número (ou texto numérico) de 0,28 a menos de 1; campos a mais são ignorados.
func parseIntent(content string) (Intent, error) {
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil || dec.More() {
		return Intent{}, ErrInvalidIntent
	}
	return intentFromMap(raw)
}

func intentFromMap(raw map[string]any) (Intent, error) {
	var in Intent
	kind, ok := raw["intencao"].(string)
	if !ok || !contains(intents, kind) {
		return Intent{}, ErrInvalidIntent
	}
	in.Kind = kind
	switch ref := raw["company_ref"].(type) {
	case nil:
	case string:
		in.CompanyRef = &ref
	default:
		return Intent{}, ErrInvalidIntent
	}
	switch pa := raw["pa"].(type) {
	case nil:
	case string:
		if !rePAPattern.MatchString(pa) {
			return Intent{}, ErrInvalidIntent
		}
		in.PA = validPA(pa)
	default:
		return Intent{}, ErrInvalidIntent
	}
	switch m := raw["meta"].(type) {
	case nil:
	case json.Number:
		d, ok := numberAsPython(m)
		if !ok || !metaInRange(d) {
			return Intent{}, ErrInvalidIntent
		}
		in.Meta = &d
	case string:
		d, special, ok := decimal.ParsePython(m)
		if !ok || special || !metaInRange(d) {
			return Intent{}, ErrInvalidIntent
		}
		in.Meta = &d
	default:
		return Intent{}, ErrInvalidIntent
	}
	return in, nil
}

// numberAsPython converte o número do JSON como o pydantic: inteiro exato; real pelo float e a
// representação mais curta (0.30 vira 0.3).
func numberAsPython(n json.Number) (decimal.Decimal, bool) {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		d, err := decimal.Parse(s)
		return d, err == nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return decimal.Decimal{}, false
	}
	d, special, ok := decimal.ParsePython(strconv.FormatFloat(f, 'g', -1, 64))
	return d, ok && !special
}

// Normalize é o _normalizar da origem: decomposição NFKD, só ASCII, minúsculas.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if r < utf8.RuneSelf {
			b.WriteRune(r)
		}
	}
	return strings.ToLower(b.String())
}

// keywords são as palavras de cada intenção, na ordem de prioridade (PALAVRAS da origem).
var keywords = []struct {
	kind  string
	words []string
}{
	{IntentPrioritize, []string{"prioriz", "carteira", "fila", "atacar primeiro", "quais empresas"}},
	{IntentSimulate, []string{"simul", "pro-labore", "pro labore", "corrigir", "correcao", "reforco"}},
	{IntentExplain, []string{"explic", "o que e", "o que significa", "por que", "porque", "como funciona"}},
	{IntentStatus, []string{"como esta", "situacao", "status", "fator r", "anexo"}},
}

var (
	reKeywordPA1  = pytext.MustCompile(`\b(0[1-9]|1[0-2])/(\d{4})\b`, false)
	reKeywordPA2  = pytext.MustCompile(`\b(\d{4})-(0[1-9]|1[0-2])\b`, false)
	reKeywordMeta = pytext.MustCompile(`meta\D{0,10}(\d{2})(?:[.,](\d{1,2}))?\s*%`, false)
)

// CitedCompany é a empresa citada no pedido (empresa_citada): o nome mais longo da lista que
// aparece no texto, comparando sem acento e sem maiúsculas. Nunca passa pelo LLM.
func CitedCompany(message string, names []string) *string {
	target := Normalize(message)
	var best *string
	bestLen := 0
	for i := range names {
		n := Normalize(names[i])
		if n == "" || !strings.Contains(target, n) {
			continue
		}
		if l := utf8.RuneCountInString(names[i]); best == nil || l > bestLen {
			best, bestLen = &names[i], l
		}
	}
	if best == nil {
		return nil
	}
	name := *best
	return &name
}

// ClassifyByKeywords é o classificador sem LLM (classificar_por_palavras): o fallback do plan.
func ClassifyByKeywords(message string, names []string) Intent {
	target := Normalize(message)
	in := Intent{Kind: IntentStatus, CompanyRef: CitedCompany(message, names)}
	for _, k := range keywords {
		if containsAny(target, k.words) {
			in.Kind = k.kind
			break
		}
	}
	if m, ok := reKeywordPA1.Search(message); ok {
		month, _ := m.Group(1)
		year, _ := m.Group(2)
		in.PA = validPA(year + "-" + month)
	} else if m, ok := reKeywordPA2.Search(message); ok {
		year, _ := m.Group(1)
		month, _ := m.Group(2)
		in.PA = validPA(year + "-" + month)
	}
	if m, ok := reKeywordMeta.Search(target); ok {
		whole, _ := m.Group(1)
		frac, ok := m.Group(2)
		if !ok {
			frac = "0"
		}
		v := decimal.MustParse(whole + "." + frac).Quo(hundred)
		if metaInRange(v) {
			in.Meta = &v
		}
	}
	return in
}

func containsAny(s string, words []string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// intentSchema é o schema da saída estruturada do plan (a ferramenta classificar_intencao da
// origem, em JSON Schema estrito para o gateway).
var intentSchema = json.RawMessage(compactJSON(`{
	"type": "object",
	"properties": {
		"intencao": {"type": "string", "enum": ["status_empresa", "simular", "explicar", "priorizar"]},
		"company_ref": {"type": ["string", "null"]},
		"pa": {"type": ["string", "null"]},
		"meta": {"type": ["number", "null"]}
	},
	"required": ["intencao", "company_ref", "pa", "meta"],
	"additionalProperties": false
}`))

func compactJSON(s string) string {
	var b bytes.Buffer
	if err := json.Compact(&b, []byte(s)); err != nil {
		panic(err)
	}
	return b.String()
}
