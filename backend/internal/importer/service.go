package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/company"
	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	"controle/internal/platform/db"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// PreviewTTL é a validade do token da prévia [T-M6-03].
const PreviewTTL = time.Hour

// Ações do resultado da importação.
const (
	ActionCreated = "CREATED"
	ActionUpdated = "UPDATED"
)

// Summary conta as linhas por classificação.
type Summary struct {
	Total      int            `json:"total"`
	Importable int            `json:"importable"`
	ByStatus   map[string]int `json:"by_status"`
}

// Preview é a resposta de POST /imports/companies/preview.
type Preview struct {
	Token          string    `json:"token"`
	FileName       string    `json:"file_name"`
	ExpiresAt      time.Time `json:"expires_at"`
	Summary        Summary   `json:"summary"`
	UnknownHeaders []string  `json:"unknown_headers"`
	Rows           []Row     `json:"rows"`
}

// CommitInput é o payload de POST /imports/companies/commit. update_existing_rows lista as
// linhas (número na planilha) de empresas existentes cuja atualização foi confirmada.
type CommitInput struct {
	Token              string `json:"token" validate:"required,uuid"`
	UpdateExistingRows []int  `json:"update_existing_rows"`
}

// CommitItem é uma empresa gravada.
type CommitItem struct {
	Line         int    `json:"line"`
	CompanyID    string `json:"company_id"`
	InternalCode string `json:"internal_code"`
	LegalName    string `json:"legal_name"`
	Action       string `json:"action"`
}

// CommitResult é o resumo da importação.
type CommitResult struct {
	Created         int          `json:"created"`
	Updated         int          `json:"updated"`
	SkippedExisting int          `json:"skipped_existing"`
	SkippedInvalid  int          `json:"skipped_invalid"`
	Companies       []CommitItem `json:"companies"`
}

// storedRow guarda na prévia o que a confirmação precisa gravar.
type storedRow struct {
	Row
	Input  company.CreateInput `json:"input"`
	Active bool                `json:"active"`
}

// Service faz a prévia e a confirmação da importação de empresas.
type Service struct {
	pool      *pgxpool.Pool
	clock     clock.Clock
	companies *company.Service
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock, companies *company.Service) *Service {
	return &Service{pool: pool, clock: clk, companies: companies}
}

// dbLookup concilia com o cadastro atual (carregado uma vez por prévia).
type dbLookup struct {
	byCnpj map[string]ExistingCompany
	byCode map[string]ExistingCompany
	users  map[string]string
}

func (l dbLookup) CompanyByCnpj(c string) (ExistingCompany, bool) { e, ok := l.byCnpj[c]; return e, ok }
func (l dbLookup) CompanyByCode(c string) (ExistingCompany, bool) {
	e, ok := l.byCode[strings.ToLower(strings.TrimSpace(c))]
	return e, ok
}
func (l dbLookup) UserID(s string) (string, bool) {
	id, ok := l.users[normalizeHeader(s)]
	return id, ok && id != ""
}

func (s *Service) lookup(ctx context.Context) (dbLookup, error) {
	q := store.New(s.pool)
	companies, err := q.ListCompaniesForImport(ctx)
	if err != nil {
		return dbLookup{}, err
	}
	lk := dbLookup{byCnpj: map[string]ExistingCompany{}, byCode: map[string]ExistingCompany{}, users: map[string]string{}}
	for _, c := range companies {
		e := ExistingCompany{ID: c.ID, InternalCode: c.InternalCode, Cnpj: c.Cnpj, LegalName: c.LegalName}
		lk.byCnpj[c.Cnpj] = e
		lk.byCode[strings.ToLower(c.InternalCode)] = e
	}
	active := true
	users, err := q.ListUsers(ctx, &active)
	if err != nil {
		return dbLookup{}, err
	}
	for _, u := range users {
		keys := []string{normalizeHeader(u.Name)}
		if u.Email != nil {
			keys = append(keys, normalizeHeader(*u.Email))
		}
		for _, k := range keys {
			if prev, dup := lk.users[k]; dup && prev != u.ID {
				lk.users[k] = "" // nome ambíguo: exige o e-mail
				continue
			}
			lk.users[k] = u.ID
		}
	}
	return lk, nil
}

func summarize(rows []Row) Summary {
	s := Summary{Total: len(rows), ByStatus: map[string]int{}}
	for st := range StatusLabels {
		s.ByStatus[st] = 0
	}
	for _, r := range rows {
		s.ByStatus[r.Status]++
		if Importable(r.Status) {
			s.Importable++
		}
	}
	return s
}

