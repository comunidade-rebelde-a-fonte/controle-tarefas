package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/competence"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/statement"
	"controle/internal/store"
)

// importOutcome é o que a importação fez com o documento.
type importOutcome struct {
	status     string // importado, revisao_extracao ou pendencia_periodo_fechado
	reason     string // motivo da revisão ou da pendência
	importID   string
	companyID  string
	inserted   int
	duplicates int
	playbook   string // "novo v2", "sucesso v1" ou vazio
}

func (o importOutcome) asMap() map[string]any {
	out := map[string]any{"status": o.status, "motivo": nilIfEmpty(o.reason), "importacao_id": nilIfEmpty(o.importID),
		"company_id": nilIfEmpty(o.companyID), "transacoes_gravadas": o.inserted, "transacoes_repetidas": o.duplicates,
		"playbook": nilIfEmpty(o.playbook)}
	return out
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// diagnosis é o diagnóstico do documento em revisão ou pendência: o motivo, os detalhes e, quando
// houver, o resultado do validador (ResultadoValidacao).
func diagnosis(reason string, result *statement.Result, details map[string]any) map[string]any {
	d := map[string]any{"motivo": reason}
	for k, v := range details {
		d[k] = v
	}
	if result != nil {
		d["validacao"] = result
	}
	return d
}

// importStatement é o importar_extrato da origem, na transação do job: sem o portão aprovado,
// nada é gravado e o documento vai para revisao_extracao com o diagnóstico [AC-015, F-004]; com
// ele, a importação, as transações (a chave de dedupe repetida é ignorada), o documento importado e
// o playbook do banco.
//
// O porte acrescenta a empresa e a trava do Contábil: a conta bancária (única no sistema) decide a
// empresa, ou a do envio; sem empresa, o documento vai para revisão (D-020). Com o Contábil
// fechado em alguma competência do extrato, nada é importado e o documento fica como
// pendencia_periodo_fechado (AC-025, F-009).
func (p *Processor) importStatement(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument,
	s statement.Statement, r statement.Result, attempts int, traceID string) (importOutcome, error) {
	q := store.New(tx)
	review := func(status, reason string, details map[string]any, result *statement.Result) (importOutcome, error) {
		out := importOutcome{status: status, reason: reason}
		return out, p.setState(ctx, q, doc.ID, status, diagnosis(reason, result, details), nil)
	}
	if !r.OK {
		return review(StatusExtractReview, ReasonBalanceMismatch, nil, &r)
	}
	identity := map[string]any{"banco": s.Bank, "agencia": s.Branch, "conta": s.Account}
	var bankAccountID *string
	companyID := ""
	acc, err := q.FindBankAccountByIdentity(ctx, store.FindBankAccountByIdentityParams{Bank: s.Bank, Branch: s.Branch, Account: s.Account})
	switch {
	case err == nil:
		if doc.CompanyID != nil && *doc.CompanyID != acc.CompanyID {
			return review(StatusExtractReview, ReasonAccountOtherFirm, identity, &r)
		}
		bankAccountID, companyID = &acc.ID, acc.CompanyID
	case errors.Is(err, pgx.ErrNoRows):
		if doc.CompanyID != nil {
			companyID = *doc.CompanyID
		}
	default:
		return importOutcome{}, err
	}
	if companyID == "" {
		return review(StatusExtractReview, ReasonNoCompany, identity, &r)
	}
	var closed []string
	for _, c := range competences(s) {
		isClosed, err := reconciliation.AccountingClosed(ctx, tx, companyID, c)
		if err != nil {
			return importOutcome{}, err
		}
		if isClosed {
			closed = append(closed, c.String())
		}
	}
	if len(closed) > 0 {
		out, err := review(StatusClosedPeriod, ReasonClosedPeriod, map[string]any{"company_id": companyID, "competencias": closed}, &r)
		out.companyID = companyID
		return out, err
	}
	now := p.clock.Now()
	importID, err := q.InsertStatementImport(ctx, store.InsertStatementImportParams{
		CompanyID: companyID, DocumentID: doc.ID, BankAccountID: bankAccountID, Bank: s.Bank, Branch: s.Branch,
		Account: s.Account, PeriodStart: s.PeriodStart, PeriodEnd: s.PeriodEnd, OpeningBalance: s.OpeningBalance,
		ClosingBalance: s.ClosingBalance, Validation: jsonBytes(r), Attempts: clamp32(int64(attempts)), TraceID: optString(traceID),
		Now: now,
	})
	if err != nil {
		return importOutcome{}, fmt.Errorf("gravar a importação: %w", err)
	}
	out := importOutcome{status: StatusImported, importID: importID, companyID: companyID}
	dates := make([]time.Time, len(s.Transactions))
	for i, t := range s.Transactions {
		dates[i] = t.Date
	}
	inDay := reconciliation.OrdersInDay(dates)
	for i, t := range s.Transactions {
		conf := t.Confidence
		n, err := q.InsertStatementTransaction(ctx, store.InsertStatementTransactionParams{
			CompanyID: companyID, StatementImportID: importID, Position: clamp32(int64(t.Order)), TxnDate: t.Date,
			Description: t.Description, DocumentRef: t.DocumentRef, Amount: t.Amount, Direction: t.Direction,
			BalanceAfter: t.BalanceAfter, DedupeKey: reconciliation.DedupeKey(s.Account, t.Date, t.Amount, t.Direction, t.Description, inDay[i]),
			Confidence: &conf, Now: now,
		})
		if err != nil {
			return importOutcome{}, fmt.Errorf("gravar a transação %d: %w", t.Order, err)
		}
		if n == 1 {
			out.inserted++
		} else {
			out.duplicates++
		}
	}
	if err := q.SaveReconciliationImportedDocument(ctx, store.SaveReconciliationImportedDocumentParams{
		CompanyID: &companyID, Bank: &s.Bank, Branch: &s.Branch, Account: &s.Account, PeriodStart: &s.PeriodStart,
		PeriodEnd: &s.PeriodEnd, Now: now, ID: doc.ID,
	}); err != nil {
		return importOutcome{}, err
	}
	if out.playbook, err = p.savePlaybook(ctx, q, s); err != nil {
		return importOutcome{}, err
	}
	return out, nil
}

// competences são as competências que a importação toca: as das transações e as do período.
func competences(s statement.Statement) []competence.Competence {
	seen := map[string]competence.Competence{}
	add := func(t time.Time) {
		c := competence.New(t.Year(), t.Month())
		seen[c.String()] = c
	}
	add(s.PeriodStart)
	add(s.PeriodEnd)
	for _, t := range s.Transactions {
		add(t.Date)
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]competence.Competence, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out
}

// savePlaybook é o _persistir_playbook da origem: com o portão aprovado e o leiaute descoberto, o
// playbook válido do banco ganha um sucesso; sem playbook válido, nasce a versão seguinte.
func (p *Processor) savePlaybook(ctx context.Context, q *store.Queries, s statement.Statement) (string, error) {
	if s.Layout == nil || strings.TrimSpace(s.Bank) == "" {
		return "", nil
	}
	now := p.clock.Now()
	pb, err := q.GetValidBankPlaybook(ctx, s.Bank)
	switch {
	case err == nil:
		return fmt.Sprintf("sucesso v%d", pb.Version), q.AddBankPlaybookSuccess(ctx, store.AddBankPlaybookSuccessParams{Now: now, ID: pb.ID})
	case !errors.Is(err, pgx.ErrNoRows):
		return "", err
	}
	version, err := q.NextBankPlaybookVersion(ctx, s.Bank)
	if err != nil {
		return "", err
	}
	l := s.Layout
	pages := l.IgnoredPages
	if pages == nil {
		pages = []int64{}
	}
	if _, err := q.InsertBankPlaybook(ctx, store.InsertBankPlaybookParams{
		Bank: s.Bank, Version: version, TableStartPage: clamp32(int64(l.TableStartPage)), IgnoredPages: jsonBytes(pages),
		DateFormat: l.DateFormat, DecimalSeparator: l.DecimalSeparator, SignRepresentation: l.SignRepresentation,
		BalanceLocation: l.BalanceLocation, WinningStrategy: l.Strategy, Now: &now,
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("novo v%d", version), nil
}
