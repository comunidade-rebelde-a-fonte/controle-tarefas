// Package receipt é o domínio do comprovante da Conciliação [REQ-023, REQ-030; TASK-017, D-028]:
// porte do ComprovanteExtraido (app/domain/schemas.py), do campos_abaixo_limiar
// (app/agentes/extrator_comprovante.py) e do parse_comprovante_texto
// (app/servicos/processar_local.py) do Conciliador (commit 2bc6e26), travados por golden gerado
// pelo próprio Python da origem (testdata/receipt_gen.py).
//
// O agente interpreta; o código valida: Parse recusa a saída fora do schema e Check aplica as
// regras de campo da M3.4 da origem. Campo essencial nulo, inválido ou abaixo do limiar não entra
// no razão: o lançamento fica parcial e vai à fila com o trecho de origem.
package receipt

import (
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pymodel"
	"controle/internal/platform/pytext"
)

// Sentidos do lançamento.
const (
	Credit = "C" // entrada na conta do cliente (recebimento)
	Debit  = "D" // saída da conta do cliente (pagamento)
)

// Campos do comprovante: as chaves de confianca_por_campo e origem_por_campo.
const (
	FieldDate            = "data"
	FieldAmount          = "valor"
	FieldDirection       = "sentido"
	FieldCounterparty    = "contraparte"
	FieldCounterpartyDoc = "contraparte_doc"
	FieldDocumentRef     = "documento"
	FieldOperationType   = "tipo_operacao"
)

// EssentialFields são os campos que o campos_abaixo_limiar confere, na ordem dele.
var EssentialFields = []string{FieldDate, FieldAmount, FieldDirection, FieldCounterparty}

// ErrInvalidOutput é a saída do extrator fora do schema (conta como tentativa).
var ErrInvalidOutput = pymodel.ErrInvalidOutput

// FieldOrigin é o OrigemCampo: onde o campo está no documento, para a fila destacar.
type FieldOrigin struct {
	Page    int       `json:"pagina"`
	Excerpt string    `json:"trecho"`
	BBox    []float64 `json:"bbox"` // [x0, y0, x1, y1] na página renderizada, ou null
}

// Receipt é o ComprovanteExtraido: todos os campos são opcionais (campo ilegível vem nulo).
type Receipt struct {
	Date            *time.Time
	Amount          *decimal.Decimal
	Direction       *string
	Counterparty    *string
	CounterpartyDoc *string
	DocumentRef     *string
	OperationType   *string
	Alerts          []string
	Confidence      map[string]decimal.Decimal
	Origin          map[string]FieldOrigin
}

type receiptJSON struct {
	Date            *pymodel.Date              `json:"data"`
	Amount          *pymodel.Decimal           `json:"valor"`
	Direction       *string                    `json:"sentido"`
	Counterparty    *string                    `json:"contraparte"`
	CounterpartyDoc *string                    `json:"contraparte_doc"`
	DocumentRef     *string                    `json:"documento"`
	OperationType   *string                    `json:"tipo_operacao"`
	Alerts          []string                   `json:"alertas"`
	Confidence      map[string]pymodel.Decimal `json:"confianca_por_campo"`
	Origin          map[string]FieldOrigin     `json:"origem_por_campo"`
}

// MarshalJSON escreve o comprovante como o model_dump(mode="json") da origem.
func (r Receipt) MarshalJSON() ([]byte, error) {
	conf := make(map[string]pymodel.Decimal, len(r.Confidence))
	for k, v := range r.Confidence {
		conf[k] = pymodel.Decimal{Decimal: v}
	}
	origin := r.Origin
	if origin == nil {
		origin = map[string]FieldOrigin{}
	}
	alerts := r.Alerts
	if alerts == nil {
		alerts = []string{}
	}
	return pymodel.Marshal(receiptJSON{
		Date: pymodel.OptDate(r.Date), Amount: pymodel.OptDecimal(r.Amount), Direction: r.Direction,
		Counterparty: r.Counterparty, CounterpartyDoc: r.CounterpartyDoc, DocumentRef: r.DocumentRef,
		OperationType: r.OperationType, Alerts: alerts, Confidence: conf, Origin: origin,
	})
}

