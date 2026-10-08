package inbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai/tracer"
	"controle/internal/company"
	"controle/internal/factorr"
	"controle/internal/factorr/pgdas"
	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/cnpj"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/filestore"
	"controle/internal/platform/httpx"
	"controle/internal/platform/pdftext"
	"controle/internal/platform/validate"
	"controle/internal/store"
	"controle/internal/user"
)

const (
	codeNotFound      = "PGDAS_DOCUMENT_NOT_FOUND"
	msgNotFound       = "Documento não encontrado."
	codeInvalidState  = "PGDAS_INVALID_STATE"
	codeNoValidCNPJ   = "PGDAS_NO_VALID_CNPJ"
	codeCNPJMismatch  = "PGDAS_CNPJ_MISMATCH"
	codeUnsupported   = "UNSUPPORTED_FILE"
	codeTooLarge      = "FILE_TOO_LARGE"
	codeUnavailable   = "PGDAS_FILE_UNAVAILABLE"
	maxOriginalName   = 255
	minRejectReason   = 3
	maxRejectReason   = 500
	defaultListLimit  = 50
	maxListLimit      = 200
	errMissingInStore = "o arquivo original não está no armazenamento"
)

func notFound() error { return apperr.NotFound(codeNotFound, msgNotFound) }

func invalidState(status, action string) error {
	return apperr.Conflict(codeInvalidState, fmt.Sprintf("Documento em '%s' não pode ser %s.", status, action)).
		WithDetails(map[string]any{"status": status})
}

// Service é o inbox na API: upload, consulta e as decisões do analista.
type Service struct {
	pool      *pgxpool.Pool
	clock     clock.Clock
	tracer    *tracer.Tracer
	queue     *jobs.Queue
	storage   Storage
	companies *company.Service
	logger    *slog.Logger
	// beforeCreateRun é um gancho de teste entre as checagens e a corrida do cadastro pelo
	// extrato (simula outro cadastro do mesmo CNPJ no meio).
	beforeCreateRun func()
}

// NewService cria o serviço. clk é o tempo operacional (o da fila e do tracer).
func NewService(pool *pgxpool.Pool, clk clock.Clock, tr *tracer.Tracer, queue *jobs.Queue, storage Storage, companies *company.Service, logger *slog.Logger) *Service {
	return &Service{pool: pool, clock: clk, tracer: tr, queue: queue, storage: storage, companies: companies, logger: logger}
}

// --- Upload ---

// FileTooLarge é o erro do arquivo acima de 10 MB (413, como a origem).
func FileTooLarge() error {
	return apperr.New(413, codeTooLarge, "Arquivo acima de 10 MB.")
}

// Upload guarda o original e enfileira o processamento [REQ-046, AC-014]. O mesmo arquivo
// enviado de novo devolve o documento existente (duplicate), sem novo job, como na origem.
func (s *Service) Upload(ctx context.Context, name string, data []byte) (UploadResult, error) {
	if len(data) > MaxFileSize {
		return UploadResult{}, FileTooLarge()
	}
	mime, err := pdftext.DetectMIME(data)
	if err != nil {
		return UploadResult{}, apperr.New(415, codeUnsupported, "Envie um PDF ou TXT do extrato do PGDAS-D.")
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	q := store.New(s.pool)
	if existing, ok, err := s.existing(ctx, q, sha); err != nil || ok {
		return existing, err
	}
	created, err := s.storage.Save(sha, data)
	if err != nil {
		return UploadResult{}, fmt.Errorf("inbox: gravar o original: %w", err)
	}
	var out UploadResult
	duplicate := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tq := store.New(tx)
		var original *string // sem nome enviado, fica nulo (como a origem)
		if strings.TrimSpace(name) != "" {
			n := filestore.SanitizeName(name, maxOriginalName)
			original = &n
		}
		uid := user.FromContext(ctx)
		id, err := tq.InsertPgdasDocument(ctx, store.InsertPgdasDocumentParams{
			Sha256: sha, FileRef: fileRef(sha), Mime: mime, SizeBytes: size32(len(data)),
			OriginalName: original, UploadedBy: &uid, Now: s.clock.Now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			duplicate = true // outro envio do mesmo arquivo gravou antes
			return nil
		}
		if err != nil {
			return err
		}
		job, _, err := s.queue.Enqueue(ctx, tx, jobs.Spec{Kind: KindParse, Payload: parsePayload{DocumentID: id}, DedupeKey: id})
		if err != nil {
			return err
		}
		if err := tq.SetPgdasDocumentJob(ctx, store.SetPgdasDocumentJobParams{JobID: &job.ID, ID: id}); err != nil {
			return err
		}
		out = UploadResult{DocumentID: id, JobID: &job.ID, Status: StatusReceived}
		return nil
	})
	if err != nil {
		// Sem documento, o arquivo não pode ficar órfão no disco (revisão B6 da origem).
		if _, ok, lookupErr := s.existing(context.WithoutCancel(ctx), q, sha); created && lookupErr == nil && !ok {
			if rmErr := s.storage.Remove(sha); rmErr != nil {
				s.logger.Error("inbox: arquivo órfão não removido", "erro", rmErr.Error())
			}
		}
		return UploadResult{}, err
	}
	if duplicate {
		existing, _, err := s.existing(ctx, q, sha)
		return existing, err
	}
	return out, nil
}

