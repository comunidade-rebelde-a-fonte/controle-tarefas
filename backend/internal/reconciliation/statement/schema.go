// Package statement é o domínio do extrato da Conciliação [REQ-020, REQ-021; TASK-016]: porte de
// app/domain/{schemas,validador,money,csv_extrato,pdf_extrato}.py do Conciliador (commit 2bc6e26).
//
//   - Statement é o ExtratoExtraido (schema congelado, PRD §7 da origem) e Triage, o
//     ResultadoTriagem. A saída do LLM passa pela mesma validação do pydantic da origem
//     (ParseStatement, ParseTriage), travada por golden gerado pelo Python da origem.
//   - Validate é o validar_extrato: o portão aritmético com tolerância R$ 0,00, usado também
//     como a ferramenta validar_saldo (a mesma função) [REQ-021, AC-016].
//   - FromCSV e FromPDFText são os leitores determinísticos paralelos ao agente (CC-08).
//
// Dinheiro sempre em decimal.Decimal com 2 casas (Money da origem); nunca float.
package statement

import (
	"errors"
	"strings"
	"time"

	"controle/internal/platform/decimal"
)

// Sentidos da transação.
const (
	Credit = "C"
	Debit  = "D"
)

// Tipos do documento na triagem.
const (
	KindStatement = "extrato"
	KindReceipt   = "comprovante"
	KindUnknown   = "desconhecido"
)

// Estratégias registradas no leiaute pelos leitores determinísticos.
const (
	StrategyCSV     = "csv_nativo"
	StrategyPDFText = "pdf_texto_nativo"
)

// ErrInvalidOutput é a saída do LLM que não passa pelo schema (TentativaParseError da origem):
// JSON inválido, campo faltando, tipo errado ou valor fora do contrato. Conta como tentativa.
var ErrInvalidOutput = errors.New("saída fora do schema")

// Transaction é uma linha do extrato (TransacaoExtraida).
type Transaction struct {
	Order        int
	Date         time.Time
	Description  string
	DocumentRef  *string
	Amount       decimal.Decimal // sempre > 0; o sinal vai em Direction
	Direction    string          // C ou D
	BalanceAfter *decimal.Decimal
	Confidence   decimal.Decimal
}

// Layout é o leiaute descoberto pelo extrator (LeiauteDescoberto), base do playbook do banco.
type Layout struct {
	TableStartPage     int
	IgnoredPages       []int64
	DateFormat         string
	DecimalSeparator   string
	SignRepresentation string
	BalanceLocation    string
	Strategy           string
}

// DefaultLayout é o leiaute com os padrões da origem.
func DefaultLayout() Layout {
	return Layout{TableStartPage: 1, IgnoredPages: []int64{}, DateFormat: "DD/MM/YYYY", DecimalSeparator: ",", Strategy: "texto_nativo"}
}

// Statement é o extrato extraído (ExtratoExtraido).
type Statement struct {
	Bank, Branch, Account string
	PeriodStart           time.Time
	PeriodEnd             time.Time
	OpeningBalance        decimal.Decimal
	ClosingBalance        decimal.Decimal
	Transactions          []Transaction
	Layout                *Layout
}

// Triage é o resultado do triador (ResultadoTriagem).
type Triage struct {
	Kind          string
	Bank          *string
	Branch        *string
	Account       *string
	PeriodStart   *time.Time
	PeriodEnd     *time.Time
	Confidence    decimal.Decimal
	Justification string
}

// --- JSON no formato do model_dump(mode="json") da origem ---

type pyDecimal struct{ d decimal.Decimal }

// MarshalJSON escreve o decimal como o str() do Python, como o pydantic.
func (p pyDecimal) MarshalJSON() ([]byte, error) { return marshalNoHTML(p.d.String()) }

type pyDate struct{ t time.Time }

func (p pyDate) MarshalJSON() ([]byte, error) { return marshalNoHTML(p.t.Format(time.DateOnly)) }

func optDecimal(d *decimal.Decimal) *pyDecimal {
	if d == nil {
		return nil
	}
	return &pyDecimal{*d}
}

func optDate(t *time.Time) *pyDate {
	if t == nil {
		return nil
	}
	return &pyDate{*t}
}

type transactionJSON struct {
	Order        int        `json:"ordem"`
	Date         pyDate     `json:"data"`
	Description  string     `json:"descricao"`
	DocumentRef  *string    `json:"documento"`
	Amount       pyDecimal  `json:"valor"`
	Direction    string     `json:"sentido"`
	BalanceAfter *pyDecimal `json:"saldo_apos"`
	Confidence   pyDecimal  `json:"confianca"`
}

