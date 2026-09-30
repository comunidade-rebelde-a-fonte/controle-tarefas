// Package credential controla procurações RFB e certificados digitais: um item ativo por
// empresa e tipo, com situação de validade calculada e etapa de renovação [PRD §19, §33,
// RN-017]. Nunca armazena senha, chave privada ou segredo [RN-018].
package credential

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/settings"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

// Tipos de credencial.
const (
	TypePowerOfAttorney = "RFB_POWER_OF_ATTORNEY"
	TypeCertificate     = "DIGITAL_CERTIFICATE"
)

// Types lista os tipos na ordem de exibição.
var Types = []string{TypePowerOfAttorney, TypeCertificate}

// Etapas de renovação [PRD §19.4].
const (
	RenewalCurrent        = "CURRENT"
	RenewalRequested      = "RENEWAL_REQUESTED"
	RenewalAwaitingClient = "AWAITING_CLIENT"
	RenewalRenewed        = "RENEWED"
)

var renewalLabels = map[string]string{
	RenewalCurrent:        "Vigente",
	RenewalRequested:      "Renovação solicitada",
	RenewalAwaitingClient: "Aguardando cliente",
	RenewalRenewed:        "Renovado",
}

const (
	companyNotFound = "COMPANY_NOT_FOUND"
	companyMissing  = "Empresa não encontrada."
)

