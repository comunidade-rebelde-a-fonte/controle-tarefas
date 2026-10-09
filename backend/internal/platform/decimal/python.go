package decimal

import (
	"regexp"
	"strings"

	"github.com/cockroachdb/apd/v3"

	"controle/internal/platform/pytext"
)

// pythonNumber é a gramática de Decimal(str) do Python depois de tirar espaços e sublinhados e
// trocar os dígitos Unicode pelos ASCII: sinal, mantissa com ponto opcional ("1.", ".5"),
// expoente, e os valores especiais (sem diferenciar maiúsculas).
var pythonNumber = regexp.MustCompile(`(?i)^[+-]?(?:(?:([0-9]+)\.?([0-9]*)|\.([0-9]+))(?:e([+-]?[0-9]+))?|(inf|infinity|s?nan[0-9]*))$`)

// ParsePython lê o texto como Decimal(texto) do Python: tira espaços das pontas (str.isspace),
// ignora sublinhados, aceita dígitos Unicode, expoente e sinal "+". special indica Infinity ou
// NaN, que este tipo não representa; ok é falso quando o Python levantaria InvalidOperation.
func ParsePython(s string) (d Decimal, special, ok bool) {
	s = strings.TrimFunc(s, pytext.IsSpace)
	s = pytext.ASCIIDigits(strings.ReplaceAll(s, "_", ""))
	m := pythonNumber.FindStringSubmatch(s)
	if m == nil {
		return Decimal{}, false, false
	}
	if m[5] != "" {
		return Decimal{}, true, true
	}
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign = "-"
	}
	intPart, frac := m[1], m[2]
	if intPart == "" {
		intPart, frac = "0", m[3]
	}
	text := sign + intPart
	if frac != "" {
		text += "." + frac
	}
	if m[4] != "" {
		text += "e" + m[4]
	}
	v, _, err := apd.NewFromString(text)
	if err != nil {
		return Decimal{}, false, false
	}
	return Decimal{v}, false, true
}

// Normalize tira os zeros à direita, como normalize() do Python: 600000.00 vira 6E+5 e 0.00 vira
// 0. O valor não muda; só a representação.
func (d Decimal) Normalize() Decimal {
	r := new(apd.Decimal)
	_, cond, err := ctx.Reduce(r, d.apd())
	must(cond, err)
	return Decimal{r}
}

// Abs devolve |d|, arredondado ao contexto como abs() do Python.
func (d Decimal) Abs() Decimal {
	r := new(apd.Decimal)
	must(ctx.Abs(r, d.apd()))
	return Decimal{r}
}

// TryQuantize é Quantize sem pânico: false onde o Python levantaria InvalidOperation (o
// resultado não cabe nos 28 dígitos do contexto).
func (d Decimal) TryQuantize(exp int32) (Decimal, bool) {
	r := new(apd.Decimal)
	if _, err := ctx.Quantize(r, d.apd(), exp); err != nil {
		return Decimal{}, false
	}
	return Decimal{r}, true
}

// Key identifica o valor (não a representação): dois decimais iguais pelo valor têm a mesma
// chave, como no hash do Decimal do Python (0, -0 e 0.00 são o mesmo).
func (d Decimal) Key() string {
	n := d.Normalize()
	if n.IsZero() {
		return "0"
	}
	return n.String()
}
