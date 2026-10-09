// Package pymodel lê a saída JSON do LLM com as regras do pydantic 2 do Conciliador de origem
// (commit 2bc6e26): json.loads(extrair_json(conteudo)) e model_validate em modo "lax", restrito aos
// tipos do schema estrito (texto, inteiro, número, nulo, lista e objeto onde o schema os pede).
// Fora deles o porte recusa, mesmo onde o pydantic convertia (booleano como número, texto como
// inteiro, data como timestamp): o gateway pede o schema estrito, e a recusa só custa a tentativa.
//
// Usado pelos schemas do extrato e da triagem (TASK-016) e do comprovante (TASK-017); os goldens
// gerados pelo Python da origem ficam nos pacotes de cada schema.
package pymodel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pytext"
)

// ErrInvalidOutput é a saída do LLM que não passa pelo schema (TentativaParseError da origem):
// JSON inválido, campo faltando, tipo errado ou valor fora do contrato.
var ErrInvalidOutput = errors.New("saída fora do schema")

// MaxInt32 é o maior inteiro das colunas integer do Postgres (ordem, página).
const MaxInt32 = int64(1<<31 - 1)

// Invalid é a recusa da saída, com o campo ou o motivo.
func Invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidOutput, msg) }

// Strip é o str.strip() do Python (espaços pelo str.isspace).
func Strip(s string) string { return strings.TrimFunc(s, pytext.IsSpace) }

// RStrip é o str.rstrip() do Python.
func RStrip(s string) string { return strings.TrimRightFunc(s, pytext.IsSpace) }

// Marshal serializa sem os escapes de HTML do encoding/json (o pydantic não os faz).
func Marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// Decimal serializa o decimal como o str() do Python, como o model_dump(mode="json").
type Decimal struct{ decimal.Decimal }

// MarshalJSON escreve o texto do str() do Python.
func (d Decimal) MarshalJSON() ([]byte, error) { return Marshal(d.String()) }

// OptDecimal é o Decimal opcional (None vira null).
func OptDecimal(d *decimal.Decimal) *Decimal {
	if d == nil {
		return nil
	}
	return &Decimal{*d}
}

// Date serializa a data em ISO (AAAA-MM-DD).
type Date struct{ time.Time }

// MarshalJSON escreve a data em ISO.
func (d Date) MarshalJSON() ([]byte, error) { return Marshal(d.Format(time.DateOnly)) }

// OptDate é a data opcional (None vira null).
func OptDate(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	return &Date{*t}
}

// ExtractJSON tira as cercas de markdown e a prosa em volta do JSON (extrair_json da origem): só
// a embalagem sai; o conteúdo continua validado pelo schema.
func ExtractJSON(text string) string {
	t := Strip(text)
	if strings.HasPrefix(t, "```") {
		if i := strings.Index(t, "\n"); i != -1 {
			t = t[i+1:]
		} else {
			t = ""
		}
		t = RStrip(t)
		t = strings.TrimSuffix(t, "```")
		t = Strip(t)
	}
	if !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
		start, end := strings.Index(t, "{"), strings.LastIndex(t, "}")
		if start != -1 && end > start {
			t = t[start : end+1]
		}
	}
	return t
}

// LoadsObject lê um único valor JSON (json.loads) e exige um objeto (o model_validate).
func LoadsObject(s string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, Invalid("JSON inválido")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, Invalid("texto depois do JSON")
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, Invalid("o JSON não é um objeto")
	}
	return obj, nil
}

// Validator acumula a primeira falha de validação de um objeto.
type Validator struct{ err error }

// Err é a primeira falha (nil se tudo passou).
func (v *Validator) Err() error { return v.err }

// Fail registra a falha do campo.
func (v *Validator) Fail(field string) {
	if v.err == nil {
		v.err = Invalid(field)
	}
}

// Str é o campo str obrigatório.
func (v *Validator) Str(raw map[string]any, key string) string {
	s, ok := raw[key].(string)
	if !ok {
		v.Fail(key)
	}
	return s
}

// OptStr é o campo str | None (ausente ou null viram nil).
func (v *Validator) OptStr(raw map[string]any, key string) *string {
	switch s := raw[key].(type) {
	case nil:
		return nil
	case string:
		return &s
	default:
		v.Fail(key)
		return nil
	}
}

// Literal é o campo Literal obrigatório.
func (v *Validator) Literal(raw map[string]any, key string, allowed ...string) string {
	s, ok := raw[key].(string)
	if ok {
		for _, a := range allowed {
			if s == a {
				return s
			}
		}
	}
	v.Fail(key)
	return ""
}

// OptLiteral é o campo Literal | None.
func (v *Validator) OptLiteral(raw map[string]any, key string, allowed ...string) *string {
	if raw[key] == nil {
		return nil
	}
	s := v.Literal(raw, key, allowed...)
	return &s
}

