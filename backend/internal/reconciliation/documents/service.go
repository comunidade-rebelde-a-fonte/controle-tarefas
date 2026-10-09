// Package documents é a ingestão manual da Conciliação [REQ-018, REQ-043, REQ-045, REQ-046;
// TASK-015]: porte de app/servicos/ingestao.py e app/storage.py do Conciliador (commit 2bc6e26).
// O original vai para o MinIO versionado, com o SHA-256 como chave e dedupe; o documento nasce
// `pendente` com o job de triagem na fila (o worker processa na TASK-016), ou fica parado como
// `pendencia_periodo_fechado` quando a competência informada está com o Contábil fechado.
package documents

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/filestore"
	"controle/internal/platform/httpx"
	"controle/internal/platform/objectstore"
	"controle/internal/platform/pdftext"
	"controle/internal/platform/validate"
	"controle/internal/reconciliation"
	"controle/internal/store"
	"controle/internal/user"
)

// KindTriage é o job que processa o documento no worker (triagem e extração; TASK-016). Até lá,
// o job espera na fila sem gastar tentativa: o worker só adquire tipos que sabe executar (D-019).
const KindTriage = "reconciliation_triage"

// MaxFileSize é o limite do envio. A origem não limitava; 20 MB cobrem extratos longos e
// comprovantes digitalizados.
const MaxFileSize = 20 << 20

// maxFileName é o tamanho do nome guardado (String(255) na origem).
const maxFileName = 255

// Estados do documento (contrato congelado da origem, D-020).
const (
	StatusPending      = "pendente"
	StatusClosedPeriod = "pendencia_periodo_fechado"
)

// Tipos aceitos, pelo conteúdo: PDF (extratos e comprovantes), imagem (foto ou print de
// comprovante) e texto (CSV/TXT de extrato). O nome e o Content-Type enviados não decidem.
const (
	MimePNG  = "image/png"
	MimeJPEG = "image/jpeg"
)

const (
	codeDuplicate   = "DUPLICATE_DOCUMENT"
	codeTooLarge    = "FILE_TOO_LARGE"
	codeUnsupported = "UNSUPPORTED_FILE"
	codeStorage     = "STORAGE_UNAVAILABLE"
	codeNotFound    = "RECONCILIATION_DOCUMENT_NOT_FOUND"
)

// Storage é o armazenamento dos originais (objectstore.Store; nos testes, o MinIO real).
type Storage interface {
	Configured() bool
	Bucket() string
	Put(ctx context.Context, key string, body []byte, contentType string) (objectstore.PutResult, error)
	Get(ctx context.Context, key, versionID string) (objectstore.Object, error)
}

// Service é a ingestão na API.
type Service struct {
	pool    *pgxpool.Pool
	clock   clock.Clock
	queue   *jobs.Queue
	storage Storage
	logger  *slog.Logger
}

// NewService cria o serviço. clk é o tempo operacional (o da fila).
func NewService(pool *pgxpool.Pool, clk clock.Clock, queue *jobs.Queue, storage Storage, logger *slog.Logger) *Service {
	return &Service{pool: pool, clock: clk, queue: queue, storage: storage, logger: logger}
}

// ObjectKey é a chave do original no bucket: só o SHA-256. O nome enviado é metadado e nunca
// entra na chave [09 §5]; a origem usava originais/<hash>/<nome>.
func ObjectKey(sha string) string { return "originais/" + sha }

// Tipos de texto guardados com o charset detectado, para o navegador mostrar o original certo.
const (
	MimeTextUTF8   = "text/plain; charset=utf-8"
	MimeTextLatin1 = "text/plain; charset=windows-1252"
)

// DetectContentType decide o tipo pelo conteúdo [09 §5].
func DetectContentType(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return MimePNG, nil
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return MimeJPEG, nil
	}
	// PDF pela assinatura; texto sem byte nulo, UTF-8 ou latin-1 imprimível (o mesmo do PGDAS-D).
	mime, err := pdftext.DetectMIME(data)
	if err != nil || mime != pdftext.MimeText {
		return mime, err
	}
	if utf8.Valid(data) {
		return MimeTextUTF8, nil
	}
	return MimeTextLatin1, nil
}

// FileTooLarge é a recusa do arquivo acima do limite (413).
func FileTooLarge() error {
	return apperr.New(413, codeTooLarge, "Arquivo acima de 20 MB.")
}