// size32 é o tamanho do arquivo na coluna integer (o upload já limita a 10 MB).
func size32(n int) int32 {
	switch {
	case n < 0:
		return 0
	case n > math.MaxInt32:
		return math.MaxInt32
	}
	return int32(n)
}

// existing devolve o documento do mesmo arquivo, se houver.
func (s *Service) existing(ctx context.Context, q *store.Queries, sha string) (UploadResult, bool, error) {
	id, err := q.GetPgdasDocumentIDBySha(ctx, sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return UploadResult{}, false, nil
	}
	if err != nil {
		return UploadResult{}, false, err
	}
	row, err := q.GetPgdasDocument(ctx, id)
	if err != nil {
		return UploadResult{}, false, err
	}
	return UploadResult{DocumentID: row.ID, JobID: row.JobID, Status: row.Status, Duplicate: true}, true, nil
}

// --- Consulta ---

// ListInput filtra a listagem.
type ListInput struct {
	Status string
	Limit  int32
	Offset int32
}

// List lista os documentos, mais recentes primeiro, com a contagem por status.
func (s *Service) List(ctx context.Context, in ListInput) (DocumentList, error) {
	var status *string
	if in.Status != "" {
		valid := false
		for _, st := range Statuses {
			valid = valid || st == in.Status
		}
		if !valid {
			return DocumentList{}, validate.Field("status", "Status inválido: use received, parsed, needs_review, linked ou rejected.")
		}
		status = &in.Status
	}
	limit := in.Limit
	if limit == 0 {
		limit = defaultListLimit
	}
	if limit < 1 || limit > maxListLimit {
		return DocumentList{}, validate.Field("limit", "Use um limite entre 1 e 200.")
	}
	if in.Offset < 0 {
		return DocumentList{}, validate.Field("offset", "O deslocamento não pode ser negativo.")
	}
	q := store.New(s.pool)
	rows, err := q.ListPgdasDocuments(ctx, store.ListPgdasDocumentsParams{Status: status, RowLimit: limit, RowOffset: in.Offset})
	if err != nil {
		return DocumentList{}, err
	}
	total, err := q.CountPgdasDocuments(ctx, status)
	if err != nil {
		return DocumentList{}, err
	}
	counts, err := q.CountPgdasDocumentsByStatus(ctx)
	if err != nil {
		return DocumentList{}, err
	}
	out := DocumentList{Items: make([]Document, 0, len(rows)), Total: total, ContagemPorStatus: map[string]int64{}}
	for _, r := range rows {
		d, err := toDocument(store.GetPgdasDocumentRow(r))
		if err != nil {
			return DocumentList{}, err
		}
		out.Items = append(out.Items, d)
	}
	for _, c := range counts {
		out.ContagemPorStatus[c.Status] = c.Total
	}
	return out, nil
}

