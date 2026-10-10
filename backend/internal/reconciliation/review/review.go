// Package review é a fila de revisão da Conciliação [REQ-019, REQ-023, REQ-024, REQ-030; 08 §7;
// TASK-021a, D-032]: a fila e as ações da rota /fila do Conciliador (commit 2bc6e26): os itens
// por balde, a linha com os candidatos, conciliar à mão, "não há comprovante" e desfazer. Ganha a
// correção campo a campo do comprovante parcial, que a origem só planejava (M2.6).
//
// Toda ação é humana, fica no log de extração, passa pela trava do Contábil (AC-025) e é
// serializada com a rodada do matching da empresa. A linha conciliada passa pela classificação
// (TASK-020); a desfeita volta para a fila e não volta sozinha ao automático.
package review

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/entries"
	"controle/internal/reconciliation/ledger"
	"controle/internal/reconciliation/reconcile"
	"controle/internal/store"
	"controle/internal/user"
)

// Baldes da fila (a contagem da origem, mais a classificação da TASK-020).
const (
	BucketReview           = "revisao"
	BucketDivergent        = "divergencia"
	BucketWithoutReceipt   = "extrato_sem"
	BucketWithoutStatement = "comprovante_sem"
	BucketClassification   = "classificacao"
)

// Buckets são os baldes, na ordem da fila.
var Buckets = []string{BucketReview, BucketDivergent, BucketWithoutReceipt, BucketWithoutStatement, BucketClassification}

// candidateWindow é a janela dos candidatos da origem (D±3).
const candidateWindow = 3

// Service atende a fila.
type Service struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	rounds *reconcile.Service
}

// NewService cria o serviço. rounds roda a competência depois da correção de um comprovante.
func NewService(pool *pgxpool.Pool, clk clock.Clock, rounds *reconcile.Service) *Service {
	return &Service{pool: pool, clock: clk, rounds: rounds}
}

// Item é um item da fila.
type Item struct {
	Kind        string           `json:"kind"`
	ID          string           `json:"id"`
	Bucket      string           `json:"bucket"`
	CompanyID   *string          `json:"company_id"`
	CompanyCode *string          `json:"company_code"`
	CompanyName *string          `json:"company_name"`
	State       string           `json:"state"`
	Date        *string          `json:"date"`
	Amount      *decimal.Decimal `json:"amount"`
	Direction   *string          `json:"direction"`
	Summary     string           `json:"summary"`
	Reason      *string          `json:"reason"`
	Competence  *string          `json:"competence"`
}

// Queue é a fila filtrada e as contagens por balde (total em "total").
type Queue struct {
	Items  []Item         `json:"items"`
	Counts map[string]int `json:"counts"`
}

// QueueQuery são os filtros da fila: empresa, competência e balde (todos opcionais).
type QueueQuery struct {
	CompanyID  *string
	Competence *competence.Competence
	Bucket     *string
	Limit      int32
	Offset     int32
}

