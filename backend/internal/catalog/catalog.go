// Package catalog mantém o catálogo configurável de obrigações: tipos, conjuntos de
// status, contextos de exibição, regras por regime e dependências [PRD §20].
// Novos tipos entram por cadastro, sem migration [PRD §15.2].
package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
)

// Módulos em que um tipo pode ser exibido (DCTFWEB é contexto, não módulo) [§38.9].
var displayModules = []string{"FISCAL", "DECLARATIONS", "ACCOUNTING", "PAYROLL", "DCTFWEB"}

var codeRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// DisplayContext é uma tela em que o tipo aparece.
type DisplayContext struct {
	ModuleCode string `json:"module_code" validate:"required,oneof=FISCAL DECLARATIONS ACCOUNTING PAYROLL DCTFWEB"`
	SortOrder  int32  `json:"sort_order"`
}

// RegimeRule é a aplicabilidade padrão do tipo para um regime (D2/D3).
type RegimeRule struct {
	TaxRegime         string `json:"tax_regime" validate:"required,oneof=SIMPLES_NACIONAL LUCRO_PRESUMIDO LUCRO_REAL LUCRO_ARBITRADO IMUNE_ISENTA"`
	DefaultApplicable *bool  `json:"default_applicable"`
	CountsInProgress  *bool  `json:"counts_in_progress"`
}

// Dependency é um pré-requisito do tipo (HARD bloqueia, SOFT alerta).
type Dependency struct {
	PrerequisiteObligationTypeID string `json:"prerequisite_obligation_type_id" validate:"required,uuid"`
	PrerequisiteCode             string `json:"prerequisite_code,omitempty"`
	DependencyMode               string `json:"dependency_mode" validate:"required,oneof=HARD SOFT"`
}

// ObligationType é a representação do tipo na API.
type ObligationType struct {
	ID                     string           `json:"id"`
	Code                   string           `json:"code"`
	Name                   string           `json:"name"`
	PrimaryModule          string           `json:"primary_module"`
	StatusSetCode          string           `json:"status_set_code"`
	Periodicity            string           `json:"periodicity"`
	Active                 bool             `json:"active"`
	SortOrder              int32            `json:"sort_order"`
	CountsInProgress       bool             `json:"counts_in_progress"`
	DueDateRuleID          *string          `json:"due_date_rule_id"`
	AllowsTransmissionDate bool             `json:"allows_transmission_date"`
	AllowsProtocol         bool             `json:"allows_protocol"`
	InitialStatusCode      string           `json:"initial_status_code"`
	StatusWhenNoPayroll    *string          `json:"status_when_no_payroll"`
	IsClosingStage         bool             `json:"is_closing_stage"`
	Version                int32            `json:"version"`
	DisplayContexts        []DisplayContext `json:"display_contexts"`
	RegimeRules            []RegimeRule     `json:"regime_rules"`
	Dependencies           []Dependency     `json:"dependencies"`
}

// CreateInput é o payload de POST /obligation-types.
type CreateInput struct {
	Code                   string            `json:"code" validate:"required,max=60"`
	Name                   string            `json:"name" validate:"notblank,max=120"`
	PrimaryModule          string            `json:"primary_module" validate:"required,oneof=FISCAL DECLARATIONS ACCOUNTING PAYROLL"`
	StatusSetCode          string            `json:"status_set_code" validate:"required"`
	SortOrder              int32             `json:"sort_order"`
	Active                 *bool             `json:"active"`
	CountsInProgress       *bool             `json:"counts_in_progress"`
	DueDateRuleID          *string           `json:"due_date_rule_id" validate:"omitempty,uuid"`
	AllowsTransmissionDate bool              `json:"allows_transmission_date"`
	AllowsProtocol         bool              `json:"allows_protocol"`
	InitialStatusCode      string            `json:"initial_status_code"`
	StatusWhenNoPayroll    *string           `json:"status_when_no_payroll" validate:"omitempty,oneof=NO_MOVEMENT NOT_APPLICABLE"`
	DisplayContexts        *[]DisplayContext `json:"display_contexts" validate:"omitempty,dive"`
	RegimeRules            []RegimeRule      `json:"regime_rules" validate:"dive"`
	Dependencies           []Dependency      `json:"dependencies" validate:"dive"`
}

