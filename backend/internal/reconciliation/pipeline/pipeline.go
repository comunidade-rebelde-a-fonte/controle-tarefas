// Package pipeline é o processamento dos documentos da Conciliação no worker [REQ-019 a REQ-023;
// TASK-016, D-027; TASK-017, D-028]: porte de app/servicos/{pipeline,importacao,processar_local,
// comprovantes}.py e app/agentes/{triador,extrator_extrato,extrator_comprovante,llm}.py do
// Conciliador (commit 2bc6e26).
//
// O agente interpreta; o código valida, calcula e persiste:
//   - job reconciliation_triage (HandleTriage): os leitores determinísticos do texto nativo do PDF
//     (o atalho processar_local da origem, agora no job: extrato e comprovante) e, se eles não
//     resolverem, o triador;
//   - job statement_extract (HandleExtract): o leitor determinístico do CSV ou o extrator com
//     até 3 tentativas (texto nativo, render 150 DPI, render 300 DPI com extrator_forte),
//     feedback exato do validador e playbook do banco;
//   - a importação é atômica e só existe com o portão aritmético aprovado (R$ 0,00) [AC-015];
//   - job receipt_extract (HandleReceipt): o extrator de comprovante (texto primeiro; render e
//     visão sem texto ou com campo fraco), as regras de campo e a duplicata sinalizada; campo
//     essencial fraco deixa o lançamento parcial, na fila de revisão [REQ-023].
//
// Tudo é gravado na transação do job; o log de extração guarda cada tentativa [CC-10].
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/encoding/charmap"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/decimal"
	"controle/internal/platform/objectstore"
	"controle/internal/platform/pdftext"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/statement"
	"controle/internal/store"
)

// Tipos de job do processamento [04 §7]. A triagem nasce no envio (documents.KindTriage).
const (
	KindTriage           = reconciliation.JobTriage
	KindStatementExtract = reconciliation.JobStatementExtract
	// KindReceiptExtract é o comprovante triado (TASK-017, ReceiptHandler).
	KindReceiptExtract = reconciliation.JobReceiptExtract
)

// Estados do documento (contrato congelado da origem, D-020).
const (
	StatusPending        = reconciliation.DocPending
	StatusTriaged        = reconciliation.DocTriaged
	StatusImported       = reconciliation.DocImported
	StatusTriageReview   = reconciliation.DocTriageReview
	StatusExtractReview  = reconciliation.DocExtractReview
	StatusClosedPeriod   = reconciliation.DocClosedPeriod
	triggerUpload        = "upload"
	agentTriador         = "triador"
	agentExtrator        = "extrator"
	aliasLocalReader     = "leitor_local"
	maxTriagePages       = 3
	maxTriageChars       = 12000
	maxExtractChars      = 20000
	maxRenderPages       = 10
	maxOriginalBytes     = 20 << 20 // o limite do envio (documents.MaxFileSize)
	triagePromptName     = "conciliacao/triador"
	extractPromptName    = "conciliacao/extrator_extrato"
	correctionPromptName = "conciliacao/extrator_extrato_correcao"
	playbookPromptName   = "conciliacao/playbook"
)

// Códigos de falha do job.
const (
	CodeInvalidPayload   = "INVALID_PAYLOAD"
	CodeDocumentNotFound = "RECONCILIATION_DOCUMENT_NOT_FOUND"
	CodeFileMissing      = "RECONCILIATION_FILE_MISSING"
	CodeTriageOutput     = "TRIAGE_INVALID_OUTPUT"
)

// Motivos gravados no diagnóstico do documento em revisão ou pendência.
const (
	ReasonLowConfidence    = "confianca_baixa"
	ReasonUnknownKind      = "tipo_desconhecido"
	ReasonUnreadable       = "documento_ilegivel"
	ReasonAgentDisabled    = "agente_desligado"
	ReasonInvalidOutput    = "saida_invalida"
	ReasonBalanceMismatch  = "saldo_nao_fecha"
	ReasonMissingMetadata  = "metadados_ausentes"
	ReasonUnreadableCSV    = "csv_ilegivel"
	ReasonNoCompany        = "empresa_nao_identificada"
	ReasonAccountOtherFirm = "conta_de_outra_empresa"
	ReasonClosedPeriod     = "periodo_fechado"
	ReasonWeakFields       = "campos_abaixo_limiar"
)

// Storage é o armazenamento dos originais (objectstore.Store).
type Storage interface {
	Get(ctx context.Context, key, versionID string) (objectstore.Object, error)
}

