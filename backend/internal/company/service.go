package company

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	"controle/internal/platform/cnpj"
	"controle/internal/platform/competence"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/opt"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// Service contém as regras do cadastro de empresas.
type Service struct {
	pool  *pgxpool.Pool
	q     *store.Queries
	clock clock.Clock
	// latestGenerated devolve a última competência gerada (injetada pelo motor, M2).
	latestGenerated func(ctx context.Context) (*competence.Competence, error)
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, q: store.New(pool), clock: clk}
}

// CreateInput é o payload de POST /companies. O regime inicial é obrigatório e vale a
// partir da competência de service_start_date [plan M1].
type CreateInput struct {
	InternalCode                  string      `json:"internal_code" validate:"notblank,max=30"`
	LegalName                     string      `json:"legal_name" validate:"notblank,max=200"`
	TradeName                     *string     `json:"trade_name" validate:"omitempty,max=200"`
	Cnpj                          string      `json:"cnpj" validate:"notblank,max=30"`
	City                          *string     `json:"city" validate:"omitempty,max=100"`
	State                         *string     `json:"state" validate:"omitempty,uf"`
	StateRegistration             *string     `json:"state_registration" validate:"omitempty,max=30"`
	MunicipalRegistration         *string     `json:"municipal_registration" validate:"omitempty,max=30"`
	ServiceStartDate              civil.Date  `json:"service_start_date" validate:"required"`
	ServiceEndDate                *civil.Date `json:"service_end_date"`
	PayrollType                   string      `json:"payroll_type" validate:"required,oneof=NO_PAYROLL PRO_LABORE PAYROLL"`
	FactorR                       *bool       `json:"factor_r"`
	FiscalResponsibleUserID       *string     `json:"fiscal_responsible_user_id" validate:"omitempty,uuid"`
	AccountingResponsibleUserID   *string     `json:"accounting_responsible_user_id" validate:"omitempty,uuid"`
	PayrollResponsibleUserID      *string     `json:"payroll_responsible_user_id" validate:"omitempty,uuid"`
	DeclarationsResponsibleUserID *string     `json:"declarations_responsible_user_id" validate:"omitempty,uuid"`
	Notes                         *string     `json:"notes" validate:"omitempty,max=4000"`
	TaxRegime                     string      `json:"tax_regime" validate:"required,oneof=SIMPLES_NACIONAL LUCRO_PRESUMIDO LUCRO_REAL LUCRO_ARBITRADO IMUNE_ISENTA"`
	// Módulos ativos desde o início do atendimento. Ausente = todos os módulos.
	Modules *[]string `json:"modules" validate:"omitempty,dive,oneof=FISCAL DECLARATIONS ACCOUNTING PAYROLL CREDENTIALS"`
}

// UpdateInput é o payload de PATCH /companies/{id}: só os campos enviados mudam.
type UpdateInput struct {
	Version                       int32                    `json:"version" validate:"required,min=1"`
	InternalCode                  *string                  `json:"internal_code" validate:"omitempty,notblank,max=30"`
	LegalName                     *string                  `json:"legal_name" validate:"omitempty,notblank,max=200"`
	TradeName                     opt.Optional[string]     `json:"trade_name"`
	Cnpj                          *string                  `json:"cnpj" validate:"omitempty,notblank,max=30"`
	City                          opt.Optional[string]     `json:"city"`
	State                         opt.Optional[string]     `json:"state"`
	StateRegistration             opt.Optional[string]     `json:"state_registration"`
	MunicipalRegistration         opt.Optional[string]     `json:"municipal_registration"`
	ServiceStartDate              *civil.Date              `json:"service_start_date"`
	ServiceEndDate                opt.Optional[civil.Date] `json:"service_end_date"`
	Active                        *bool                    `json:"active"`
	PayrollType                   *string                  `json:"payroll_type" validate:"omitempty,oneof=NO_PAYROLL PRO_LABORE PAYROLL"`
	FactorR                       opt.Optional[bool]       `json:"factor_r"`
	FiscalResponsibleUserID       opt.Optional[string]     `json:"fiscal_responsible_user_id"`
	AccountingResponsibleUserID   opt.Optional[string]     `json:"accounting_responsible_user_id"`
	PayrollResponsibleUserID      opt.Optional[string]     `json:"payroll_responsible_user_id"`
	DeclarationsResponsibleUserID opt.Optional[string]     `json:"declarations_responsible_user_id"`
	Notes                         opt.Optional[string]     `json:"notes"`
}