// UpdateInput é o payload de PATCH /obligation-types/{id}. Listas enviadas substituem as atuais.
type UpdateInput struct {
	Version                int32             `json:"version" validate:"required,min=1"`
	Name                   *string           `json:"name" validate:"omitempty,notblank,max=120"`
	Active                 *bool             `json:"active"`
	SortOrder              *int32            `json:"sort_order"`
	CountsInProgress       *bool             `json:"counts_in_progress"`
	DueDateRuleID          *string           `json:"due_date_rule_id" validate:"omitempty,uuid"`
	ClearDueDateRule       bool              `json:"clear_due_date_rule"`
	AllowsTransmissionDate *bool             `json:"allows_transmission_date"`
	AllowsProtocol         *bool             `json:"allows_protocol"`
	InitialStatusCode      *string           `json:"initial_status_code"`
	StatusWhenNoPayroll    *string           `json:"status_when_no_payroll" validate:"omitempty,oneof=NO_MOVEMENT NOT_APPLICABLE NONE"`
	DisplayContexts        *[]DisplayContext `json:"display_contexts" validate:"omitempty,dive"`
	RegimeRules            *[]RegimeRule     `json:"regime_rules" validate:"omitempty,dive"`
	Dependencies           *[]Dependency     `json:"dependencies" validate:"omitempty,dive"`
}

// Service contém as regras do catálogo.
type Service struct {
	pool *pgxpool.Pool
	q    *store.Queries
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, q: store.New(pool)} }

// List devolve todos os tipos com contextos, regras e dependências ativos.
func (s *Service) List(ctx context.Context) ([]ObligationType, error) {
	rows, err := s.q.ListObligationTypes(ctx)
	if err != nil {
		return nil, err
	}
	extras, err := s.loadExtras(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ObligationType, len(rows))
	for i, r := range rows {
		out[i] = extras.fill(fromRow(store.GetObligationTypeRow(r)))
	}
	return out, nil
}

// Get devolve um tipo.
func (s *Service) Get(ctx context.Context, id string) (ObligationType, error) {
	r, err := s.q.GetObligationType(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ObligationType{}, notFound()
		}
		return ObligationType{}, err
	}
	extras, err := s.loadExtras(ctx)
	if err != nil {
		return ObligationType{}, err
	}
	return extras.fill(fromRow(r)), nil
}

func notFound() error {
	return apperr.NotFound("OBLIGATION_TYPE_NOT_FOUND", "Tipo de obrigação não encontrado.")
}

func fromRow(r store.GetObligationTypeRow) ObligationType {
	return ObligationType{
		ID: r.ID, Code: r.Code, Name: r.Name, PrimaryModule: r.PrimaryModule, StatusSetCode: r.StatusSetCode,
		Periodicity: r.Periodicity, Active: r.Active, SortOrder: r.SortOrder, CountsInProgress: r.CountsInProgress,
		DueDateRuleID: r.DueDateRuleID, AllowsTransmissionDate: r.AllowsTransmissionDate, AllowsProtocol: r.AllowsProtocol,
		InitialStatusCode: r.InitialStatusCode, StatusWhenNoPayroll: r.StatusWhenNoPayroll, IsClosingStage: r.IsClosingStage,
		Version: r.Version,
	}
}

type extras struct {
	contexts map[string][]DisplayContext
	rules    map[string][]RegimeRule
	deps     map[string][]Dependency
}

func (e extras) fill(t ObligationType) ObligationType {
	t.DisplayContexts = orEmpty(e.contexts[t.ID])
	t.RegimeRules = orEmpty(e.rules[t.ID])
	t.Dependencies = orEmpty(e.deps[t.ID])
	return t
}