// ConfidenceText é a confiança por campo como a origem gravava: o str() de cada Decimal.
func (r Receipt) ConfidenceText() map[string]string {
	out := make(map[string]string, len(r.Confidence))
	for k, v := range r.Confidence {
		out[k] = v.String()
	}
	return out
}

var (
	zero = decimal.FromInt(0)
	one  = decimal.FromInt(1)
)

// Parse valida a saída do extrator como o ComprovanteExtraido da origem (extra="ignore", todos os
// campos opcionais), com json.loads(extrair_json(conteudo)). Além do que o pydantic recusava,
// recusa a confiança fora de 0 a 1 (guarda do porte, como no extrato).
func Parse(content string) (Receipt, error) {
	raw, err := pymodel.LoadsObject(pymodel.ExtractJSON(content))
	if err != nil {
		return Receipt{}, err
	}
	var v pymodel.Validator
	r := Receipt{
		Date:            v.OptDate(raw, FieldDate),
		Amount:          v.OptMoney(raw, FieldAmount),
		Direction:       v.OptLiteral(raw, FieldDirection, Credit, Debit),
		Counterparty:    v.OptStr(raw, FieldCounterparty),
		CounterpartyDoc: v.OptStr(raw, FieldCounterpartyDoc),
		DocumentRef:     v.OptStr(raw, FieldDocumentRef),
		OperationType:   v.OptStr(raw, FieldOperationType),
		Alerts:          []string{},
		Confidence:      map[string]decimal.Decimal{},
		Origin:          map[string]FieldOrigin{},
	}
	if a, present := raw["alertas"]; present {
		r.Alerts = v.StrList(a, "alertas")
	}
	if c, present := raw["confianca_por_campo"]; present {
		obj, ok := c.(map[string]any)
		if !ok {
			v.Fail("confianca_por_campo")
		}
		for k, x := range obj {
			r.Confidence[k] = v.Decimal(x, "confianca_por_campo."+k)
		}
	}
	if o, present := raw["origem_por_campo"]; present {
		obj, ok := o.(map[string]any)
		if !ok {
			v.Fail("origem_por_campo")
		}
		for k, x := range obj {
			r.Origin[k] = parseOrigin(&v, k, x)
		}
	}
	if err := v.Err(); err != nil {
		return Receipt{}, err
	}
	for _, c := range r.Confidence {
		if c.Sign() < 0 || c.Cmp(one) > 0 {
			return Receipt{}, pymodel.Invalid("confiança fora de 0 a 1")
		}
	}
	return r, nil
}

// parseOrigin é o OrigemCampo: pagina (int, padrão 1), trecho (str, padrão "") e bbox
// (list[float] | None); chaves a mais são ignoradas.
func parseOrigin(v *pymodel.Validator, key string, x any) FieldOrigin {
	field := "origem_por_campo." + key
	raw, ok := x.(map[string]any)
	if !ok {
		v.Fail(field)
		return FieldOrigin{}
	}
	o := FieldOrigin{Page: 1}
	if p, present := raw["pagina"]; present {
		o.Page = v.Int32(p, field+".pagina")
	}
	if t, present := raw["trecho"]; present {
		s, ok := t.(string)
		if !ok {
			v.Fail(field + ".trecho")
		}
		o.Excerpt = s
	}
	if b, present := raw["bbox"]; present && b != nil {
		items, ok := b.([]any)
		if !ok {
			v.Fail(field + ".bbox")
			return o
		}
		o.BBox = make([]float64, 0, len(items))
		for _, it := range items {
			n, ok := it.(json.Number)
			f, err := n.Float64()
			if !ok || err != nil || math.IsInf(f, 0) {
				v.Fail(field + ".bbox")
				return o
			}
			o.BBox = append(o.BBox, f)
		}
	}
	return o
}

