// Package reconcile é a rodada do matching da Conciliação numa empresa e competência [REQ-024,
// REQ-030; TASK-018, D-029, ADR-008]: o rodar_matching da origem. Carrega as linhas livres da
// competência, os vizinhos (para as bordas) e os lançamentos da empresa, aplica a política do
// matching (internal/reconciliation/matching) e grava, na mesma transação, as conciliações
// automáticas e as marcas da fila. Toda rodada passa pela trava do Contábil (AC-025) e é
// serializada por empresa.
//
// Roda pela API (POST /companies/{id}/reconciliation/run) e pelo job reconciliation_run, que o
// pipeline e os lançamentos manuais enfileiram quando chegam dados novos (a origem rodava o
// matching depois de cada envio).
package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/entries"
	"controle/internal/reconciliation/matching"
	"controle/internal/store"
)

// Service executa a rodada.
type Service struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	opts   matching.Options
	logger *slog.Logger
}

// NewService cria o serviço. opts são os parâmetros da cascata (OptionsFrom).
func NewService(pool *pgxpool.Pool, clk clock.Clock, opts matching.Options, logger *slog.Logger) *Service {
	return &Service{pool: pool, clock: clk, opts: opts, logger: logger}
}

// OptionsFrom são os parâmetros da cascata com o limiar do passe 3 da configuração (LIMIAR_FUZZY),
// lido como o float() do Python. Zero (configuração não carregada) fica no padrão da origem: com
// limiar zero, o passe 3 casaria qualquer par de mesmo valor.
func OptionsFrom(fuzzy decimal.Decimal) matching.Options {
	o := matching.DefaultOptions()
	if f, err := strconv.ParseFloat(fuzzy.String(), 64); err == nil && f > 0 {
		o.FuzzyThreshold = f
	}
	return o
}

// Summary é o resultado da rodada e os baldes da competência (PRD §8 da origem).
type Summary struct {
	CompanyID  string `json:"company_id"`
	Competence string `json:"competence"`
	// NewMatches são as conciliações automáticas desta rodada; ByPass, por passe.
	NewMatches int            `json:"new_matches"`
	ByPass     map[string]int `json:"by_pass"`
	// Reconciled são as linhas da competência conciliadas (balde "conciliado").
	Reconciled int `json:"reconciled"`
	// Divergent são as linhas livres marcadas para a fila: divergência ou caso de borda.
	Divergent int `json:"divergent"`
	// WithoutReceipt são as linhas livres sem marca (balde "extrato sem comprovante").
	WithoutReceipt int `json:"without_receipt"`
	// WithoutStatement são os lançamentos livres da competência que passaram pelo automático sem
	// linha (balde "comprovante sem extrato").
	WithoutStatement int `json:"without_statement"`
	// AwaitingReview são os lançamentos parciais da competência: ficam fora do automático até a
	// correção campo a campo (ADR-008).
	AwaitingReview int `json:"awaiting_review"`
}

// Run executa a rodada numa transação própria (a API).
func (s *Service) Run(ctx context.Context, companyID string, comp competence.Competence) (Summary, error) {
	if !httpx.IsUUID(companyID) {
		return Summary{}, companyNotFound()
	}
	var out Summary
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := store.New(tx).GetCompanyIdentity(ctx, companyID); errors.Is(err, pgx.ErrNoRows) {
			return companyNotFound()
		} else if err != nil {
			return err
		}
		var err error
		out, err = s.RunTx(ctx, tx, companyID, comp)
		return err
	})
	return out, err
}

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

// eligible informa se o lançamento entra no automático: livre, completo e extraído ou manual (o
// parcial espera a revisão, ADR-008).
func eligible(r store.ListClientEntriesForMatchingRow) bool {
	return r.Free && r.EntryDate != nil && r.Amount != nil && r.Direction != nil &&
		(r.Status == entries.StatusExtracted || r.Status == entries.StatusManual)
}

