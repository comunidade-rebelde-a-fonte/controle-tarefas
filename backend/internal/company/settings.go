package company

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// Aplicabilidade por empresa [PRD §9.2].
const (
	ApplicabilityDefault       = "DEFAULT"
	ApplicabilityApplicable    = "APPLICABLE"
	ApplicabilityNotApplicable = "NOT_APPLICABLE"
)

// Period é um intervalo de vigência de uma configuração.
type Period struct {
	EffectiveFrom competence.Competence  `json:"effective_from"`
	EffectiveTo   *competence.Competence `json:"effective_to"`
	CreatedBy     UserRef                `json:"created_by"`
	CreatedAt     time.Time              `json:"created_at"`
}

// ModulePeriod é um intervalo de ativação de módulo.
type ModulePeriod struct {
	Period
	Enabled bool `json:"enabled"`
}

// ModuleState é o estado de um módulo na competência de referência + histórico.
type ModuleState struct {
	Module   string         `json:"module"`
	Enabled  bool           `json:"enabled"`
	Timeline []ModulePeriod `json:"timeline"`
}

// ModulesResponse é a resposta de GET /companies/{id}/modules.
type ModulesResponse struct {
	ReferenceCompetence    competence.Competence `json:"reference_competence"`
	SuggestedEffectiveFrom competence.Competence `json:"suggested_effective_from"`
	Items                  []ModuleState         `json:"items"`
}

// OverridePeriod é um intervalo de override de aplicabilidade.
type OverridePeriod struct {
	Period
	Applicability string `json:"applicability"`
}

// ObligationTypeRef identifica um tipo do catálogo.
type ObligationTypeRef struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	PrimaryModule string `json:"primary_module"`
}

// OverrideState descreve a aplicabilidade de um tipo para a empresa na competência de referência.
type OverrideState struct {
	ObligationType ObligationTypeRef `json:"obligation_type"`
	// Configuração da empresa: DEFAULT, APPLICABLE ou NOT_APPLICABLE.
	Applicability string `json:"applicability"`
	// Regra padrão do catálogo para o regime vigente (nil = tipo não gerado no regime) [D3].
	DefaultApplicable *bool `json:"default_applicable"`
	// Resultado: override > regra padrão (nil = não gerado para o regime) [D6].
	EffectiveApplicable *bool            `json:"effective_applicable"`
	Timeline            []OverridePeriod `json:"timeline"`
}

// OverridesResponse é a resposta de GET /companies/{id}/obligation-overrides.
type OverridesResponse struct {
	ReferenceCompetence    competence.Competence `json:"reference_competence"`
	SuggestedEffectiveFrom competence.Competence `json:"suggested_effective_from"`
	TaxRegime              *string               `json:"tax_regime"`
	Items                  []OverrideState       `json:"items"`
}

// SetModuleInput é o payload de PUT /companies/{id}/modules/{module}.
type SetModuleInput struct {
	Enabled       *bool  `json:"enabled" validate:"required"`
	EffectiveFrom string `json:"effective_from" validate:"required,competence"`
}

// SetOverrideInput é o payload de PUT /companies/{id}/obligation-overrides/{obligationTypeId}.
type SetOverrideInput struct {
	Applicability string `json:"applicability" validate:"required,oneof=DEFAULT APPLICABLE NOT_APPLICABLE"`
	EffectiveFrom string `json:"effective_from" validate:"required,competence"`
}

// ModulesAt devolve os módulos habilitados da empresa na competência.
func ModulesAt(ctx context.Context, q *store.Queries, companyID string, c competence.Competence) ([]string, error) {
	return q.ModulesAt(ctx, store.ModulesAtParams{CompanyID: companyID, Competence: c.FirstDay()})
}

// OverridesAt devolve os overrides vigentes (tipo → aplicabilidade) na competência.
func OverridesAt(ctx context.Context, q *store.Queries, companyID string, c competence.Competence) (map[string]string, error) {
	rows, err := q.OverridesAt(ctx, store.OverridesAtParams{CompanyID: companyID, Competence: c.FirstDay()})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ObligationTypeID] = r.Applicability
	}
	return out, nil
}

// SuggestedEffectiveFrom é a próxima competência ainda não gerada: alterações a partir
// dela não esbarram em instâncias existentes [plan R6].
func (s *Service) SuggestedEffectiveFrom(ctx context.Context) (competence.Competence, error) {
	cur := s.currentCompetence()
	if s.latestGenerated == nil {
		return cur, nil
	}
	latest, err := s.latestGenerated(ctx)
	if err != nil {
		return competence.Competence{}, err
	}
	if latest != nil && !latest.Before(cur) {
		return latest.Next(), nil
	}
	return cur, nil
}

// SetLatestGenerated injeta a consulta da última competência gerada (motor de competências).
func (s *Service) SetLatestGenerated(fn func(ctx context.Context) (*competence.Competence, error)) {
	s.latestGenerated = fn
}

