package obligation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// Origem da alteração registrada no histórico [PRD §38.12].
const (
	SourceInline = "INLINE"
	SourceBulk   = "BULK"
	SourceImport = "IMPORT"
	SourceSystem = "SYSTEM"
)

// Campos do histórico.
const (
	FieldStatus           = "STATUS"
	FieldDueDate          = "DUE_DATE"
	FieldApplicable       = "APPLICABLE"
	FieldTransmissionDate = "TRANSMISSION_DATE"
	FieldProtocol         = "PROTOCOL"
)

// Instance é a instância com os derivados, devolvida pela API.
type Instance struct {
	ID                    string            `json:"id"`
	CompanyID             string            `json:"company_id"`
	Competence            comp.Competence   `json:"competence"`
	ObligationType        ObligationTypeRef `json:"obligation_type"`
	StatusCode            string            `json:"status_code"`
	StatusLabel           string            `json:"status_label"`
	Applicable            bool              `json:"applicable"`
	CountsInProgress      bool              `json:"counts_in_progress"`
	IsFinal               bool              `json:"is_final"`
	IsStarted             bool              `json:"is_started"`
	IsBlocking            bool              `json:"is_blocking"`
	IsOverdue             bool              `json:"is_overdue"`
	DueDateOriginal       *civil.Date       `json:"due_date_original"`
	DueDateEffective      *civil.Date       `json:"due_date_effective"`
	DueOverridden         bool              `json:"due_overridden"`
	DueDateOverrideReason *string           `json:"due_date_override_reason"`
	TransmissionDate      *civil.Date       `json:"transmission_date"`
	Protocol              *string           `json:"protocol"`
	HasHistory            bool              `json:"has_history"`
	Version               int32             `json:"version"`
	LastChangedAt         *time.Time        `json:"last_changed_at"`
	LastChangedBy         *string           `json:"last_changed_by"`
}

// ObligationTypeRef identifica o tipo da instância.
type ObligationTypeRef struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	PrimaryModule string `json:"primary_module"`
}

// RowSummary é o agregado atualizado da linha (empresa × módulo) após a alteração.
type RowSummary struct {
	Module          string      `json:"module"`
	Progress        Progress    `json:"progress"`
	Overdue         int         `json:"overdue"`
	NearestDeadline *civil.Date `json:"nearest_deadline"`
}

// StatusChangeResult é a resposta de PATCH /obligation-instances/{id}/status.
type StatusChangeResult struct {
	Instance Instance   `json:"instance"`
	Row      RowSummary `json:"row"`
}

// StatusChangeInput é o payload da alteração de status.
type StatusChangeInput struct {
	StatusCode       string  `json:"status_code" validate:"required,max=40"`
	Version          int32   `json:"version" validate:"required,min=1"`
	ConfirmException bool    `json:"confirm_exception"`
	ExceptionReason  *string `json:"exception_reason" validate:"omitempty,max=1000"`
}

// Service contém as regras de alteração das instâncias.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// Today é a data de negócio corrente (fuso configurado).
func (s *Service) Today() time.Time { return clock.Today(s.clock) }

func notFound() error {
	return apperr.NotFound("INSTANCE_NOT_FOUND", "Obrigação não encontrada.")
}

// ErrPeriodClosed é devolvido ao editar instância de módulo fechado [plan R9].
//
// A VALIDAR (Q4): bloquear (423) até a reabertura, em vez de apenas alertar.
func errPeriodClosed(module string) error {
	return apperr.Locked("PERIOD_CLOSED", "O módulo está fechado nesta competência. Reabra o fechamento para alterar a obrigação.").
		WithDetails(map[string]any{"module": module})
}

// loadForUpdate trava a instância e valida versão, período e aplicabilidade.
func loadForUpdate(ctx context.Context, q *store.Queries, id string, version int32) (store.GetInstanceForUpdateRow, error) {
	inst, err := q.GetInstanceForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return inst, notFound()
		}
		return inst, err
	}
	if version != 0 && inst.Version != version {
		return inst, apperr.VersionConflict().WithDetails(map[string]any{"current_version": inst.Version})
	}
	if inst.PeriodState == "CLOSED" {
		return inst, errPeriodClosed(inst.PrimaryModule)
	}
	return inst, nil
}

