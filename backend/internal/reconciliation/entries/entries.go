// Package entries são os lançamentos do cliente da Conciliação [REQ-023, CC-12; TASK-017, D-028]:
// os lidos dos comprovantes (pelo pipeline) e os inseridos à mão, pelo formulário e pela
// importação CSV (rotas /lancamentos e /lancamentos/csv do Conciliador, commit 2bc6e26).
//
// O lançamento manual nasce com o status "manual", passa pela trava do Contábil (AC-025) e fica no
// log de extração como intervenção humana. O CSV é tudo ou nada: com uma linha inválida, nada é
// gravado e a resposta traz o erro de cada linha (a origem pulava em silêncio a linha curta e
// derrubava a importação na primeira data ou valor ilegível).
package entries

import (
	"encoding/json"
	"time"

	"controle/internal/store"
)

// Status do lançamento (o lancamento_cliente da origem).
const (
	StatusPending   = "pendente"
	StatusPartial   = "parcial"  // comprovante com campo a revisar: fora do razão
	StatusExtracted = "extraido" // comprovante com os campos essenciais acima do limiar
	StatusManual    = "manual"   // inserido à mão
)

// Statuses são os status válidos (o CHECK da tabela).
var Statuses = []string{StatusPending, StatusPartial, StatusExtracted, StatusManual}

// Entry é o lançamento do cliente.
type Entry struct {
	ID                    string            `json:"id"`
	CompanyID             string            `json:"company_id"`
	DocumentID            *string           `json:"document_id"`
	Date                  *string           `json:"date"`
	Amount                *string           `json:"amount"`
	Direction             *string           `json:"direction"`
	Counterparty          *string           `json:"counterparty"`
	CounterpartyDoc       *string           `json:"counterparty_doc"`
	DocumentRef           *string           `json:"document_ref"`
	OperationType         *string           `json:"operation_type"`
	Status                string            `json:"status"`
	FieldConfidence       map[string]string `json:"field_confidence"`
	FieldOrigin           json.RawMessage   `json:"field_origin"`
	DuplicateFlagged      bool              `json:"duplicate_flagged"`
	Flags                 []string          `json:"flags"`
	AccountCode           *string           `json:"account_code"`
	ClassificationOrigin  *string           `json:"classification_origin"`
	ClassificationPending bool              `json:"classification_pending"`
	TraceID               *string           `json:"trace_id"`
	CreatedBy             *string           `json:"created_by"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// EntryList é uma página de lançamentos.
type EntryList struct {
	Items []Entry `json:"items"`
	Total int64   `json:"total"`
}

// FromRow converte a linha do banco.
func FromRow(r store.ClientEntry) Entry {
	e := Entry{
		ID: r.ID, CompanyID: r.CompanyID, DocumentID: r.DocumentID, Direction: r.Direction, Counterparty: r.Counterparty,
		CounterpartyDoc: r.CounterpartyDoc, DocumentRef: r.DocumentRef, OperationType: r.OperationType, Status: r.Status,
		FieldConfidence: map[string]string{}, FieldOrigin: json.RawMessage("{}"), DuplicateFlagged: r.DuplicateFlagged,
		Flags: []string{}, AccountCode: r.AccountCode, ClassificationOrigin: r.ClassificationOrigin,
		ClassificationPending: r.ClassificationPending, TraceID: r.TraceID, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if r.EntryDate != nil {
		d := r.EntryDate.Format(time.DateOnly)
		e.Date = &d
	}
	if r.Amount != nil {
		a := r.Amount.Text()
		e.Amount = &a
	}
	if len(r.FieldConfidence) > 0 {
		_ = json.Unmarshal(r.FieldConfidence, &e.FieldConfidence)
	}
	if len(r.FieldOrigin) > 0 {
		e.FieldOrigin = json.RawMessage(r.FieldOrigin)
	}
	// As marcas são texto; outra forma (de uma TASK futura) não entra na lista.
	var flags []any
	if json.Unmarshal(r.Flags, &flags) == nil {
		for _, f := range flags {
			if s, ok := f.(string); ok {
				e.Flags = append(e.Flags, s)
			}
		}
	}
	return e
}
