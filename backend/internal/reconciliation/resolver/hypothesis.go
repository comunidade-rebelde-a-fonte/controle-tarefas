// Package resolver é o domínio do resolvedor de exceções da Conciliação [REQ-025; TASK-019,
// D-030]: porte do HipoteseResolvedor (app/domain/schemas.py), do contexto que o modelo recebe
// (contexto_resolvedor) e do aceite (aceitar_hip) do Conciliador (commit 2bc6e26).
//
// O resolvedor só propõe: a hipótese, a justificativa e os pares são gravados como proposta, e só
// o aceite humano cria conciliação (origem manual, método resolvedor) [AC-019]. A validação da
// saída é travada por golden gerado pelo próprio Python da origem (testdata/resolver_gen.py).
package resolver

import (
	"encoding/json"

	"controle/internal/platform/decimal"
	"controle/internal/platform/pymodel"
)

// ErrInvalidOutput é a saída do resolvedor fora do schema.
var ErrInvalidOutput = pymodel.ErrInvalidOutput

// Tipos de par da origem (o prompt pede um deles; o schema aceita qualquer texto).
const (
	KindOneToOne          = "1:1"
	KindNToOne            = "N:1"
	KindWithoutReceipt    = "extrato_sem_comprovante"
	KindWithoutStatement  = "comprovante_sem_extrato"
	KindGrossNet          = "bruto_liquido"
	KindInternalTransfer  = "transferencia_interna"
	KindReversal          = "estorno"
	KindOther             = "outro"
	pairConfidenceDefault = "0.00"
)

// Kinds são os tipos de par do prompt da origem.
var Kinds = []string{KindOneToOne, KindNToOne, KindWithoutReceipt, KindWithoutStatement, KindGrossNet, KindInternalTransfer, KindReversal, KindOther}

// Pair é o ParProposto: a linha do extrato (ou nenhuma), os lançamentos, o tipo e a confiança.
type Pair struct {
	StatementID *string
	EntryIDs    []string
	Kind        string
	Confidence  decimal.Decimal
}

// Hypothesis é o HipoteseResolvedor.
type Hypothesis struct {
	Hypothesis    string
	Justification string
	Pairs         []Pair
	Confidence    decimal.Decimal
	AsksHuman     bool
}

type pairJSON struct {
	StatementID *string         `json:"extrato_transacao_id"`
	EntryIDs    []string        `json:"lancamento_cliente_ids"`
	Kind        string          `json:"tipo"`
	Confidence  pymodel.Decimal `json:"confianca"`
}

type hypothesisJSON struct {
	Hypothesis    string          `json:"hipotese"`
	Justification string          `json:"justificativa"`
	Pairs         []pairJSON      `json:"pares_propostos"`
	Confidence    pymodel.Decimal `json:"confianca"`
	AsksHuman     bool            `json:"pede_humano"`
}

// MarshalJSON escreve a hipótese como o model_dump(mode="json") da origem: é o payload da proposta.
func (h Hypothesis) MarshalJSON() ([]byte, error) {
	pairs := make([]pairJSON, len(h.Pairs))
	for i, p := range h.Pairs {
		ids := p.EntryIDs
		if ids == nil {
			ids = []string{}
		}
		pairs[i] = pairJSON{StatementID: p.StatementID, EntryIDs: ids, Kind: p.Kind, Confidence: pymodel.Decimal{Decimal: p.Confidence}}
	}
	return pymodel.Marshal(hypothesisJSON{Hypothesis: h.Hypothesis, Justification: h.Justification, Pairs: pairs,
		Confidence: pymodel.Decimal{Decimal: h.Confidence}, AsksHuman: h.AsksHuman})
}

var one = decimal.FromInt(1)

// Parse valida a saída do resolvedor como o HipoteseResolvedor da origem (extra ignorado; par sem
// confiança vale 0,00; pede_humano padrão true). Além do que o pydantic recusava, recusa a
// confiança fora de 0 a 1 (guarda do porte, como nos extratores).
func Parse(content string) (Hypothesis, error) {
	raw, err := pymodel.LoadsObject(pymodel.ExtractJSON(content))
	if err != nil {
		return Hypothesis{}, err
	}
	var v pymodel.Validator
	h := Hypothesis{
		Hypothesis:    v.Str(raw, "hipotese"),
		Justification: v.Str(raw, "justificativa"),
		Pairs:         []Pair{},
		Confidence:    v.Decimal(raw["confianca"], "confianca"),
		AsksHuman:     true,
	}
	if p, present := raw["pares_propostos"]; present {
		items, ok := p.([]any)
		if !ok {
			v.Fail("pares_propostos")
		}
		for _, it := range items {
			h.Pairs = append(h.Pairs, parsePair(&v, it))
		}
	}
	if b, present := raw["pede_humano"]; present {
		asks, ok := b.(bool)
		if !ok {
			v.Fail("pede_humano")
		}
		h.AsksHuman = asks
	}
	if err := v.Err(); err != nil {
		return Hypothesis{}, err
	}
	for _, c := range append([]decimal.Decimal{h.Confidence}, pairConfidences(h.Pairs)...) {
		if c.Sign() < 0 || c.Cmp(one) > 0 {
			return Hypothesis{}, pymodel.Invalid("confiança fora de 0 a 1")
		}
	}
	return h, nil
}

func pairConfidences(pairs []Pair) []decimal.Decimal {
	out := make([]decimal.Decimal, len(pairs))
	for i, p := range pairs {
		out[i] = p.Confidence
	}
	return out
}

// parsePair é o ParProposto (extra ignorado).
func parsePair(v *pymodel.Validator, x any) Pair {
	raw, ok := x.(map[string]any)
	if !ok {
		v.Fail("pares_propostos")
		return Pair{}
	}
	p := Pair{
		StatementID: v.OptStr(raw, "extrato_transacao_id"),
		EntryIDs:    []string{},
		Kind:        v.Str(raw, "tipo"),
		Confidence:  decimal.MustParse(pairConfidenceDefault),
	}
	if ids, present := raw["lancamento_cliente_ids"]; present {
		p.EntryIDs = v.StrList(ids, "lancamento_cliente_ids")
	}
	if c, present := raw["confianca"]; present {
		p.Confidence = v.Decimal(c, "pares_propostos.confianca")
	}
	return p
}

// Payload é a hipótese como a proposta a grava (o JSON da origem).
func (h Hypothesis) Payload() []byte {
	b, err := json.Marshal(h)
	if err != nil {
		panic(err) // só tipos do pacote
	}
	return b
}