// ChangeStatus altera o status de uma instância [PRD §41.7]: valida o código no conjunto do
// tipo (422), a versão (409) e o período do módulo (423); aplica as dependências; grava o
// histórico INLINE e devolve a instância com derivados e o agregado da linha.
func (s *Service) ChangeStatus(ctx context.Context, id string, in StatusChangeInput) (StatusChangeResult, error) {
	in.StatusCode = strings.ToUpper(strings.TrimSpace(in.StatusCode))
	if err := validate.Struct(in); err != nil {
		return StatusChangeResult{}, err
	}
	now := s.clock.Now()
	userID := user.FromContext(ctx)
	var companyID, competenceID, module string
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		inst, err := loadForUpdate(ctx, q, id, in.Version)
		if err != nil {
			return err
		}
		companyID, competenceID, module = inst.CompanyID, inst.CompetenceID, inst.PrimaryModule
		if !inst.Applicable {
			return apperr.Unprocessable("INSTANCE_NOT_APPLICABLE", "Obrigação não aplicável nesta competência. Altere a aplicabilidade antes do status.")
		}
		def, err := q.GetStatusDefinition(ctx, store.GetStatusDefinitionParams{StatusSetID: inst.StatusSetID, Code: in.StatusCode})
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Unprocessable("INVALID_STATUS", fmt.Sprintf("Status %s não é válido para %s.", in.StatusCode, inst.TypeName)).
				WithDetails(map[string]any{"status_code": in.StatusCode})
		} else if err != nil {
			return err
		}
		if inst.StatusCode == in.StatusCode {
			return nil // nada a alterar
		}
		reason, err := s.checkDependencies(ctx, q, inst, def, in)
		if err != nil {
			return err
		}
		n, err := q.UpdateInstanceStatus(ctx, store.UpdateInstanceStatusParams{
			ID: id, Version: inst.Version, StatusCode: in.StatusCode, LastChangedBy: &userID, LastChangedAt: &now,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.VersionConflict()
		}
		old := inst.StatusCode
		return q.InsertStatusHistory(ctx, store.InsertStatusHistoryParams{
			ObligationInstanceID: id, Field: FieldStatus, OldValue: &old, NewValue: &in.StatusCode,
			UserID: userID, ChangeSource: SourceInline, OverrideReason: reason, CreatedAt: now,
		})
	})
	if err != nil {
		return StatusChangeResult{}, err
	}
	return s.result(ctx, id, companyID, competenceID, module)
}

// loadDependencyState carrega as dependências ativas e as instâncias da mesma empresa e
// competência (estado usado por CheckHard/CheckSoft).
func loadDependencyState(ctx context.Context, q *store.Queries, companyID, competenceID string) ([]Dependency, map[string]DepInstance, error) {
	rows, err := q.ListActiveDependencies(ctx)
	if err != nil {
		return nil, nil, err
	}
	deps := make([]Dependency, len(rows))
	for i, r := range rows {
		deps[i] = Dependency{TypeID: r.ObligationTypeID, PrereqID: r.PrerequisiteObligationTypeID, Mode: r.DependencyMode}
	}
	insts, err := q.CompanyCompetenceInstances(ctx, store.CompanyCompetenceInstancesParams{CompanyID: companyID, CompetenceID: competenceID})
	if err != nil {
		return nil, nil, err
	}
	byType := make(map[string]DepInstance, len(insts))
	for _, r := range insts {
		byType[r.ObligationTypeID] = DepInstance{
			TypeID: r.ObligationTypeID, Code: r.TypeCode, Name: r.TypeName, StatusCode: r.StatusCode,
			Applicable: r.Applicable, IsFinal: r.IsFinal, Order: int(r.TypeSortOrder),
		}
	}
	return deps, byType, nil
}