// ListFilter são os filtros de GET /companies [PRD §41.1].
type ListFilter struct {
	Q         string
	Cnpj      string
	Code      string
	TaxRegime string
	Active    *bool
	Module    string
}

// clean remove espaços e converte vazio em nil.
func clean(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func cleanOpt(o opt.Optional[string]) *string { return clean(o.Value) }

func upperPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.ToUpper(*s)
	return &v
}

// normalizeCnpj sanitiza e valida os dígitos verificadores [RN-014].
func normalizeCnpj(raw string) (string, error) {
	d := cnpj.Sanitize(raw)
	if !cnpj.Valid(d) {
		return "", validate.Field("cnpj", "CNPJ inválido: confira os 14 dígitos e os dígitos verificadores.")
	}
	return d, nil
}

func (s *Service) currentCompetence() competence.Competence { return competence.Current(s.clock) }

// checkResponsibles garante que os responsáveis informados existem e estão ativos.
func (s *Service) checkResponsibles(ctx context.Context, ids map[string]*string) error {
	var list []string
	for _, id := range ids {
		if id != nil && !slices.Contains(list, *id) {
			list = append(list, *id)
		}
	}
	if len(list) == 0 {
		return nil
	}
	n, err := s.q.CountActiveUsers(ctx, list)
	if err != nil {
		return err
	}
	if int(n) != len(list) {
		fields := map[string]any{}
		for field, id := range ids {
			if id == nil {
				continue
			}
			if c, err := s.q.CountActiveUsers(ctx, []string{*id}); err == nil && c == 0 {
				fields[field] = "Responsável não encontrado ou inativo."
			}
		}
		return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
			WithDetails(map[string]any{"fields": fields})
	}
	return nil
}