// File é o documento público vigente (sem chave privada) [PRD §19.1].
type File struct {
	Name       string    `json:"name"`
	MimeType   *string   `json:"mime_type"`
	SizeBytes  *int64    `json:"size_bytes"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// Control é a procuração ou o certificado de uma empresa. Sem registro gravado, é o item
// padrão: sem data, Vigente, version 0 (o PUT com version 0 cria o registro).
type Control struct {
	Type          string      `json:"type"`
	ExpiresAt     *civil.Date `json:"expires_at"`
	RenewalStatus string      `json:"renewal_status"`
	RenewalLabel  string      `json:"renewal_label"`
	Situation     Situation   `json:"situation"`
	Notes         *string     `json:"notes"`
	File          *File       `json:"file"`
	Version       int32       `json:"version"`
	UpdatedAt     *time.Time  `json:"updated_at"`
}

// CompanyRef identifica a empresa na grade.
type CompanyRef struct {
	ID           string  `json:"id"`
	InternalCode string  `json:"internal_code"`
	LegalName    string  `json:"legal_name"`
	TradeName    *string `json:"trade_name"`
	Cnpj         string  `json:"cnpj"`
	TaxRegime    string  `json:"tax_regime"`
}

// Row é uma linha da tela: a empresa com a procuração e o certificado [PRD §33].
type Row struct {
	Company            CompanyRef `json:"company"`
	RFBPowerOfAttorney Control    `json:"rfb_power_of_attorney"`
	DigitalCertificate Control    `json:"digital_certificate"`
}

// List é a resposta de GET /credential-controls.
type List struct {
	Items      []Row `json:"items"`
	Total      int   `json:"total"`
	Thresholds []int `json:"thresholds"`
}

// ListParams são os filtros da grade.
type ListParams struct {
	Filter string
	// Tipo considerado pelo filtro; vazio = procuração ou certificado.
	Type string
}

// UpdateInput é o payload de PUT /companies/{id}/credential-controls/{type}.
type UpdateInput struct {
	Version       int32       `json:"version" validate:"min=0"`
	ExpiresAt     *civil.Date `json:"expires_at"`
	RenewalStatus string      `json:"renewal_status" validate:"required,oneof=CURRENT RENEWAL_REQUESTED AWAITING_CLIENT RENEWED"`
	Notes         *string     `json:"notes" validate:"omitempty,max=2000"`
}

// Service contém as regras de procurações e certificados.
type Service struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	settings *settings.Service
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk, settings: settings.NewService(pool)}
}

func (s *Service) thresholds(ctx context.Context) ([]int, error) {
	t, err := s.settings.CredentialThresholds(ctx)
	if err != nil {
		return nil, err
	}
	return t.Days, nil
}

// ref é o dia 1 da competência corrente (módulo e regime vigentes hoje).
func (s *Service) ref() time.Time { return comp.Current(s.clock).FirstDay() }

func (s *Service) toControl(typ string, rec *store.CredentialControl, thresholds []int) Control {
	today := clock.Today(s.clock)
	c := Control{Type: typ, RenewalStatus: RenewalCurrent}
	if rec != nil {
		c.ExpiresAt = civil.Ptr(rec.ExpiresAt)
		c.RenewalStatus = rec.RenewalStatus
		c.Notes = rec.Notes
		c.Version = rec.Version
		updated := rec.UpdatedAt
		c.UpdatedAt = &updated
		if rec.FileName != nil && rec.FileUploadedAt != nil {
			c.File = &File{Name: *rec.FileName, MimeType: rec.FileMimeType, SizeBytes: rec.FileSizeBytes, UploadedAt: *rec.FileUploadedAt}
		}
	}
	c.RenewalLabel = renewalLabels[c.RenewalStatus]
	c.Situation = SituationOf(civil.TimePtr(c.ExpiresAt), today, thresholds)
	return c
}

func companyRef(id, code, legal string, trade *string, cnpj, regime string) CompanyRef {
	return CompanyRef{ID: id, InternalCode: code, LegalName: legal, TradeName: trade, Cnpj: cnpj, TaxRegime: regime}
}

func (s *Service) buildRow(c CompanyRef, recs map[string]*store.CredentialControl, thresholds []int) Row {
	return Row{
		Company:            c,
		RFBPowerOfAttorney: s.toControl(TypePowerOfAttorney, recs[TypePowerOfAttorney], thresholds),
		DigitalCertificate: s.toControl(TypeCertificate, recs[TypeCertificate], thresholds),
	}
}

func (s *Service) records(ctx context.Context, q *store.Queries, companyIDs []string) (map[string]map[string]*store.CredentialControl, error) {
	rows, err := q.ListCredentialControls(ctx, companyIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]*store.CredentialControl, len(companyIDs))
	for i := range rows {
		r := &rows[i]
		if out[r.CompanyID] == nil {
			out[r.CompanyID] = map[string]*store.CredentialControl{}
		}
		out[r.CompanyID][r.Type] = r
	}
	return out, nil
}

// List devolve a grade de procurações e certificados das empresas com o módulo ativo,
// aplicando o filtro rápido [PRD §33].
func (s *Service) List(ctx context.Context, p ListParams) (List, error) {
	f, err := ParseFilter(p.Filter)
	if err != nil {
		return List{}, err
	}
	if p.Type != "" && p.Type != TypePowerOfAttorney && p.Type != TypeCertificate {
		return List{}, validate.Field("type", "Tipo inválido: use RFB_POWER_OF_ATTORNEY ou DIGITAL_CERTIFICATE.")
	}
	thresholds, err := s.thresholds(ctx)
	if err != nil {
		return List{}, err
	}
	q := store.New(s.pool)
	companies, err := q.ListCredentialCompanies(ctx, s.ref())
	if err != nil {
		return List{}, err
	}
	ids := make([]string, len(companies))
	for i, c := range companies {
		ids[i] = c.ID
	}
	recs, err := s.records(ctx, q, ids)
	if err != nil {
		return List{}, err
	}
	out := List{Items: []Row{}, Thresholds: thresholds}
	for _, c := range companies {
		row := s.buildRow(companyRef(c.ID, c.InternalCode, c.LegalName, c.TradeName, c.Cnpj, c.TaxRegime), recs[c.ID], thresholds)
		if matchRow(row, f, p.Type) {
			out.Items = append(out.Items, row)
		}
	}
	out.Total = len(out.Items)
	return out, nil
}

func matchRow(r Row, f Filter, typ string) bool {
	switch typ {
	case TypePowerOfAttorney:
		return f.Match(r.RFBPowerOfAttorney)
	case TypeCertificate:
		return f.Match(r.DigitalCertificate)
	}
	return f.Match(r.RFBPowerOfAttorney) || f.Match(r.DigitalCertificate)
}

// ForCompany devolve a procuração e o certificado de uma empresa (mesmo sem o módulo ativo).
func (s *Service) ForCompany(ctx context.Context, companyID string) (Row, error) {
	thresholds, err := s.thresholds(ctx)
	if err != nil {
		return Row{}, err
	}
	q := store.New(s.pool)
	c, err := q.GetCredentialCompany(ctx, store.GetCredentialCompanyParams{Ref: s.ref(), ID: companyID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Row{}, apperr.NotFound(companyNotFound, companyMissing)
	}
	if err != nil {
		return Row{}, err
	}
	recs, err := s.records(ctx, q, []string{companyID})
	if err != nil {
		return Row{}, err
	}
	return s.buildRow(companyRef(c.ID, c.InternalCode, c.LegalName, c.TradeName, c.Cnpj, c.TaxRegime), recs[companyID], thresholds), nil
}

// Update grava vencimento, etapa de renovação e observação de um tipo [UC-05]. "Renovado"
// exige uma nova data de validade e devolve o item ao acompanhamento normal (Vigente)
// [PRD §19.4]. version 0 cria o registro; divergência de versão → 409.
func (s *Service) Update(ctx context.Context, companyID, typ string, in UpdateInput) (Control, error) {
	if err := validate.Struct(in); err != nil {
		return Control{}, err
	}
	if in.Notes != nil {
		n := strings.TrimSpace(*in.Notes)
		if n == "" {
			in.Notes = nil
		} else {
			in.Notes = &n
		}
	}
	thresholds, err := s.thresholds(ctx)
	if err != nil {
		return Control{}, err
	}
	q := store.New(s.pool)
	if _, err := q.GetCredentialCompany(ctx, store.GetCredentialCompanyParams{Ref: s.ref(), ID: companyID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Control{}, apperr.NotFound(companyNotFound, companyMissing)
		}
		return Control{}, err
	}
	current, err := s.records(ctx, q, []string{companyID})
	if err != nil {
		return Control{}, err
	}
	prev := current[companyID][typ]

	status := in.RenewalStatus
	if status == RenewalRenewed {
		if in.ExpiresAt == nil {
			return Control{}, apperr.Unprocessable("RENEWAL_DATE_REQUIRED", "Informe a nova data de validade para marcar como Renovado.").
				WithDetails(map[string]any{"fields": map[string]string{"expires_at": "Informe a nova data de validade."}})
		}
		if prev != nil && prev.ExpiresAt != nil && civil.Of(*prev.ExpiresAt) == *in.ExpiresAt {
			return Control{}, apperr.Unprocessable("RENEWAL_DATE_REQUIRED", "A nova data de validade deve ser diferente da atual.").
				WithDetails(map[string]any{"fields": map[string]string{"expires_at": "Informe a nova data de validade."}})
		}
		status = RenewalCurrent
	}

	now := s.clock.Now()
	uid := user.FromContext(ctx)
	var rec store.CredentialControl
	if in.Version == 0 {
		if prev != nil {
			return Control{}, apperr.VersionConflict().WithDetails(map[string]any{"current_version": prev.Version})
		}
		rec, err = q.InsertCredentialControl(ctx, store.InsertCredentialControlParams{
			CompanyID: companyID, Type: typ, ExpiresAt: civil.TimePtr(in.ExpiresAt), RenewalStatus: status,
			Notes: in.Notes, UpdatedBy: &uid, UpdatedAt: now,
		})
	} else {
		rec, err = q.UpdateCredentialControl(ctx, store.UpdateCredentialControlParams{
			CompanyID: companyID, Type: typ, Version: in.Version, ExpiresAt: civil.TimePtr(in.ExpiresAt),
			RenewalStatus: status, Notes: in.Notes, UpdatedBy: &uid, UpdatedAt: now,
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		details := map[string]any{}
		if prev != nil {
			details["current_version"] = prev.Version
		}
		return Control{}, apperr.VersionConflict().WithDetails(details)
	}
	if err != nil {
		return Control{}, err
	}
	return s.toControl(typ, &rec, thresholds), nil
}
