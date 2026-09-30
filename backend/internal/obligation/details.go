package obligation

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/opt"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// DetailsInput é o payload de PATCH /obligation-instances/{id}/details. Só os campos enviados
// mudam; null limpa transmissão/protocolo [PRD §15.3, §21.3; plan R7].
type DetailsInput struct {
	Version               int32                    `json:"version" validate:"required,min=1"`
	TransmissionDate      opt.Optional[civil.Date] `json:"transmission_date"`
	Protocol              opt.Optional[string]     `json:"protocol"`
	DueDateEffective      *civil.Date              `json:"due_date_effective"`
	DueDateOverrideReason *string                  `json:"due_date_override_reason" validate:"omitempty,max=1000"`
	Applicable            *bool                    `json:"applicable"`
}

// HistoryEntry é uma linha do histórico da instância [PRD §23].
type HistoryEntry struct {
	ID             string    `json:"id"`
	Field          string    `json:"field"`
	OldValue       *string   `json:"old_value"`
	NewValue       *string   `json:"new_value"`
	OldLabel       *string   `json:"old_label"`
	NewLabel       *string   `json:"new_label"`
	ChangeSource   string    `json:"change_source"`
	OverrideReason *string   `json:"override_reason"`
	User           UserRef   `json:"user"`
	CreatedAt      time.Time `json:"created_at"`
}

// UserRef identifica quem fez a alteração.
type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func dateText(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(time.DateOnly)
	return &s
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// historyChange é uma mudança a registrar no histórico.
type historyChange struct {
	field    string
	old, new *string
	reason   *string
}

// UpdateDetails altera transmissão, protocolo, vencimento efetivo (com motivo) e
// aplicabilidade, registrando cada mudança no histórico INLINE.
func (s *Service) UpdateDetails(ctx context.Context, id string, in DetailsInput) (StatusChangeResult, error) {
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
		info, err := q.GetInstanceOverrideInfo(ctx, id)
		if err != nil {
			return err
		}
		p := store.UpdateInstanceDetailsParams{
			ID: id, Version: inst.Version, TransmissionDate: inst.TransmissionDate, Protocol: inst.Protocol,
			DueDateEffective: inst.DueDateEffective, DueDateOverrideReason: info.DueDateOverrideReason,
			DueDateOverriddenBy: info.DueDateOverriddenBy, DueDateOverriddenAt: info.DueDateOverriddenAt,
			Applicable: inst.Applicable, StatusCode: inst.StatusCode, LastChangedBy: &userID, LastChangedAt: &now,
		}
		var changes []historyChange

		if in.TransmissionDate.Set {
			if !inst.AllowsTransmissionDate {
				return apperr.Unprocessable("FIELD_NOT_ALLOWED", "Esta obrigação não registra data de transmissão.")
			}
			p.TransmissionDate = civil.TimePtr(in.TransmissionDate.Value)
			if !sameDate(inst.TransmissionDate, p.TransmissionDate) {
				changes = append(changes, historyChange{field: FieldTransmissionDate, old: dateText(inst.TransmissionDate), new: dateText(p.TransmissionDate)})
			}
		}
		if in.Protocol.Set {
			if !inst.AllowsProtocol {
				return apperr.Unprocessable("FIELD_NOT_ALLOWED", "Esta obrigação não registra protocolo.")
			}
			var proto *string
			if in.Protocol.Value != nil {
				if v := strings.TrimSpace(*in.Protocol.Value); v != "" {
					if len(v) > 120 {
						return validate.Field("protocol", "Máximo de 120 caracteres.")
					}
					proto = &v
				}
			}
			p.Protocol = proto
			if !sameText(inst.Protocol, proto) {
				changes = append(changes, historyChange{field: FieldProtocol, old: inst.Protocol, new: proto})
			}
		}
		if in.DueDateEffective != nil {
			reason := ""
			if in.DueDateOverrideReason != nil {
				reason = strings.TrimSpace(*in.DueDateOverrideReason)
			}
			if reason == "" {
				return apperr.Unprocessable("REASON_REQUIRED", "Informe o motivo da alteração do vencimento.").
					WithDetails(map[string]any{"fields": map[string]any{"due_date_override_reason": "Campo obrigatório."}})
			}
			newDue := in.DueDateEffective.Time
			if !sameDate(inst.DueDateEffective, &newDue) {
				p.DueDateEffective = &newDue
				p.DueDateOverrideReason = &reason
				p.DueDateOverriddenBy = &userID
				p.DueDateOverriddenAt = &now
				changes = append(changes, historyChange{field: FieldDueDate, old: dateText(inst.DueDateEffective), new: dateText(&newDue), reason: &reason})
			}
		}
		if in.Applicable != nil && *in.Applicable != inst.Applicable {
			p.Applicable = *in.Applicable
			oldA, newA := boolText(inst.Applicable), boolText(*in.Applicable)
			changes = append(changes, historyChange{field: FieldApplicable, old: &oldA, new: &newA})
			// Não aplicável fica visível, fora do denominador; no Fiscal usa "Não se aplica" [D6].
			codes, err := statusCodes(ctx, q, inst.StatusSetID)
			if err != nil {
				return err
			}
			newStatus := inst.StatusCode
			switch {
			case !*in.Applicable && slices.Contains(codes, statusNotApplicable):
				newStatus = statusNotApplicable
			case *in.Applicable && inst.StatusCode == statusNotApplicable:
				newStatus = info.InitialStatusCode
			}
			if newStatus != inst.StatusCode {
				old := inst.StatusCode
				p.StatusCode = newStatus
				changes = append(changes, historyChange{field: FieldStatus, old: &old, new: &newStatus})
			}
		}
		if len(changes) == 0 {
			return nil
		}
		n, err := q.UpdateInstanceDetails(ctx, p)
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.VersionConflict()
		}
		for _, c := range changes {
			if err := q.InsertStatusHistory(ctx, store.InsertStatusHistoryParams{
				ObligationInstanceID: id, Field: c.field, OldValue: c.old, NewValue: c.new, UserID: userID,
				ChangeSource: SourceInline, OverrideReason: c.reason, CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return StatusChangeResult{}, err
	}
	return s.result(ctx, id, companyID, competenceID, module)
}

const statusNotApplicable = "NOT_APPLICABLE"

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func statusCodes(ctx context.Context, q *store.Queries, setID string) ([]string, error) {
	defs, err := q.ListStatusDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range defs {
		if d.StatusSetID == setID {
			out = append(out, d.Code)
		}
	}
	return out, nil
}

// History devolve o histórico da instância, mais recente primeiro.
func (s *Service) History(ctx context.Context, id string) ([]HistoryEntry, error) {
	q := store.New(s.pool)
	if _, err := q.GetInstanceView(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFound()
		}
		return nil, err
	}
	rows, err := q.ListInstanceHistory(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryEntry, len(rows))
	for i, r := range rows {
		out[i] = HistoryEntry{
			ID: r.ID, Field: r.Field, OldValue: r.OldValue, NewValue: r.NewValue, OldLabel: r.OldLabel, NewLabel: r.NewLabel,
			ChangeSource: r.ChangeSource, OverrideReason: r.OverrideReason, User: UserRef{ID: r.UserID, Name: r.UserName}, CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

func (h *Handler) updateDetails(w http.ResponseWriter, r *http.Request) {
	id, err := instanceID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in DetailsInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	res, err := h.svc.UpdateDetails(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	id, err := instanceID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	items, err := h.svc.History(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