type layoutJSON struct {
	TableStartPage     int     `json:"pagina_inicio_tabela"`
	IgnoredPages       []int64 `json:"paginas_ignorar"`
	DateFormat         string  `json:"formato_data"`
	DecimalSeparator   string  `json:"separador_decimal"`
	SignRepresentation string  `json:"representacao_sinal"`
	BalanceLocation    string  `json:"localizacao_saldos"`
	Strategy           string  `json:"estrategia"`
}

type statementJSON struct {
	Bank           string            `json:"banco"`
	Branch         string            `json:"agencia"`
	Account        string            `json:"conta"`
	PeriodStart    pyDate            `json:"periodo_inicio"`
	PeriodEnd      pyDate            `json:"periodo_fim"`
	OpeningBalance pyDecimal         `json:"saldo_inicial"`
	ClosingBalance pyDecimal         `json:"saldo_final"`
	Transactions   []transactionJSON `json:"transacoes"`
	Layout         *layoutJSON       `json:"leiaute"`
}

func (l Layout) json() layoutJSON {
	pages := l.IgnoredPages
	if pages == nil {
		pages = []int64{}
	}
	return layoutJSON{
		TableStartPage: l.TableStartPage, IgnoredPages: pages, DateFormat: l.DateFormat,
		DecimalSeparator: l.DecimalSeparator, SignRepresentation: l.SignRepresentation,
		BalanceLocation: l.BalanceLocation, Strategy: l.Strategy,
	}
}

// MarshalJSON escreve o leiaute no formato da origem (as chaves do PRD).
func (l Layout) MarshalJSON() ([]byte, error) { return marshalNoHTML(l.json()) }

// MarshalJSON escreve o extrato como o model_dump(mode="json") da origem: as chaves do PRD §7,
// datas ISO e decimais como texto.
func (s Statement) MarshalJSON() ([]byte, error) {
	out := statementJSON{
		Bank: s.Bank, Branch: s.Branch, Account: s.Account,
		PeriodStart: pyDate{s.PeriodStart}, PeriodEnd: pyDate{s.PeriodEnd},
		OpeningBalance: pyDecimal{s.OpeningBalance}, ClosingBalance: pyDecimal{s.ClosingBalance},
		Transactions: make([]transactionJSON, 0, len(s.Transactions)),
	}
	for _, t := range s.Transactions {
		out.Transactions = append(out.Transactions, transactionJSON{
			Order: t.Order, Date: pyDate{t.Date}, Description: t.Description, DocumentRef: t.DocumentRef,
			Amount: pyDecimal{t.Amount}, Direction: t.Direction, BalanceAfter: optDecimal(t.BalanceAfter),
			Confidence: pyDecimal{t.Confidence},
		})
	}
	if s.Layout != nil {
		l := s.Layout.json()
		out.Layout = &l
	}
	return marshalNoHTML(out)
}

type triageJSON struct {
	Kind          string    `json:"tipo"`
	Bank          *string   `json:"banco"`
	Branch        *string   `json:"agencia"`
	Account       *string   `json:"conta"`
	PeriodStart   *pyDate   `json:"periodo_inicio"`
	PeriodEnd     *pyDate   `json:"periodo_fim"`
	Confidence    pyDecimal `json:"confianca"`
	Justification string    `json:"justificativa"`
}

// MarshalJSON escreve a triagem como o model_dump(mode="json") da origem.
func (t Triage) MarshalJSON() ([]byte, error) {
	return marshalNoHTML(triageJSON{
		Kind: t.Kind, Bank: t.Bank, Branch: t.Branch, Account: t.Account,
		PeriodStart: optDate(t.PeriodStart), PeriodEnd: optDate(t.PeriodEnd),
		Confidence: pyDecimal{t.Confidence}, Justification: t.Justification,
	})
}

// --- Saída do LLM: json.loads(extrair_json(conteudo)) e model_validate ---

// ExtractJSON tira as cercas de markdown e a prosa em volta do JSON (extrair_json da origem): só
// a embalagem sai; o conteúdo continua validado pelo schema.
func ExtractJSON(text string) string {
	t := pyStrip(text)
	if strings.HasPrefix(t, "```") {
		if i := strings.Index(t, "\n"); i != -1 {
			t = t[i+1:]
		} else {
			t = ""
		}
		t = pyRstrip(t)
		t = strings.TrimSuffix(t, "```")
		t = pyStrip(t)
	}
	if !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
		start, end := strings.Index(t, "{"), strings.LastIndex(t, "}")
		if start != -1 && end > start {
			t = t[start : end+1]
		}
	}
	return t
}