func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func (s *Service) loadExtras(ctx context.Context) (extras, error) {
	e := extras{contexts: map[string][]DisplayContext{}, rules: map[string][]RegimeRule{}, deps: map[string][]Dependency{}}
	ctxs, err := s.q.ListDisplayContexts(ctx)
	if err != nil {
		return e, err
	}
	for _, c := range ctxs {
		e.contexts[c.ObligationTypeID] = append(e.contexts[c.ObligationTypeID], DisplayContext{ModuleCode: c.ModuleCode, SortOrder: c.SortOrder})
	}
	rules, err := s.q.ListRegimeRules(ctx)
	if err != nil {
		return e, err
	}
	for _, r := range rules {
		def, counts := r.DefaultApplicable, r.CountsInProgress
		e.rules[r.ObligationTypeID] = append(e.rules[r.ObligationTypeID], RegimeRule{TaxRegime: r.TaxRegime, DefaultApplicable: &def, CountsInProgress: &counts})
	}
	deps, err := s.q.ListDependencies(ctx)
	if err != nil {
		return e, err
	}
	for _, d := range deps {
		e.deps[d.ObligationTypeID] = append(e.deps[d.ObligationTypeID], Dependency{
			PrerequisiteObligationTypeID: d.PrerequisiteObligationTypeID, PrerequisiteCode: d.PrerequisiteCode, DependencyMode: d.DependencyMode,
		})
	}
	return e, nil
}

func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// checkStatus garante que o status existe no conjunto do tipo.
func checkStatus(ctx context.Context, q *store.Queries, setID, field, code string) error {
	ok, err := q.StatusExistsInSet(ctx, store.StatusExistsInSetParams{StatusSetID: setID, Code: code})
	if err != nil {
		return err
	}
	if !ok {
		return validate.Field(field, fmt.Sprintf("O status %s não existe no conjunto do tipo.", code))
	}
	return nil
}

func validateLists(ctxs *[]DisplayContext, rules *[]RegimeRule, deps *[]Dependency, selfID string) error {
	if ctxs != nil {
		seen := map[string]bool{}
		for _, c := range *ctxs {
			if seen[c.ModuleCode] || !slices.Contains(displayModules, c.ModuleCode) {
				return validate.Field("display_contexts", "Contextos de exibição inválidos ou repetidos.")
			}
			seen[c.ModuleCode] = true
		}
	}
	if rules != nil {
		seen := map[string]bool{}
		for _, r := range *rules {
			if seen[r.TaxRegime] {
				return validate.Field("regime_rules", "Regime repetido nas regras.")
			}
			seen[r.TaxRegime] = true
		}
	}
	if deps != nil {
		seen := map[string]bool{}
		for _, d := range *deps {
			if d.PrerequisiteObligationTypeID == selfID || seen[d.PrerequisiteObligationTypeID] {
				return validate.Field("dependencies", "Dependências inválidas (o tipo não pode depender de si mesmo nem repetir pré-requisitos).")
			}
			seen[d.PrerequisiteObligationTypeID] = true
		}
	}
	return nil
}