// conflictFor devolve o 409 que indica a empresa que já usa o CNPJ ou o código [CA-002].
func (s *Service) conflictFor(ctx context.Context, cnpjDigits, code, exceptID string) error {
	if row, err := s.q.FindCompanyByCnpj(ctx, cnpjDigits); err == nil && row.ID != exceptID {
		return apperr.Conflict("CNPJ_CONFLICT",
			fmt.Sprintf("Já existe uma empresa com este CNPJ: %s — %s.", row.InternalCode, row.LegalName)).
			WithDetails(map[string]any{"field": "cnpj", "conflicting_company": ConflictingCompany(row)})
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if row, err := s.q.FindCompanyByCode(ctx, code); err == nil && row.ID != exceptID {
		return apperr.Conflict("CODE_CONFLICT",
			fmt.Sprintf("Já existe uma empresa com este código: %s — %s.", row.InternalCode, row.LegalName)).
			WithDetails(map[string]any{"field": "internal_code", "conflicting_company": ConflictingCompany(row)})
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

func (s *Service) mapWriteErr(ctx context.Context, err error, cnpjDigits, code, exceptID string) error {
	if db.IsConstraint(err, db.UniqueViolation, "companies_cnpj_uq") || db.IsConstraint(err, db.UniqueViolation, "companies_internal_code_uq") {
		if c := s.conflictFor(ctx, cnpjDigits, code, exceptID); c != nil {
			return c
		}
	}
	return err
}

func validatePeriod(start civil.Date, end *civil.Date) error {
	if end != nil && end.Before(start.Time) {
		return validate.Field("service_end_date", "O fim do atendimento deve ser igual ou posterior ao início.")
	}
	return nil
}

// Create cadastra a empresa com regime inicial e módulos, numa transação [CA-001].
func (s *Service) Create(ctx context.Context, in CreateInput) (Company, error) {
	var id string
	var p preparedCreate
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		p, err = s.prepareCreate(ctx, in)
		if err != nil {
			return err
		}
		id, err = s.insertPrepared(ctx, s.q.WithTx(tx), p)
		return err
	})
	if err != nil {
		return Company{}, s.mapWriteErr(ctx, err, p.digits, p.code, "")
	}
	return s.Get(ctx, id)
}

// CreateTx cadastra a empresa dentro de uma transação existente (importação em lote, que
// grava todas as linhas ou nenhuma). Aplica as mesmas validações de Create.
func (s *Service) CreateTx(ctx context.Context, tx pgx.Tx, in CreateInput) (string, error) {
	p, err := s.prepareCreate(ctx, in)
	if err != nil {
		return "", err
	}
	id, err := s.insertPrepared(ctx, s.q.WithTx(tx), p)
	if err != nil {
		return "", s.mapWriteErr(ctx, err, p.digits, p.code, "")
	}
	return id, nil
}

// preparedCreate é a entrada de Create já validada e normalizada.
type preparedCreate struct {
	in      CreateInput
	digits  string
	code    string
	resp    map[string]*string
	modules []string
}

func (s *Service) prepareCreate(ctx context.Context, in CreateInput) (preparedCreate, error) {
	in.State = upperPtr(clean(in.State))
	if err := validate.Struct(in); err != nil {
		return preparedCreate{}, err
	}
	digits, err := normalizeCnpj(in.Cnpj)
	if err != nil {
		return preparedCreate{}, err
	}
	code := strings.TrimSpace(in.InternalCode)
	if err := validatePeriod(in.ServiceStartDate, in.ServiceEndDate); err != nil {
		return preparedCreate{}, err
	}
	resp := map[string]*string{
		"fiscal_responsible_user_id":       clean(in.FiscalResponsibleUserID),
		"accounting_responsible_user_id":   clean(in.AccountingResponsibleUserID),
		"payroll_responsible_user_id":      clean(in.PayrollResponsibleUserID),
		"declarations_responsible_user_id": clean(in.DeclarationsResponsibleUserID),
	}
	if err := s.checkResponsibles(ctx, resp); err != nil {
		return preparedCreate{}, err
	}
	if err := s.conflictFor(ctx, digits, code, ""); err != nil {
		return preparedCreate{}, err
	}
	modules := Modules
	if in.Modules != nil {
		modules = *in.Modules
	}
	return preparedCreate{in: in, digits: digits, code: code, resp: resp, modules: modules}, nil
}

// insertPrepared grava empresa, regime inicial e módulos com as queries da transação.
func (s *Service) insertPrepared(ctx context.Context, q *store.Queries, p preparedCreate) (string, error) {
	in := p.in
	userID := user.FromContext(ctx)
	from := competence.FromDate(in.ServiceStartDate.Time).FirstDay()
	c, err := q.InsertCompany(ctx, store.InsertCompanyParams{
		InternalCode:                  p.code,
		LegalName:                     strings.TrimSpace(in.LegalName),
		TradeName:                     clean(in.TradeName),
		Cnpj:                          p.digits,
		City:                          clean(in.City),
		State:                         in.State,
		StateRegistration:             clean(in.StateRegistration),
		MunicipalRegistration:         clean(in.MunicipalRegistration),
		ServiceStartDate:              in.ServiceStartDate.Time,
		ServiceEndDate:                civil.TimePtr(in.ServiceEndDate),
		PayrollType:                   in.PayrollType,
		FactorR:                       in.FactorR,
		FiscalResponsibleUserID:       p.resp["fiscal_responsible_user_id"],
		AccountingResponsibleUserID:   p.resp["accounting_responsible_user_id"],
		PayrollResponsibleUserID:      p.resp["payroll_responsible_user_id"],
		DeclarationsResponsibleUserID: p.resp["declarations_responsible_user_id"],
		Notes:                         clean(in.Notes),
		CreatedBy:                     userID,
	})
	if err != nil {
		return "", err
	}
	if _, err := q.InsertRegime(ctx, store.InsertRegimeParams{
		CompanyID: c.ID, TaxRegime: in.TaxRegime, EffectiveFromCompetence: from, CreatedBy: userID,
	}); err != nil {
		return "", err
	}
	for _, m := range dedupe(p.modules) {
		if _, err := q.InsertModuleSetting(ctx, store.InsertModuleSettingParams{
			CompanyID: c.ID, ModuleCode: m, Enabled: true, EffectiveFromCompetence: from, CreatedBy: userID,
		}); err != nil {
			return "", err
		}
	}
	return c.ID, nil
}

func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// refCompetence é a competência usada para "regime/módulos vigentes": a corrente, ou o
// início do atendimento quando ele ainda não começou.
func (s *Service) refCompetence(start civil.Date) competence.Competence {
	cur := s.currentCompetence()
	if st := competence.FromDate(start.Time); st.After(cur) {
		return st
	}
	return cur
}

// Get devolve a empresa com regime e módulos vigentes.
func (s *Service) Get(ctx context.Context, id string) (Company, error) {
	c, err := s.q.GetCompany(ctx, id)
	if err != nil {
		return Company{}, mapNotFound(err)
	}
	ref := s.refCompetence(civil.Of(c.ServiceStartDate)).FirstDay()
	regime, err := s.q.CompanyCurrentRegime(ctx, store.CompanyCurrentRegimeParams{CompanyID: id, Ref: ref})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Company{}, err
	}
	modules, err := s.q.CompanyActiveModules(ctx, store.CompanyActiveModulesParams{CompanyID: id, Ref: ref})
	if err != nil {
		return Company{}, err
	}
	names, err := s.userNames(ctx, c.FiscalResponsibleUserID, c.AccountingResponsibleUserID, c.PayrollResponsibleUserID, c.DeclarationsResponsibleUserID)
	if err != nil {
		return Company{}, err
	}
	out := Company{
		ID: c.ID, InternalCode: c.InternalCode, LegalName: c.LegalName, TradeName: c.TradeName, Cnpj: c.Cnpj,
		City: c.City, State: c.State, StateRegistration: c.StateRegistration, MunicipalRegistration: c.MunicipalRegistration,
		ServiceStartDate: civil.Of(c.ServiceStartDate), ServiceEndDate: civil.Ptr(c.ServiceEndDate), Active: c.Active,
		PayrollType: c.PayrollType, FactorR: c.FactorR, Notes: c.Notes, ActiveModules: modules, Version: c.Version,
		Responsibles: Responsibles{
			Fiscal: names.ref(c.FiscalResponsibleUserID), Accounting: names.ref(c.AccountingResponsibleUserID),
			Payroll: names.ref(c.PayrollResponsibleUserID), Declarations: names.ref(c.DeclarationsResponsibleUserID),
		},
		CreatedAt: &c.CreatedAt, UpdatedAt: &c.UpdatedAt,
	}
	if regime != "" {
		g := RegimeGroup(regime)
		out.CurrentTaxRegime, out.CurrentRegimeGroup = &regime, &g
	}
	return out, nil
}