// Preview lê, valida e classifica a planilha e guarda o resultado por 1 hora [PRD §35.3].
// Nada é gravado no cadastro: erros aparecem antes da confirmação [CA-018].
func (s *Service) Preview(ctx context.Context, fileName string, data []byte) (Preview, error) {
	sheet, err := Parse(fileName, data)
	if err != nil {
		return Preview{}, err
	}
	lk, err := s.lookup(ctx)
	if err != nil {
		return Preview{}, err
	}
	rows := Classify(sheet.Records, lk)
	stored := make([]storedRow, len(rows))
	for i, r := range rows {
		stored[i] = storedRow{Row: r, Input: r.Input, Active: r.Active}
	}
	rowsJSON, err := json.Marshal(stored)
	if err != nil {
		return Preview{}, err
	}
	summary := summarize(rows)
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return Preview{}, err
	}
	now := s.clock.Now()
	name := sanitizeName(fileName)
	id, err := store.New(s.pool).InsertImportPreview(ctx, store.InsertImportPreviewParams{
		FileName: name, Rows: rowsJSON, Summary: summaryJSON, CreatedBy: user.FromContext(ctx),
		CreatedAt: now, ExpiresAt: now.Add(PreviewTTL),
	})
	if err != nil {
		return Preview{}, err
	}
	unknown := sheet.UnknownHeaders
	if unknown == nil {
		unknown = []string{}
	}
	return Preview{Token: id, FileName: name, ExpiresAt: now.Add(PreviewTTL), Summary: summary, UnknownHeaders: unknown, Rows: rows}, nil
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if r := []rune(name); len(r) > 200 {
		name = string(r[:200])
	}
	if name == "" {
		name = "planilha"
	}
	return name
}

// Commit grava, numa única transação, as linhas novas e as existentes confirmadas; linhas
// com erro e existentes não confirmadas ficam de fora [PRD §35.5]. Token expirado → 410.
func (s *Service) Commit(ctx context.Context, in CommitInput) (CommitResult, error) {
	if err := validate.Struct(in); err != nil {
		return CommitResult{}, err
	}
	now := s.clock.Now()
	userID := user.FromContext(ctx)
	var result CommitResult
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		p, err := q.GetImportPreviewForUpdate(ctx, in.Token)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("IMPORT_NOT_FOUND", "Prévia de importação não encontrada. Envie a planilha de novo.")
		}
		if err != nil {
			return err
		}
		if p.CommittedAt != nil {
			return apperr.Conflict("IMPORT_ALREADY_COMMITTED", "Esta importação já foi confirmada.")
		}
		if !now.Before(p.ExpiresAt) {
			return apperr.Gone("IMPORT_TOKEN_EXPIRED", "A prévia expirou (validade de 1 hora). Envie a planilha de novo.")
		}
		var rows []storedRow
		if err := json.Unmarshal(p.Rows, &rows); err != nil {
			return fmt.Errorf("prévia corrompida: %w", err)
		}
		confirmed := map[int]bool{}
		for _, line := range in.UpdateExistingRows {
			idx := slices.IndexFunc(rows, func(r storedRow) bool { return r.Line == line })
			if idx < 0 || rows[idx].Status != StatusExisting {
				return apperr.Unprocessable("INVALID_CONFIRMATION", fmt.Sprintf("A linha %d não é uma empresa existente da prévia.", line))
			}
			confirmed[line] = true
		}
		result = CommitResult{Companies: []CommitItem{}}
		for _, r := range rows {
			switch {
			case r.Status == StatusNew:
				id, err := s.companies.CreateTx(ctx, tx, r.Input)
				if err != nil {
					return lineError(r.Line, err)
				}
				if !r.Active {
					if err := setActive(ctx, q, id, false, userID); err != nil {
						return lineError(r.Line, err)
					}
				}
				result.Created++
				result.Companies = append(result.Companies, CommitItem{Line: r.Line, CompanyID: id, InternalCode: r.Input.InternalCode, LegalName: r.Input.LegalName, Action: ActionCreated})
			case r.Status == StatusExisting && confirmed[r.Line]:
				if r.ExistingID == nil {
					return lineError(r.Line, errors.New("empresa existente sem identificador"))
				}
				if err := updateExisting(ctx, q, *r.ExistingID, r.Input, userID); err != nil {
					return lineError(r.Line, err)
				}
				result.Updated++
				result.Companies = append(result.Companies, CommitItem{Line: r.Line, CompanyID: *r.ExistingID, InternalCode: r.Input.InternalCode, LegalName: r.Input.LegalName, Action: ActionUpdated})
			case r.Status == StatusExisting:
				result.SkippedExisting++
			default:
				result.SkippedInvalid++
			}
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return q.MarkImportCommitted(ctx, store.MarkImportCommittedParams{ID: in.Token, CommittedAt: &now, CommittedBy: &userID, Result: resultJSON})
	})
	if err != nil {
		return CommitResult{}, err
	}
	return result, nil
}