// Get devolve o documento.
func (s *Service) Get(ctx context.Context, id string) (Document, error) {
	row, err := store.New(s.pool).GetPgdasDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, notFound()
	}
	if err != nil {
		return Document{}, err
	}
	return toDocument(row)
}

// File devolve o original (download autenticado, nunca por diretório público).
func (s *Service) File(ctx context.Context, id string) (data []byte, mime string, err error) {
	row, err := store.New(s.pool).GetPgdasDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", notFound()
	}
	if err != nil {
		return nil, "", err
	}
	data, err = s.storage.Read(row.Sha256)
	if err != nil {
		s.logger.Warn("inbox: original indisponível", "document_id", id, "erro", errMissingInStore)
		return nil, "", apperr.NotFound(codeUnavailable, "Arquivo original indisponível.")
	}
	return data, row.Mime, nil
}

func decodeStored(raw []byte) (pgdas.Stored, error) {
	var st pgdas.Stored
	if len(raw) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return st, fmt.Errorf("inbox: parsed_json: %w", err)
	}
	return st, nil
}

// suggestion oferece o cadastro só para CNPJ válido fora da carteira (sugestao_cadastro).
func suggestion(status string, motivo *string, st pgdas.Stored) *Suggestion {
	if status != StatusNeedsReview || motivo == nil || *motivo != reasonCNPJNotFound || st.CNPJ == nil || !pgdas.CNPJValido(*st.CNPJ) {
		return nil
	}
	return &Suggestion{
		CNPJ: *st.CNPJ, CNPJFormatado: cnpj.Format(*st.CNPJ),
		NomeEmpresarial: st.Identificacao.NomeEmpresarial, SujeitaFatorR: st.Identificacao.SujeitaFatorR,
		InicioAtividade: st.Identificacao.InicioAtividade,
	}
}