type nameMap map[string]string

func (m nameMap) ref(id *string) *UserRef {
	if id == nil {
		return nil
	}
	return &UserRef{ID: *id, Name: m[*id]}
}

func (s *Service) userNames(ctx context.Context, ids ...*string) (nameMap, error) {
	var list []string
	for _, id := range ids {
		if id != nil && !slices.Contains(list, *id) {
			list = append(list, *id)
		}
	}
	m := nameMap{}
	if len(list) == 0 {
		return m, nil
	}
	rows, err := s.q.UserNames(ctx, list)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		m[r.ID] = r.Name
	}
	return m, nil
}

// escapeLike escapa curingas do LIKE no termo digitado pelo usuário.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// List busca empresas. q casa nome, razão social, nome fantasia, código e CNPJ com ou sem
// máscara [PRD §26.5]: quando o termo tem dígitos, eles também são comparados ao CNPJ.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Company, error) {
	p := store.ListCompaniesParams{Ref: s.currentCompetence().FirstDay(), Active: f.Active}
	if q := strings.TrimSpace(f.Q); q != "" {
		esc := escapeLike(q)
		p.Q = &esc
		if d := cnpj.Sanitize(q); len(d) >= 2 && looksNumeric(q) {
			p.QDigits = &d
		}
	}
	if v := cnpj.Sanitize(f.Cnpj); v != "" {
		p.Cnpj = &v
	}
	if v := strings.TrimSpace(f.Code); v != "" {
		p.Code = &v
	}
	if f.TaxRegime != "" {
		p.TaxRegime = &f.TaxRegime
	}
	if f.Module != "" {
		p.Module = &f.Module
	}
	rows, err := s.q.ListCompanies(ctx, p)
	if err != nil {
		return nil, err
	}
	var ids []*string
	for i := range rows {
		r := &rows[i]
		ids = append(ids, r.FiscalResponsibleUserID, r.AccountingResponsibleUserID, r.PayrollResponsibleUserID, r.DeclarationsResponsibleUserID)
	}
	names, err := s.userNames(ctx, ids...)
	if err != nil {
		return nil, err
	}
	out := make([]Company, len(rows))
	for i, r := range rows {
		c := Company{
			ID: r.ID, InternalCode: r.InternalCode, LegalName: r.LegalName, TradeName: r.TradeName, Cnpj: r.Cnpj,
			City: r.City, State: r.State, ServiceStartDate: civil.Of(r.ServiceStartDate), ServiceEndDate: civil.Ptr(r.ServiceEndDate),
			Active: r.Active, PayrollType: r.PayrollType, FactorR: r.FactorR, ActiveModules: r.ActiveModules, Version: r.Version,
			Responsibles: Responsibles{
				Fiscal: names.ref(r.FiscalResponsibleUserID), Accounting: names.ref(r.AccountingResponsibleUserID),
				Payroll: names.ref(r.PayrollResponsibleUserID), Declarations: names.ref(r.DeclarationsResponsibleUserID),
			},
		}
		if r.CurrentTaxRegime != "" {
			regime, g := r.CurrentTaxRegime, RegimeGroup(r.CurrentTaxRegime)
			c.CurrentTaxRegime, c.CurrentRegimeGroup = &regime, &g
		}
		out[i] = c
	}
	return out, nil
}