// checkDependencies aplica as dependências do tipo ao concluir (status final):
//   - HARD (Contábil): etapa anterior aplicável não final bloqueia com 422 DEPENDENCY_BLOCKED;
//     "Sem movimento" pode ser marcado sem checar e satisfaz as etapas seguintes [PRD §18.2].
//
// Devolve o motivo de exceção a registrar no histórico, se houver.
func (s *Service) checkDependencies(ctx context.Context, q *store.Queries, inst store.GetInstanceForUpdateRow, def store.StatusDefinition, _ StatusChangeInput) (*string, error) {
	if !def.IsFinal || def.Code == StatusNoMovement {
		return nil, nil
	}
	deps, byType, err := loadDependencyState(ctx, q, inst.CompanyID, inst.CompetenceID)
	if err != nil {
		return nil, err
	}
	if pending := CheckHard(inst.ObligationTypeID, deps, byType); len(pending) > 0 {
		return nil, apperr.Unprocessable("DEPENDENCY_BLOCKED", BlockedMessage(inst.TypeName, pending)).
			WithDetails(map[string]any{"pending": pending})
	}
	return nil, nil
}

// GetInstance devolve a instância com os derivados.
func (s *Service) GetInstance(ctx context.Context, id string) (Instance, error) {
	v, err := store.New(s.pool).GetInstanceView(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidText(err) {
			return Instance{}, notFound()
		}
		return Instance{}, err
	}
	c, err := comp.Parse(v.CompetenceKey)
	if err != nil {
		return Instance{}, err
	}
	return Instance{
		ID: v.ID, CompanyID: v.CompanyID, Competence: c,
		ObligationType: ObligationTypeRef{ID: v.ObligationTypeID, Code: v.TypeCode, Name: v.TypeName, PrimaryModule: v.PrimaryModule},
		StatusCode:     v.StatusCode, StatusLabel: v.StatusLabel, Applicable: v.Applicable, CountsInProgress: v.CountsInProgress,
		IsFinal: v.IsFinal, IsStarted: v.CountsAsStarted, IsBlocking: v.IsBlocking,
		IsOverdue:       IsOverdue(v.Applicable, v.IsFinal, v.DueDateEffective, s.Today()),
		DueDateOriginal: civil.Ptr(v.DueDateOriginal), DueDateEffective: civil.Ptr(v.DueDateEffective),
		DueOverridden:         v.DueDateOverrideReason != nil,
		DueDateOverrideReason: v.DueDateOverrideReason, TransmissionDate: civil.Ptr(v.TransmissionDate), Protocol: v.Protocol,
		HasHistory: v.HasHistory, Version: v.Version, LastChangedAt: v.LastChangedAt, LastChangedBy: v.LastChangedByName,
	}, nil
}

// RowSummary devolve o agregado da empresa no módulo da instância.
func (s *Service) RowSummary(ctx context.Context, companyID, competenceID, module string) (RowSummary, error) {
	rows, err := ModuleProgress(ctx, s.pool, competenceID, s.Today(), &companyID)
	if err != nil {
		return RowSummary{}, err
	}
	out := RowSummary{Module: module, Progress: ProgressFromCounts(0, 0, 0, 0)}
	for _, r := range rows {
		if r.Module == module {
			out = RowSummary{Module: module, Progress: r.Progress(), Overdue: r.Overdue, NearestDeadline: civil.Ptr(r.NearestDeadline)}
		}
	}
	return out, nil
}

func (s *Service) result(ctx context.Context, id, companyID, competenceID, module string) (StatusChangeResult, error) {
	inst, err := s.GetInstance(ctx, id)
	if err != nil {
		return StatusChangeResult{}, err
	}
	row, err := s.RowSummary(ctx, companyID, competenceID, module)
	if err != nil {
		return StatusChangeResult{}, err
	}
	return StatusChangeResult{Instance: inst, Row: row}, nil
}