func writeLists(ctx context.Context, q *store.Queries, typeID string, ctxs *[]DisplayContext, rules *[]RegimeRule, deps *[]Dependency) error {
	if ctxs != nil {
		if err := q.DeactivateDisplayContexts(ctx, typeID); err != nil {
			return err
		}
		for _, c := range *ctxs {
			if err := q.UpsertDisplayContext(ctx, store.UpsertDisplayContextParams{ObligationTypeID: typeID, ModuleCode: c.ModuleCode, SortOrder: c.SortOrder}); err != nil {
				return err
			}
		}
	}
	if rules != nil {
		if err := q.DeactivateRegimeRules(ctx, typeID); err != nil {
			return err
		}
		for _, r := range *rules {
			if err := q.UpsertRegimeRule(ctx, store.UpsertRegimeRuleParams{
				ObligationTypeID: typeID, TaxRegime: r.TaxRegime,
				DefaultApplicable: boolOr(r.DefaultApplicable, true), CountsInProgress: boolOr(r.CountsInProgress, true),
			}); err != nil {
				return err
			}
		}
	}
	if deps != nil {
		if err := q.DeactivateDependencies(ctx, typeID); err != nil {
			return err
		}
		for _, d := range *deps {
			err := q.UpsertDependency(ctx, store.UpsertDependencyParams{
				ObligationTypeID: typeID, PrerequisiteObligationTypeID: d.PrerequisiteObligationTypeID, DependencyMode: d.DependencyMode,
			})
			if db.IsConstraint(err, db.ForeignKeyViolation, "") {
				return validate.Field("dependencies", "Pré-requisito inexistente.")
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Create cadastra um novo tipo (ex.: nova declaração) sem migration [§15.2].
func (s *Service) Create(ctx context.Context, in CreateInput) (ObligationType, error) {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if err := validate.Struct(in); err != nil {
		return ObligationType{}, err
	}
	if !codeRe.MatchString(in.Code) {
		return ObligationType{}, validate.Field("code", "Use letras maiúsculas, dígitos e _ (ex.: NOVA_DECLARACAO).")
	}
	set, err := s.q.GetStatusSetByCode(ctx, in.StatusSetCode)
	if err != nil {
		return ObligationType{}, validate.Field("status_set_code", "Conjunto de status inexistente.")
	}
	if in.InitialStatusCode == "" {
		in.InitialStatusCode = "NOT_STARTED"
	}
	if err := checkStatus(ctx, s.q, set.ID, "initial_status_code", in.InitialStatusCode); err != nil {
		return ObligationType{}, err
	}
	if in.StatusWhenNoPayroll != nil && *in.StatusWhenNoPayroll == "NO_MOVEMENT" {
		if err := checkStatus(ctx, s.q, set.ID, "status_when_no_payroll", "NO_MOVEMENT"); err != nil {
			return ObligationType{}, err
		}
	}
	if in.DueDateRuleID != nil {
		if ok, err := s.q.DueDateRuleExists(ctx, *in.DueDateRuleID); err != nil || !ok {
			return ObligationType{}, validate.Field("due_date_rule_id", "Regra de vencimento inexistente.")
		}
	}
	ctxs := in.DisplayContexts
	if ctxs == nil {
		ctxs = &[]DisplayContext{{ModuleCode: in.PrimaryModule, SortOrder: in.SortOrder}}
	}
	if err := validateLists(ctxs, &in.RegimeRules, &in.Dependencies, ""); err != nil {
		return ObligationType{}, err
	}
	var id string
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var err error
		id, err = q.InsertObligationType(ctx, store.InsertObligationTypeParams{
			Code: in.Code, Name: strings.TrimSpace(in.Name), PrimaryModule: in.PrimaryModule, StatusSetID: set.ID,
			Active: boolOr(in.Active, true), SortOrder: in.SortOrder, CountsInProgress: boolOr(in.CountsInProgress, true),
			DueDateRuleID: in.DueDateRuleID, AllowsTransmissionDate: in.AllowsTransmissionDate, AllowsProtocol: in.AllowsProtocol,
			InitialStatusCode: in.InitialStatusCode, StatusWhenNoPayroll: in.StatusWhenNoPayroll,
		})
		if db.IsConstraint(err, db.UniqueViolation, "obligation_types_code_uq") {
			return apperr.Conflict("CODE_CONFLICT", "Já existe um tipo de obrigação com este código.")
		}
		if err != nil {
			return err
		}
		return writeLists(ctx, q, id, ctxs, &in.RegimeRules, &in.Dependencies)
	})
	if err != nil {
		return ObligationType{}, err
	}
	return s.Get(ctx, id)
}

// Update altera um tipo com controle de versão. Alterações valem para as próximas
// gerações de competência; instâncias existentes não são reescritas [RN-004].
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (ObligationType, error) {
	if err := validate.Struct(in); err != nil {
		return ObligationType{}, err
	}
	cur, err := s.q.GetObligationType(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ObligationType{}, notFound()
		}
		return ObligationType{}, err
	}
	if cur.Version != in.Version {
		return ObligationType{}, apperr.VersionConflict()
	}
	p := store.UpdateObligationTypeParams{
		ID: id, Version: in.Version, Name: cur.Name, Active: cur.Active, SortOrder: cur.SortOrder,
		CountsInProgress: cur.CountsInProgress, DueDateRuleID: cur.DueDateRuleID, AllowsTransmissionDate: cur.AllowsTransmissionDate,
		AllowsProtocol: cur.AllowsProtocol, InitialStatusCode: cur.InitialStatusCode, StatusWhenNoPayroll: cur.StatusWhenNoPayroll,
	}
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.Active != nil {
		p.Active = *in.Active
	}
	if in.SortOrder != nil {
		p.SortOrder = *in.SortOrder
	}
	if in.CountsInProgress != nil {
		p.CountsInProgress = *in.CountsInProgress
	}
	if in.ClearDueDateRule {
		p.DueDateRuleID = nil
	} else if in.DueDateRuleID != nil {
		if ok, err := s.q.DueDateRuleExists(ctx, *in.DueDateRuleID); err != nil || !ok {
			return ObligationType{}, validate.Field("due_date_rule_id", "Regra de vencimento inexistente.")
		}
		p.DueDateRuleID = in.DueDateRuleID
	}
	if in.AllowsTransmissionDate != nil {
		p.AllowsTransmissionDate = *in.AllowsTransmissionDate
	}
	if in.AllowsProtocol != nil {
		p.AllowsProtocol = *in.AllowsProtocol
	}
	if in.InitialStatusCode != nil {
		if err := checkStatus(ctx, s.q, cur.StatusSetID, "initial_status_code", *in.InitialStatusCode); err != nil {
			return ObligationType{}, err
		}
		p.InitialStatusCode = *in.InitialStatusCode
	}
	if in.StatusWhenNoPayroll != nil {
		switch *in.StatusWhenNoPayroll {
		case "NONE":
			p.StatusWhenNoPayroll = nil
		case "NO_MOVEMENT":
			if err := checkStatus(ctx, s.q, cur.StatusSetID, "status_when_no_payroll", "NO_MOVEMENT"); err != nil {
				return ObligationType{}, err
			}
			p.StatusWhenNoPayroll = in.StatusWhenNoPayroll
		default:
			p.StatusWhenNoPayroll = in.StatusWhenNoPayroll
		}
	}
	if err := validateLists(in.DisplayContexts, in.RegimeRules, in.Dependencies, id); err != nil {
		return ObligationType{}, err
	}
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if _, err := q.UpdateObligationType(ctx, p); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperr.VersionConflict()
			}
			return err
		}
		return writeLists(ctx, q, id, in.DisplayContexts, in.RegimeRules, in.Dependencies)
	})
	if err != nil {
		return ObligationType{}, err
	}
	return s.Get(ctx, id)
}