// stringFlags são as marcas de texto do lançamento (as do comprovante).
func stringFlags(raw []byte) []string {
	var list []any
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	var out []string
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// RunTx executa a rodada na transação dada (o job e a API).
func (s *Service) RunTx(ctx context.Context, tx pgx.Tx, companyID string, comp competence.Competence) (Summary, error) {
	q := store.New(tx)
	if err := q.LockReconciliationCompany(ctx, companyID); err != nil {
		return Summary{}, err
	}
	if err := reconciliation.RequireOpen(ctx, tx, companyID, comp); err != nil {
		return Summary{}, err
	}
	first := comp.FirstDay()
	free, err := q.ListFreeStatementTransactions(ctx, store.ListFreeStatementTransactionsParams{CompanyID: companyID, Competence: &first})
	if err != nil {
		return Summary{}, err
	}
	near, err := q.ListStatementTransactionsBetween(ctx, store.ListStatementTransactionsBetweenParams{
		CompanyID: companyID, DateFrom: first.AddDate(0, 0, -matching.ReversalWindow),
		DateTo: comp.LastDay().AddDate(0, 0, matching.ReversalWindow),
	})
	if err != nil {
		return Summary{}, err
	}
	rows, err := q.ListClientEntriesForMatching(ctx, companyID)
	if err != nil {
		return Summary{}, err
	}
	in := matching.RunInput{Options: s.opts}
	for _, t := range free {
		in.Statements = append(in.Statements, matching.Statement{ID: t.ID, Date: t.TxnDate, Amount: t.Amount, Direction: t.Direction, Description: t.Description})
	}
	for _, t := range near {
		in.Neighbors = append(in.Neighbors, matching.Statement{ID: t.ID, Date: t.TxnDate, Amount: t.Amount, Direction: t.Direction, Description: t.Description})
	}
	for _, r := range rows {
		rec := matching.EntryRecord{ID: r.ID, Date: r.EntryDate, Amount: r.Amount, DocumentRef: r.DocumentRef, Flags: stringFlags(r.Flags)}
		if r.Direction != nil {
			rec.Direction = *r.Direction
		}
		in.Records = append(in.Records, rec)
		if eligible(r) {
			cp := ""
			if r.Counterparty != nil {
				cp = *r.Counterparty
			}
			in.Entries = append(in.Entries, matching.Entry{ID: r.ID, Date: *r.EntryDate, Amount: *r.Amount, Direction: *r.Direction, Counterparty: cp})
		}
	}
	out := matching.Run(in)
	sum := Summary{CompanyID: companyID, Competence: comp.String(), ByPass: map[string]int{"passe_1": 0, "passe_2": 0, "passe_3": 0, "passe_4": 0}}
	now := s.clock.Now()
	matched, usedEntries := map[string]bool{}, map[string]bool{}
	for _, m := range out.Matches {
		pass, score := passNumber(m.Pass), m.Score
		id, err := q.InsertAutoReconciliation(ctx, store.InsertAutoReconciliationParams{
			CompanyID: companyID, StatementTransactionID: m.StatementID, Score: &score, Pass: &pass, Method: m.Method, Now: now,
		})
		if err != nil {
			return Summary{}, fmt.Errorf("gravar a conciliação da linha %s: %w", m.StatementID, err)
		}
		for _, e := range m.EntryIDs {
			if err := q.InsertReconciliationItem(ctx, store.InsertReconciliationItemParams{ReconciliationID: id, CompanyID: companyID, ClientEntryID: e}); err != nil {
				return Summary{}, fmt.Errorf("gravar o lançamento %s da conciliação: %w", e, err)
			}
			usedEntries[e] = true
		}
		matched[m.StatementID] = true
		sum.NewMatches++
		sum.ByPass[m.Method]++
	}
	for _, t := range free {
		var marks []matching.StatementMark
		if !matched[t.ID] {
			marks = out.Marks[t.ID]
			if len(marks) > 0 {
				sum.Divergent++
			} else {
				sum.WithoutReceipt++
			}
		}
		flags, changed, err := replaceMarks(t.Flags, marks)
		if err != nil {
			return Summary{}, err
		}
		if changed {
			if err := q.SetStatementTransactionFlags(ctx, store.SetStatementTransactionFlagsParams{Flags: flags, ID: t.ID}); err != nil {
				return Summary{}, err
			}
		}
	}
	reconciled, err := q.CountActiveReconciliationsByCompetence(ctx, store.CountActiveReconciliationsByCompetenceParams{CompanyID: companyID, Competence: &first})
	if err != nil {
		return Summary{}, err
	}
	sum.Reconciled = int(reconciled)
	for _, r := range rows {
		if !r.Free || usedEntries[r.ID] || r.EntryDate == nil || competence.FromDate(*r.EntryDate) != comp {
			continue
		}
		switch {
		case eligible(r):
			sum.WithoutStatement++
		case r.Status == entries.StatusPartial:
			sum.AwaitingReview++
		}
	}
	return sum, nil
}

// passNumber é o passe da cascata (1 a 4) na coluna smallint.
func passNumber(p int) int16 {
	switch p {
	case 1:
		return 1
	case 2:
		return 2
	case 3:
		return 3
	}
	return 4
}

// derived são os tipos de marca que a rodada recalcula; as demais marcas da linha ficam.
var derived = map[string]bool{matching.MarkDivergence: true, matching.MarkEdge: true}

// replaceMarks troca as marcas derivadas da linha pelas da rodada (a linha conciliada fica sem
// elas) e informa se mudou.
func replaceMarks(raw []byte, marks []matching.StatementMark) ([]byte, bool, error) {
	var list []any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, false, fmt.Errorf("marcas da linha: %w", err)
		}
	}
	kept := []any{}
	for _, x := range list {
		if obj, ok := x.(map[string]any); ok {
			if tipo, _ := obj["tipo"].(string); derived[tipo] {
				continue
			}
		}
		kept = append(kept, x)
	}
	for _, m := range marks {
		kept = append(kept, m)
	}
	b, err := json.Marshal(kept)
	if err != nil {
		return nil, false, err
	}
	return b, !jsonEqual(raw, b), nil
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

// HandleRun é o handler do job reconciliation_run. Com o Contábil fechado na competência, não faz
// nada: a competência fechada não muda (AC-025).
func (s *Service) HandleRun(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	var p reconciliation.RunPayload
	if err := json.Unmarshal(job.Payload, &p); err != nil || !httpx.IsUUID(p.CompanyID) {
		return jobs.Permanent("INVALID_PAYLOAD", errors.New("reconciliation_run: payload sem empresa"))
	}
	comp, err := competence.Parse(p.Competence)
	if err != nil {
		return jobs.Permanent("INVALID_PAYLOAD", errors.New("reconciliation_run: competência inválida"))
	}
	sum, err := s.RunTx(ctx, tx, p.CompanyID, comp)
	if reconciliation.IsPeriodClosed(err) {
		s.logger.Info("rodada da conciliação ignorada: Contábil fechado", "company_id", p.CompanyID, "competence", p.Competence)
		return nil
	}
	if err == nil {
		s.logger.Info("rodada da conciliação", "company_id", p.CompanyID, "competence", p.Competence, "conciliadas", sum.NewMatches,
			"divergentes", sum.Divergent)
	}
	return err
}
