// Package decimal representa dinheiro e percentuais como números decimais exatos, nunca
// float [CLAUDE.md]. A aritmética reproduz o decimal do Python usado pelo Fator R de origem
// (contexto padrão: 28 dígitos significativos, arredondamento half-even, especificação
// General Decimal Arithmetic), para que o motor portado dê os mesmos resultados, dígito a
// dígito (ADR-007).
//
// O valor é imutável: toda operação devolve um Decimal novo. O valor zero de Decimal é 0.
// Condições que o Python transforma em exceção (divisão por zero, operação inválida,
// estouro) causam pânico: quem chama garante, como no domínio de origem, que não ocorrem.
package decimal

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"

	"github.com/cockroachdb/apd/v3"
	"github.com/jackc/pgx/v5/pgtype"
)

// Precision é a precisão do contexto padrão do decimal do Python.
const Precision = 28

// ctx é o contexto padrão do Python: 28 dígitos e half-even. Os métodos de apd.Context não
// alteram o contexto, então ele pode ser compartilhado entre goroutines.
var ctx = &apd.Context{
	Precision:   Precision,
	MaxExponent: apd.MaxExponent,
	MinExponent: apd.MinExponent,
	Traps:       apd.DefaultTraps,
	Rounding:    apd.RoundHalfEven,
}

// zero é o 0 com expoente 0, o mesmo que Decimal("0") no Python. Nunca é alterado.
var zero apd.Decimal

// Decimal é um número decimal exato e imutável.
type Decimal struct{ v *apd.Decimal }

// Zero é o 0 (expoente 0), equivalente a Decimal("0") do Python.
var Zero = Decimal{}

// plain aceita só a notação de ponto fixo: sem expoente, sinal "+", NaN ou infinito.
var plain = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// ErrSyntax indica um texto que não é um decimal em notação de ponto fixo.
var ErrSyntax = errors.New("decimal: use dígitos com ponto decimal opcional, como 1234.56")

func (d Decimal) apd() *apd.Decimal {
	if d.v == nil {
		return &zero
	}
	return d.v
}

func must(_ apd.Condition, err error) {
	if err != nil {
		panic(fmt.Errorf("decimal: %w", err))
	}
}

// Parse lê um decimal em notação de ponto fixo ("1234.50", "-0.3"), preservando o expoente
// do texto, como Decimal("1234.50") no Python.
func Parse(s string) (Decimal, error) {
	if !plain.MatchString(s) {
		return Decimal{}, ErrSyntax
	}
	v, _, err := apd.NewFromString(s)
	if err != nil {
		return Decimal{}, ErrSyntax
	}
	return Decimal{v}, nil
}

// number aceita a sintaxe de número do JSON, com expoente (ex.: o custo "1.25e-05" que o
// gateway de IA informa).
var number = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// ParseNumber lê um número na sintaxe do JSON, com expoente opcional. Para valores vindos de
// serviços externos; entrada do usuário usa Parse.
func ParseNumber(s string) (Decimal, error) {
	if !number.MatchString(s) {
		return Decimal{}, ErrSyntax
	}
	v, _, err := apd.NewFromString(s)
	if err != nil {
		return Decimal{}, ErrSyntax
	}
	return Decimal{v}, nil
}

// MustParse é Parse que entra em pânico em erro (constantes e testes).
func MustParse(s string) Decimal {
	d, err := Parse(s)
	if err != nil {
		panic(fmt.Errorf("decimal %q: %w", s, err))
	}
	return d
}

// FromInt converte um inteiro (expoente 0), como Decimal(12) no Python.
func FromInt(i int64) Decimal { return Decimal{apd.New(i, 0)} }

// Add devolve d + o.
func (d Decimal) Add(o Decimal) Decimal {
	r := new(apd.Decimal)
	must(ctx.Add(r, d.apd(), o.apd()))
	return Decimal{r}
}

// Sub devolve d - o.
func (d Decimal) Sub(o Decimal) Decimal {
	r := new(apd.Decimal)
	must(ctx.Sub(r, d.apd(), o.apd()))
	return Decimal{r}
}

// Mul devolve d × o.
func (d Decimal) Mul(o Decimal) Decimal {
	r := new(apd.Decimal)
	must(ctx.Mul(r, d.apd(), o.apd()))
	return Decimal{r}
}

// Quo devolve d ÷ o. Divisor zero causa pânico (ZeroDivisionError no Python).
//
// Quando a divisão é exata, o resultado fica no expoente ideal (expoente de d menos o de
// o), como no Python: 139950.00 ÷ 500000.00 = 0.2799, e não 0.2799000…0 com 28 dígitos,
// que é como o apd devolve. O valor é o mesmo; só a representação muda.
func (d Decimal) Quo(o Decimal) Decimal {
	x, y := d.apd(), o.apd()
	r := new(apd.Decimal)
	cond, err := ctx.Quo(r, x, y)
	must(cond, err)
	if cond.Inexact() || r.IsZero() {
		return Decimal{r}
	}
	ideal := x.Exponent - y.Exponent
	var q, rem apd.BigInt
	ten := apd.NewBigInt(10)
	for r.Exponent < ideal {
		q.QuoRem(&r.Coeff, ten, &rem)
		if rem.Sign() != 0 {
			break
		}
		r.Coeff.Set(&q)
		r.Exponent++
	}
	return Decimal{r}
}

