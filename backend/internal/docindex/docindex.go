// Package docindex é a área Documentos consolidada (TASK-026, D-040): um índice de navegação
// sobre os documentos de cada módulo — os extratos do PGDAS-D do Fator R (armazenamento local),
// os documentos da Conciliação (MinIO) e os arquivos de procuração e certificado do Controle
// (armazenamento local) —, sem tabela nova e sem mover arquivo (REQ-037, REQ-043, REQ-044).
// Cada documento mostra o status do seu módulo; a situação comum (em processamento, em revisão,
// concluído, descartado) é só leitura, para filtrar entre módulos (decisão do usuário). "Abrir"
// leva à tela do módulo.
package docindex

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/civil"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
)

// Módulos (origens) do índice.
const (
	ModuleFactorR        = "factor_r"
	ModuleReconciliation = "reconciliation"
	ModuleCredentials    = "credentials"
)

// Modules são as origens, na ordem da tela.
var Modules = []string{ModuleFactorR, ModuleReconciliation, ModuleCredentials}

// Situations são as situações comuns de leitura.
var Situations = []string{"em_processamento", "em_revisao", "concluido", "descartado"}

// Kinds são os tipos de documento de cada origem.
var Kinds = map[string][]string{
	ModuleFactorR:        {"pgdas"},
	ModuleReconciliation: {"extrato", "comprovante", "desconhecido"},
	ModuleCredentials:    {"procuracao", "certificado"},
}

// Statuses são os status de cada módulo (os da tabela de cada um, sem tradução).
var Statuses = map[string][]string{
	ModuleFactorR:        {"received", "parsed", "needs_review", "linked", "rejected"},
	ModuleReconciliation: {"pendente", "triado", "extraido", "importado", "revisao_triagem", "revisao_extracao", "duplicado", "pendencia_periodo_fechado"},
	ModuleCredentials:    {"arquivado"},
}

// Link é a rota da interface que abre o documento no seu módulo.
type Link struct {
	Path  string            `json:"path"`
	Query map[string]string `json:"query"`
}

// CompanyRef identifica a empresa do documento.
type CompanyRef struct {
	ID           string `json:"id"`
	InternalCode string `json:"internal_code"`
	LegalName    string `json:"legal_name"`
}