func toPeriod(from time.Time, to *time.Time, by, byName string, at time.Time) Period {
	p := Period{EffectiveFrom: competence.FromDate(from), CreatedBy: UserRef{ID: by, Name: byName}, CreatedAt: at}
	if to != nil {
		t := competence.FromDate(*to)
		p.EffectiveTo = &t
	}
	return p
}

func covers(p Period, c competence.Competence) bool {
	return !c.Before(p.EffectiveFrom) && (p.EffectiveTo == nil || !c.After(*p.EffectiveTo))
}

// Modules devolve o estado dos 5 módulos na competência de referência e o histórico.
func (s *Service) Modules(ctx context.Context, companyID string) (ModulesResponse, error) {
	c, err := s.q.GetCompany(ctx, companyID)
	if err != nil {
		return ModulesResponse{}, mapNotFound(err)
	}
	rows, err := s.q.ListModuleSettings(ctx, companyID)
	if err != nil {
		return ModulesResponse{}, err
	}
	suggested, err := s.SuggestedEffectiveFrom(ctx)
	if err != nil {
		return ModulesResponse{}, err
	}
	ref := s.refCompetence(civilOf(c.ServiceStartDate))
	byModule := map[string][]ModulePeriod{}
	for _, r := range rows {
		byModule[r.ModuleCode] = append(byModule[r.ModuleCode], ModulePeriod{
			Period:  toPeriod(r.EffectiveFromCompetence, r.EffectiveToCompetence, r.CreatedBy, r.CreatedByName, r.CreatedAt),
			Enabled: r.Enabled,
		})
	}
	out := ModulesResponse{ReferenceCompetence: ref, SuggestedEffectiveFrom: suggested}
	for _, m := range Modules {
		st := ModuleState{Module: m, Timeline: byModule[m]}
		if st.Timeline == nil {
			st.Timeline = []ModulePeriod{}
		}
		for _, p := range st.Timeline {
			if covers(p.Period, ref) {
				st.Enabled = p.Enabled
			}
		}
		out.Items = append(out.Items, st)
	}
	return out, nil
}

// SetModule liga/desliga um módulo a partir de effective_from. A vigência aberta é
// encerrada no mês anterior; competências já geradas não são reescritas [RN-004, R6].
func (s *Service) SetModule(ctx context.Context, companyID, module string, in SetModuleInput) (ModulesResponse, error) {
	if !IsModule(module) {
		return ModulesResponse{}, apperr.NotFound("MODULE_NOT_FOUND", "Módulo inexistente.")
	}
	if err := validate.Struct(in); err != nil {
		return ModulesResponse{}, err
	}
	from := competence.MustParse(in.EffectiveFrom)
	userID := user.FromContext(ctx)
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.LockCompany(ctx, companyID); err != nil {
			return mapNotFound(err)
		}
		open, err := q.OpenModuleSettingForUpdate(ctx, store.OpenModuleSettingForUpdateParams{CompanyID: companyID, ModuleCode: module})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Sem histórico: cria a vigência aberta.
		case err != nil:
			return err
		default:
			openFrom := competence.FromDate(open.EffectiveFromCompetence)
			switch {
			case from.Before(openFrom):
				return invalidFrom(openFrom)
			case open.Enabled == *in.Enabled:
				return nil // já está nesse estado
			case from.Equal(openFrom):
				return q.SetModuleSettingEnabled(ctx, store.SetModuleSettingEnabledParams{ID: open.ID, Enabled: *in.Enabled, CreatedBy: userID})
			}
			to := from.Prev().FirstDay()
			if err := q.CloseModuleSetting(ctx, store.CloseModuleSettingParams{ID: open.ID, EffectiveToCompetence: &to}); err != nil {
				return err
			}
		}
		_, err = q.InsertModuleSetting(ctx, store.InsertModuleSettingParams{
			CompanyID: companyID, ModuleCode: module, Enabled: *in.Enabled, EffectiveFromCompetence: from.FirstDay(), CreatedBy: userID,
		})
		if db.IsConstraint(err, db.ExclusionViolation, "") {
			return apperr.Unprocessable("SETTING_OVERLAP", "A vigência informada se sobrepõe a outra configuração do módulo.")
		}
		return err
	})
	if err != nil {
		return ModulesResponse{}, err
	}
	return s.Modules(ctx, companyID)
}

func invalidFrom(openFrom competence.Competence) error {
	return apperr.Unprocessable("INVALID_EFFECTIVE_FROM",
		fmt.Sprintf("A vigência deve ser igual ou posterior a %s, início da configuração atual.", openFrom.Display())).
		WithDetails(map[string]any{"current_effective_from": openFrom.String()})
}

