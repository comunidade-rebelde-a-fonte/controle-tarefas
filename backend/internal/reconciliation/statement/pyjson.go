package statement

import (
	"bytes"
	"encoding/json"
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

// A saída do LLM é lida como na origem: json.loads e o model_validate do pydantic 2 em modo
// "lax", restrito aos tipos do schema estrito (texto, inteiro, número, nulo, lista e objeto onde o
// schema os pede). Fora deles, o porte recusa, mesmo onde o pydantic convertia (booleano como
// número, texto como inteiro, data como timestamp): o gateway pede o schema estrito, e a recusa
// só custa a tentativa.

func invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalidOutput, msg) }

func pyStrip(s string) string  { return strings.TrimFunc(s, pytext.IsSpace) }
func pyRstrip(s string) string { return strings.TrimRightFunc(s, pytext.IsSpace) }

// marshalNoHTML serializa sem os escapes de HTML do encoding/json (o pydantic não os faz).
func marshalNoHTML(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// loadsObject lê um único valor JSON (json.loads) e exige um objeto (o model_validate).
func loadsObject(s string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, invalid("JSON inválido")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, invalid("texto depois do JSON")
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, invalid("o JSON não é um objeto")
	}
	return obj, nil
}

// validator acumula a primeira falha de validação.
type validator struct{ err error }

func (v *validator) fail(field string) {
	if v.err == nil {
		v.err = invalid(field)
	}
}

func (v *validator) str(raw map[string]any, key string) string {
	s, ok := raw[key].(string)
	if !ok {
		v.fail(key)
	}
	return s
}

func (v *validator) optStr(raw map[string]any, key string) *string {
	switch s := raw[key].(type) {
	case nil:
		return nil
	case string:
		return &s
	default:
		v.fail(key)
		return nil
	}
}

func (v *validator) literal(raw map[string]any, key string, allowed ...string) string {
	s, ok := raw[key].(string)
	if ok {
		for _, a := range allowed {
			if s == a {
				return s
			}
		}
	}
	v.fail(key)
	return ""
}

func (v *validator) date(raw map[string]any, key string) time.Time {
	s, ok := raw[key].(string)
	if ok {
		if d, ok := parseDate(s); ok {
			return d
		}
	}
	v.fail(key)
	return time.Time{}
}

func (v *validator) optDate(raw map[string]any, key string) *time.Time {
	if raw[key] == nil {
		return nil
	}
	d := v.date(raw, key)
	return &d
}

// money é o Money da origem (parse_money): texto pelo Decimal() do Python, ou inteiro;
// arredondado a centavos (half-even). Número com fração ou expoente (float no Python) é recusado,
// como na origem ("float é proibido em valor monetário").
func (v *validator) money(raw map[string]any, key string) decimal.Decimal {
	d, ok := moneyValue(raw[key])
	if !ok {
		v.fail(key)
	}
	return d
}

func moneyValue(x any) (decimal.Decimal, bool) {
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

// decimalField é um campo Decimal do pydantic (confiança): inteiro exato, real pelo repr do
// float (0.30 vira 0.3) ou texto pelo Decimal(); infinito e NaN são recusados.
func (v *validator) decimalField(x any, field string) decimal.Decimal {
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
		v.fail(field)
	}
	return d
}

// intField é um campo int do pydantic: inteiro, ou real sem parte fracionária (1.0 vira 1).
func intField(x any) (int64, bool) {
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

func (v *validator) intValue(x any, field string) int {
	i, ok := intField(x)
	if !ok || i > maxInt32 || i < -maxInt32-1 {
		v.fail(field)
		return 0
	}
	return int(i)
}

// transactionKeys são os campos da TransacaoExtraida (extra="forbid": chave a mais recusa a linha).
var transactionKeys = map[string]bool{
	"ordem": true, "data": true, "descricao": true, "documento": true, "valor": true,
	"sentido": true, "saldo_apos": true, "confianca": true,
}

func (v *validator) transaction(item any) Transaction {
	raw, ok := item.(map[string]any)
	if !ok {
		v.fail("transacoes")
		return Transaction{}
	}
	for k := range raw {
		if !transactionKeys[k] {
			v.fail("transacoes." + k)
		}
	}
	t := Transaction{
		Order:       v.intValue(raw["ordem"], "ordem"),
		Date:        v.date(raw, "data"),
		Description: v.str(raw, "descricao"),
		DocumentRef: v.optStr(raw, "documento"),
		Amount:      v.money(raw, "valor"),
		Direction:   v.literal(raw, "sentido", Credit, Debit),
		Confidence:  decimal.MustParse("0.0000"),
	}
	if t.Amount.Sign() <= 0 {
		v.fail("valor deve ser > 0")
	}
	if b, present := raw["saldo_apos"]; present && b != nil {
		d := v.money(raw, "saldo_apos")
		t.BalanceAfter = &d
	}
	if c, present := raw["confianca"]; present {
		t.Confidence = v.decimalField(c, "confianca")
	}
	return t
}

// layout é o LeiauteDescoberto: campos opcionais com os padrões da origem; chaves a mais são
// ignoradas.
func (v *validator) layout(x any) Layout {
	raw, ok := x.(map[string]any)
	if !ok {
		v.fail("leiaute")
		return Layout{}
	}
	l := DefaultLayout()
	if p, present := raw["pagina_inicio_tabela"]; present {
		l.TableStartPage = v.intValue(p, "leiaute.pagina_inicio_tabela")
	}
	if p, present := raw["paginas_ignorar"]; present {
		items, ok := p.([]any)
		if !ok {
			v.fail("leiaute.paginas_ignorar")
		}
		l.IgnoredPages = make([]int64, 0, len(items))
		for _, it := range items {
			i, ok := intField(it)
			if !ok {
				v.fail("leiaute.paginas_ignorar")
			}
			l.IgnoredPages = append(l.IgnoredPages, i)
		}
	}
	for key, dst := range map[string]*string{
		"formato_data": &l.DateFormat, "separador_decimal": &l.DecimalSeparator,
		"representacao_sinal": &l.SignRepresentation, "localizacao_saldos": &l.BalanceLocation,
		"estrategia": &l.Strategy,
	} {
		if s, present := raw[key]; present {
			str, ok := s.(string)
			if !ok {
				v.fail("leiaute." + key)
			}
			*dst = str
		}
	}
	return l
}

// Datas como o pydantic 2 lê o texto: AAAA-MM-DD ou data e hora com hora zero (com ou sem
// segundos, fração e fuso); o resto é recusado.
var (
	reDate     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reDateTime = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[Tt ](\d{2}):(\d{2})(?::(\d{2})(?:[.,](\d+))?)?(?:[Zz]|[+-]\d{2}(?::?\d{2})?)?$`)
)

func parseDate(s string) (time.Time, bool) {
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