// Neg devolve -d, arredondado ao contexto como o operador unário do Python.
func (d Decimal) Neg() Decimal {
	r := new(apd.Decimal)
	must(ctx.Neg(r, d.apd()))
	return Decimal{r}
}

// Quantize arredonda d (half-even) para o expoente exp: -2 para centavos, -6 para
// percentuais com seis casas. Equivale a d.quantize(Decimal("1e<exp>"), ROUND_HALF_EVEN).
func (d Decimal) Quantize(exp int32) Decimal {
	r := new(apd.Decimal)
	must(ctx.Quantize(r, d.apd(), exp))
	return Decimal{r}
}

// Cmp compara pelo valor (-1, 0, 1), como os operadores do Python: 0.30 == 0.300000.
func (d Decimal) Cmp(o Decimal) int { return d.apd().Cmp(o.apd()) }

// Sign devolve -1, 0 ou 1.
func (d Decimal) Sign() int { return d.apd().Sign() }

// IsZero indica se o valor é zero (com qualquer expoente).
func (d Decimal) IsZero() bool { return d.apd().IsZero() }

// Exponent é o expoente da representação (-2 em 1234.50).
func (d Decimal) Exponent() int32 { return d.apd().Exponent }

// Max segue o max() do Python com dois argumentos: devolve b só se b > a; no empate, a.
// Max(Zero, 0.0000) é Zero, e isso preserva a representação do resultado de origem.
func Max(a, b Decimal) Decimal {
	if b.Cmp(a) > 0 {
		return b
	}
	return a
}

// Sum soma os valores a partir de Zero, como sum(valores, Decimal("0")) no Python.
func Sum(values ...Decimal) Decimal {
	total := Zero
	for _, v := range values {
		total = total.Add(v)
	}
	return total
}

// Fits indica se o valor cabe em intDigits dígitos inteiros e places casas decimais,
// ignorando zeros à direita da parte decimal (1.500 tem uma casa). É a regra do Numeric do
// Pydantic na origem (max_digits e decimal_places) e do numeric(p,s) do Postgres.
func (d Decimal) Fits(intDigits, places int) bool {
	v := d.apd()
	if v.Form != apd.Finite {
		return false
	}
	reduced := new(apd.Decimal)
	_, _ = reduced.Reduce(v)
	if reduced.IsZero() {
		return true
	}
	decimals := max(0, -int64(reduced.Exponent))
	whole := reduced.NumDigits() + int64(reduced.Exponent)
	return decimals <= int64(places) && whole <= int64(intDigits)
}

// String é a representação do str() do Python (to-scientific-string da especificação):
// "0.2799", "4000.0000", "0E+2", "0E-9". Usada para comparar com o golden da origem.
//
// O apd escreve o zero de expoente muito negativo sem notação exponencial ("0.000000000"),
// como o Postgres; a especificação e o Python usam "0E-9" quando o expoente ajustado fica
// abaixo de -6. Para o zero, o expoente ajustado é o próprio expoente.
func (d Decimal) String() string {
	v := d.apd()
	if v.Form == apd.Finite && v.IsZero() && v.Exponent < -6 {
		sign := ""
		if v.Negative {
			sign = "-"
		}
		return fmt.Sprintf("%s0E%d", sign, v.Exponent)
	}
	return v.String()
}

// Text é a notação de ponto fixo, sem expoente ("0.2799", "4000.0000", "0"): a forma que vai
// para a API. O zero de expoente positivo (0E+2) sai como "0", e não "000".
func (d Decimal) Text() string {
	v := d.apd()
	if v.IsZero() && v.Exponent > 0 {
		return "0"
	}
	return v.Text('f')
}

// MarshalJSON serializa como string em ponto fixo, preservando a precisão [06 §12].
func (d Decimal) MarshalJSON() ([]byte, error) { return json.Marshal(d.Text()) }

// UnmarshalJSON lê uma string em ponto fixo. Número JSON não é aceito: o contrato da API
// representa valores decimais como string.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return ErrSyntax
	}
	p, err := Parse(s)
	if err != nil {
		return err
	}
	*d = p
	return nil
}

// ScanNumeric lê uma coluna numeric do Postgres (pgx). NULL só é aceito em *Decimal.
//
// O pgx devolve o zero de qualquer escala com expoente 0 ("0.00" vira 0): quem precisa da
// escala da coluna, como o motor do Fator R, usa Quantize ao montar os valores.
func (d *Decimal) ScanNumeric(n pgtype.Numeric) error {
	if !n.Valid {
		return errors.New("decimal: NULL numa coluna não anulável")
	}
	if n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return errors.New("decimal: o banco devolveu NaN ou infinito")
	}
	v := new(apd.Decimal)
	v.Coeff.SetMathBigInt(new(big.Int).Abs(n.Int))
	v.Negative = n.Int.Sign() < 0
	v.Exponent = n.Exp
	*d = Decimal{v}
	return nil
}

// NumericValue grava o valor numa coluna numeric (pgx).
func (d Decimal) NumericValue() (pgtype.Numeric, error) {
	v := d.apd()
	i := v.Coeff.MathBigInt()
	if v.Negative {
		i.Neg(i)
	}
	return pgtype.Numeric{Int: i, Exp: v.Exponent, Valid: true}, nil
}
