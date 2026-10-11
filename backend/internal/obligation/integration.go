package obligation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/store"
)

// SourceIntegration é a origem das mudanças automáticas entre módulos [PRD §6.6]: a Conciliação
// concluída ou reaberta muda a obrigação Conciliação do Contábil (TASK-022, D-035).
const SourceIntegration = "SYSTEM_INTEGRATION"

// IntegrationChange é a mudança de status pedida por outro módulo: a instância, o status, quem
// disparou o evento e o evento causador, que vai para o histórico.
type IntegrationChange struct {
	InstanceID string
	StatusCode string
	UserID     string
	Event      string
	Now        time.Time
}

// ApplyIntegrationStatus aplica a mudança na transação de quem chama, com as regras do
// ChangeStatus: o período do módulo aberto (423), a instância aplicável, o status do conjunto do
// tipo (422) e, ao concluir, as dependências HARD (422 DEPENDENCY_BLOCKED). Dependência SOFT
// pendente também recusa: não há quem confirme a exceção. Não há versão do cliente: a instância é
// travada (FOR UPDATE). O histórico leva a origem SYSTEM_INTEGRATION e o evento causador.
// Devolve o status anterior; com o mesmo status, nada muda (changed = false).
func ApplyIntegrationStatus(ctx context.Context, q *store.Queries, c IntegrationChange) (old string, changed bool, err error) {
	code := strings.ToUpper(strings.TrimSpace(c.StatusCode))
	inst, err := loadForUpdate(ctx, q, c.InstanceID, 0)
	if err != nil {
		return "", false, err
	}
	if !inst.Applicable {
		return inst.StatusCode, false, apperr.Unprocessable("INSTANCE_NOT_APPLICABLE",
			"Obrigação não aplicável nesta competência. Altere a aplicabilidade antes do status.")
	}
	def, err := q.GetStatusDefinition(ctx, store.GetStatusDefinitionParams{StatusSetID: inst.StatusSetID, Code: code})
	if errors.Is(err, pgx.ErrNoRows) {
		return inst.StatusCode, false, apperr.Unprocessable("INVALID_STATUS", fmt.Sprintf("Status %s não é válido para %s.", code, inst.TypeName)).
			WithDetails(map[string]any{"status_code": code})
	} else if err != nil {
		return inst.StatusCode, false, err
	}
	if inst.StatusCode == code {
		return inst.StatusCode, false, nil
	}
	if def.IsFinal {
		deps, byType, err := LoadDependencyState(ctx, q, inst.CompanyID, inst.CompetenceID)
		if err != nil {
			return inst.StatusCode, false, err
		}
		if def.Code != StatusNoMovement {
			if pending := CheckHard(inst.ObligationTypeID, deps, byType); len(pending) > 0 {
				return inst.StatusCode, false, apperr.Unprocessable("DEPENDENCY_BLOCKED", BlockedMessage(inst.TypeName, pending)).
					WithDetails(map[string]any{"pending": pending})
			}
		}
		if pending := CheckSoft(inst.ObligationTypeID, deps, byType); len(pending) > 0 {
			return inst.StatusCode, false, apperr.Conflict("DEPENDENCY_WARNING", SoftWarningMessage(inst.TypeName, pending)).
				WithDetails(map[string]any{"pending": pending})
		}
	}
	n, err := q.UpdateInstanceStatus(ctx, store.UpdateInstanceStatusParams{
		ID: inst.ID, Version: inst.Version, StatusCode: code, LastChangedBy: &c.UserID, LastChangedAt: &c.Now,
	})
	if err != nil {
		return inst.StatusCode, false, err
	}
	if n == 0 {
		return inst.StatusCode, false, apperr.VersionConflict()
	}
	event := c.Event
	if err := q.InsertStatusHistory(ctx, store.InsertStatusHistoryParams{
		ObligationInstanceID: inst.ID, Field: FieldStatus, OldValue: &inst.StatusCode, NewValue: &code,
		UserID: c.UserID, ChangeSource: SourceIntegration, OverrideReason: &event, CreatedAt: c.Now,
	}); err != nil {
		return inst.StatusCode, false, err
	}
	return inst.StatusCode, true, nil
}