// Gateway é o cliente do LiteLLM (litellm.Client).
type Gateway interface {
	Chat(ctx context.Context, req litellm.Request) (litellm.Response, error)
}

// Resolver resolve o alias no início da corrida (aliases.Resolver).
type Resolver interface {
	Resolve(ctx context.Context, db store.DBTX, a ai.Alias) (ai.Resolved, error)
}

// Config são os limites do processamento.
type Config struct {
	// TriageThreshold é o LIMIAR_TRIAGEM (0,70 na origem).
	TriageThreshold decimal.Decimal
	// FieldThreshold é o LIMIAR_CAMPO do comprovante (0,70 na origem).
	FieldThreshold decimal.Decimal
	// AgentOnly pula os leitores determinísticos: só a avaliação do extrator contra o golden
	// (avaliar_extrator da origem) usa, para medir o agente nos PDFs que o leitor leria.
	AgentOnly bool
}

// Processor executa os jobs do processamento no worker.
type Processor struct {
	clock    clock.Clock
	tracer   *tracer.Tracer
	queue    *jobs.Queue
	storage  Storage
	pdf      *pdftext.Extractor
	gateway  Gateway
	resolver Resolver
	cfg      Config
	logger   *slog.Logger
}

// New cria o processador. clk é o tempo operacional (o da fila).
func New(clk clock.Clock, tr *tracer.Tracer, queue *jobs.Queue, storage Storage, pdf *pdftext.Extractor,
	gateway Gateway, resolver Resolver, cfg Config, logger *slog.Logger) *Processor {
	return &Processor{clock: clk, tracer: tr, queue: queue, storage: storage, pdf: pdf, gateway: gateway,
		resolver: resolver, cfg: cfg, logger: logger}
}

type payload struct {
	DocumentID string `json:"document_id"`
}

func documentID(job jobs.Claimed) (string, error) {
	var p payload
	if err := json.Unmarshal(job.Payload, &p); err != nil || p.DocumentID == "" {
		return "", jobs.Permanent(CodeInvalidPayload, fmt.Errorf("%s: payload sem document_id", job.Kind))
	}
	return p.DocumentID, nil
}

// lockDocument trava o documento do job; inexistente é falha permanente.
func lockDocument(ctx context.Context, q *store.Queries, id string) (store.ReconciliationDocument, error) {
	doc, err := q.GetReconciliationDocumentForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return doc, jobs.Permanent(CodeDocumentNotFound, errors.New("documento da conciliação inexistente"))
	}
	return doc, err
}

// --- O original e o conteúdo dele ---

// Tipos do original, pelo conteúdo (documents.DetectContentType).
const (
	mimePDF  = "application/pdf"
	mimePNG  = "image/png"
	mimeJPEG = "image/jpeg"
)

// source é o original lido do MinIO, com o texto já extraído.
type source struct {
	data []byte
	mime string // application/pdf, image/png, image/jpeg ou text/plain
	// PDF: o texto de todas as páginas (extrair_texto_pdf) ou o erro de leitura.
	pages     []string
	pageCount int
	pdfErr    error
	// Texto (CSV/TXT): o conteúdo decodificado pelo charset do envio.
	text string
}

func (s source) isPDF() bool    { return s.mime == mimePDF }
func (s source) readable() bool { return s.pdfErr == nil }
func (s source) isImage() bool  { return s.mime == mimePNG || s.mime == mimeJPEG }
func (s source) isText() bool   { return strings.HasPrefix(s.mime, "text/plain") }