// storageUnavailable é a recusa sem gravar nada quando o MinIO não responde ou não está
// configurado [11 §9 "MinIO fora"; 06 §10].
func storageUnavailable(err error) error {
	return apperr.New(503, codeStorage, "O armazenamento dos originais (MinIO) não está disponível. Nada foi gravado; tente de novo.").Wrap(err)
}

// UploadInput é um envio: o arquivo e, opcionalmente, a empresa e a competência.
type UploadInput struct {
	FileName   string
	Data       []byte
	CompanyID  string
	Competence string
}

// UploadResult é a resposta do envio (202).
type UploadResult struct {
	DocumentID string  `json:"document_id"`
	JobID      *string `json:"job_id"`
	Status     string  `json:"status"`
}

type triagePayload struct {
	DocumentID string `json:"document_id"`
}

// Upload guarda o original e enfileira a triagem sem esperar o processamento [AC-013, AC-014].
// O mesmo arquivo de novo é recusado (409 DUPLICATE_DOCUMENT), como na origem.
func (s *Service) Upload(ctx context.Context, in UploadInput) (UploadResult, error) {
	if len(in.Data) > MaxFileSize {
		return UploadResult{}, FileTooLarge()
	}
	contentType, err := DetectContentType(in.Data)
	if err != nil {
		return UploadResult{}, apperr.New(415, codeUnsupported, "Envie um PDF, uma imagem (PNG ou JPEG) ou um texto (CSV ou TXT).")
	}
	companyID, comp, err := s.target(ctx, in)
	if err != nil {
		return UploadResult{}, err
	}
	sum := sha256.Sum256(in.Data)
	sha := hex.EncodeToString(sum[:])
	q := store.New(s.pool)
	if err := s.duplicate(ctx, q, sha); err != nil {
		return UploadResult{}, err
	}
	status := StatusPending
	if companyID != nil && comp != nil {
		// Documento tardio: com o Contábil fechado na competência, o original fica guardado, mas
		// parado como pendência, sem processamento (ingerir_arquivo da origem; F-009).
		closed, err := reconciliation.AccountingClosed(ctx, s.pool, *companyID, *comp)
		if err != nil {
			return UploadResult{}, err
		}
		if closed {
			status = StatusClosedPeriod
		}
	}
	if s.storage == nil || !s.storage.Configured() {
		return UploadResult{}, storageUnavailable(objectstore.ErrNotConfigured)
	}
	key := ObjectKey(sha)
	put, err := s.storage.Put(ctx, key, in.Data, contentType)
	if err != nil {
		s.logger.Warn("conciliação: original não gravado no MinIO", "erro", err.Error())
		if errors.Is(err, objectstore.ErrUnavailable) || errors.Is(err, objectstore.ErrNotConfigured) || errors.Is(err, objectstore.ErrDenied) {
			return UploadResult{}, storageUnavailable(err)
		}
		return UploadResult{}, fmt.Errorf("conciliação: gravar o original: %w", err)
	}
	var version *string
	if put.VersionID != "" {
		version = &put.VersionID
	}
	name := filestore.SanitizeName(in.FileName, maxFileName)
	size := int64(len(in.Data))
	uid := user.FromContext(ctx)
	var competenceDate *time.Time
	if comp != nil {
		d := comp.FirstDay()
		competenceDate = &d
	}
	out := UploadResult{Status: status}
	duplicate := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := store.New(tx)
		id, err := tq.InsertReconciliationDocument(ctx, store.InsertReconciliationDocumentParams{
			CompanyID: companyID, Competence: competenceDate, Bucket: s.storage.Bucket(), ObjectKey: key,
			ObjectVersionID: version, Sha256: sha, FileName: name, ContentType: contentType, SizeBytes: &size,
			Status: status, UploadedBy: &uid, Now: s.clock.Now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			duplicate = true // outro envio do mesmo arquivo gravou antes
			return nil
		}
		if err != nil {
			return err
		}
		out.DocumentID = id
		if status != StatusPending {
			return nil
		}
		job, _, err := s.queue.Enqueue(ctx, tx, jobs.Spec{Kind: KindTriage, Payload: triagePayload{DocumentID: id}, DedupeKey: id})
		if err != nil {
			return err
		}
		out.JobID = &job.ID
		return tq.SetReconciliationDocumentJob(ctx, store.SetReconciliationDocumentJobParams{JobID: &job.ID, ID: id})
	})
	if err != nil {
		// O original fica no bucket: é imutável e endereçado pelo hash; um novo envio do mesmo
		// arquivo grava outra versão da mesma chave.
		return UploadResult{}, err
	}
	if duplicate {
		if err := s.duplicate(ctx, q, sha); err != nil {
			return UploadResult{}, err
		}
		return UploadResult{}, apperr.Conflict(codeDuplicate, "Este arquivo já foi enviado.")
	}
	return out, nil
}

