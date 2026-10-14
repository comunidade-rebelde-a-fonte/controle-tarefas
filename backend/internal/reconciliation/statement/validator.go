package statement

import (
	"slices"

	"controle/internal/platform/decimal"
)

// Motivos de linha rejeitada pelo validador.
const (
	ReasonNonPositive  = "valor_nao_positivo"
	ReasonBadDirection = "sentido_invalido"
)

// Dicas determinísticas para a nova tentativa do extrator (dicas_correcao da origem).
const (
	HintSkippedOrDuplicated = "diferenca_igual_a_linha_possivel_pulo_ou_duplicata"
	HintMagnitude           = "diferenca_ordem_grandeza_separador_decimal"
	HintDecimalSeparator    = "diferenca_sugere_separador_decimal"
)

// RejectedLine é uma linha rejeitada pelo validador.
type RejectedLine struct {
	Order  int    `json:"ordem"`
	Reason string `json:"motivo"`
}

// Result é o ResultadoValidacao da origem: o feedback do loop de correção e o registro de
// auditoria. O mesmo objeto decide a persistência (portão) e responde à ferramenta validar_saldo.
type Result struct {
	OK              bool
	Credits         decimal.Decimal
	Debits          decimal.Decimal
	ComputedBalance decimal.Decimal
	ExpectedBalance decimal.Decimal
	Difference      decimal.Decimal
	RejectedLines   []RejectedLine
	OrderGaps       []int
	DatesOutside    []int
}

type resultJSON struct {
	OK              bool           `json:"ok"`
	Credits         pyDecimal      `json:"soma_c"`
	Debits          pyDecimal      `json:"soma_d"`
	ComputedBalance pyDecimal      `json:"saldo_calculado"`
	ExpectedBalance pyDecimal      `json:"saldo_esperado"`
	Difference      pyDecimal      `json:"diferenca"`
	RejectedLines   []RejectedLine `json:"linhas_rejeitadas"`
	OrderGaps       []int          `json:"lacunas_ordem"`
	DatesOutside    []int          `json:"datas_fora_periodo"`
}

// MarshalJSON escreve o resultado como o model_dump_json() da origem, byte a byte: é o texto que
// vai no feedback da nova tentativa.
func (r Result) MarshalJSON() ([]byte, error) {
	nonNil := func(s []int) []int {
		if s == nil {
			return []int{}
		}
		return s
	}
	lines := r.RejectedLines
	if lines == nil {
		lines = []RejectedLine{}
	}
	return marshalNoHTML(resultJSON{
		OK: r.OK, Credits: pyDecimal{Decimal: r.Credits}, Debits: pyDecimal{Decimal: r.Debits},
		ComputedBalance: pyDecimal{Decimal: r.ComputedBalance}, ExpectedBalance: pyDecimal{Decimal: r.ExpectedBalance},
		Difference: pyDecimal{Decimal: r.Difference}, RejectedLines: lines, OrderGaps: nonNil(r.OrderGaps),
		DatesOutside: nonNil(r.DatesOutside),
	})
}

var cent int32 = -2

func money(d decimal.Decimal) decimal.Decimal { return d.Quantize(cent) }

// Validate é o validar_extrato da origem, puro e sem IO [REQ-021]. As 6 regras do PRD §7:
//   - valor > 0 e sentido C/D: a linha é rejeitada (e o lote não passa);
//   - data fora do período e lacuna na ordem: só sinalizam;
//   - saldo_inicial + ΣC − ΣD = saldo_final, com tolerância R$ 0,00: é a última regra e a única
//     que rejeita a importação inteira [AC-016].
func Validate(s Statement) Result {
	r := Result{RejectedLines: []RejectedLine{}, OrderGaps: []int{}, DatesOutside: []int{}}
	for _, t := range s.Transactions {
		if t.Amount.Sign() <= 0 {
			r.RejectedLines = append(r.RejectedLines, RejectedLine{Order: t.Order, Reason: ReasonNonPositive})
		}
		if t.Direction != Credit && t.Direction != Debit {
			r.RejectedLines = append(r.RejectedLines, RejectedLine{Order: t.Order, Reason: ReasonBadDirection})
		}
		if t.Date.Before(s.PeriodStart) || t.Date.After(s.PeriodEnd) {
			r.DatesOutside = append(r.DatesOutside, t.Order)
		}
	}
	if len(s.Transactions) > 0 {
		orders := make([]int, 0, len(s.Transactions))
		seen := map[int]bool{}
		for _, t := range s.Transactions {
			orders = append(orders, t.Order)
			seen[t.Order] = true
		}
		lo, hi := slices.Min(orders), slices.Max(orders)
		for n := lo; n <= hi; n++ {
			if !seen[n] {
				r.OrderGaps = append(r.OrderGaps, n)
			}
		}
	}
	credits, debits := decimal.MustParse("0.00"), decimal.MustParse("0.00")
	for _, t := range s.Transactions {
		switch t.Direction {
		case Credit:
			credits = credits.Add(t.Amount)
		case Debit:
			debits = debits.Add(t.Amount)
		}
	}
	r.Credits, r.Debits = money(credits), money(debits)
	r.ComputedBalance = money(s.OpeningBalance.Add(r.Credits).Sub(r.Debits))
	r.ExpectedBalance = money(s.ClosingBalance)
	r.Difference = money(r.ComputedBalance.Sub(r.ExpectedBalance))
	r.OK = r.Difference.IsZero() && len(r.RejectedLines) == 0
	return r
}

var (
	hundred      = decimal.FromInt(100)
	hundredth    = decimal.MustParse("0.01")
	hundredMoney = decimal.MustParse("100.00")
	ninetyNine   = decimal.FromInt(99)
	hundredOne   = decimal.FromInt(101)
)

// Hints são as dicas determinísticas da diferença (dicas_correcao da origem), na ordem dela:
// diferença igual ao valor de uma linha (linha pulada ou duplicada) e diferença de ordem de
// grandeza (separador decimal).
func Hints(s Statement, r Result) []string {
	hints := []string{}
	diff := r.Difference.Abs()
	if diff.IsZero() {
		return hints
	}
	for _, t := range s.Transactions {
		if t.Amount.Cmp(diff) == 0 {
			hints = append(hints, HintSkippedOrDuplicated)
			break
		}
	}
	if len(s.Transactions) > 0 {
		sample := s.Transactions[0].Amount
		ratio := decimal.Decimal{}
		if !sample.IsZero() {
			ratio = diff.Quo(sample)
		}
		switch {
		case !sample.IsZero() && (ratio.Cmp(hundred) == 0 || ratio.Cmp(hundredth) == 0):
			hints = append(hints, HintMagnitude)
		case r.Difference.Abs().Cmp(hundredMoney) >= 0 && r.Difference.Abs().Cmp(money(r.Difference.Abs())) == 0:
			if diff.Cmp(ninetyNine) >= 0 && diff.Cmp(hundredOne.Mul(decimal.Max(sample, decimal.FromInt(1)))) <= 0 {
				hints = append(hints, HintDecimalSeparator)
			}
		}
	}
	return hints
}
