package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/store"
)

// Detail é a conciliação no drill-down do razão (AC-021): a classificação (com a regra, a linha do
// histórico ou a sugestão que a produziu), a conciliação, a linha do extrato com o original e os
// comprovantes com os originais deles.
type Detail struct {
	ID             string           `json:"id"`
	CompanyID      string           `json:"company_id"`
	Method         string           `json:"method"`
	Origin         string           `json:"origin"`
	Pass           *int16           `json:"pass"`
	Score          *decimal.Decimal `json:"score"`
	ProposalID     *string          `json:"proposal_id"`
	CreatedBy      *string          `json:"created_by"`
	CreatedAt      time.Time        `json:"created_at"`
	UndoneAt       *time.Time       `json:"undone_at"`
	Transaction    Transaction      `json:"statement_transaction"`
	Entries        []Entry          `json:"entries"`
	Classification Classification   `json:"classification"`
}

// Transaction é a linha do extrato, com a importação e o original.
type Transaction struct {
	ID                  string          `json:"id"`
	Date                string          `json:"date"`
	Competence          string          `json:"competence"`
	Description         string          `json:"description"`
	Amount              decimal.Decimal `json:"amount"`
	Direction           string          `json:"direction"`
	BankAccount         string          `json:"bank_account"`
	StatementImportID   string          `json:"statement_import_id"`
	StatementDocumentID string          `json:"statement_document_id"`
}

// Entry é um comprovante (lançamento do cliente) da conciliação.
type Entry struct {
	ID              string           `json:"id"`
	Date            *string          `json:"date"`
	Amount          *decimal.Decimal `json:"amount"`
	Direction       *string          `json:"direction"`
	Counterparty    *string          `json:"counterparty"`
	CounterpartyDoc *string          `json:"counterparty_doc"`
	DocumentRef     *string          `json:"document_ref"`
	Status          string           `json:"status"`
	DocumentID      *string          `json:"document_id"`
}

// Classification é a classificação da linha e o que a produziu.
type Classification struct {
	AccountCode   *string    `json:"account_code"`
	AccountName   *string    `json:"account_name"`
	Origin        *string    `json:"origin"`
	Pending       bool       `json:"pending"`
	Rule          *Rule      `json:"rule"`
	HistoryLineID *string    `json:"history_line_id"`
	SuggestionID  *string    `json:"suggestion_id"`
	ClassifiedBy  *string    `json:"classified_by"`
	ClassifiedAt  *time.Time `json:"classified_at"`
}

// Rule é a regra que classificou.
type Rule struct {
	ID       string `json:"id"`
	RuleType string `json:"rule_type"`
	Pattern  string `json:"pattern"`
}

func reconciliationNotFound() error {
	return apperr.NotFound("RECONCILIATION_NOT_FOUND", "Conciliação não encontrada.")
}

// Reconciliation lê a conciliação para o drill-down (404 se não existe).
func (s *Service) Reconciliation(ctx context.Context, id string) (Detail, error) {
	if !httpx.IsUUID(id) {
		return Detail{}, reconciliationNotFound()
	}
	q := store.New(s.pool)
	r, err := q.GetReconciliationDetail(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, reconciliationNotFound()
	}
	if err != nil {
		return Detail{}, err
	}
	out := Detail{
		ID: r.ID, CompanyID: r.CompanyID, Method: r.Method, Origin: r.Origin, Pass: r.Pass, Score: r.Score, ProposalID: r.ProposalID,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UndoneAt: r.UndoneAt,
		Transaction: Transaction{
			ID: r.StatementTransactionID, Date: r.TxnDate.Format(time.DateOnly), Competence: competence.FromDate(r.TxnDate).String(),
			Description: r.Description, Amount: r.Amount, Direction: r.Direction, BankAccount: BankLabel(r.Bank, r.Branch, r.BankAccount),
			StatementImportID: r.StatementImportID, StatementDocumentID: r.StatementDocumentID,
		},
		Entries: []Entry{},
		Classification: Classification{
			AccountCode: r.AccountCode, AccountName: r.AccountName, Origin: r.ClassificationOrigin, Pending: r.ClassificationPending,
			HistoryLineID: r.ClassificationHistoryID, SuggestionID: r.ClassificationSuggestionID, ClassifiedBy: r.ClassifiedBy,
			ClassifiedAt: r.ClassifiedAt,
		},
	}
	if r.ClassificationRuleID != nil && r.RuleType != nil && r.RulePattern != nil {
		out.Classification.Rule = &Rule{ID: *r.ClassificationRuleID, RuleType: *r.RuleType, Pattern: *r.RulePattern}
	}
	entries, err := q.ListReconciliationEntries(ctx, id)
	if err != nil {
		return Detail{}, err
	}
	for _, e := range entries {
		var date *string
		if e.EntryDate != nil {
			d := e.EntryDate.Format(time.DateOnly)
			date = &d
		}
		out.Entries = append(out.Entries, Entry{ID: e.ID, Date: date, Amount: e.Amount, Direction: e.Direction, Counterparty: e.Counterparty,
			CounterpartyDoc: e.CounterpartyDoc, DocumentRef: e.DocumentRef, Status: e.Status, DocumentID: e.DocumentID})
	}
	return out, nil
}