// target valida a empresa e a competência opcionais do envio.
func (s *Service) target(ctx context.Context, in UploadInput) (*string, *competence.Competence, error) {
	var companyID *string
	var comp *competence.Competence
	if v := strings.TrimSpace(in.CompanyID); v != "" {
		if !httpx.IsUUID(v) {
			return nil, nil, validate.Field("company_id", "Empresa inválida.")
		}
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM companies WHERE id = $1)`, v).Scan(&exists); err != nil {
			return nil, nil, err
		}
		if !exists {
			return nil, nil, validate.Field("company_id", "Empresa não encontrada.")
		}
		companyID = &v
	}
	if v := strings.TrimSpace(in.Competence); v != "" {
		c, err := competence.Parse(v)
		if err != nil {
			return nil, nil, validate.Field("competence", "Competência inválida: use AAAA-MM.")
		}
		comp = &c
	}
	return companyID, comp, nil
}

// duplicate recusa o arquivo já enviado, apontando o documento existente.
func (s *Service) duplicate(ctx context.Context, q *store.Queries, sha string) error {
	id, err := q.GetReconciliationDocumentIDBySha(ctx, sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return apperr.Conflict(codeDuplicate, "Este arquivo já foi enviado.").WithDetails(map[string]any{"document_id": id})
}

// --- Consulta ---

// JobState é o estado do job que processa o documento [AC-014].
type JobState struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	Attempts      int32   `json:"attempts"`
	MaxAttempts   int32   `json:"max_attempts"`
	LastErrorCode *string `json:"last_error_code"`
}

// Document é o documento da Conciliação na API.
type Document struct {
	ID               string          `json:"id"`
	Status           string          `json:"status"`
	CompanyID        *string         `json:"company_id"`
	Competence       *string         `json:"competence"`
	FileName         string          `json:"file_name"`
	ContentType      string          `json:"content_type"`
	SizeBytes        *int64          `json:"size_bytes"`
	SHA256           string          `json:"sha256"`
	ObjectVersionID  *string         `json:"object_version_id"`
	Kind             *string         `json:"kind"`
	Bank             *string         `json:"bank"`
	Branch           *string         `json:"branch"`
	Account          *string         `json:"account"`
	PeriodStart      *string         `json:"period_start"`
	PeriodEnd        *string         `json:"period_end"`
	TriageConfidence *string         `json:"triage_confidence"`
	Diagnosis        json.RawMessage `json:"diagnosis"`
	Job              *JobState       `json:"job"`
	FileURL          string          `json:"file_url"`
	UploadedBy       *string         `json:"uploaded_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// DocumentList é uma página de documentos com a contagem por status de todos os documentos.
type DocumentList struct {
	Items         []Document       `json:"items"`
	Total         int64            `json:"total"`
	CountByStatus map[string]int64 `json:"count_by_status"`
}

func dateText(t *time.Time, layout string) *string {
	if t == nil {
		return nil
	}
	s := t.Format(layout)
	return &s
}

func toDocument(r store.GetReconciliationDocumentRow) Document {
	d := Document{
		ID: r.ID, Status: r.Status, CompanyID: r.CompanyID, Competence: dateText(r.Competence, "2006-01"),
		FileName: r.FileName, ContentType: r.ContentType, SizeBytes: r.SizeBytes, SHA256: r.Sha256,
		ObjectVersionID: r.ObjectVersionID, Kind: r.Kind, Bank: r.Bank, Branch: r.Branch, Account: r.Account,
		PeriodStart: dateText(r.PeriodStart, "2006-01-02"), PeriodEnd: dateText(r.PeriodEnd, "2006-01-02"),
		Diagnosis: json.RawMessage("null"), FileURL: "/api/v1/reconciliation/documents/" + r.ID + "/file",
		UploadedBy: r.UploadedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.TriageConfidence != nil {
		v := r.TriageConfidence.String()
		d.TriageConfidence = &v
	}
	if len(r.Diagnosis) > 0 {
		d.Diagnosis = json.RawMessage(r.Diagnosis)
	}
	if r.JobID != nil && r.JobStatus != nil {
		d.Job = &JobState{ID: *r.JobID, Status: *r.JobStatus, LastErrorCode: r.JobLastErrorCode}
		if r.JobAttempts != nil {
			d.Job.Attempts = *r.JobAttempts
		}
		if r.JobMaxAttempts != nil {
			d.Job.MaxAttempts = *r.JobMaxAttempts
		}
	}
	return d
}

// ListInput são os filtros da lista [06 §4].
type ListInput struct {
	Status     string
	CompanyID  string
	Kind       string
	Bank       string
	Competence string
	Limit      int32
	Offset     int32
}

// Statuses são os estados do documento (D-020).
var Statuses = []string{"pendente", "triado", "extraido", "importado", "revisao_triagem", "revisao_extracao", "duplicado", "pendencia_periodo_fechado"}

// Kinds são os tipos definidos pela triagem.
var Kinds = []string{"extrato", "comprovante", "desconhecido"}

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func optional(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// List lista os documentos, mais recentes primeiro.
func (s *Service) List(ctx context.Context, in ListInput) (DocumentList, error) {
	p := store.ListReconciliationDocumentsParams{Status: optional(in.Status), CompanyID: optional(in.CompanyID), Kind: optional(in.Kind), Bank: optional(in.Bank)}
	if p.Status != nil && !oneOf(*p.Status, Statuses) {
		return DocumentList{}, validate.Field("status", "Status inválido.")
	}
	if p.CompanyID != nil && !httpx.IsUUID(*p.CompanyID) {
		return DocumentList{}, validate.Field("company_id", "Empresa inválida.")
	}
	if p.Kind != nil && !oneOf(*p.Kind, Kinds) {
		return DocumentList{}, validate.Field("kind", "Tipo inválido: use extrato, comprovante ou desconhecido.")
	}
	if v := optional(in.Competence); v != nil {
		c, err := competence.Parse(*v)
		if err != nil {
			return DocumentList{}, validate.Field("competence", "Competência inválida: use AAAA-MM.")
		}
		d := c.FirstDay()
		p.Competence = &d
	}
	p.RowLimit = in.Limit
	if p.RowLimit == 0 {
		p.RowLimit = 50
	}
	if p.RowLimit < 1 || p.RowLimit > 200 {
		return DocumentList{}, validate.Field("limit", "Use um limite entre 1 e 200.")
	}
	if in.Offset < 0 {
		return DocumentList{}, validate.Field("offset", "O deslocamento não pode ser negativo.")
	}
	p.RowOffset = in.Offset
	q := store.New(s.pool)
	rows, err := q.ListReconciliationDocuments(ctx, p)
	if err != nil {
		return DocumentList{}, err
	}
	total, err := q.CountReconciliationDocuments(ctx, store.CountReconciliationDocumentsParams{
		Status: p.Status, CompanyID: p.CompanyID, Kind: p.Kind, Bank: p.Bank, Competence: p.Competence,
	})
	if err != nil {
		return DocumentList{}, err
	}
	counts, err := q.CountReconciliationDocumentsByStatus(ctx)
	if err != nil {
		return DocumentList{}, err
	}
	out := DocumentList{Items: make([]Document, 0, len(rows)), Total: total, CountByStatus: map[string]int64{}}
	for _, r := range rows {
		out.Items = append(out.Items, toDocument(store.GetReconciliationDocumentRow(r)))
	}
	for _, c := range counts {
		out.CountByStatus[c.Status] = c.Total
	}
	return out, nil
}

func notFound() error {
	return apperr.NotFound(codeNotFound, "Documento da Conciliação não encontrado.")
}

// Get devolve o documento.
func (s *Service) Get(ctx context.Context, id string) (Document, error) {
	row, err := store.New(s.pool).GetReconciliationDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, notFound()
	}
	if err != nil {
		return Document{}, err
	}
	return toDocument(row), nil
}

// Open abre o original gravado (a versão do envio) para leitura; o chamador fecha o corpo.
func (s *Service) Open(ctx context.Context, id string) (Document, objectstore.Object, error) {
	row, err := store.New(s.pool).GetReconciliationDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, objectstore.Object{}, notFound()
	}
	if err != nil {
		return Document{}, objectstore.Object{}, err
	}
	if s.storage == nil || !s.storage.Configured() {
		return Document{}, objectstore.Object{}, storageUnavailable(objectstore.ErrNotConfigured)
	}
	version := ""
	if row.ObjectVersionID != nil {
		version = *row.ObjectVersionID
	}
	obj, err := s.storage.Get(ctx, row.ObjectKey, version)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return Document{}, objectstore.Object{}, fmt.Errorf("conciliação: original do documento %s ausente no bucket: %w", id, err)
		}
		return Document{}, objectstore.Object{}, storageUnavailable(err)
	}
	return toDocument(row), obj, nil
}
