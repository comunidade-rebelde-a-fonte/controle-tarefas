package proposals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/resolver"
	"controle/internal/store"
	"controle/internal/user"
)

// Estados da proposta (D-020).
const (
	StatusPending  = "pendente"
	StatusAccepted = "aceita"
	StatusRejected = "rejeitada"
)

// Service atende a API do resolvedor: pedir a hipótese, listar as propostas da linha e aceitar ou
// rejeitar. Toda ação é humana, fica no log de extração e passa pela trava do Contábil (AC-025).
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
	queue *jobs.Queue
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock, queue *jobs.Queue) *Service {
	return &Service{pool: pool, clock: clk, queue: queue}
}

// Proposal é a proposta do resolvedor.
type Proposal struct {
	ID                     string          `json:"id"`
	CompanyID              string          `json:"company_id"`
	StatementTransactionID string          `json:"statement_transaction_id"`
	Status                 string          `json:"status"`
	Hypothesis             json.RawMessage `json:"hypothesis"`
	TraceID                *string         `json:"trace_id"`
	CreatedBy              *string         `json:"created_by"`
	CreatedAt              time.Time       `json:"created_at"`
	DecidedBy              *string         `json:"decided_by"`
	DecidedAt              *time.Time      `json:"decided_at"`
}

func toProposal(p store.ResolverProposal) Proposal {
	return Proposal{ID: p.ID, CompanyID: p.CompanyID, StatementTransactionID: p.StatementTransactionID, Status: p.Status,
		Hypothesis: json.RawMessage(p.Payload), TraceID: p.TraceID, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt,
		DecidedBy: p.DecidedBy, DecidedAt: p.DecidedAt}
}

func lineNotFound() error {
	return apperr.NotFound("RECONCILIATION_LINE_NOT_FOUND", "Linha do extrato não encontrada.")
}

func proposalNotFound() error {
	return apperr.NotFound("PROPOSAL_NOT_FOUND", "Proposta do resolvedor não encontrada para esta linha.")
}

// line lê a linha do extrato (404 se não existe).
func line(ctx context.Context, q *store.Queries, id string) (store.GetStatementLineRow, error) {
	if !httpx.IsUUID(id) {
		return store.GetStatementLineRow{}, lineNotFound()
	}
	l, err := q.GetStatementLine(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, lineNotFound()
	}
	return l, err
}

// audit registra a ação humana no log de extração (o _auditar da origem).
func (s *Service) audit(ctx context.Context, q *store.Queries, companyID string, change map[string]any) error {
	uid := user.FromContext(ctx)
	b, err := json.Marshal(change)
	if err != nil {
		return err
	}
	text := string(b)
	_, err = q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
		CompanyID: &companyID, Alias: "humano", Attempt: 0, ActorID: &uid, Change: &text, Now: s.clock.Now(),
	})
	return err
}

// RequestResult é o pedido de hipótese: o job e se ele já estava na fila.
type RequestResult struct {
	JobID         string `json:"job_id"`
	Status        string `json:"status"`
	AlreadyQueued bool   `json:"already_queued"`
}

// Request pede ao resolvedor uma hipótese para a linha (o pedir_hipotese da origem, agora
// assíncrono): enfileira o job agent_resolver. Com um pedido da mesma linha já na fila, devolve o
// mesmo job.
func (s *Service) Request(ctx context.Context, lineID string) (RequestResult, error) {
	var out RequestResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		l, err := line(ctx, q, lineID)
		if err != nil {
			return err
		}
		if l.Reconciled {
			return apperr.Conflict("LINE_RECONCILED", "A linha já está conciliada: não há o que propor.")
		}
		if err := reconciliation.RequireOpen(ctx, tx, l.CompanyID, competence.FromDate(l.TxnDate)); err != nil {
			return err
		}
		uid := user.FromContext(ctx)
		job, created, err := reconciliation.EnqueueResolve(ctx, s.queue, tx, l.ID, &uid)
		if err != nil {
			return err
		}
		out = RequestResult{JobID: job.ID, Status: job.Status, AlreadyQueued: !created}
		if !created {
			return nil
		}
		return s.audit(ctx, q, l.CompanyID, map[string]any{"acao": "pedir_hipotese", "linha": l.ID, "job": job.ID})
	})
	return out, err
}

// List devolve as propostas da linha, da mais recente para a mais antiga.
func (s *Service) List(ctx context.Context, lineID string) ([]Proposal, error) {
	q := store.New(s.pool)
	if _, err := line(ctx, q, lineID); err != nil {
		return nil, err
	}
	rows, err := q.ListResolverProposalsByLine(ctx, lineID)
	if err != nil {
		return nil, err
	}
	out := make([]Proposal, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProposal(r))
	}
	return out, nil
}

// pending trava a proposta pendente da linha.
func pending(ctx context.Context, q *store.Queries, lineID, proposalID string) (store.ResolverProposal, error) {
	if !httpx.IsUUID(proposalID) {
		return store.ResolverProposal{}, proposalNotFound()
	}
	p, err := q.GetResolverProposalForUpdate(ctx, proposalID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.StatementTransactionID != lineID) {
		return p, proposalNotFound()
	}
	if err != nil {
		return p, err
	}
	if p.Status != StatusPending {
		return p, apperr.Conflict("PROPOSAL_DECIDED", "A proposta já foi decidida.").WithDetails(map[string]any{"status": p.Status})
	}
	return p, nil
}

