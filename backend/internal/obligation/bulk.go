package obligation

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// BulkTarget seleciona as instâncias por competência + tipo + empresas.
type BulkTarget struct {
	Competence       string   `json:"competence" validate:"required,competence"`
	ObligationTypeID string   `json:"obligation_type_id" validate:"required,uuid"`
	CompanyIDs       []string `json:"company_ids" validate:"required,min=1,max=2000,dive,uuid"`
}

// BulkInput é o payload de POST /obligation-instances/bulk-status [PRD §26.3].
type BulkInput struct {
	InstanceIDs      []string    `json:"instance_ids" validate:"omitempty,max=5000,dive,uuid"`
	Target           *BulkTarget `json:"target"`
	StatusCode       string      `json:"status_code" validate:"required,max=40"`
	DryRun           bool        `json:"dry_run"`
	ConfirmException bool        `json:"confirm_exception"`
	ExceptionReason  *string     `json:"exception_reason" validate:"omitempty,max=1000"`
}

// BulkSkipped é uma instância pulada, com o motivo.
type BulkSkipped struct {
	ID          string `json:"id"`
	CompanyCode string `json:"company_code"`
	CompanyName string `json:"company_name"`
	Reason      string `json:"reason"`
	Message     string `json:"message"`
}

// BulkResult é a resposta: atualizadas (ou que seriam, no dry_run) e puladas com motivo.
type BulkResult struct {
	DryRun   bool          `json:"dry_run"`
	Selected int           `json:"selected"`
	Updated  int           `json:"updated"`
	Skipped  []BulkSkipped `json:"skipped"`
	// Instâncias com componentes SOFT pendentes que exigem confirmação de exceção.
	SoftWarnings int `json:"soft_warnings"`
}

// Motivos de exclusão do lote.
const (
	SkipPeriodClosed  = "PERIOD_CLOSED"
	SkipNotApplicable = "NOT_APPLICABLE"
	SkipBlocked       = "DEPENDENCY_BLOCKED"
	SkipInvalidStatus = "INVALID_STATUS"
	SkipUnchanged     = "UNCHANGED"
)