func toDocument(r store.GetPgdasDocumentRow) (Document, error) {
	st, err := decodeStored(r.ParsedJson)
	if err != nil {
		return Document{}, err
	}
	d := Document{
		ID: r.ID, Status: r.Status, Motivo: r.Motivo, CompanyID: r.CompanyID, NomeOriginal: r.OriginalName,
		Mime: r.Mime, TamanhoBytes: r.SizeBytes,
		Campos: Fields{
			CNPJ: st.CNPJ, PA: st.PA, RBT12: st.RBT12, RPA: st.RPA, FS12: st.FS12, FatorR: st.FatorR, DAS: st.DAS, Anexo: st.Anexo,
		},
		ConfiancaCampos: st.ConfiancaCampos, Motivos: st.Motivos,
		Identificacao: Identification{
			NomeEmpresarial: st.Identificacao.NomeEmpresarial, SujeitaFatorR: st.Identificacao.SujeitaFatorR,
			InicioAtividade: st.Identificacao.InicioAtividade,
		},
		SeriesAnteriores: PriorSeries{
			Receitas: st.SeriesAnteriores.Receitas, Folhas: st.SeriesAnteriores.Folhas,
			ReceitasConferem: st.SeriesAnteriores.ReceitasConferem, FolhasConferem: st.SeriesAnteriores.FolhasConferem,
		},
		ParserVersion: r.ParserVersion, TraceID: r.TraceID,
		SugestaoCadastro: suggestion(r.Status, r.Motivo, st), Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
	if r.Confidence != nil {
		c := r.Confidence.Quantize(-4)
		d.Confianca = &c
	}
	if r.TextoAgente != "" {
		d.TextoAgente = &r.TextoAgente
	}
	if d.ConfiancaCampos == nil {
		d.ConfiancaCampos = map[string]string{}
	}
	if d.Motivos == nil {
		d.Motivos = []string{}
	}
	if d.SeriesAnteriores.Receitas == nil {
		d.SeriesAnteriores.Receitas = map[string]string{}
	}
	if d.SeriesAnteriores.Folhas == nil {
		d.SeriesAnteriores.Folhas = map[string]string{}
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
	return d, nil
}

// --- Decisões do analista (cada uma numa corrida do parser_pgdas) ---

func linkable(status string) bool { return status == StatusNeedsReview || status == StatusParsed }

// lockLinkable trava o documento e confere de novo o estado (corrida entre duas decisões).
func lockLinkable(ctx context.Context, q *store.Queries, id string) (store.FactorRPgdasDocument, pgdas.Stored, error) {
	doc, err := q.GetPgdasDocumentForUpdate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return doc, pgdas.Stored{}, notFound()
	}
	if err != nil {
		return doc, pgdas.Stored{}, err
	}
	if !linkable(doc.Status) {
		return doc, pgdas.Stored{}, invalidState(doc.Status, "vinculado")
	}
	st, err := decodeStored(doc.ParsedJson)
	return doc, st, err
}

// linkDecision vincula o documento à empresa na corrida aberta e grava a decisão
// (_decidir_vinculo da origem). Devolve o texto do vínculo.
func (s *Service) linkDecision(ctx context.Context, tx pgx.Tx, run *tracer.Run, doc store.FactorRPgdasDocument, st pgdas.Stored,
	co company.Company, kind string, extra map[string]any) (map[string]any, string, error) {
	q := store.New(tx)
	now := s.clock.Now()
	uid := user.FromContext(ctx)
	w := writer{q: q, now: now, userID: &uid, docID: doc.ID, companyID: co.ID}
	mov, p, err := w.apply(ctx, st)
	if err != nil {
		return nil, "", err
	}
	trace := run.TraceID()
	if err := q.SavePgdasDecision(ctx, store.SavePgdasDecisionParams{
		Status: StatusLinked, CompanyID: &co.ID, TraceID: &trace, Now: now, ID: doc.ID,
	}); err != nil {
		return nil, "", err
	}
	decision := map[string]any{
		"document_id": doc.ID, "status": StatusLinked, "company_id": co.ID, "movimento": string(mov),
		"competencia": st.PA, "meses_anteriores": p.asMap(), "origem_decisao": "analista",
	}
	for k, v := range extra {
		decision[k] = v
	}
	if err := run.RecordDecision(ctx, kind, decision); err != nil {
		return nil, "", err
	}
	return decision, linkedText(st, mov, co.LegalName, p), nil
}

// Link vincula manualmente um documento em revisão a uma empresa (vincular_manual) [AC-008].
func (s *Service) Link(ctx context.Context, id string, in LinkInput) (Document, error) {
	doc, err := s.Get(ctx, id)
	if err != nil {
		return Document{}, err
	}
	if !linkable(doc.Status) {
		return Document{}, invalidState(doc.Status, "vinculado")
	}
	if !httpx.IsUUID(in.CompanyID) {
		return Document{}, validate.Field("company_id", "Informe a empresa (id).")
	}
	co, err := s.companies.Get(ctx, in.CompanyID)
	if err != nil {
		return Document{}, err
	}
	uid := user.FromContext(ctx)
	_, err = s.tracer.Do(ctx, s.pool, tracer.Spec{
		Agent: AgentName, Trigger: TriggerManualLink, UserID: &uid, CompanyID: &co.ID,
		Input: map[string]any{"document_id": id, "company_id": co.ID},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		cur, st, err := lockLinkable(ctx, store.New(tx), id)
		if err != nil {
			return tracer.Result{}, err
		}
		if err := run.Span(tracer.SpanTool, map[string]any{"company_id": co.ID}, func(sp tracer.SpanHandle) error {
			sp.Output(map[string]any{"empresa": co.LegalName})
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		var decision map[string]any
		var text string
		if err := run.Span(tracer.SpanDecide, nil, func(sp tracer.SpanHandle) error {
			var err error
			decision, text, err = s.linkDecision(ctx, tx, run, cur, st, co, "pgdas_vinculo_manual", nil)
			sp.Output(decision)
			return err
		}); err != nil {
			return tracer.Result{}, err
		}
		text = factorr.WithDisclaimer(text)
		if err := run.Span(tracer.SpanRender, nil, func(sp tracer.SpanHandle) error {
			sp.Output(map[string]any{"texto": text})
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		if err := gold(ctx, s.tracer, tx, store.New(tx), run.TraceID(), st, co.ID, co.Cnpj); err != nil {
			return tracer.Result{}, err
		}
		return tracer.Result{Status: tracer.StatusOK, Decision: decision, Text: text}, nil
	})
	if err != nil {
		return Document{}, err
	}
	return s.Get(ctx, id)
}

// Review aplica a decisão de revisão do analista; hoje, a rejeição (rejeitar da origem).
func (s *Service) Review(ctx context.Context, id string, in ReviewInput) (Document, error) {
	if in.Decisao != DecisionReject {
		return Document{}, validate.Field("decisao", "Decisão inválida: use rejeitar.")
	}
	if n := utf8.RuneCountInString(in.Motivo); n < minRejectReason || n > maxRejectReason {
		return Document{}, validate.Field("motivo", "Informe o motivo da rejeição (de 3 a 500 caracteres).")
	}
	doc, err := s.Get(ctx, id)
	if err != nil {
		return Document{}, err
	}
	if doc.Status == StatusRejected || doc.Status == StatusLinked {
		return Document{}, invalidState(doc.Status, "rejeitado")
	}
	uid := user.FromContext(ctx)
	_, err = s.tracer.Do(ctx, s.pool, tracer.Spec{
		Agent: AgentName, Trigger: TriggerReject, UserID: &uid,
		Input: map[string]any{"document_id": id, "motivo": in.Motivo},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		q := store.New(tx)
		cur, err := q.GetPgdasDocumentForUpdate(ctx, id)
		if err != nil {
			return tracer.Result{}, err
		}
		if cur.Status == StatusRejected || cur.Status == StatusLinked {
			return tracer.Result{}, invalidState(cur.Status, "rejeitado")
		}
		var decision map[string]any
		if err := run.Span(tracer.SpanDecide, nil, func(sp tracer.SpanHandle) error {
			trace := run.TraceID()
			motivo := in.Motivo
			if err := q.SavePgdasDecision(ctx, store.SavePgdasDecisionParams{
				Status: StatusRejected, Motivo: &motivo, TraceID: &trace, Now: s.clock.Now(), ID: id,
			}); err != nil {
				return err
			}
			decision = map[string]any{"document_id": id, "status": StatusRejected, "motivo": in.Motivo}
			if err := run.RecordDecision(ctx, "pgdas_rejeicao", decision); err != nil {
				return err
			}
			sp.Output(decision)
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		return tracer.Result{Status: tracer.StatusOK, Decision: decision, Text: factorr.WithDisclaimer(rejectedText)}, nil
	})
	if err != nil {
		return Document{}, err
	}
	return s.Get(ctx, id)
}

// CreateCompany cadastra a empresa do extrato e vincula o documento numa transação só
// (cadastrar_e_vincular), sempre por pedido explícito do analista. As recusas previsíveis
// acontecem antes da corrida e não gravam nada.
func (s *Service) CreateCompany(ctx context.Context, id string, in CreateCompanyInput) (CreateCompanyResult, error) {
	doc, err := s.Get(ctx, id)
	if err != nil {
		return CreateCompanyResult{}, err
	}
	if !linkable(doc.Status) {
		return CreateCompanyResult{}, invalidState(doc.Status, "vinculado")
	}
	lido := doc.Campos.CNPJ
	if lido == nil || !pgdas.CNPJValido(*lido) {
		return CreateCompanyResult{}, apperr.Unprocessable(codeNoValidCNPJ, "O extrato não tem CNPJ válido para cadastrar a empresa.")
	}
	if cnpj.Sanitize(in.Empresa.Cnpj) != *lido {
		return CreateCompanyResult{}, apperr.Unprocessable(codeCNPJMismatch, "O CNPJ do cadastro precisa ser o mesmo lido no extrato.")
	}
	var inicio *competence.Competence
	if in.InicioAtividade != nil {
		c, err := competence.Parse(strings.TrimSpace(*in.InicioAtividade))
		if err != nil {
			return CreateCompanyResult{}, validate.Field("inicio_atividade", "Informe o início da atividade no formato AAAA-MM.")
		}
		inicio = &c
	}
	// Mesmas validações do cadastro de empresas, inclusive CNPJ já cadastrado (409).
	if err := s.companies.ValidateCreate(ctx, in.Empresa); err != nil {
		return CreateCompanyResult{}, err
	}
	sug := doc.Identificacao.SujeitaFatorR
	uid := user.FromContext(ctx)
	if s.beforeCreateRun != nil {
		s.beforeCreateRun()
	}
	var companyID string
	_, err = s.tracer.Do(ctx, s.pool, tracer.Spec{
		Agent: AgentName, Trigger: TriggerCreateCompany, UserID: &uid,
		Input: map[string]any{"document_id": id, "cnpj": *lido},
	}, func(ctx context.Context, tx pgx.Tx, run *tracer.Run) (tracer.Result, error) {
		cur, st, err := lockLinkable(ctx, store.New(tx), id)
		if err != nil {
			return tracer.Result{}, err
		}
		if err := run.Span(tracer.SpanTool, map[string]any{"cnpj": *lido}, func(sp tracer.SpanHandle) error {
			sp.Output(map[string]any{"cnpj_na_carteira": false})
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		var decision map[string]any
		var text string
		var co company.Company
		if err := run.Span(tracer.SpanDecide, nil, func(sp tracer.SpanHandle) error {
			var err error
			if companyID, err = s.companies.CreateTx(ctx, tx, in.Empresa); err != nil {
				return err
			}
			if inicio != nil {
				if err := insertActivityStart(ctx, store.New(tx), companyID, *inicio, uid, s.clock.Now()); err != nil {
					return err
				}
			}
			if err := run.LinkCompany(ctx, companyID); err != nil {
				return err
			}
			legal := strings.TrimSpace(in.Empresa.LegalName)
			co = company.Company{ID: companyID, LegalName: legal, Cnpj: *lido}
			var subjectMatches any
			if sug != nil {
				subjectMatches = in.Empresa.FactorR != nil && *in.Empresa.FactorR == *sug
			}
			// Só booleanos: medem a leitura do parser sem mandar o nome ao Langfuse.
			extra := map[string]any{
				"empresa_criada":         true,
				"nome_igual_extrato":     st.Identificacao.NomeEmpresarial != nil && legal == *st.Identificacao.NomeEmpresarial,
				"sujeita_igual_sugestao": subjectMatches,
			}
			decision, text, err = s.linkDecision(ctx, tx, run, cur, st, co, "pgdas_cadastro_pelo_extrato", extra)
			sp.Output(decision)
			return err
		}); err != nil {
			return tracer.Result{}, err
		}
		text = factorr.WithDisclaimer(fmt.Sprintf("Empresa %s cadastrada a partir do extrato. %s", co.LegalName, text))
		if err := run.Span(tracer.SpanRender, nil, func(sp tracer.SpanHandle) error {
			sp.Output(map[string]any{"texto": text})
			return nil
		}); err != nil {
			return tracer.Result{}, err
		}
		if err := gold(ctx, s.tracer, tx, store.New(tx), run.TraceID(), st, companyID, *lido); err != nil {
			return tracer.Result{}, err
		}
		return tracer.Result{Status: tracer.StatusOK, Decision: decision, Text: text}, nil
	})
	if err != nil {
		return CreateCompanyResult{}, err
	}
	d, err := s.Get(ctx, id)
	if err != nil {
		return CreateCompanyResult{}, err
	}
	co, err := s.companies.Get(ctx, companyID)
	if err != nil {
		return CreateCompanyResult{}, err
	}
	return CreateCompanyResult{Documento: d, Empresa: co}, nil
}

// insertActivityStart grava o início da atividade lido do extrato na configuração do Fator R
// da empresa recém-criada (os demais campos ficam nos padrões).
func insertActivityStart(ctx context.Context, q *store.Queries, companyID string, inicio competence.Competence, uid string, now time.Time) error {
	d := inicio.FirstDay()
	_, err := q.InsertFactorRSettings(ctx, store.InsertFactorRSettingsParams{
		CompanyID: companyID, QuantidadeSocios: 1, HonorarioMensal: decimal.MustParse("0.00"),
		InicioAtividade: &d, UpdatedBy: &uid, UpdatedAt: now,
	})
	return err
}