// AcceptResult é o aceite: a proposta e as conciliações criadas.
type AcceptResult struct {
	Proposal        Proposal `json:"proposal"`
	Reconciliations []string `json:"reconciliation_ids"`
}

// Accept é o aceitar_hip da origem: só aqui os pares viram conciliação, com origem manual, o
// método resolvedor (ou sem_comprovante), o autor e a proposta [AC-019]. Linha ou lançamento
// inventado, de outra empresa ou já conciliado é ignorado; sem nada a conciliar, a proposta fica
// pendente (409). Serializado com a rodada do matching da empresa.
func (s *Service) Accept(ctx context.Context, lineID, proposalID string) (AcceptResult, error) {
	var out AcceptResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := pending(ctx, q, lineID, proposalID)
		if err != nil {
			return err
		}
		if err := q.LockReconciliationCompany(ctx, p.CompanyID); err != nil {
			return err
		}
		h, err := resolver.Parse(string(p.Payload))
		if err != nil {
			return fmt.Errorf("proposta %s com payload inválido: %w", p.ID, err)
		}
		lineIDs, entryIDs := resolver.CitedIDs(h, p.StatementTransactionID)
		free, err := q.ListFreeStatementLinesByIDs(ctx, store.ListFreeStatementLinesByIDsParams{CompanyID: p.CompanyID, Ids: lineIDs})
		if err != nil {
			return err
		}
		freeEntries, err := q.ListFreeClientEntriesByIDs(ctx, store.ListFreeClientEntriesByIDsParams{CompanyID: p.CompanyID, Ids: entryIDs})
		if err != nil {
			return err
		}
		st := resolver.AcceptState{FreeLines: map[string]bool{}, FreeEntries: map[string]bool{}}
		dates := map[string]time.Time{}
		for _, l := range free {
			st.FreeLines[l.ID] = true
			dates[l.ID] = l.TxnDate
		}
		for _, id := range freeEntries {
			st.FreeEntries[id] = true
		}
		plan := resolver.PlanAccept(h, p.StatementTransactionID, st)
		if len(plan) == 0 {
			return apperr.Conflict("PROPOSAL_NOTHING_TO_ACCEPT",
				"Nenhum par da proposta pode ser conciliado: as linhas ou os lançamentos já foram conciliados ou não existem. Rejeite a proposta.")
		}
		comps := map[string]competence.Competence{}
		for _, c := range plan {
			comp := competence.FromDate(dates[c.StatementID])
			comps[comp.String()] = comp
		}
		keys := make([]string, 0, len(comps))
		for k := range comps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := reconciliation.RequireOpen(ctx, tx, p.CompanyID, comps[k]); err != nil {
				return err
			}
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		for _, c := range plan {
			id, err := q.InsertManualReconciliation(ctx, store.InsertManualReconciliationParams{
				CompanyID: p.CompanyID, StatementTransactionID: c.StatementID, Method: c.Method, ProposalID: &p.ID, CreatedBy: &uid, Now: now,
			})
			if err != nil {
				return fmt.Errorf("gravar a conciliação da linha %s: %w", c.StatementID, err)
			}
			for _, e := range c.EntryIDs {
				if err := q.InsertReconciliationItem(ctx, store.InsertReconciliationItemParams{ReconciliationID: id, CompanyID: p.CompanyID, ClientEntryID: e}); err != nil {
					return fmt.Errorf("gravar o lançamento %s da conciliação: %w", e, err)
				}
			}
			out.Reconciliations = append(out.Reconciliations, id)
		}
		if err := q.DecideResolverProposal(ctx, store.DecideResolverProposalParams{Status: StatusAccepted, DecidedBy: &uid, Now: &now, ID: p.ID}); err != nil {
			return err
		}
		if err := s.audit(ctx, q, p.CompanyID, map[string]any{"acao": "aceitar_proposta", "proposta": p.ID, "conciliacoes": out.Reconciliations}); err != nil {
			return err
		}
		p.Status, p.DecidedBy, p.DecidedAt = StatusAccepted, &uid, &now
		out.Proposal = toProposal(p)
		return nil
	})
	return out, err
}

// Reject é o rejeitar_hip da origem: a proposta fica rejeitada, com o autor; nada é conciliado.
func (s *Service) Reject(ctx context.Context, lineID, proposalID string) (Proposal, error) {
	var out Proposal
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := pending(ctx, q, lineID, proposalID)
		if err != nil {
			return err
		}
		l, err := line(ctx, q, p.StatementTransactionID)
		if err != nil {
			return err
		}
		if err := reconciliation.RequireOpen(ctx, tx, p.CompanyID, competence.FromDate(l.TxnDate)); err != nil {
			return err
		}
		uid := user.FromContext(ctx)
		now := s.clock.Now()
		if err := q.DecideResolverProposal(ctx, store.DecideResolverProposalParams{Status: StatusRejected, DecidedBy: &uid, Now: &now, ID: p.ID}); err != nil {
			return err
		}
		if err := s.audit(ctx, q, p.CompanyID, map[string]any{"acao": "rejeitar_proposta", "proposta": p.ID}); err != nil {
			return err
		}
		p.Status, p.DecidedBy, p.DecidedAt = StatusRejected, &uid, &now
		out = toProposal(p)
		return nil
	})
	return out, err
}
