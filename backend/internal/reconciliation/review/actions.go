package review

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/classify"
	"controle/internal/reconciliation/entries"
	"controle/internal/reconciliation/reconcile"
	"controle/internal/store"
	"controle/internal/user"
)

// Métodos da conciliação feita na fila (D-020).
const (
	MethodManual         = "manual"
	MethodWithoutReceipt = "sem_comprovante"
)

const maxReason = 500

func validation(fields map[string]any) error {
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").WithDetails(map[string]any{"fields": fields})
}

// freeLine trava a empresa e lê a linha livre (404, 409 conciliada), com a trava do Contábil.
func freeLine(ctx context.Context, tx pgx.Tx, q *store.Queries, lineID string) (store.GetReviewLineRow, error) {
	if !httpx.IsUUID(lineID) {
		return store.GetReviewLineRow{}, lineNotFound()
	}
	l, err := q.GetReviewLine(ctx, lineID)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, lineNotFound()
	}
	if err != nil {
		return l, err
	}
	if err := q.LockReconciliationCompany(ctx, l.CompanyID); err != nil {
		return l, err
	}
	// Relida depois da trava: a rodada da empresa pode ter conciliado a linha enquanto esperava.
	if l, err = q.GetReviewLine(ctx, lineID); err != nil {
		return l, err
	}
	if l.ReconciliationID != nil {
		return l, apperr.Conflict("LINE_RECONCILED", "A linha já está conciliada.").WithDetails(map[string]any{"reconciliation_id": *l.ReconciliationID})
	}
	return l, requireOpen(ctx, tx, l.CompanyID, l.TxnDate)
}

// MatchResult é a conciliação feita na fila, com a classificação aplicada à linha.
type MatchResult struct {
	ReconciliationID string          `json:"reconciliation_id"`
	Method           string          `json:"method"`
	EntriesTotal     decimal.Decimal `json:"entries_total"`
	Difference       decimal.Decimal `json:"difference"`
	Classification   string          `json:"classification"`
}

// ManualMatch é o conciliar_manual da origem: a linha com os lançamentos escolhidos pelo contador
// (um ou mais: N:1), com origem manual e o autor. Os lançamentos precisam ser da empresa e estar
// livres. O valor não é conferido, como na origem: a diferença vai na resposta.
func (s *Service) ManualMatch(ctx context.Context, lineID string, entryIDs []string) (MatchResult, error) {
	ids := []string{}
	for _, id := range entryIDs {
		if !httpx.IsUUID(id) {
			return MatchResult{}, validation(map[string]any{"entry_ids": "Lançamento inválido."})
		}
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return MatchResult{}, validation(map[string]any{"entry_ids": "Escolha ao menos um lançamento; sem comprovante, use \"não há comprovante\"."})
	}
	var out MatchResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := freeLine(ctx, tx, q, lineID)
		if err != nil {
			return err
		}
		free, err := q.ListFreeClientEntriesByIDs(ctx, store.ListFreeClientEntriesByIDsParams{CompanyID: l.CompanyID, Ids: ids})
		if err != nil {
			return err
		}
		if len(free) != len(ids) {
			return apperr.Conflict("ENTRY_NOT_FREE", "Algum lançamento já está conciliado ou não é desta empresa.")
		}
		total := decimal.MustParse("0.00")
		for _, id := range ids {
			e, err := q.GetClientEntry(ctx, id)
			if err != nil {
				return err
			}
			if e.Amount != nil {
				total = total.Add(*e.Amount)
			}
		}
		recID, err := s.create(ctx, q, l, MethodManual, ids)
		if err != nil {
			return err
		}
		out = MatchResult{ReconciliationID: recID, Method: MethodManual, EntriesTotal: total, Difference: l.Amount.Sub(total)}
		if out.Classification, err = s.classify(ctx, tx, l); err != nil {
			return err
		}
		return s.audit(ctx, q, l.CompanyID, nil, map[string]any{"acao": "conciliar", "linha": l.ID, "lancamentos": ids, "conciliacao": recID})
	})
	return out, err
}

// ConfirmNoReceipt é o marcar_sem da origem ("não há comprovante"): a linha fica conciliada sem
// lançamento, com o método sem_comprovante e o autor.
func (s *Service) ConfirmNoReceipt(ctx context.Context, lineID string) (MatchResult, error) {
	var out MatchResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := freeLine(ctx, tx, q, lineID)
		if err != nil {
			return err
		}
		recID, err := s.create(ctx, q, l, MethodWithoutReceipt, nil)
		if err != nil {
			return err
		}
		zero := decimal.MustParse("0.00")
		out = MatchResult{ReconciliationID: recID, Method: MethodWithoutReceipt, EntriesTotal: zero, Difference: zero}
		if out.Classification, err = s.classify(ctx, tx, l); err != nil {
			return err
		}
		return s.audit(ctx, q, l.CompanyID, nil, map[string]any{"acao": "sem_comprovante", "linha": l.ID, "conciliacao": recID})
	})
	return out, err
}

