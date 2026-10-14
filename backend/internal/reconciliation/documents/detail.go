package documents

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/reconciliation/entries"
	"controle/internal/store"
)

// StatementImport é a importação do extrato do documento (só existe com o portão aprovado).
type StatementImport struct {
	ID             string          `json:"id"`
	CompanyID      string          `json:"company_id"`
	BankAccountID  *string         `json:"bank_account_id"`
	Bank           string          `json:"bank"`
	Branch         string          `json:"branch"`
	Account        string          `json:"account"`
	PeriodStart    string          `json:"period_start"`
	PeriodEnd      string          `json:"period_end"`
	OpeningBalance string          `json:"opening_balance"`
	ClosingBalance string          `json:"closing_balance"`
	Attempts       int32           `json:"attempts"`
	Validation     json.RawMessage `json:"validation"`
	Transactions   int64           `json:"transactions"`
	TraceID        *string         `json:"trace_id"`
	CreatedAt      time.Time       `json:"created_at"`
}

// ExtractionAttempt é uma linha do log de extração: uma tentativa do agente, a leitura
// determinística (tentativa 0) ou uma intervenção humana [CC-10]. A saída bruta não é exposta.
type ExtractionAttempt struct {
	ID            string          `json:"id"`
	Alias         string          `json:"alias"`
	Attempt       int32           `json:"attempt"`
	Model         *string         `json:"model"`
	PromptVersion *string         `json:"prompt_version"`
	Validation    json.RawMessage `json:"validation"`
	TokensIn      *int32          `json:"tokens_in"`
	TokensOut     *int32          `json:"tokens_out"`
	CostUSD       *string         `json:"cost_usd"`
	LatencyMs     *int32          `json:"latency_ms"`
	TraceID       *string         `json:"trace_id"`
	JobID         *string         `json:"job_id"`
	ActorID       *string         `json:"actor_id"`
	Change        *string         `json:"change"`
	Error         *string         `json:"error"`
	CreatedAt     time.Time       `json:"created_at"`
}

// DocumentDetail é o documento com a importação do extrato ou o lançamento do comprovante e o
// histórico das tentativas (AC-014).
type DocumentDetail struct {
	Document
	Import   *StatementImport    `json:"import"`
	Entry    *entries.Entry      `json:"entry"`
	Attempts []ExtractionAttempt `json:"attempts"`
}

func rawOrNull(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

// detail completa o documento com a importação e o log de extração.
func (s *Service) detail(ctx context.Context, doc Document) (DocumentDetail, error) {
	q := store.New(s.pool)
	out := DocumentDetail{Document: doc, Attempts: []ExtractionAttempt{}}
	imp, err := q.GetStatementImportByDocument(ctx, doc.ID)
	switch {
	case err == nil:
		out.Import = &StatementImport{
			ID: imp.ID, CompanyID: imp.CompanyID, BankAccountID: imp.BankAccountID, Bank: imp.Bank, Branch: imp.Branch,
			Account: imp.Account, PeriodStart: imp.PeriodStart.Format(time.DateOnly), PeriodEnd: imp.PeriodEnd.Format(time.DateOnly),
			OpeningBalance: imp.OpeningBalance.Text(), ClosingBalance: imp.ClosingBalance.Text(), Attempts: imp.Attempts,
			Validation: rawOrNull(imp.Validation), Transactions: imp.Transactions, TraceID: imp.TraceID, CreatedAt: imp.CreatedAt,
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return DocumentDetail{}, err
	}
	if out.Entry, err = entries.ByDocument(ctx, s.pool, doc.ID); err != nil {
		return DocumentDetail{}, err
	}
	logs, err := q.ListExtractionLogsByDocument(ctx, &doc.ID)
	if err != nil {
		return DocumentDetail{}, err
	}
	for _, l := range logs {
		a := ExtractionAttempt{
			ID: l.ID, Alias: l.Alias, Attempt: l.Attempt, Model: l.ResolvedModel, PromptVersion: l.PromptVersion,
			Validation: rawOrNull(l.ValidatorResult), TokensIn: l.TokensIn, TokensOut: l.TokensOut, LatencyMs: l.LatencyMs,
			TraceID: l.TraceID, JobID: l.JobID, ActorID: l.ActorID, Change: l.Change, Error: l.Error, CreatedAt: l.CreatedAt,
		}
		if l.CostUsd != nil {
			c := l.CostUsd.Text()
			a.CostUSD = &c
		}
		out.Attempts = append(out.Attempts, a)
	}
	return out, nil
}