// hasText indica se o PDF tem camada de texto (alguma página com texto).
func (s source) hasText() bool {
	for _, p := range s.pages {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	return false
}

// load lê a versão gravada do original e o texto dele. Armazenamento fora do ar é falha
// transitória (o job tenta de novo); original ausente, permanente.
func (p *Processor) load(ctx context.Context, doc store.ReconciliationDocument) (source, error) {
	version := ""
	if doc.ObjectVersionID != nil {
		version = *doc.ObjectVersionID
	}
	obj, err := p.storage.Get(ctx, doc.ObjectKey, version)
	switch {
	case errors.Is(err, objectstore.ErrNotFound):
		return source{}, jobs.Permanent(CodeFileMissing, errors.New("original ausente no MinIO"))
	case err != nil:
		return source{}, fmt.Errorf("ler o original no MinIO: %w", err)
	}
	defer func() { _ = obj.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(obj.Body, maxOriginalBytes+1))
	if err != nil {
		return source{}, fmt.Errorf("ler o original no MinIO: %w", err)
	}
	if len(data) > maxOriginalBytes {
		return source{}, jobs.Permanent(CodeFileMissing, errors.New("original maior que o limite do envio"))
	}
	src := source{data: data, mime: doc.ContentType}
	switch {
	case src.isPDF():
		d, err := p.pdf.Pages(ctx, src.data, 0)
		switch {
		case err == nil:
			src.pages, src.pageCount = d.Pages, d.PageCount
		case errors.Is(err, pdftext.ErrUnreadable) || errors.Is(err, pdftext.ErrLimits):
			src.pdfErr = err
		default:
			return source{}, err
		}
	case src.isText():
		src.text = decodeText(src.data, doc.ContentType)
	}
	return src, nil
}

// decodeText decodifica o texto pelo charset detectado no envio, sem o BOM do UTF-8.
func decodeText(data []byte, contentType string) string {
	if strings.Contains(contentType, "windows-1252") {
		if s, err := charmap.Windows1252.NewDecoder().Bytes(data); err == nil {
			return string(s)
		}
	}
	return strings.TrimPrefix(string(data), "\xef\xbb\xbf")
}

// truncateRunes corta o texto em n caracteres, como o fatiamento [:n] do Python.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for k := range s {
		if i == n {
			return s[:k]
		}
		i++
	}
	return s
}

// --- Gravação ---

func jsonBytes(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("pipeline: JSON: %v", err)) // só tipos do próprio pacote
	}
	return b
}

// setState muda o estado do documento com o diagnóstico (nil apaga) e, opcional, o job atual.
func (p *Processor) setState(ctx context.Context, q *store.Queries, docID, status string, diagnosis any, jobID *string) error {
	var diag []byte
	if diagnosis != nil {
		diag = jsonBytes(diagnosis)
	}
	return q.SetReconciliationDocumentState(ctx, store.SetReconciliationDocumentStateParams{
		Status: status, Diagnosis: diag, JobID: jobID, Now: p.clock.Now(), ID: docID,
	})
}

// enqueue enfileira o próximo job do documento na transação e o aponta no documento.
func (p *Processor) enqueue(ctx context.Context, tx pgx.Tx, q *store.Queries, docID, kind, status string) (string, error) {
	job, _, err := p.queue.Enqueue(ctx, tx, jobs.Spec{Kind: kind, Payload: payload{DocumentID: docID}, DedupeKey: docID})
	if err != nil {
		return "", err
	}
	return job.ID, p.setState(ctx, q, docID, status, nil, &job.ID)
}

// logEntry é uma linha do log de extração [CC-10].
type logEntry struct {
	alias         string
	attempt       int
	resolvedModel string
	prompt        promptRef
	rawOutput     string
	validator     *statement.Result
	check         map[string]any // regras de campo do comprovante (no lugar do validador do extrato)
	tokensIn      *int32
	tokensOut     *int32
	costUSD       *decimal.Decimal
	latency       time.Duration
	traceID       string
	errText       string
}

// clamp32 converte para int32 dentro da faixa (ordem, página e tentativas já vêm limitadas pelo
// schema; a latência é limitada aqui).
func clamp32(v int64) int32 {
	switch {
	case v < math.MinInt32:
		return math.MinInt32
	case v > math.MaxInt32:
		return math.MaxInt32
	}
	return int32(v)
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (p *Processor) writeLog(ctx context.Context, q *store.Queries, doc store.ReconciliationDocument, jobID string, e logEntry) error {
	params := store.InsertExtractionLogParams{
		CompanyID: doc.CompanyID, DocumentID: &doc.ID, FileSha256: &doc.Sha256, Alias: e.alias,
		ResolvedModel: optString(e.resolvedModel), PromptHash: optString(e.prompt.hash),
		PromptVersion: optString(e.prompt.version), RawOutput: optString(e.rawOutput), Attempt: clamp32(int64(e.attempt)),
		TokensIn: e.tokensIn, TokensOut: e.tokensOut, CostUsd: e.costUSD, TraceID: optString(e.traceID),
		JobID: optString(jobID), Error: optString(e.errText), Now: p.clock.Now(),
	}
	switch {
	case e.validator != nil:
		params.ValidatorResult = jsonBytes(e.validator)
	case e.check != nil:
		params.ValidatorResult = jsonBytes(e.check)
	}
	if e.latency > 0 {
		ms := clamp32(e.latency.Milliseconds())
		params.LatencyMs = &ms
	}
	_, err := q.InsertExtractionLog(ctx, params)
	return err
}
