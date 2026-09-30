package company

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// RegimePeriod é um intervalo da linha do tempo de regimes.
type RegimePeriod struct {
	ID            string                 `json:"id"`
	TaxRegime     string                 `json:"tax_regime"`
	RegimeGroup   string                 `json:"regime_group"`
	EffectiveFrom competence.Competence  `json:"effective_from"`
	EffectiveTo   *competence.Competence `json:"effective_to"`
	CreatedBy     UserRef                `json:"created_by"`
	CreatedAt     time.Time              `json:"created_at"`
}

// RegimeChangeInput é o payload de POST /companies/{id}/tax-regimes [UC-04].
type RegimeChangeInput struct {
	TaxRegime     string `json:"tax_regime" validate:"required,oneof=SIMPLES_NACIONAL LUCRO_PRESUMIDO LUCRO_REAL LUCRO_ARBITRADO IMUNE_ISENTA"`
	EffectiveFrom string `json:"effective_from" validate:"required,competence"`
}

// RegimeAt resolve o regime vigente na competência (RN-003). Devolve "" quando não há
// regime para a competência (ex.: antes do início do atendimento).
func RegimeAt(ctx context.Context, q *store.Queries, companyID string, c competence.Competence) (string, error) {
	r, err := q.RegimeAt(ctx, store.RegimeAtParams{CompanyID: companyID, Competence: c.FirstDay()})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return r, err
}

// Regimes devolve a linha do tempo de regimes da empresa.
func (s *Service) Regimes(ctx context.Context, companyID string) ([]RegimePeriod, error) {
	if _, err := s.q.GetCompany(ctx, companyID); err != nil {
		return nil, mapNotFound(err)
	}
	rows, err := s.q.ListRegimes(ctx, companyID)
	if err != nil {
		return nil, err
	}
	out := make([]RegimePeriod, len(rows))
	for i, r := range rows {
		p := RegimePeriod{
			ID: r.ID, TaxRegime: r.TaxRegime, RegimeGroup: RegimeGroup(r.TaxRegime),
			EffectiveFrom: competence.FromDate(r.EffectiveFromCompetence),
			CreatedBy:     UserRef{ID: r.CreatedBy, Name: r.CreatedByName}, CreatedAt: r.CreatedAt,
		}
		if r.EffectiveToCompetence != nil {
			to := competence.FromDate(*r.EffectiveToCompetence)
			p.EffectiveTo = &to
		}
		out[i] = p
	}
	return out, nil
}

// ChangeRegime registra um novo regime a partir de effective_from e encerra a vigência
// aberta em effective_from - 1 mês, na mesma transação [UC-04]. Competências já geradas
// não são alteradas: o regime usado fica no snapshot da competência [RN-004].
func (s *Service) ChangeRegime(ctx context.Context, companyID string, in RegimeChangeInput) ([]RegimePeriod, error) {
	if err := validate.Struct(in); err != nil {
		return nil, err
	}
	from := competence.MustParse(in.EffectiveFrom)
	userID := user.FromContext(ctx)
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.LockCompany(ctx, companyID); err != nil {
			return mapNotFound(err)
		}
		open, err := q.OpenRegimeForUpdate(ctx, companyID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			openFrom := competence.FromDate(open.EffectiveFromCompetence)
			if !from.After(openFrom) {
				return apperr.Unprocessable("INVALID_EFFECTIVE_FROM",
					fmt.Sprintf("A vigência do novo regime deve ser posterior a %s, início do regime atual.", openFrom.Display())).
					WithDetails(map[string]any{"current_effective_from": openFrom.String()})
			}
			if open.TaxRegime == in.TaxRegime {
				return apperr.Unprocessable("SAME_REGIME", "O regime informado já é o regime vigente.")
			}
			to := from.Prev().FirstDay()
			if err := q.CloseRegime(ctx, store.CloseRegimeParams{ID: open.ID, EffectiveToCompetence: &to}); err != nil {
				return err
			}
		}
		_, err = q.InsertRegime(ctx, store.InsertRegimeParams{
			CompanyID: companyID, TaxRegime: in.TaxRegime, EffectiveFromCompetence: from.FirstDay(), CreatedBy: userID,
		})
		if db.IsConstraint(err, db.ExclusionViolation, "company_tax_regime_no_overlap") {
			return apperr.Unprocessable("REGIME_OVERLAP", "A vigência informada se sobrepõe a outro regime da empresa.")
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.Regimes(ctx, companyID)
}

func (h *Handler) listRegimes(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	items, err := h.svc.Regimes(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) changeRegime(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in RegimeChangeInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	items, err := h.svc.ChangeRegime(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"items": items})
}