// Document é um documento do índice.
type Document struct {
	ID             string      `json:"id"`
	Module         string      `json:"module"`
	StorageBackend string      `json:"storage_backend"`
	Kind           string      `json:"kind"`
	Status         string      `json:"status"`
	Situation      string      `json:"situation"`
	Company        *CompanyRef `json:"company"`
	Competence     *string     `json:"competence"`
	PeriodStart    *civil.Date `json:"period_start"`
	PeriodEnd      *civil.Date `json:"period_end"`
	Bank           *string     `json:"bank"`
	FileName       string      `json:"file_name"`
	// ExpiresAt é a validade da procuração ou do certificado.
	ExpiresAt  *civil.Date `json:"expires_at"`
	ReceivedAt time.Time   `json:"received_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Detail     Link        `json:"detail"`
}

// Count é a quantidade de documentos de um módulo numa situação e num status.
type Count struct {
	Module    string `json:"module"`
	Situation string `json:"situation"`
	Status    string `json:"status"`
	Total     int    `json:"total"`
}

// List é uma página do índice com as contagens para os filtros.
type List struct {
	Items  []Document `json:"items"`
	Total  int        `json:"total"`
	Counts []Count    `json:"counts"`
}

// Query são os filtros da área (08 §8): origem, empresa, competência ou período, tipo, status,
// situação e banco.
type Query struct {
	Module     string
	CompanyID  string
	Competence string
	Kind       string
	Status     string
	Situation  string
	Bank       string
	Limit      int32
	Offset     int32
}

// Service lê o índice.
type Service struct{ pool *pgxpool.Pool }

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func opt(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func badRequest(field, msg string) error { return validate.Field(field, msg) }

// List lê uma página do índice, a mais recente primeiro.
func (s *Service) List(ctx context.Context, in Query) (List, error) {
	p := store.ListDocumentIndexParams{Module: opt(in.Module), Situation: opt(in.Situation), Status: opt(in.Status),
		CompanyID: opt(in.CompanyID), Kind: opt(in.Kind), Bank: opt(in.Bank)}
	if p.Module != nil && !slices.Contains(Modules, *p.Module) {
		return List{}, badRequest("module", "Use factor_r, reconciliation ou credentials.")
	}
	if p.Situation != nil && !slices.Contains(Situations, *p.Situation) {
		return List{}, badRequest("situation", "Use em_processamento, em_revisao, concluido ou descartado.")
	}
	if p.Status != nil && (p.Module == nil || !slices.Contains(Statuses[*p.Module], *p.Status)) {
		return List{}, badRequest("status", "O status é o do módulo: escolha o módulo e um status dele.")
	}
	if p.Kind != nil && !slices.ContainsFunc(Modules, func(m string) bool { return slices.Contains(Kinds[m], *p.Kind) }) {
		return List{}, badRequest("kind", "Tipo inválido.")
	}
	if p.CompanyID != nil && !httpx.IsUUID(*p.CompanyID) {
		return List{}, badRequest("company_id", "Empresa inválida.")
	}
	if v := opt(in.Competence); v != nil {
		c, err := competence.Parse(*v)
		if err != nil {
			return List{}, badRequest("competence", "Use AAAA-MM.")
		}
		d := c.FirstDay()
		p.Competence = &d
	}
	switch {
	case in.Limit == 0:
		p.RowLimit = 50
	case in.Limit < 1 || in.Limit > 200:
		return List{}, badRequest("limit", "Use um limite entre 1 e 200.")
	default:
		p.RowLimit = in.Limit
	}
	if in.Offset < 0 {
		return List{}, badRequest("offset", "O deslocamento não pode ser negativo.")
	}
	p.RowOffset = in.Offset

	q := store.New(s.pool)
	rows, err := q.ListDocumentIndex(ctx, p)
	if err != nil {
		return List{}, err
	}
	counts, err := q.CountDocumentIndex(ctx, store.CountDocumentIndexParams{CompanyID: p.CompanyID, Kind: p.Kind, Bank: p.Bank, Competence: p.Competence})
	if err != nil {
		return List{}, err
	}
	out := List{Items: make([]Document, 0, len(rows)), Counts: make([]Count, 0, len(counts))}
	for _, r := range rows {
		out.Total = int(r.Total)
		out.Items = append(out.Items, toDocument(r))
	}
	for _, c := range counts {
		out.Counts = append(out.Counts, Count{Module: c.Module, Situation: c.Situation, Status: c.Status, Total: int(c.Total)})
	}
	return out, nil
}

func toDocument(r store.ListDocumentIndexRow) Document {
	d := Document{
		ID: r.ID, Module: r.Module, StorageBackend: r.StorageBackend, Kind: r.Kind, Status: r.Status, Situation: r.Situation,
		PeriodStart: civil.Ptr(r.PeriodStart), PeriodEnd: civil.Ptr(r.PeriodEnd), Bank: r.Bank, FileName: r.FileName,
		ExpiresAt: civil.Ptr(r.ExpiresAt), ReceivedAt: r.ReceivedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.CompanyID != nil {
		d.Company = &CompanyRef{ID: *r.CompanyID, InternalCode: r.CompanyCode, LegalName: r.CompanyName}
	}
	if r.Competence != nil {
		c := competence.FromDate(*r.Competence).String()
		d.Competence = &c
	}
	d.Detail = detail(d)
	return d
}

// detail é a tela do módulo que abre o documento.
func detail(d Document) Link {
	switch d.Module {
	case ModuleFactorR:
		return Link{Path: "/factor-r/inbox/" + d.ID, Query: map[string]string{}}
	case ModuleReconciliation:
		return Link{Path: "/reconciliation/documents/" + d.ID, Query: map[string]string{}}
	default:
		if d.Company == nil {
			return Link{Path: "/credentials", Query: map[string]string{}}
		}
		return Link{Path: "/companies/" + d.Company.ID, Query: map[string]string{"tab": "credentials"}}
	}
}