// StatusDefinition descreve um status e seus metadados [PRD §11].
type StatusDefinition struct {
	Code                        string `json:"code"`
	Label                       string `json:"label"`
	SortOrder                   int32  `json:"sort_order"`
	CountsAsStarted             bool   `json:"counts_as_started"`
	CountsAsOperationalProgress bool   `json:"counts_as_operational_progress"`
	IsFinal                     bool   `json:"is_final"`
	IsBlocking                  bool   `json:"is_blocking"`
	ExcludedFromDenominator     bool   `json:"excluded_from_denominator"`
}

// StatusSet é um conjunto de status.
type StatusSet struct {
	ID       string             `json:"id"`
	Code     string             `json:"code"`
	Name     string             `json:"name"`
	Statuses []StatusDefinition `json:"statuses"`
}

// StatusSets devolve os conjuntos de status com suas definições.
func (s *Service) StatusSets(ctx context.Context) ([]StatusSet, error) {
	sets, err := s.q.ListStatusSets(ctx)
	if err != nil {
		return nil, err
	}
	defs, err := s.q.ListStatusDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]StatusSet, len(sets))
	for i, set := range sets {
		out[i] = StatusSet{ID: set.ID, Code: set.Code, Name: set.Name, Statuses: []StatusDefinition{}}
		for _, d := range defs {
			if d.StatusSetID == set.ID {
				out[i].Statuses = append(out[i].Statuses, StatusDefinition{
					Code: d.Code, Label: d.Label, SortOrder: d.SortOrder, CountsAsStarted: d.CountsAsStarted,
					CountsAsOperationalProgress: d.CountsAsOperationalProgress, IsFinal: d.IsFinal, IsBlocking: d.IsBlocking,
					ExcludedFromDenominator: d.ExcludedFromDenominator,
				})
			}
		}
	}
	return out, nil
}

// Handler expõe /obligation-types e /status-sets.
type Handler struct{ svc *Service }

// NewHandler cria o handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/obligation-types", h.list)
	r.Post("/obligation-types", h.create)
	r.Get("/obligation-types/{id}", h.get)
	r.Patch("/obligation-types/{id}", h.update)
	r.Get("/status-sets", h.statusSets)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "OBLIGATION_TYPE_NOT_FOUND", "Tipo de obrigação não encontrado.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	t, err := h.svc.Create(r.Context(), in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, t)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "OBLIGATION_TYPE_NOT_FOUND", "Tipo de obrigação não encontrado.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in UpdateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	t, err := h.svc.Update(r.Context(), id, in)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func (h *Handler) statusSets(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.StatusSets(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