func (s *Service) create(ctx context.Context, q *store.Queries, l store.GetReviewLineRow, method string, entryIDs []string) (string, error) {
	uid := user.FromContext(ctx)
	id, err := q.InsertManualReconciliation(ctx, store.InsertManualReconciliationParams{
		CompanyID: l.CompanyID, StatementTransactionID: l.ID, Method: method, CreatedBy: &uid, Now: s.clock.Now(),
	})
	if err != nil {
		return "", err
	}
	for _, e := range entryIDs {
		if err := q.InsertReconciliationItem(ctx, store.InsertReconciliationItemParams{ReconciliationID: id, CompanyID: l.CompanyID, ClientEntryID: e}); err != nil {
			return "", err
		}
	}
	return id, nil
}

// classify passa a linha conciliada pela classificação (regra → histórico; sem os dois, à espera
// do agente ou do contador; TASK-020) e devolve a origem aplicada.
func (s *Service) classify(ctx context.Context, tx pgx.Tx, l store.GetReviewLineRow) (string, error) {
	applied, err := classify.Lines(ctx, tx, l.CompanyID, []string{l.ID}, s.clock.Now())
	return applied[l.ID], err
}

// UndoResult é a conciliação desfeita.
type UndoResult struct {
	ReconciliationID       string    `json:"reconciliation_id"`
	StatementTransactionID string    `json:"statement_transaction_id"`
	UndoneAt               time.Time `json:"undone_at"`
}

// Undo desfaz a conciliação, automática ou manual (decisão do usuário, D-032), com o motivo
// obrigatório. Nada é apagado: a conciliação guarda quem, quando e por quê, e os lançamentos
// ficam livres. A linha perde a classificação e ganha a marca "desfeita": volta para a fila e não
// volta sozinha ao automático. A origem apagava a conciliação, e só a manual.
func (s *Service) Undo(ctx context.Context, id, reason string) (UndoResult, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return UndoResult{}, validation(map[string]any{"reason": "Diga por que a conciliação está sendo desfeita."})
	case utf8.RuneCountInString(reason) > maxReason:
		return UndoResult{}, validation(map[string]any{"reason": "Máximo de 500 caracteres."})
	}
	if !httpx.IsUUID(id) {
		return UndoResult{}, reconciliationNotFound()
	}
	var out UndoResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		r, err := q.GetReconciliationForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return reconciliationNotFound()
		}
		if err != nil {
			return err
		}
		if r.UndoneAt != nil {
			return apperr.Conflict("RECONCILIATION_UNDONE", "A conciliação já foi desfeita.")
		}
		if err := q.LockReconciliationCompany(ctx, r.CompanyID); err != nil {
			return err
		}
		if err := requireOpen(ctx, tx, r.CompanyID, r.TxnDate); err != nil {
			return err
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		if err := q.UndoReconciliation(ctx, store.UndoReconciliationParams{Now: &now, UndoneBy: &uid, Reason: &reason, ID: id}); err != nil {
			return err
		}
		if err := q.ReleaseReconciliationItems(ctx, store.ReleaseReconciliationItemsParams{Now: &now, ReconciliationID: id}); err != nil {
			return err
		}
		if err := q.ResetLineClassification(ctx, r.StatementTransactionID); err != nil {
			return err
		}
		mark, err := json.Marshal(map[string]any{"tipo": reconciliation.MarkUndone, "motivo": "conciliacao_desfeita", "conciliacao": id,
			"metodo": r.Method, "origem": r.Origin, "justificativa": reason})
		if err != nil {
			return err
		}
		if err := q.AppendStatementTransactionMark(ctx, store.AppendStatementTransactionMarkParams{Mark: mark, ID: r.StatementTransactionID}); err != nil {
			return err
		}
		out = UndoResult{ReconciliationID: id, StatementTransactionID: r.StatementTransactionID, UndoneAt: now}
		return s.audit(ctx, q, r.CompanyID, nil, map[string]any{"acao": "desfazer", "conciliacao": id, "linha": r.StatementTransactionID,
			"metodo": r.Method, "origem": r.Origin, "motivo": reason})
	})
	return out, err
}

func reconciliationNotFound() error {
	return apperr.NotFound("RECONCILIATION_NOT_FOUND", "Conciliação não encontrada.")
}

// FieldsResult é a correção do comprovante: o lançamento corrigido e a rodada da competência.
type FieldsResult struct {
	Entry entries.Entry      `json:"entry"`
	Round *reconcile.Summary `json:"round"`
}