// Date é o campo date obrigatório.
func (v *Validator) Date(raw map[string]any, key string) time.Time {
	s, ok := raw[key].(string)
	if ok {
		if d, ok := ParseDate(s); ok {
			return d
		}
	}
	v.Fail(key)
	return time.Time{}
}

// OptDate é o campo date | None.
func (v *Validator) OptDate(raw map[string]any, key string) *time.Time {
	if raw[key] == nil {
		return nil
	}
	d := v.Date(raw, key)
	return &d
}

// Money é o Money da origem (parse_money): texto pelo Decimal() do Python, ou inteiro;
// arredondado a centavos (half-even). Número com fração ou expoente (float no Python) é recusado,
// como na origem ("float é proibido em valor monetário").
func (v *Validator) Money(raw map[string]any, key string) decimal.Decimal {
	d, ok := MoneyValue(raw[key])
	if !ok {
		v.Fail(key)
	}
	return d
}

// OptMoney é o campo Money | None.
func (v *Validator) OptMoney(raw map[string]any, key string) *decimal.Decimal {
	if raw[key] == nil {
		return nil
	}
	d := v.Money(raw, key)
	return &d
}

// MoneyValue converte o valor como o parse_money da origem.
func MoneyValue(x any) (decimal.Decimal, bool) {
	var d decimal.Decimal
	switch n := x.(type) {
	case string:
		p, special, ok := decimal.ParsePython(n)
		if !ok || special {
			return decimal.Decimal{}, false
		}
		d = p
	case json.Number:
		if isFloatLiteral(n) {
			return decimal.Decimal{}, false
		}
		p, err := decimal.Parse(n.String())
		if err != nil {
			return decimal.Decimal{}, false
		}
		d = p
	default:
		return decimal.Decimal{}, false
	}
	return d.TryQuantize(-2)
}

// isFloatLiteral: o json.loads do Python lê como float o número com ponto ou expoente.
func isFloatLiteral(n json.Number) bool { return strings.ContainsAny(n.String(), ".eE") }

// Decimal é um campo Decimal do pydantic (confiança): inteiro exato, real pelo repr do float (0.30
// vira 0.3) ou texto pelo Decimal(); infinito e NaN são recusados.
func (v *Validator) Decimal(x any, field string) decimal.Decimal {
	var d decimal.Decimal
	ok := false
	switch n := x.(type) {
	case json.Number:
		if isFloatLiteral(n) {
			f, err := strconv.ParseFloat(n.String(), 64)
			if err == nil {
				d, ok = decimal.FromPythonFloat(f)
			}
		} else {
			p, err := decimal.Parse(n.String())
			d, ok = p, err == nil
		}
	case string:
		p, special, valid := decimal.ParsePython(n)
		d, ok = p, valid && !special
	}
	if !ok {
		v.Fail(field)
	}
	return d
}

// IntValue é um campo int do pydantic: inteiro, ou real sem parte fracionária (1.0 vira 1).
func IntValue(x any) (int64, bool) {
	n, ok := x.(json.Number)
	if !ok {
		return 0, false
	}
	if !isFloatLiteral(n) {
		i, err := strconv.ParseInt(n.String(), 10, 64)
		return i, err == nil
	}
	f, err := strconv.ParseFloat(n.String(), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || math.Trunc(f) != f || math.Abs(f) >= 1<<63 {
		return 0, false
	}
	return int64(f), true
}

// Int32 é o campo int que vai para uma coluna integer: fora da faixa, recusa.
func (v *Validator) Int32(x any, field string) int {
	i, ok := IntValue(x)
	if !ok || i > MaxInt32 || i < -MaxInt32-1 {
		v.Fail(field)
		return 0
	}
	return int(i)
}

// StrList é o campo list[str].
func (v *Validator) StrList(x any, field string) []string {
	items, ok := x.([]any)
	if !ok {
		v.Fail(field)
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			v.Fail(field)
			return nil
		}
		out = append(out, s)
	}
	return out
}

// Datas como o pydantic 2 lê o texto: AAAA-MM-DD ou data e hora com hora zero (com ou sem
// segundos, fração e fuso); o resto é recusado.
var (
	reDate     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reDateTime = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[Tt ](\d{2}):(\d{2})(?::(\d{2})(?:[.,](\d+))?)?(?:[Zz]|[+-]\d{2}(?::?\d{2})?)?$`)
)

// ParseDate lê a data como o pydantic 2.
func ParseDate(s string) (time.Time, bool) {
	m := reDate.FindStringSubmatch(s)
	if m == nil {
		m = reDateTime.FindStringSubmatch(s)
		if m == nil {
			return time.Time{}, false
		}
		// Hora diferente de zero: date_from_datetime_inexact.
		for _, part := range m[4:] {
			if strings.Trim(part, "0") != "" {
				return time.Time{}, false
			}
		}
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	if y < 1 || mo < 1 || mo > 12 || d < 1 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d || int(t.Month()) != mo {
		return time.Time{}, false
	}
	return t, true
}
