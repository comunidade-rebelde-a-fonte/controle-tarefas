package entries

import (
	"time"

	"controle/internal/platform/decimal"
)

// FieldsPatch é a correção campo a campo do comprovante na fila (TASK-021a): só os campos enviados
// mudam; texto vazio limpa o campo opcional (contraparte, documento da contraparte, documento).
// As regras de cada campo são as do lançamento manual.
type FieldsPatch struct {
	Date            *string `json:"date"`
	Amount          *string `json:"amount"`
	Direction       *string `json:"direction"`
	Counterparty    *string `json:"counterparty"`
	CounterpartyDoc *string `json:"counterparty_doc"`
	DocumentRef     *string `json:"document_ref"`
}

// Patched são os campos validados da correção: os novos valores e as chaves (as da origem:
// data, valor, sentido, contraparte, contraparte_doc, documento) dos campos que mudaram.
type Patched struct {
	Date            *time.Time
	Amount          *decimal.Decimal
	Direction       *string
	Counterparty    *string
	CounterpartyDoc *string
	DocumentRef     *string
	Set             []string
}

// Validate valida a correção. Os erros vêm por campo (os nomes da API).
func (in FieldsPatch) Validate() (Patched, map[string]string) {
	errs := fieldErrors{}
	var p Patched
	if in.Date != nil {
		d := parseDate(*in.Date, errs)
		p.Date, p.Set = &d, append(p.Set, "data")
	}
	if in.Amount != nil {
		a := parseAmount(*in.Amount, errs)
		p.Amount, p.Set = &a, append(p.Set, "valor")
	}
	if in.Direction != nil {
		d := parseDirection(*in.Direction, errs)
		p.Direction, p.Set = &d, append(p.Set, "sentido")
	}
	if in.Counterparty != nil {
		p.Counterparty, p.Set = optionalText(in.Counterparty, "counterparty", errs), append(p.Set, "contraparte")
	}
	if in.CounterpartyDoc != nil {
		p.CounterpartyDoc, p.Set = parseDoc(in.CounterpartyDoc, errs), append(p.Set, "contraparte_doc")
	}
	if in.DocumentRef != nil {
		p.DocumentRef, p.Set = optionalText(in.DocumentRef, "document_ref", errs), append(p.Set, "documento")
	}
	if len(p.Set) == 0 {
		errs.add("fields", "Informe ao menos um campo para corrigir.")
	}
	if len(errs) > 0 {
		return Patched{}, errs
	}
	return p, nil
}