// essential são os campos que tiram o comprovante do parcial (receipt.EssentialFields).
func complete(e store.ClientEntry) bool {
	return e.EntryDate != nil && e.Amount != nil && e.Direction != nil && e.Counterparty != nil
}

// CorrectFields é a correção campo a campo do comprovante (a revisão de campo de baixa confiança,
// REQ-023): só o lançamento livre. O campo corrigido passa a ter confiança 1 e origem humana; com
// os campos essenciais preenchidos, o parcial sai da revisão (status extraido; o manual continua
// manual) e a competência roda de novo (D-029).
func (s *Service) CorrectFields(ctx context.Context, id string, in entries.FieldsPatch) (FieldsResult, error) {
	if !httpx.IsUUID(id) {
		return FieldsResult{}, entryNotFound()
	}
	p, errs := in.Validate()
	if errs != nil {
		fields := map[string]any{}
		for k, v := range errs {
			fields[k] = v
		}
		return FieldsResult{}, validation(fields)
	}
	var out FieldsResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		cur, err := q.GetClientEntryForUpdate(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return entryNotFound()
		}
		if err != nil {
			return err
		}
		if cur.Reconciled {
			return apperr.Conflict("ENTRY_RECONCILED", "O lançamento está conciliado: desfaça a conciliação antes de corrigir.")
		}
		if err := q.LockReconciliationCompany(ctx, cur.CompanyID); err != nil {
			return err
		}
		dates := []time.Time{}
		if cur.EntryDate != nil {
			dates = append(dates, *cur.EntryDate)
		}
		if p.Date != nil {
			dates = append(dates, *p.Date)
		}
		if err := requireOpen(ctx, tx, cur.CompanyID, dates...); err != nil {
			return err
		}
		next := store.ClientEntry{EntryDate: cur.EntryDate, Amount: cur.Amount, Direction: cur.Direction, Counterparty: cur.Counterparty,
			CounterpartyDoc: cur.CounterpartyDoc, DocumentRef: cur.DocumentRef}
		for _, f := range p.Set {
			switch f {
			case "data":
				next.EntryDate = p.Date
			case "valor":
				next.Amount = p.Amount
			case "sentido":
				next.Direction = p.Direction
			case "contraparte":
				next.Counterparty = p.Counterparty
			case "contraparte_doc":
				next.CounterpartyDoc = p.CounterpartyDoc
			case "documento":
				next.DocumentRef = p.DocumentRef
			}
		}
		status := cur.Status
		if status == entries.StatusPartial && complete(next) {
			status = entries.StatusExtracted
		}
		conf, origin, err := humanFields(cur.FieldConfidence, cur.FieldOrigin, p.Set)
		if err != nil {
			return err
		}
		row, err := q.UpdateClientEntryFields(ctx, store.UpdateClientEntryFieldsParams{
			EntryDate: next.EntryDate, Amount: next.Amount, Direction: next.Direction, Counterparty: next.Counterparty,
			CounterpartyDoc: next.CounterpartyDoc, DocumentRef: next.DocumentRef, Status: status, FieldConfidence: conf, FieldOrigin: origin,
			Now: s.clock.Now(), ID: id,
		})
		if err != nil {
			return err
		}
		if err := s.audit(ctx, q, cur.CompanyID, cur.DocumentID, map[string]any{"acao": "corrigir_campos", "lancamento": id, "campos": p.Set,
			"de": cur.Status, "para": status}); err != nil {
			return err
		}
		out.Entry = entries.FromRow(row)
		if row.EntryDate != nil && status != entries.StatusPartial {
			sum, err := s.rounds.RunTx(ctx, tx, cur.CompanyID, competence.FromDate(*row.EntryDate))
			if err != nil {
				return err
			}
			out.Round = &sum
		}
		return nil
	})
	return out, err
}

// humanFields marca os campos corrigidos: confiança 1 e origem humana (a fila mostra quem leu o
// quê). Os nomes são os da origem (data, valor, sentido, contraparte...).
func humanFields(confRaw, originRaw []byte, set []string) ([]byte, []byte, error) {
	conf, origin := map[string]any{}, map[string]any{}
	if len(confRaw) > 0 {
		if err := json.Unmarshal(confRaw, &conf); err != nil {
			return nil, nil, err
		}
	}
	if len(originRaw) > 0 {
		if err := json.Unmarshal(originRaw, &origin); err != nil {
			return nil, nil, err
		}
	}
	for _, f := range set {
		conf[f] = "1.00"
		origin[f] = map[string]any{"origem": "humano"}
	}
	c, err := json.Marshal(conf)
	if err != nil {
		return nil, nil, err
	}
	o, err := json.Marshal(origin)
	return c, o, err
}