// BulkStatus aplica o mesmo status a várias instâncias [PRD §26.3, CA-016]: a regra de cada
// instância é a da alteração individual; bloqueadas por HARD, em período fechado, não
// aplicáveis ou já no status são puladas sem abortar o lote. Dependências SOFT exigem
// confirmação global com motivo. Tudo numa transação, com histórico BULK por instância.
func (s *Service) BulkStatus(ctx context.Context, in BulkInput) (BulkResult, error) {
	in.StatusCode = strings.ToUpper(strings.TrimSpace(in.StatusCode))
	if err := validate.Struct(in); err != nil {
		return BulkResult{}, err
	}
	if (len(in.InstanceIDs) == 0) == (in.Target == nil) {
		return BulkResult{}, validate.Field("instance_ids", "Informe instance_ids ou target (competência, obrigação e empresas).")
	}
	reason := ""
	if in.ExceptionReason != nil {
		reason = strings.TrimSpace(*in.ExceptionReason)
	}
	now := s.clock.Now()
	userID := user.FromContext(ctx)
	res := BulkResult{DryRun: in.DryRun, Skipped: []BulkSkipped{}}

	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		ids := in.InstanceIDs
		if in.Target != nil {
			c, err := comp.Parse(in.Target.Competence)
			if err != nil {
				return validate.Field("target.competence", "Competência inválida.")
			}
			if ids, err = q.ResolveBulkTargets(ctx, store.ResolveBulkTargetsParams{
				CompetenceKey: c.String(), ObligationTypeID: in.Target.ObligationTypeID, CompanyIds: in.Target.CompanyIDs,
			}); err != nil {
				return err
			}
		}
		insts, err := q.GetInstancesForUpdate(ctx, dedupe(ids))
		if err != nil {
			return err
		}
		res.Selected = len(insts)
		if len(insts) == 0 {
			return nil
		}
		deps, state, err := s.bulkDependencyState(ctx, q, insts)
		if err != nil {
			return err
		}
		defsBySet := map[string]map[string]store.StatusDefinition{}
		allDefs, err := q.ListStatusDefinitions(ctx)
		if err != nil {
			return err
		}
		for _, d := range allDefs {
			if defsBySet[d.StatusSetID] == nil {
				defsBySet[d.StatusSetID] = map[string]store.StatusDefinition{}
			}
			defsBySet[d.StatusSetID][d.Code] = d
		}

		type planned struct {
			inst store.GetInstancesForUpdateRow
			soft bool
		}
		var plan []planned
		skip := func(i store.GetInstancesForUpdateRow, code, msg string) {
			res.Skipped = append(res.Skipped, BulkSkipped{ID: i.ID, CompanyCode: i.CompanyCode, CompanyName: i.CompanyName, Reason: code, Message: msg})
		}
		for _, i := range insts {
			def, ok := defsBySet[i.StatusSetID][in.StatusCode]
			switch {
			case !ok:
				skip(i, SkipInvalidStatus, "Status não é válido para "+i.TypeName+".")
				continue
			case i.PeriodState == "CLOSED":
				skip(i, SkipPeriodClosed, "Módulo fechado nesta competência.")
				continue
			case !i.Applicable:
				skip(i, SkipNotApplicable, "Obrigação não aplicável.")
				continue
			case i.StatusCode == in.StatusCode:
				skip(i, SkipUnchanged, "Já está neste status.")
				continue
			}
			soft := false
			if def.IsFinal {
				byType := state[i.CompanyID]
				if def.Code != StatusNoMovement {
					if pending := CheckHard(i.ObligationTypeID, deps, byType); len(pending) > 0 {
						skip(i, SkipBlocked, BlockedMessage(i.TypeName, pending))
						continue
					}
				}
				soft = len(CheckSoft(i.ObligationTypeID, deps, byType)) > 0
			}
			if soft {
				res.SoftWarnings++
			}
			plan = append(plan, planned{inst: i, soft: soft})
		}
		res.Updated = len(plan)
		if in.DryRun {
			return nil
		}
		if res.SoftWarnings > 0 {
			if !in.ConfirmException {
				return apperr.Conflict("DEPENDENCY_WARNING", "Há obrigações com pré-requisitos pendentes. Confirme a exceção e informe o motivo para concluir o lote.").
					WithDetails(map[string]any{"soft_warnings": res.SoftWarnings})
			}
			if reason == "" {
				return validate.Field("exception_reason", "Informe o motivo da exceção.")
			}
		}
		for _, p := range plan {
			var r *string
			if p.soft {
				r = &reason
			}
			n, err := q.UpdateInstanceStatus(ctx, store.UpdateInstanceStatusParams{
				ID: p.inst.ID, Version: p.inst.Version, StatusCode: in.StatusCode, LastChangedBy: &userID, LastChangedAt: &now,
			})
			if err != nil {
				return err
			}
			if n == 0 {
				return apperr.VersionConflict()
			}
			old := p.inst.StatusCode
			if err := q.InsertStatusHistory(ctx, store.InsertStatusHistoryParams{
				ObligationInstanceID: p.inst.ID, Field: FieldStatus, OldValue: &old, NewValue: &in.StatusCode, UserID: userID,
				ChangeSource: SourceBulk, OverrideReason: r, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return BulkResult{}, err
	}
	return res, nil
}

// bulkDependencyState carrega dependências e instâncias das empresas do lote (por competência).
func (s *Service) bulkDependencyState(ctx context.Context, q *store.Queries, insts []store.GetInstancesForUpdateRow) ([]Dependency, map[string]map[string]DepInstance, error) {
	rows, err := q.ListActiveDependencies(ctx)
	if err != nil {
		return nil, nil, err
	}
	deps := make([]Dependency, len(rows))
	for i, r := range rows {
		deps[i] = Dependency{TypeID: r.ObligationTypeID, PrereqID: r.PrerequisiteObligationTypeID, Mode: r.DependencyMode}
	}
	byCompetence := map[string][]string{}
	for _, i := range insts {
		if !slices.Contains(byCompetence[i.CompetenceID], i.CompanyID) {
			byCompetence[i.CompetenceID] = append(byCompetence[i.CompetenceID], i.CompanyID)
		}
	}
	state := map[string]map[string]DepInstance{}
	for competenceID, companies := range byCompetence {
		all, err := q.CompanyInstancesForDependencies(ctx, store.CompanyInstancesForDependenciesParams{CompetenceID: competenceID, CompanyIds: companies})
		if err != nil {
			return nil, nil, err
		}
		for _, r := range all {
			if state[r.CompanyID] == nil {
				state[r.CompanyID] = map[string]DepInstance{}
			}
			state[r.CompanyID][r.ObligationTypeID] = DepInstance{
				TypeID: r.ObligationTypeID, Code: r.TypeCode, Name: r.TypeName, StatusCode: r.StatusCode,
				Applicable: r.Applicable, IsFinal: r.IsFinal, Order: int(r.TypeSortOrder),
			}
		}
	}
	return deps, state, nil
}

func dedupe(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func (h *Handler) bulkStatus(w http.ResponseWriter, r *http.Request) {
	var in BulkInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := h.svc.BulkStatus(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
