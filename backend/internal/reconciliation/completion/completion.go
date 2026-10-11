// Package completion é a conclusão da Conciliação da empresa na competência (TASK-022, D-035; F-008).
//
// A conciliação está pronta quando o extrato está todo resolvido (decisão do usuário): ao menos um
// extrato importado cobre a competência, toda linha está conciliada (ou confirmada sem comprovante)
// e classificada, e nenhum documento nem comprovante parcial está em revisão; comprovante sem
// extrato só avisa. O contador confirma a conclusão, e a obrigação Conciliação do Contábil vai para
// OK pelo serviço de obrigações, com a origem SYSTEM_INTEGRATION, na mesma transação. Concluída, a
// competência fica travada para a Conciliação até a reabertura com motivo, que volta a obrigação
// para Pendente. O fechamento do Contábil continua a única autoridade (ADR-003): com ele fechado,
// não se conclui nem se reabre.
package completion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/obligation"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/review"
	"controle/internal/store"
	"controle/internal/user"
)

// TypeReconciliation é a obrigação canônica Conciliação do módulo Contábil (05 §9).
const TypeReconciliation = "ACCOUNTING_RECONCILIATION"

// Os status da obrigação na conclusão e na reabertura (decisão do usuário, D-035).
const (
	StatusDone     = "OK"
	StatusReopened = "PENDING"
)

// Estados da conciliação na competência (F-008).
const (
	StateInProgress = "em_andamento"
	StateReady      = "pronta"
	StateCompleted  = "concluida"
)

// O que a sincronização fez com a obrigação, na conclusão e na reabertura.
const (
	SyncDone          = "concluida"
	SyncAlreadyFinal  = "ja_final"
	SyncNotApplicable = "nao_aplicavel"
	SyncNoObligation  = "sem_obrigacao"
	SyncPending       = "pendente"
	SyncKept          = "mantida"
)

const maxReason = 1000

// Service conclui e reabre a conciliação.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// Blocking é o que ainda impede a conclusão.
type Blocking struct {
	MissingStatement bool `json:"missing_statement"`
	Divergent        int  `json:"divergent"`
	WithoutReceipt   int  `json:"without_receipt"`
	Review           int  `json:"review"`
	Classification   int  `json:"classification"`
}

func (b Blocking) empty() bool {
	return !b.MissingStatement && b.Divergent == 0 && b.WithoutReceipt == 0 && b.Review == 0 && b.Classification == 0
}

// Warnings é o que só avisa: comprovantes sem linha do extrato (podem ser de outra conta ou de
// outro mês).
type Warnings struct {
	WithoutStatement int `json:"without_statement"`
}

// Obligation é a obrigação Conciliação do Contábil na competência.
type Obligation struct {
	InstanceID  string `json:"instance_id"`
	StatusCode  string `json:"status_code"`
	StatusLabel string `json:"status_label"`
	IsFinal     bool   `json:"is_final"`
	Applicable  bool   `json:"applicable"`
}

// Record é uma conclusão, com a reabertura quando houver.
type Record struct {
	ID                     string     `json:"id"`
	CompletedBy            string     `json:"completed_by"`
	CompletedAt            time.Time  `json:"completed_at"`
	ObligationStatusBefore *string    `json:"obligation_status_before"`
	ObligationSync         string     `json:"obligation_sync"`
	ReopenedBy             *string    `json:"reopened_by"`
	ReopenedAt             *time.Time `json:"reopened_at"`
	ReopenReason           *string    `json:"reopen_reason"`
	ReopenObligationSync   *string    `json:"reopen_obligation_sync"`
}

// State é a conclusão da conciliação na competência.
type State struct {
	State      string      `json:"state"`
	Ready      bool        `json:"ready"`
	Blocking   Blocking    `json:"blocking"`
	Warnings   Warnings    `json:"warnings"`
	Obligation *Obligation `json:"obligation"`
	Current    *Record     `json:"current"`
	History    []Record    `json:"history"`
}

