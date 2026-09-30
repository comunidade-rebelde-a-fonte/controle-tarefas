// Package period controla o fechamento independente por empresa + módulo + competência:
// fechar, reabrir com motivo, observação geral e eventos [PRD §10.3-§10.6, §22, RN-006,
// RN-007, RN-016].
package period

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/obligation"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// Estados e eventos do período.
const (
	StateOpen      = "OPEN"
	StateClosed    = "CLOSED"
	EventClosed    = "CLOSED"
	EventReopened  = "REOPENED"
	maxNoteLength  = 2000
	notFoundCode   = "PERIOD_NOT_FOUND"
	notFoundDetail = "Período do módulo não encontrado."
)

// UserRef identifica quem fez a ação.
type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Period é o estado de empresa + módulo + competência.
type Period struct {
	ID          string          `json:"id"`
	CompanyID   string          `json:"company_id"`
	Competence  comp.Competence `json:"competence"`
	Module      string          `json:"module"`
	State       string          `json:"state"`
	ClosedAt    *time.Time      `json:"closed_at"`
	ClosedBy    *UserRef        `json:"closed_by"`
	GeneralNote *string         `json:"general_note"`
	Version     int32           `json:"version"`
}

// Event é uma linha do histórico de abertura, fechamento e reabertura [PRD §10.6].
type Event struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	Reason    *string   `json:"reason"`
	User      UserRef   `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

// CloseInput é o payload de POST /company-module-periods/{id}/close.
type CloseInput struct {
	Version           int32 `json:"version" validate:"required,min=1"`
	ConfirmSetClosing bool  `json:"confirm_set_closing"`
}

// ReopenInput é o payload de POST /company-module-periods/{id}/reopen. O motivo é obrigatório.
type ReopenInput struct {
	Version int32  `json:"version" validate:"omitempty,min=1"`
	Reason  string `json:"reason" validate:"max=1000"`
}

// NoteInput é o payload de PATCH /company-module-periods/{id}/note; vazio limpa a observação.
type NoteInput struct {
	Version     int32   `json:"version" validate:"required,min=1"`
	GeneralNote *string `json:"general_note"`
}

// Service contém as regras do período.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

func notFound() error { return apperr.NotFound(notFoundCode, notFoundDetail) }

func loadForUpdate(ctx context.Context, q *store.Queries, id string, version int32) (store.GetPeriodForUpdateRow, error) {
	p, err := q.GetPeriodForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return p, notFound()
		}
		return p, err
	}
	if version != 0 && p.Version != version {
		return p, apperr.VersionConflict().WithDetails(map[string]any{"current_version": p.Version})
	}
	return p, nil
}

// Close fecha o módulo da empresa na competência, sem afetar os outros módulos [RN-006,
// CA-013]. Se o estágio de fechamento do módulo (Fiscal: coluna Fechamento) não estiver no
// status final, exige confirm_set_closing (409 CLOSING_STATUS_REQUIRED); confirmado, o
// estágio é concluído na mesma transação, com histórico [PRD §13.6].
func (s *Service) Close(ctx context.Context, id string, in CloseInput) (Period, error) {
	if err := validate.Struct(in); err != nil {
		return Period{}, err
	}
	now := s.clock.Now()
	userID := user.FromContext(ctx)
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := loadForUpdate(ctx, q, id, in.Version)
		if err != nil {
			return err
		}
		if p.State == StateClosed {
			return apperr.Unprocessable("PERIOD_ALREADY_CLOSED", "O módulo já está fechado nesta competência.")
		}
		if err := s.completeClosingStage(ctx, q, p, in.ConfirmSetClosing, userID, now); err != nil {
			return err
		}
		n, err := q.ClosePeriod(ctx, store.ClosePeriodParams{ID: id, Version: p.Version, ClosedAt: &now, ClosedBy: &userID})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.VersionConflict()
		}
		return q.InsertPeriodEvent(ctx, store.InsertPeriodEventParams{
			CompanyModulePeriodID: id, EventType: EventClosed, UserID: userID, CreatedAt: now,
		})
	})
	if err != nil {
		return Period{}, err
	}
	return s.Get(ctx, id)
}

// completeClosingStage garante o estágio de fechamento do módulo no status final.
func (s *Service) completeClosingStage(ctx context.Context, q *store.Queries, p store.GetPeriodForUpdateRow, confirm bool, userID string, now time.Time) error {
	stages, err := q.ClosingStageInstances(ctx, store.ClosingStageInstancesParams{
		CompanyID: p.CompanyID, CompetenceID: p.CompetenceID, PrimaryModule: p.ModuleCode,
	})
	if err != nil {
		return err
	}
	var pending []store.ClosingStageInstancesRow
	for _, st := range stages {
		if st.Applicable && st.StatusCode != st.FinalCode {
			pending = append(pending, st)
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if !confirm {
		st := pending[0]
		return apperr.Conflict("CLOSING_STATUS_REQUIRED",
			st.TypeName+" está como "+st.StatusLabel+". Confirme para marcá-lo como "+st.FinalLabel+" e fechar a competência.").
			WithDetails(map[string]any{
				"obligation_instance_id": st.ID, "status_code": st.StatusCode, "status_label": st.StatusLabel,
				"target_status_code": st.FinalCode, "target_status_label": st.FinalLabel,
			})
	}
	deps, byType, err := obligation.LoadDependencyState(ctx, q, p.CompanyID, p.CompetenceID)
	if err != nil {
		return err
	}
	for _, st := range pending {
		if blocked := obligation.CheckHard(st.ObligationTypeID, deps, byType); len(blocked) > 0 {
			return apperr.Unprocessable("DEPENDENCY_BLOCKED", obligation.BlockedMessage(st.TypeName, blocked)).
				WithDetails(map[string]any{"pending": blocked})
		}
		n, err := q.UpdateInstanceStatus(ctx, store.UpdateInstanceStatusParams{
			ID: st.ID, Version: st.Version, StatusCode: st.FinalCode, LastChangedBy: &userID, LastChangedAt: &now,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.VersionConflict()
		}
		old, final := st.StatusCode, st.FinalCode
		if err := q.InsertStatusHistory(ctx, store.InsertStatusHistoryParams{
			ObligationInstanceID: st.ID, Field: obligation.FieldStatus, OldValue: &old, NewValue: &final,
			UserID: userID, ChangeSource: obligation.SourceInline, CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Reopen reabre o módulo fechado; exige motivo e grava evento REOPENED [RN-007, CA-015].
func (s *Service) Reopen(ctx context.Context, id string, in ReopenInput) (Period, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if err := validate.Struct(in); err != nil {
		return Period{}, err
	}
	if in.Reason == "" {
		return Period{}, apperr.Unprocessable("REASON_REQUIRED", "Informe o motivo da reabertura.").
			WithDetails(map[string]any{"fields": map[string]any{"reason": "Campo obrigatório."}})
	}
	now := s.clock.Now()
	userID := user.FromContext(ctx)
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := loadForUpdate(ctx, q, id, in.Version)
		if err != nil {
			return err
		}
		if p.State != StateClosed {
			return apperr.Unprocessable("PERIOD_NOT_CLOSED", "O módulo não está fechado nesta competência.")
		}
		n, err := q.ReopenPeriod(ctx, store.ReopenPeriodParams{ID: id, Version: p.Version, UpdatedAt: now})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.VersionConflict()
		}
		return q.InsertPeriodEvent(ctx, store.InsertPeriodEventParams{
			CompanyModulePeriodID: id, EventType: EventReopened, Reason: &in.Reason, UserID: userID, CreatedAt: now,
		})
	})
	if err != nil {
		return Period{}, err
	}
	return s.Get(ctx, id)
}

// UpdateNote grava a observação geral da empresa no módulo e competência [RN-016]. A
// observação não é obrigação: pode ser editada também com o módulo fechado.
func (s *Service) UpdateNote(ctx context.Context, id string, in NoteInput) (Period, error) {
	if err := validate.Struct(in); err != nil {
		return Period{}, err
	}
	var note *string
	if in.GeneralNote != nil {
		if v := strings.TrimSpace(*in.GeneralNote); v != "" {
			if len([]rune(v)) > maxNoteLength {
				return Period{}, validate.Field("general_note", "Máximo de 2000 caracteres.")
			}
			note = &v
		}
	}
	now := s.clock.Now()
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := loadForUpdate(ctx, q, id, in.Version)
		if err != nil {
			return err
		}
		n, err := q.UpdatePeriodNote(ctx, store.UpdatePeriodNoteParams{ID: id, Version: p.Version, GeneralNote: note, UpdatedAt: now})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.VersionConflict()
		}
		return nil
	})
	if err != nil {
		return Period{}, err
	}
	return s.Get(ctx, id)
}

// Get devolve o período.
func (s *Service) Get(ctx context.Context, id string) (Period, error) {
	r, err := store.New(s.pool).GetPeriod(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Period{}, notFound()
		}
		return Period{}, err
	}
	c, err := comp.Parse(r.CompetenceKey)
	if err != nil {
		return Period{}, err
	}
	out := Period{
		ID: r.ID, CompanyID: r.CompanyID, Competence: c, Module: r.ModuleCode, State: r.State,
		ClosedAt: r.ClosedAt, GeneralNote: r.GeneralNote, Version: r.Version,
	}
	if r.ClosedByID != nil && r.ClosedByName != nil {
		out.ClosedBy = &UserRef{ID: *r.ClosedByID, Name: *r.ClosedByName}
	}
	return out, nil
}

// Events devolve os eventos do período, mais recente primeiro.
func (s *Service) Events(ctx context.Context, id string) ([]Event, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).ListPeriodEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]Event, len(rows))
	for i, r := range rows {
		out[i] = Event{ID: r.ID, EventType: r.EventType, Reason: r.Reason, User: UserRef{ID: r.UserID, Name: r.UserName}, CreatedAt: r.CreatedAt}
	}
	return out, nil
}