// Overrides devolve a aplicabilidade de cada tipo ativo do catálogo para a empresa.
func (s *Service) Overrides(ctx context.Context, companyID string) (OverridesResponse, error) {
	c, err := s.q.GetCompany(ctx, companyID)
	if err != nil {
		return OverridesResponse{}, mapNotFound(err)
	}
	ref := s.refCompetence(civilOf(c.ServiceStartDate))
	regime, err := RegimeAt(ctx, s.q, companyID, ref)
	if err != nil {
		return OverridesResponse{}, err
	}
	var regimePtr *string
	if regime != "" {
		regimePtr = &regime
	}
	types, err := s.q.ListObligationTypesForCompany(ctx, regimePtr)
	if err != nil {
		return OverridesResponse{}, err
	}
	rows, err := s.q.ListOverrides(ctx, companyID)
	if err != nil {
		return OverridesResponse{}, err
	}
	suggested, err := s.SuggestedEffectiveFrom(ctx)
	if err != nil {
		return OverridesResponse{}, err
	}
	byType := map[string][]OverridePeriod{}
	for _, r := range rows {
		byType[r.ObligationTypeID] = append(byType[r.ObligationTypeID], OverridePeriod{
			Period:        toPeriod(r.EffectiveFromCompetence, r.EffectiveToCompetence, r.CreatedBy, r.CreatedByName, r.CreatedAt),
			Applicability: r.Applicability,
		})
	}
	out := OverridesResponse{ReferenceCompetence: ref, SuggestedEffectiveFrom: suggested, TaxRegime: regimePtr, Items: []OverrideState{}}
	for _, t := range types {
		st := OverrideState{
			ObligationType:    ObligationTypeRef{ID: t.ID, Code: t.Code, Name: t.Name, PrimaryModule: t.PrimaryModule},
			Applicability:     ApplicabilityDefault,
			DefaultApplicable: t.DefaultApplicable,
			Timeline:          byType[t.ID],
		}
		if st.Timeline == nil {
			st.Timeline = []OverridePeriod{}
		}
		for _, p := range st.Timeline {
			if covers(p.Period, ref) {
				st.Applicability = p.Applicability
			}
		}
		st.EffectiveApplicable = ResolveApplicable(t.DefaultApplicable, st.Applicability)
		out.Items = append(out.Items, st)
	}
	return out, nil
}

// ResolveApplicable aplica override > regra padrão [plan D2, D3, D6]. Devolve nil quando o
// tipo não tem regra para o regime (não é gerado), independentemente do override.
func ResolveApplicable(defaultApplicable *bool, applicability string) *bool {
	if defaultApplicable == nil {
		return nil
	}
	v := *defaultApplicable
	switch applicability {
	case ApplicabilityApplicable:
		v = true
	case ApplicabilityNotApplicable:
		v = false
	}
	return &v
}

// SetOverride configura a aplicabilidade de um tipo a partir de effective_from.
// A dispensa persiste nas competências seguintes até nova alteração [RN-015].
func (s *Service) SetOverride(ctx context.Context, companyID, typeID string, in SetOverrideInput) (OverridesResponse, error) {
	if err := validate.Struct(in); err != nil {
		return OverridesResponse{}, err
	}
	exists, err := s.q.ObligationTypeExists(ctx, typeID)
	if err != nil || !exists {
		return OverridesResponse{}, apperr.NotFound("OBLIGATION_TYPE_NOT_FOUND", "Tipo de obrigação não encontrado.")
	}
	from := competence.MustParse(in.EffectiveFrom)
	userID := user.FromContext(ctx)
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.LockCompany(ctx, companyID); err != nil {
			return mapNotFound(err)
		}
		open, err := q.OpenOverrideForUpdate(ctx, store.OpenOverrideForUpdateParams{CompanyID: companyID, ObligationTypeID: typeID})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if in.Applicability == ApplicabilityDefault {
				return nil // já segue a regra padrão
			}
		case err != nil:
			return err
		default:
			openFrom := competence.FromDate(open.EffectiveFromCompetence)
			switch {
			case from.Before(openFrom):
				return invalidFrom(openFrom)
			case open.Applicability == in.Applicability:
				return nil
			case from.Equal(openFrom):
				return q.SetOverrideApplicability(ctx, store.SetOverrideApplicabilityParams{ID: open.ID, Applicability: in.Applicability, CreatedBy: userID})
			}
			to := from.Prev().FirstDay()
			if err := q.CloseOverride(ctx, store.CloseOverrideParams{ID: open.ID, EffectiveToCompetence: &to}); err != nil {
				return err
			}
		}
		_, err = q.InsertOverride(ctx, store.InsertOverrideParams{
			CompanyID: companyID, ObligationTypeID: typeID, Applicability: in.Applicability, EffectiveFromCompetence: from.FirstDay(), CreatedBy: userID,
		})
		return err
	})
	if err != nil {
		return OverridesResponse{}, err
	}
	return s.Overrides(ctx, companyID)
}

func (h *Handler) getModules(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	resp, err := h.svc.Modules(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) setModule(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in SetModuleInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	resp, err := h.svc.SetModule(r.Context(), id, chi.URLParam(r, "module"), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getOverrides(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	resp, err := h.svc.Overrides(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) setOverride(w http.ResponseWriter, r *http.Request) {
	id, err := companyID(r)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	typeID, err := httpx.PathUUID(r, "obligationTypeId", "OBLIGATION_TYPE_NOT_FOUND", "Tipo de obrigação não encontrado.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in SetOverrideInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	resp, err := h.svc.SetOverride(r.Context(), id, typeID, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