// value informa se o campo essencial veio preenchido (não None).
func (r Receipt) value(field string) bool {
	switch field {
	case FieldDate:
		return r.Date != nil
	case FieldAmount:
		return r.Amount != nil
	case FieldDirection:
		return r.Direction != nil
	case FieldCounterparty:
		return r.Counterparty != nil
	}
	return false
}

// Weak é o campos_abaixo_limiar da origem: os campos essenciais nulos ou com confiança abaixo do
// limiar (LIMIAR_CAMPO, 0,70 na origem); confiança ausente conta como 0.
func Weak(r Receipt, threshold decimal.Decimal) []string {
	weak := []string{}
	for _, f := range EssentialFields {
		conf, ok := r.Confidence[f]
		if !ok {
			conf = zero
		}
		if !r.value(f) || conf.Cmp(threshold) < 0 {
			weak = append(weak, f)
		}
	}
	return weak
}

// maxAmount é o maior valor de numeric(14,2), a coluna do lançamento.
var maxAmount = decimal.MustParse("999999999999.99")

// Check aplica as regras de campo da M3.4 da origem (valor > 0, sentido C/D, data válida e
// confiança ≥ limiar; o schema já garante sentido e data) e os formatos do banco, e devolve o
// comprovante pronto para gravar com os campos a revisar (Weak):
//   - valor ≤ 0 ou fora de numeric(14,2) fica nulo (e o valor vai à revisão);
//   - texto vazio fica nulo (contraparte vazia vai à revisão);
//   - contraparte_doc fica só com os dígitos; sem 11 (CPF) ou 14 (CNPJ), como o mascarado do
//     PIX, fica nulo. Não é campo essencial na origem: não manda o lançamento à revisão.
func Check(r Receipt, threshold decimal.Decimal) (Receipt, []string) {
	out := r
	if out.Amount != nil && (out.Amount.Sign() <= 0 || out.Amount.Cmp(maxAmount) > 0) {
		out.Amount = nil
	}
	out.Counterparty = nonEmpty(out.Counterparty)
	out.DocumentRef = nonEmpty(out.DocumentRef)
	out.OperationType = nonEmpty(out.OperationType)
	if out.CounterpartyDoc != nil {
		d := digits(*out.CounterpartyDoc)
		out.CounterpartyDoc = nil
		if len(d) == 11 || len(d) == 14 {
			out.CounterpartyDoc = &d
		}
	}
	return out, Weak(out, threshold)
}

func nonEmpty(s *string) *string {
	if s == nil || pymodel.Strip(*s) == "" {
		return nil
	}
	return s
}

// digits são os dígitos do texto, em ASCII.
func digits(s string) string {
	var b strings.Builder
	for _, r := range pytext.ASCIIDigits(s) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Marcas de borda do comprovante (alertas da origem): o matching (TASK-018) não concilia sozinho o
// lançamento marcado; ele vai à fila [REQ-030, PRD §12 da origem].
const (
	FlagInstallment = "parcelado"
	FlagInterest    = "juros"
	FlagISSWithheld = "iss_retido"
	// FlagInvoice é o prefixo da nota fiscal citada ("nf:NF 1041").
	FlagInvoice = "nf:"
)

// KnownFlags filtra os alertas do agente ao vocabulário da origem (as marcas que o código lê).
func KnownFlags(alerts []string) []string {
	out := []string{}
	for _, a := range alerts {
		a = pymodel.Strip(a)
		switch {
		case a == FlagInstallment || a == FlagInterest || a == FlagISSWithheld:
		case strings.HasPrefix(a, FlagInvoice) && strings.TrimFunc(a[len(FlagInvoice):], unicode.IsSpace) != "":
		default:
			continue
		}
		out = append(out, a)
	}
	return out
}

// MergeFlags junta marcas sem repetir, na ordem em que aparecem.
func MergeFlags(lists ...[]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, l := range lists {
		for _, f := range l {
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	return out
}