// lineError identifica a linha que impediu a importação (nada é gravado).
func lineError(line int, err error) error {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		out := *ae
		out.Message = fmt.Sprintf("Linha %d: %s Nenhuma empresa foi importada.", line, ae.Message)
		details := map[string]any{"line": line}
		for k, v := range ae.Details {
			details[k] = v
		}
		out.Details = details
		return &out
	}
	return fmt.Errorf("linha %d: %w", line, err)
}

func setActive(ctx context.Context, q *store.Queries, id string, active bool, userID string) error {
	cur, err := q.GetCompanyForImportUpdate(ctx, id)
	if err != nil {
		return err
	}
	p := paramsFrom(cur, userID)
	p.Active = active
	_, err = q.UpdateCompany(ctx, p)
	return err
}

func paramsFrom(c store.Company, userID string) store.UpdateCompanyParams {
	return store.UpdateCompanyParams{
		ID: c.ID, Version: c.Version, InternalCode: c.InternalCode, LegalName: c.LegalName, TradeName: c.TradeName,
		Cnpj: c.Cnpj, City: c.City, State: c.State, StateRegistration: c.StateRegistration,
		MunicipalRegistration: c.MunicipalRegistration, ServiceStartDate: c.ServiceStartDate, ServiceEndDate: c.ServiceEndDate,
		Active: c.Active, PayrollType: c.PayrollType, FactorR: c.FactorR,
		FiscalResponsibleUserID: c.FiscalResponsibleUserID, AccountingResponsibleUserID: c.AccountingResponsibleUserID,
		PayrollResponsibleUserID: c.PayrollResponsibleUserID, DeclarationsResponsibleUserID: c.DeclarationsResponsibleUserID,
		Notes: c.Notes, UpdatedBy: userID,
	}
}

// updateExisting atualiza os dados cadastrais de uma empresa existente confirmada. Campos
// vazios na planilha mantêm o valor atual; regime, módulos e situação não mudam pela
// importação (seguem pelos fluxos com vigência da ficha da empresa).
func updateExisting(ctx context.Context, q *store.Queries, id string, in company.CreateInput, userID string) error {
	cur, err := q.GetCompanyForImportUpdate(ctx, id)
	if err != nil {
		return err
	}
	p := paramsFrom(cur, userID)
	p.InternalCode = strings.TrimSpace(in.InternalCode)
	p.LegalName = strings.TrimSpace(in.LegalName)
	p.ServiceStartDate = in.ServiceStartDate.Time
	p.PayrollType = in.PayrollType
	keep := func(dst **string, v *string) {
		if v != nil {
			*dst = v
		}
	}
	keep(&p.TradeName, in.TradeName)
	keep(&p.City, in.City)
	keep(&p.State, in.State)
	keep(&p.StateRegistration, in.StateRegistration)
	keep(&p.MunicipalRegistration, in.MunicipalRegistration)
	keep(&p.Notes, in.Notes)
	keep(&p.FiscalResponsibleUserID, in.FiscalResponsibleUserID)
	keep(&p.AccountingResponsibleUserID, in.AccountingResponsibleUserID)
	keep(&p.PayrollResponsibleUserID, in.PayrollResponsibleUserID)
	keep(&p.DeclarationsResponsibleUserID, in.DeclarationsResponsibleUserID)
	if in.ServiceEndDate != nil {
		p.ServiceEndDate = civil.TimePtr(in.ServiceEndDate)
	}
	if in.FactorR != nil {
		p.FactorR = in.FactorR
	}
	_, err = q.UpdateCompany(ctx, p)
	if db.IsConstraint(err, db.UniqueViolation, "companies_internal_code_uq") {
		return apperr.Conflict("CODE_CONFLICT", "O código interno "+p.InternalCode+" já é usado por outra empresa.")
	}
	return err
}