// looksNumeric indica termo composto só por dígitos e pontuação de CNPJ.
func looksNumeric(q string) bool {
	for _, r := range q {
		if (r < '0' || r > '9') && !strings.ContainsRune("./- ", r) {
			return false
		}
	}
	return true
}

// Update aplica um PATCH com controle de versão (409 se divergente) [plan D8].
// Tipo de folha e fator R alteram só as competências futuras (snapshot, D4).
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (Company, error) {
	if err := validate.Struct(in); err != nil {
		return Company{}, err
	}
	cur, err := s.q.GetCompany(ctx, id)
	if err != nil {
		return Company{}, mapNotFound(err)
	}
	if cur.Version != in.Version {
		return Company{}, apperr.VersionConflict()
	}
	p := store.UpdateCompanyParams{
		ID: id, Version: in.Version, InternalCode: cur.InternalCode, LegalName: cur.LegalName, TradeName: cur.TradeName,
		Cnpj: cur.Cnpj, City: cur.City, State: cur.State, StateRegistration: cur.StateRegistration,
		MunicipalRegistration: cur.MunicipalRegistration, ServiceStartDate: cur.ServiceStartDate, ServiceEndDate: cur.ServiceEndDate,
		Active: cur.Active, PayrollType: cur.PayrollType, FactorR: cur.FactorR,
		FiscalResponsibleUserID: cur.FiscalResponsibleUserID, AccountingResponsibleUserID: cur.AccountingResponsibleUserID,
		PayrollResponsibleUserID: cur.PayrollResponsibleUserID, DeclarationsResponsibleUserID: cur.DeclarationsResponsibleUserID,
		Notes: cur.Notes, UpdatedBy: user.FromContext(ctx),
	}
	if in.InternalCode != nil {
		p.InternalCode = strings.TrimSpace(*in.InternalCode)
	}
	if in.LegalName != nil {
		p.LegalName = strings.TrimSpace(*in.LegalName)
	}
	if in.Cnpj != nil {
		d, err := normalizeCnpj(*in.Cnpj)
		if err != nil {
			return Company{}, err
		}
		p.Cnpj = d
	}
	if in.TradeName.Set {
		p.TradeName = cleanOpt(in.TradeName)
	}
	if in.City.Set {
		p.City = cleanOpt(in.City)
	}
	if in.State.Set {
		p.State = upperPtr(cleanOpt(in.State))
		if p.State != nil && (len(*p.State) != 2 || strings.Trim(*p.State, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "") {
			return Company{}, validate.Field("state", "UF inválida (use a sigla com 2 letras maiúsculas).")
		}
	}
	if in.StateRegistration.Set {
		p.StateRegistration = cleanOpt(in.StateRegistration)
	}
	if in.MunicipalRegistration.Set {
		p.MunicipalRegistration = cleanOpt(in.MunicipalRegistration)
	}
	if in.ServiceStartDate != nil {
		p.ServiceStartDate = in.ServiceStartDate.Time
	}
	if in.ServiceEndDate.Set {
		p.ServiceEndDate = civil.TimePtr(in.ServiceEndDate.Value)
	}
	if err := validatePeriod(civil.Of(p.ServiceStartDate), civil.Ptr(p.ServiceEndDate)); err != nil {
		return Company{}, err
	}
	if in.Active != nil {
		p.Active = *in.Active
	}
	if in.PayrollType != nil {
		p.PayrollType = *in.PayrollType
	}
	if in.FactorR.Set {
		p.FactorR = in.FactorR.Value
	}
	resp := map[string]*string{}
	setResp := func(field string, o opt.Optional[string], dst **string) {
		if o.Set {
			*dst = cleanOpt(o)
			resp[field] = *dst
		}
	}
	setResp("fiscal_responsible_user_id", in.FiscalResponsibleUserID, &p.FiscalResponsibleUserID)
	setResp("accounting_responsible_user_id", in.AccountingResponsibleUserID, &p.AccountingResponsibleUserID)
	setResp("payroll_responsible_user_id", in.PayrollResponsibleUserID, &p.PayrollResponsibleUserID)
	setResp("declarations_responsible_user_id", in.DeclarationsResponsibleUserID, &p.DeclarationsResponsibleUserID)
	for field, v := range resp {
		if v != nil && !httpx.IsUUID(*v) {
			return Company{}, validate.Field(field, "Identificador inválido.")
		}
	}
	if err := s.checkResponsibles(ctx, resp); err != nil {
		return Company{}, err
	}
	if in.Notes.Set {
		p.Notes = cleanOpt(in.Notes)
	}
	if p.InternalCode == "" || p.LegalName == "" {
		return Company{}, validate.Field("legal_name", "Campo obrigatório.")
	}
	if err := s.conflictFor(ctx, p.Cnpj, p.InternalCode, id); err != nil {
		return Company{}, err
	}
	if _, err := s.q.UpdateCompany(ctx, p); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Company{}, apperr.VersionConflict()
		}
		return Company{}, s.mapWriteErr(ctx, err, p.Cnpj, p.InternalCode, id)
	}
	return s.Get(ctx, id)
}

// Deactivate inativa a empresa. Não há exclusão física [RN-008].
func (s *Service) Deactivate(ctx context.Context, id string, version *int32) (Company, error) {
	cur, err := s.q.GetCompany(ctx, id)
	if err != nil {
		return Company{}, mapNotFound(err)
	}
	v := cur.Version
	if version != nil {
		v = *version
	}
	f := false
	return s.Update(ctx, id, UpdateInput{Version: v, Active: &f})
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) || db.IsInvalidText(err) {
		return apperr.NotFound("COMPANY_NOT_FOUND", "Empresa não encontrada.")
	}
	return err
}

func civilOf(t time.Time) civil.Date { return civil.Of(t) }