func companyNotFound() error { return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.") }

func requireCompany(ctx context.Context, q *store.Queries, id string) error {
	if !httpx.IsUUID(id) {
		return companyNotFound()
	}
	if _, err := q.GetCompanyIdentity(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return companyNotFound()
	} else if err != nil {
		return err
	}
	return nil
}

func findObligation(ctx context.Context, q *store.Queries, companyID string, day time.Time) (*Obligation, error) {
	o, err := q.FindObligationInstance(ctx, store.FindObligationInstanceParams{CompanyID: companyID, Competence: day, TypeCode: TypeReconciliation})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Obligation{InstanceID: o.ID, StatusCode: o.StatusCode, StatusLabel: o.StatusLabel, IsFinal: o.IsFinal, Applicable: o.Applicable}, nil
}

// Load lê a conclusão da conciliação na competência (sem conferir a empresa).
func Load(ctx context.Context, q *store.Queries, companyID string, c competence.Competence) (State, error) {
	day := c.FirstDay()
	statements, err := q.CountCompetenceStatements(ctx, store.CountCompetenceStatementsParams{CompanyID: companyID, Competence: day})
	if err != nil {
		return State{}, err
	}
	counts, err := review.Counts(ctx, q, &companyID, &day)
	if err != nil {
		return State{}, err
	}
	out := State{
		Blocking: Blocking{MissingStatement: statements == 0, Divergent: counts[review.BucketDivergent],
			WithoutReceipt: counts[review.BucketWithoutReceipt], Review: counts[review.BucketReview],
			Classification: counts[review.BucketClassification]},
		Warnings: Warnings{WithoutStatement: counts[review.BucketWithoutStatement]},
		History:  []Record{},
	}
	if out.Obligation, err = findObligation(ctx, q, companyID, day); err != nil {
		return State{}, err
	}
	rows, err := q.ListCompletions(ctx, store.ListCompletionsParams{CompanyID: companyID, Competence: day})
	if err != nil {
		return State{}, err
	}
	for _, r := range rows {
		rec := Record{ID: r.ID, CompletedBy: r.CompletedByName, CompletedAt: r.CompletedAt, ObligationStatusBefore: r.ObligationStatusBefore,
			ObligationSync: r.ObligationSync, ReopenedBy: r.ReopenedByName, ReopenedAt: r.ReopenedAt, ReopenReason: r.ReopenReason,
			ReopenObligationSync: r.ReopenObligationSync}
		out.History = append(out.History, rec)
		if r.ReopenedAt == nil {
			current := rec
			out.Current = &current
		}
	}
	out.Ready = out.Blocking.empty()
	switch {
	case out.Current != nil:
		out.State = StateCompleted
	case out.Ready:
		out.State = StateReady
	default:
		out.State = StateInProgress
	}
	return out, nil
}

// State lê a conclusão da conciliação da empresa na competência.
func (s *Service) State(ctx context.Context, companyID string, c competence.Competence) (State, error) {
	q := store.New(s.pool)
	if err := requireCompany(ctx, q, companyID); err != nil {
		return State{}, err
	}
	return Load(ctx, q, companyID, c)
}

// audit registra a ação humana no log de extração da empresa.
func audit(ctx context.Context, q *store.Queries, companyID, uid string, now time.Time, change map[string]any) error {
	b, err := json.Marshal(change)
	if err != nil {
		return err
	}
	text := string(b)
	_, err = q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
		CompanyID: &companyID, Alias: "humano", Attempt: 0, ActorID: &uid, Change: &text, Now: now,
	})
	return err
}

// Complete conclui a conciliação: confere que está pronta (409 RECONCILIATION_NOT_READY), aplica o
// OK na obrigação Conciliação pelo serviço de obrigações (as dependências HARD valem: 422
// DEPENDENCY_BLOCKED) e grava a conclusão, tudo na mesma transação, serializada pela trava
// exclusiva da empresa. Já concluída, devolve o estado sem gravar de novo (idempotente). Com o
// Contábil fechado, 423.
func (s *Service) Complete(ctx context.Context, companyID string, c competence.Competence) (State, error) {
	uid := user.FromContext(ctx)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		if err := q.LockReconciliationCompany(ctx, companyID); err != nil {
			return err
		}
		day := c.FirstDay()
		if _, err := q.GetActiveCompletion(ctx, store.GetActiveCompletionParams{CompanyID: companyID, Competence: day}); err == nil {
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		closed, err := reconciliation.AccountingClosed(ctx, tx, companyID, c)
		if err != nil {
			return err
		}
		if closed {
			return reconciliation.ErrPeriodClosed(c)
		}
		st, err := Load(ctx, q, companyID, c)
		if err != nil {
			return err
		}
		if !st.Ready {
			return apperr.Conflict("RECONCILIATION_NOT_READY",
				"A conciliação de "+c.Display()+" ainda não está pronta: resolva o que falta no extrato antes de concluir.").
				WithDetails(map[string]any{"blocking": st.Blocking})
		}
		now := s.clock.Now()
		params := store.InsertCompletionParams{CompanyID: companyID, Competence: day, CompletedBy: uid, Now: now, ObligationSync: SyncNoObligation}
		if o := st.Obligation; o != nil {
			before := o.StatusCode
			params.ObligationInstanceID, params.ObligationStatusBefore = &o.InstanceID, &before
			switch {
			case !o.Applicable:
				params.ObligationSync = SyncNotApplicable
			case o.IsFinal:
				params.ObligationSync = SyncAlreadyFinal
			default:
				if _, _, err := obligation.ApplyIntegrationStatus(ctx, q, obligation.IntegrationChange{
					InstanceID: o.InstanceID, StatusCode: StatusDone, UserID: uid, Now: now,
					Event: "Conciliação de " + c.Display() + " concluída na Conciliação.",
				}); err != nil {
					return err
				}
				params.ObligationSync = SyncDone
			}
		}
		rec, err := q.InsertCompletion(ctx, params)
		if err != nil {
			return err
		}
		return audit(ctx, q, companyID, uid, now, map[string]any{"acao": "concluir_conciliacao", "conclusao": rec.ID,
			"competencia": c.String(), "obrigacao": params.ObligationSync})
	})
	if err != nil {
		return State{}, err
	}
	return s.State(ctx, companyID, c)
}