func dateText(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// List lê a fila.
func (s *Service) List(ctx context.Context, in QueueQuery) (Queue, error) {
	q := store.New(s.pool)
	var comp *time.Time
	if in.Competence != nil {
		d := in.Competence.FirstDay()
		comp = &d
	}
	rows, err := q.ListReviewQueue(ctx, store.ListReviewQueueParams{CompanyID: in.CompanyID, Competence: comp, Bucket: in.Bucket,
		RowLimit: in.Limit, RowOffset: in.Offset})
	if err != nil {
		return Queue{}, err
	}
	counts, err := q.CountReviewQueue(ctx, store.CountReviewQueueParams{CompanyID: in.CompanyID, Competence: comp})
	if err != nil {
		return Queue{}, err
	}
	out := Queue{Items: make([]Item, 0, len(rows)), Counts: map[string]int{"total": 0}}
	for _, b := range Buckets {
		out.Counts[b] = 0
	}
	for _, c := range counts {
		out.Counts[c.Bucket] = int(c.Total)
		out.Counts["total"] += int(c.Total)
	}
	for _, r := range rows {
		it := Item{Kind: r.Kind, ID: r.ID, Bucket: r.Bucket, CompanyID: r.CompanyID, CompanyCode: r.CompanyCode, CompanyName: r.CompanyName,
			State: r.State, Date: dateText(r.ItemDate), Amount: r.Amount, Direction: r.Direction, Summary: r.Summary,
			Reason: nonEmpty(r.Reason)}
		if r.Competence != nil {
			c := competence.FromDate(*r.Competence).String()
			it.Competence = &c
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

func lineNotFound() error {
	return apperr.NotFound("RECONCILIATION_LINE_NOT_FOUND", "Linha do extrato não encontrada.")
}

// Line é a linha do extrato na fila.
type Line struct {
	ID                  string           `json:"id"`
	CompanyID           string           `json:"company_id"`
	CompanyCode         string           `json:"company_code"`
	CompanyName         string           `json:"company_name"`
	Date                string           `json:"date"`
	Competence          string           `json:"competence"`
	Description         string           `json:"description"`
	DocumentRef         *string          `json:"document_ref"`
	Amount              decimal.Decimal  `json:"amount"`
	Direction           string           `json:"direction"`
	BalanceAfter        *decimal.Decimal `json:"balance_after"`
	BankAccount         string           `json:"bank_account"`
	StatementImportID   string           `json:"statement_import_id"`
	StatementDocumentID string           `json:"statement_document_id"`
	Marks               json.RawMessage  `json:"marks"`
	AccountCode         *string          `json:"account_code"`
	ClassificationOrig  *string          `json:"classification_origin"`
	ClassificationPend  bool             `json:"classification_pending"`
}

// Reconciled é a conciliação ativa da linha, com os comprovantes.
type Reconciled struct {
	ID        string          `json:"id"`
	Method    string          `json:"method"`
	Origin    string          `json:"origin"`
	Pass      *int16          `json:"pass"`
	CreatedBy *string         `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
	Entries   []ledger.Entry  `json:"entries"`
	Total     decimal.Decimal `json:"entries_total"`
}

// Candidate é um lançamento candidato da linha.
type Candidate struct {
	ledger.Entry
	OperationType *string `json:"operation_type"`
	DaysApart     *int    `json:"days_apart"`
	Annotated     bool    `json:"annotated"`
}

// LineDetail é a linha, a conciliação (se houver) e os candidatos (se livre).
type LineDetail struct {
	Line           Line        `json:"line"`
	Reconciliation *Reconciled `json:"reconciliation"`
	Candidates     []Candidate `json:"candidates"`
}

func days(a, b time.Time) int {
	d := int(a.Sub(b).Hours() / 24)
	if d < 0 {
		return -d
	}
	return d
}

// annotated são os lançamentos que o matching anotou na divergência da linha.
func annotated(flags []byte) map[string]bool {
	out := map[string]bool{}
	var list []any
	if json.Unmarshal(flags, &list) != nil {
		return out
	}
	for _, x := range list {
		obj, ok := x.(map[string]any)
		if !ok || obj["tipo"] != "divergencia" {
			continue
		}
		cands, _ := obj["candidatos"].([]any)
		for _, c := range cands {
			if id, ok := c.(string); ok {
				out[id] = true
			}
		}
	}
	return out
}

func toEntry(id string, date *time.Time, amount *decimal.Decimal, dir, cp, doc, ref *string, status string, document *string) ledger.Entry {
	return ledger.Entry{ID: id, Date: dateText(date), Amount: amount, Direction: dir, Counterparty: cp, CounterpartyDoc: doc,
		DocumentRef: ref, Status: status, DocumentID: document}
}

// Line lê a linha: a conciliação ativa (com os comprovantes) ou, livre, os candidatos da origem
// (candidatos_para_extrato): lançamentos livres de mesmo valor e sentido em D±3, mais os que o
// matching anotou, do mais perto ao mais longe.
func (s *Service) Line(ctx context.Context, id string) (LineDetail, error) {
	if !httpx.IsUUID(id) {
		return LineDetail{}, lineNotFound()
	}
	q := store.New(s.pool)
	r, err := q.GetReviewLine(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return LineDetail{}, lineNotFound()
	}
	if err != nil {
		return LineDetail{}, err
	}
	marks := json.RawMessage(r.Flags)
	if len(marks) == 0 {
		marks = json.RawMessage("[]")
	}
	out := LineDetail{
		Line: Line{ID: r.ID, CompanyID: r.CompanyID, CompanyCode: r.CompanyCode, CompanyName: r.CompanyName, Date: r.TxnDate.Format(time.DateOnly),
			Competence: competence.FromDate(r.TxnDate).String(), Description: r.Description, DocumentRef: r.DocumentRef, Amount: r.Amount,
			Direction: r.Direction, BalanceAfter: r.BalanceAfter, BankAccount: ledger.BankLabel(r.Bank, r.Branch, r.BankAccount),
			StatementImportID: r.StatementImportID, StatementDocumentID: r.StatementDocumentID, Marks: marks, AccountCode: r.AccountCode,
			ClassificationOrig: r.ClassificationOrigin, ClassificationPend: r.ClassificationPending},
		Candidates: []Candidate{},
	}
	if r.ReconciliationID != nil {
		rec := Reconciled{ID: *r.ReconciliationID, Method: *r.ReconciliationMethod, Origin: *r.ReconciliationOrigin, Pass: r.ReconciliationPass,
			CreatedBy: r.ReconciledBy, CreatedAt: *r.ReconciledAt, Entries: []ledger.Entry{}, Total: decimal.MustParse("0.00")}
		rows, err := q.ListReconciliationEntries(ctx, *r.ReconciliationID)
		if err != nil {
			return LineDetail{}, err
		}
		for _, e := range rows {
			rec.Entries = append(rec.Entries, toEntry(e.ID, e.EntryDate, e.Amount, e.Direction, e.Counterparty, e.CounterpartyDoc, e.DocumentRef,
				e.Status, e.DocumentID))
			if e.Amount != nil {
				rec.Total = rec.Total.Add(*e.Amount)
			}
		}
		out.Reconciliation = &rec
		return out, nil
	}
	notes := annotated(r.Flags)
	rows, err := q.ListLineCandidateEntries(ctx, r.CompanyID)
	if err != nil {
		return LineDetail{}, err
	}
	for _, e := range rows {
		exact := e.EntryDate != nil && e.Amount != nil && e.Direction != nil && e.Amount.Cmp(r.Amount) == 0 && *e.Direction == r.Direction &&
			days(*e.EntryDate, r.TxnDate) <= candidateWindow
		if !exact && !notes[e.ID] {
			continue
		}
		c := Candidate{Entry: toEntry(e.ID, e.EntryDate, e.Amount, e.Direction, e.Counterparty, e.CounterpartyDoc, e.DocumentRef, e.Status,
			e.DocumentID), OperationType: e.OperationType, Annotated: notes[e.ID]}
		if e.EntryDate != nil {
			d := days(*e.EntryDate, r.TxnDate)
			c.DaysApart = &d
		}
		out.Candidates = append(out.Candidates, c)
	}
	sort.SliceStable(out.Candidates, func(i, j int) bool { return distance(out.Candidates[i]) < distance(out.Candidates[j]) })
	return out, nil
}

// distance ordena os candidatos como a origem: sem data, 99 dias.
func distance(c Candidate) int {
	if c.DaysApart == nil {
		return 99
	}
	return *c.DaysApart
}

func entryNotFound() error {
	return apperr.NotFound("ENTRY_NOT_FOUND", "Lançamento do cliente não encontrado.")
}

// LineCandidate é uma linha candidata do comprovante.
type LineCandidate struct {
	ID          string          `json:"id"`
	Date        string          `json:"date"`
	Amount      decimal.Decimal `json:"amount"`
	Direction   string          `json:"direction"`
	Description string          `json:"description"`
	BankAccount string          `json:"bank_account"`
	DaysApart   int             `json:"days_apart"`
}

// EntryDetail é o comprovante na fila: o lançamento, a conciliação (se houver) e as linhas
// candidatas (se livre e completo).
type EntryDetail struct {
	Entry          entries.Entry   `json:"entry"`
	Reconciliation *string         `json:"reconciliation_id"`
	LineID         *string         `json:"statement_transaction_id"`
	Candidates     []LineCandidate `json:"candidates"`
}

// Entry lê o comprovante e os candidatos da origem (candidatos_para_comprovante): linhas livres de
// mesmo valor e sentido em D±3, da mais perto à mais longe.
func (s *Service) Entry(ctx context.Context, id string) (EntryDetail, error) {
	if !httpx.IsUUID(id) {
		return EntryDetail{}, entryNotFound()
	}
	q := store.New(s.pool)
	e, err := q.GetClientEntry(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return EntryDetail{}, entryNotFound()
	}
	if err != nil {
		return EntryDetail{}, err
	}
	out := EntryDetail{Entry: entries.FromRow(e), Candidates: []LineCandidate{}}
	recs, err := q.ListActiveReconciliationsOfEntry(ctx, id)
	if err != nil {
		return EntryDetail{}, err
	}
	if len(recs) > 0 {
		out.Reconciliation, out.LineID = &recs[0].ID, &recs[0].StatementTransactionID
		return out, nil
	}
	if e.EntryDate == nil || e.Amount == nil || e.Direction == nil {
		return out, nil
	}
	lines, err := q.ListEntryCandidateLines(ctx, store.ListEntryCandidateLinesParams{
		CompanyID: e.CompanyID, Amount: *e.Amount, Direction: *e.Direction, DateFrom: e.EntryDate.AddDate(0, 0, -candidateWindow),
		DateTo: e.EntryDate.AddDate(0, 0, candidateWindow), EntryDate: *e.EntryDate,
	})
	if err != nil {
		return EntryDetail{}, err
	}
	for _, l := range lines {
		out.Candidates = append(out.Candidates, LineCandidate{ID: l.ID, Date: l.TxnDate.Format(time.DateOnly), Amount: l.Amount,
			Direction: l.Direction, Description: l.Description, BankAccount: ledger.BankLabel(l.Bank, l.Branch, l.BankAccount),
			DaysApart: days(l.TxnDate, *e.EntryDate)})
	}
	return out, nil
}

// Recent é uma conciliação recente (a lista "desfazer" da origem).
type Recent struct {
	ID                     string          `json:"id"`
	CompanyID              string          `json:"company_id"`
	CompanyCode            string          `json:"company_code"`
	Method                 string          `json:"method"`
	Origin                 string          `json:"origin"`
	Pass                   *int16          `json:"pass"`
	CreatedBy              *string         `json:"created_by"`
	CreatedAt              time.Time       `json:"created_at"`
	StatementTransactionID string          `json:"statement_transaction_id"`
	Date                   string          `json:"date"`
	Amount                 decimal.Decimal `json:"amount"`
	Direction              string          `json:"direction"`
	Description            string          `json:"description"`
}

// Recent lista as últimas conciliações ativas (da empresa e da origem, se dadas).
func (s *Service) Recent(ctx context.Context, companyID, origin *string, limit int32) ([]Recent, error) {
	rows, err := store.New(s.pool).ListRecentReconciliations(ctx, store.ListRecentReconciliationsParams{CompanyID: companyID, Origin: origin, RowLimit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Recent, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recent{ID: r.ID, CompanyID: r.CompanyID, CompanyCode: r.CompanyCode, Method: r.Method, Origin: r.Origin, Pass: r.Pass,
			CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, StatementTransactionID: r.StatementTransactionID, Date: r.TxnDate.Format(time.DateOnly),
			Amount: r.Amount, Direction: r.Direction, Description: r.Description})
	}
	return out, nil
}

// audit registra a ação humana no log de extração (o _auditar da origem).
func (s *Service) audit(ctx context.Context, q *store.Queries, companyID string, documentID *string, change map[string]any) error {
	uid := user.FromContext(ctx)
	b, err := json.Marshal(change)
	if err != nil {
		return err
	}
	text := string(b)
	_, err = q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
		CompanyID: &companyID, DocumentID: documentID, Alias: "humano", Attempt: 0, ActorID: &uid, Change: &text, Now: s.clock.Now(),
	})
	return err
}

// requireOpen trava pelas competências dadas (AC-025), em ordem.
func requireOpen(ctx context.Context, tx pgx.Tx, companyID string, dates ...time.Time) error {
	seen := map[string]competence.Competence{}
	for _, d := range dates {
		c := competence.FromDate(d)
		seen[c.String()] = c
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := reconciliation.RequireOpen(ctx, tx, companyID, seen[k]); err != nil {
			return err
		}
	}
	return nil
}