// Limites dos campos no banco (numeric(14,2), integer e a faixa da confiança).
var (
	moneyLimit = decimal.MustParse("1000000000000")
	one        = decimal.FromInt(1)
	maxInt32   = int64(1<<31 - 1)
)

// ParseStatement valida a saída do extrator como ExtratoExtraido.model_validate da origem, mais
// os limites do banco (ordem e página de 1 a 2³¹−1, confiança de 0 a 1, valor dentro de
// numeric(14,2), período em ordem). Recusa com ErrInvalidOutput.
func ParseStatement(content string) (Statement, error) {
	raw, err := loadsObject(ExtractJSON(content))
	if err != nil {
		return Statement{}, err
	}
	var v validator
	s := Statement{
		Bank:           v.str(raw, "banco"),
		Branch:         v.str(raw, "agencia"),
		Account:        v.str(raw, "conta"),
		PeriodStart:    v.date(raw, "periodo_inicio"),
		PeriodEnd:      v.date(raw, "periodo_fim"),
		OpeningBalance: v.money(raw, "saldo_inicial"),
		ClosingBalance: v.money(raw, "saldo_final"),
	}
	items, ok := raw["transacoes"].([]any)
	if !ok {
		v.fail("transacoes")
	}
	s.Transactions = make([]Transaction, 0, len(items))
	for _, it := range items {
		s.Transactions = append(s.Transactions, v.transaction(it))
	}
	if l, present := raw["leiaute"]; present && l != nil {
		layout := v.layout(l)
		s.Layout = &layout
	}
	if v.err != nil {
		return Statement{}, v.err
	}
	if err := guardStatement(s); err != nil {
		return Statement{}, err
	}
	return s, nil
}

// guardStatement aplica os limites do banco que o pydantic da origem não tinha.
func guardStatement(s Statement) error {
	if s.PeriodEnd.Before(s.PeriodStart) {
		return invalid("período invertido")
	}
	money := []decimal.Decimal{s.OpeningBalance, s.ClosingBalance}
	for _, t := range s.Transactions {
		if t.Order < 1 || int64(t.Order) > maxInt32 {
			return invalid("ordem fora de 1 a 2³¹−1")
		}
		if t.Confidence.Sign() < 0 || t.Confidence.Cmp(one) > 0 {
			return invalid("confiança fora de 0 a 1")
		}
		money = append(money, t.Amount)
		if t.BalanceAfter != nil {
			money = append(money, *t.BalanceAfter)
		}
	}
	for _, m := range money {
		if m.Abs().Cmp(moneyLimit) >= 0 {
			return invalid("valor fora do limite")
		}
	}
	if l := s.Layout; l != nil {
		if l.TableStartPage < 1 || int64(l.TableStartPage) > maxInt32 {
			return invalid("página inicial fora de 1 a 2³¹−1")
		}
		for _, p := range l.IgnoredPages {
			if p > maxInt32 || p < -maxInt32 {
				return invalid("página ignorada fora do limite")
			}
		}
	}
	return nil
}

// ParseTriage valida a saída do triador como ResultadoTriagem.model_validate da origem, mais os
// limites do banco (confiança de 0 a 1, período em ordem).
func ParseTriage(content string) (Triage, error) {
	raw, err := loadsObject(ExtractJSON(content))
	if err != nil {
		return Triage{}, err
	}
	var v validator
	t := Triage{
		Kind:          v.literal(raw, "tipo", KindStatement, KindReceipt, KindUnknown),
		Bank:          v.optStr(raw, "banco"),
		Branch:        v.optStr(raw, "agencia"),
		Account:       v.optStr(raw, "conta"),
		PeriodStart:   v.optDate(raw, "periodo_inicio"),
		PeriodEnd:     v.optDate(raw, "periodo_fim"),
		Confidence:    v.decimalField(raw["confianca"], "confianca"),
		Justification: "",
	}
	if j, present := raw["justificativa"]; present {
		s, ok := j.(string)
		if !ok {
			v.fail("justificativa")
		}
		t.Justification = s
	}
	if v.err != nil {
		return Triage{}, v.err
	}
	if t.Confidence.Sign() < 0 || t.Confidence.Cmp(one) > 0 {
		return Triage{}, invalid("confiança fora de 0 a 1")
	}
	if t.PeriodStart != nil && t.PeriodEnd != nil && t.PeriodEnd.Before(*t.PeriodStart) {
		return Triage{}, invalid("período invertido")
	}
	return t, nil
}