// Reopen reabre a conciliação concluída, com motivo: a obrigação volta para Pendente, se ainda
// estiver no OK que a conclusão aplicou e ninguém mudou o status depois; senão, fica como está
// (mantida). Sem conclusão em vigor, 409; com o Contábil fechado, 423 (reabra o Contábil antes).
func (s *Service) Reopen(ctx context.Context, companyID string, c competence.Competence, reason string) (State, error) {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return State{}, validation("reason", "Informe o motivo da reabertura.")
	case len([]rune(reason)) > maxReason:
		return State{}, validation("reason", fmt.Sprintf("O motivo passa de %d caracteres.", maxReason))
	}
	uid := user.FromContext(ctx)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, companyID); err != nil {
			return err
		}
		if err := q.LockReconciliationCompany(ctx, companyID); err != nil {
			return err
		}
		day := c.FirstDay()
		active, err := q.GetActiveCompletion(ctx, store.GetActiveCompletionParams{CompanyID: companyID, Competence: day})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("RECONCILIATION_NOT_COMPLETED", "A conciliação de "+c.Display()+" não está concluída.")
		}
		if err != nil {
			return err
		}
		closed, err := reconciliation.AccountingClosed(ctx, tx, companyID, c)
		if err != nil {
			return err
		}
		if closed {
			return reconciliation.ErrPeriodClosed(c)
		}
		now := s.clock.Now()
		sync, err := revert(ctx, q, active, c, uid, reason, now)
		if err != nil {
			return err
		}
		n, err := q.ReopenCompletion(ctx, store.ReopenCompletionParams{ReopenedBy: &uid, Now: &now, Reason: &reason, ObligationSync: &sync, ID: active.ID})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.Conflict("RECONCILIATION_NOT_COMPLETED", "A conciliação de "+c.Display()+" não está concluída.")
		}
		return audit(ctx, q, companyID, uid, now, map[string]any{"acao": "reabrir_conciliacao", "conclusao": active.ID,
			"competencia": c.String(), "motivo": reason, "obrigacao": sync})
	})
	if err != nil {
		return State{}, err
	}
	return s.State(ctx, companyID, c)
}

// revert volta a obrigação para Pendente na reabertura, só se ela ainda estiver no OK da conclusão.
func revert(ctx context.Context, q *store.Queries, active store.ReconciliationCompletion, c competence.Competence, uid, reason string,
	now time.Time) (string, error) {
	o, err := findObligation(ctx, q, active.CompanyID, c.FirstDay())
	if err != nil {
		return "", err
	}
	switch {
	case o == nil:
		return SyncNoObligation, nil
	case !o.Applicable:
		return SyncNotApplicable, nil
	case active.ObligationSync != SyncDone || o.StatusCode != StatusDone:
		return SyncKept, nil
	}
	last, err := q.GetLastStatusChange(ctx, o.InstanceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncKept, nil
	}
	if err != nil {
		return "", err
	}
	if last.ChangeSource != obligation.SourceIntegration || last.NewValue == nil || *last.NewValue != StatusDone {
		return SyncKept, nil
	}
	if _, _, err := obligation.ApplyIntegrationStatus(ctx, q, obligation.IntegrationChange{
		InstanceID: o.InstanceID, StatusCode: StatusReopened, UserID: uid, Now: now,
		Event: "Conciliação de " + c.Display() + " reaberta na Conciliação: " + reason,
	}); err != nil {
		return "", err
	}
	return SyncPending, nil
}

func validation(field, msg string) error {
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
		WithDetails(map[string]any{"fields": map[string]any{field: msg}})
}
