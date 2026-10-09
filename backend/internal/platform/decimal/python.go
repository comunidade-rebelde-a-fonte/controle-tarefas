package decimal

import (
	"math"
	"regexp"
	"strconv"
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

// FromPythonFloat é Decimal(repr(f)) do Python, o que o pydantic faz com um número real do JSON
// num campo Decimal: 0.30 vira 0.3, 1.0 fica 1.0 e 1e-07 vira 1E-7. Infinito e NaN dão false.
func FromPythonFloat(f float64) (Decimal, bool) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return Decimal{}, false
	}
	d, special, ok := ParsePython(PythonFloatRepr(f))
	return d, ok && !special
}

// PythonFloatRepr é o repr() de um float finito no Python: os dígitos mais curtos que voltam ao
// mesmo float; notação fixa (sempre com ".0" quando inteiro) de 1e-4 até 1e16, científica fora
// disso ("1e-05", "1.5e+16").
func PythonFloatRepr(f float64) string {
	s := strconv.FormatFloat(f, 'e', -1, 64)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	mant, expText, _ := strings.Cut(s, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, _ := strconv.Atoi(expText)
	if exp >= -4 && exp < 16 {
		var intPart, frac string
		switch {
		case exp < 0:
			intPart, frac = "0", strings.Repeat("0", -exp-1)+digits
		case len(digits) <= exp+1:
			intPart, frac = digits+strings.Repeat("0", exp+1-len(digits)), "0"
		default:
			intPart, frac = digits[:exp+1], digits[exp+1:]
		}
		return sign + intPart + "." + frac
	}
	m := digits[:1]
	if len(digits) > 1 {
		m += "." + digits[1:]
	}
	es := "+"
	if exp < 0 {
		es, exp = "-", -exp
	}
	e := strconv.Itoa(exp)
	if len(e) < 2 {
		e = "0" + e
	}
	return sign + m + "e" + es + e
}
