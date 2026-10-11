package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/prompts"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/cnpj"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/pdftext"
	"controle/internal/reconciliation"
	"controle/internal/reconciliation/receipt"
	"controle/internal/reconciliation/statement"
	"controle/internal/store"
)

// Comprovante (TASK-017, D-028): porte de app/agentes/extrator_comprovante.py,
// app/servicos/comprovantes.py e do atalho de comprovante do processar_local da origem.

const (
	agentReceipt      = "extrator_comprovante"
	receiptPromptName = "conciliacao/extrator_comprovante"
	maxReceiptChars   = 16000 // o texto[:16000] da origem
	maxReceiptPages   = 3
	receiptDPI        = 150
	// Estado do lançamento lido (o status do lancamento_cliente da origem).
	entryExtracted = "extraido"
	entryPartial   = "parcial"
)

// Estratégias do extrator de comprovante (M3.3 da origem: texto primeiro; sem texto ou com campo
// fraco, render e visão).
const (
	strategyNativeText = "texto_nativo"
	strategyRender     = "render_150"
	strategyImage      = "imagem"
	strategyTextFile   = "texto"
)

// receiptCompany é a empresa do comprovante, com o nome para o leitor e para o extrator.
type receiptCompany struct {
	id     string
	client receipt.Client
	label  string // "Razão Social (nome fantasia X), CNPJ 12.345.678/0001-90"
}

func newReceiptCompany(id, legal string, trade *string, digits string) *receiptCompany {
	names, label := []string{legal}, legal
	if trade != nil && strings.TrimSpace(*trade) != "" {
		names = append(names, *trade)
		label += " (nome fantasia " + *trade + ")"
	}
	return &receiptCompany{id: id, client: receipt.NewClient(names...), label: label + ", CNPJ " + cnpj.Format(digits)}
}

// findReceiptCompany identifica a empresa do comprovante: a do envio ou, sem ela, a única empresa
// cadastrada cujo CNPJ está impresso no texto. Sem nenhuma, ou com mais de uma, devolve nil e
// quantas empresas cadastradas o texto cita (a revisão humana decide).
func findReceiptCompany(ctx context.Context, q *store.Queries, doc store.ReconciliationDocument, text string) (*receiptCompany, int, error) {
	if doc.CompanyID != nil {
		c, err := q.GetCompanyIdentity(ctx, *doc.CompanyID)
		if err != nil {
			return nil, 0, err
		}
		return newReceiptCompany(c.ID, c.LegalName, c.TradeName, c.Cnpj), 1, nil
	}
	cnpjs := receipt.CNPJs(text)
	if len(cnpjs) == 0 {
		return nil, 0, nil
	}
	rows, err := q.ListCompaniesByCnpj(ctx, cnpjs)
	if err != nil || len(rows) != 1 {
		return nil, len(rows), err
	}
	return newReceiptCompany(rows[0].ID, rows[0].LegalName, rows[0].TradeName, rows[0].Cnpj), 1, nil
}

// receiptCheck é o resultado das regras de campo, para o log de extração.
func receiptCheck(weak []string, threshold decimal.Decimal) map[string]any {
	return map[string]any{"ok": len(weak) == 0, "campos_a_revisar": weak, "limiar": threshold.String()}
}

// localReceipt é o atalho processar_local da origem para comprovantes, no job da triagem (D-028).
// O PDF com texto nativo que se apresenta como comprovante, de uma empresa identificada (a do
// envio ou a única cadastrada com o CNPJ impresso), passa pelo leitor determinístico. A leitura
// vale com a empresa achada no comprovante (pagadora, recebedora ou dona da conta debitada) e os
// campos essenciais válidos; senão, o documento segue para o triador e o extrator. A origem
// gravava qualquer leitura com data e valor, com o sentido suposto quando não achava a empresa.
func (p *Processor) localReceipt(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument, src source, jobID string) (bool, receiptOutcome, error) {
	if !src.isPDF() || !src.readable() || !src.hasText() {
		return false, receiptOutcome{}, nil
	}
	text := strings.Join(src.pages, "\n")
	if statement.IsStatement(text) || !receipt.IsReceiptText(text) {
		return false, receiptOutcome{}, nil
	}
	q := store.New(tx)
	company, _, err := findReceiptCompany(ctx, q, doc, text)
	if err != nil || company == nil {
		return false, receiptOutcome{}, err
	}
	r, ok, err := receipt.FromText(text, company.client)
	if err != nil {
		return false, receiptOutcome{}, p.writeLog(ctx, q, doc, jobID, logEntry{alias: aliasLocalReader, attempt: 0, errText: err.Error()})
	}
	if !ok {
		return false, receiptOutcome{}, nil
	}
	checked, weak := receipt.Check(r, p.cfg.FieldThreshold)
	named := receipt.ClientNamed(text, company.client)
	entry := logEntry{alias: aliasLocalReader, attempt: 0, rawOutput: string(jsonBytes(r)), check: receiptCheck(weak, p.cfg.FieldThreshold)}
	switch {
	case !named:
		entry.errText = "a empresa não aparece como pagadora nem como recebedora: o comprovante segue para o agente"
	case len(weak) > 0:
		entry.errText = "campos a revisar (" + strings.Join(weak, ", ") + "): o comprovante segue para o agente"
	}
	if err := p.writeLog(ctx, q, doc, jobID, entry); err != nil {
		return false, receiptOutcome{}, err
	}
	if !named || len(weak) > 0 {
		return false, receiptOutcome{}, nil
	}
	out, err := p.saveReceipt(ctx, tx, doc, company, checked, weak, "")
	return true, out, err
}

// receiptOutcome é o que a gravação fez com o comprovante.
type receiptOutcome struct {
	status    string // importado, revisao_extracao ou pendencia_periodo_fechado
	reason    string // motivo da revisão ou da pendência
	entryID   string
	companyID string
	weak      []string
	duplicate bool
	flags     []string
}

func (o receiptOutcome) asMap() map[string]any {
	weak, flags := o.weak, o.flags
	if weak == nil {
		weak = []string{}
	}
	if flags == nil {
		flags = []string{}
	}
	return map[string]any{"status": o.status, "motivo": nilIfEmpty(o.reason), "lancamento_id": nilIfEmpty(o.entryID),
		"company_id": nilIfEmpty(o.companyID), "campos_a_revisar": weak, "duplicata": o.duplicate, "marcas": flags}
}

// saveReceipt é o persistir_comprovante da origem, na transação do job: o lançamento com a
// confiança e a origem de cada campo e as marcas de borda; parcial (documento em revisao_extracao)
// com campo a revisar, extraído (documento importado) sem. A duplicata de leiaute diferente (mesmo
// valor, mesma data e contraparte normalizada, na empresa) é sinalizada e nunca descartada [M3.5].
// O porte acrescenta a trava do Contábil: com a competência do comprovante fechada, nada é gravado
// e o documento fica como pendencia_periodo_fechado (AC-025).
func (p *Processor) saveReceipt(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument, company *receiptCompany,
	r receipt.Receipt, weak []string, traceID string) (receiptOutcome, error) {
	q := store.New(tx)
	out := receiptOutcome{companyID: company.id, weak: weak, flags: r.Alerts}
	comp := docCompetence(doc)
	if r.Date != nil {
		c := competence.New(r.Date.Year(), r.Date.Month())
		comp = &c
	}
	if comp != nil {
		reason, err := reconciliation.LockReason(ctx, tx, company.id, *comp)
		if err != nil {
			return out, err
		}
		if reason != "" {
			out.status, out.reason = StatusClosedPeriod, reason
			return out, p.setState(ctx, q, doc.ID, StatusClosedPeriod, diagnosis(reason, nil,
				map[string]any{"company_id": company.id, "competencias": []string{comp.String()}}), nil)
		}
	}
	if r.Amount != nil && r.Date != nil {
		others, err := q.ListClientEntryCounterparties(ctx, store.ListClientEntryCounterpartiesParams{
			CompanyID: company.id, Amount: r.Amount, EntryDate: r.Date,
		})
		if err != nil {
			return out, err
		}
		target := reconciliation.NormalizeDescription(deref(r.Counterparty))
		for _, o := range others {
			if reconciliation.NormalizeDescription(deref(o)) == target {
				out.duplicate = true
				break
			}
		}
	}
	status, docStatus := entryExtracted, StatusImported
	var diag []byte
	if len(weak) > 0 {
		status, docStatus, out.reason = entryPartial, StatusExtractReview, ReasonWeakFields
		diag = jsonBytes(diagnosis(ReasonWeakFields, nil, map[string]any{"campos": weak, "limiar": p.cfg.FieldThreshold.String()}))
	}
	flags := r.Alerts
	if flags == nil {
		flags = []string{}
	}
	origin := r.Origin
	if origin == nil {
		origin = map[string]receipt.FieldOrigin{}
	}
	now := p.clock.Now()
	id, err := q.InsertClientEntry(ctx, store.InsertClientEntryParams{
		CompanyID: company.id, DocumentID: &doc.ID, EntryDate: r.Date, Amount: r.Amount, Direction: r.Direction,
		Counterparty: r.Counterparty, CounterpartyDoc: r.CounterpartyDoc, DocumentRef: r.DocumentRef,
		OperationType: r.OperationType, Status: status, FieldConfidence: jsonBytes(r.ConfidenceText()),
		FieldOrigin: jsonBytes(origin), DuplicateFlagged: out.duplicate, Flags: jsonBytes(flags), TraceID: optString(traceID),
		Now: now,
	})
	if err != nil {
		return out, fmt.Errorf("gravar o lançamento do comprovante: %w", err)
	}
	out.entryID, out.status = id, docStatus
	if err := q.SaveReconciliationReceiptDocument(ctx, store.SaveReconciliationReceiptDocumentParams{
		Status: docStatus, CompanyID: &company.id, Diagnosis: diag, Now: now, ID: doc.ID,
	}); err != nil {
		return out, err
	}
	// Lançamento novo: a rodada do matching da competência dele (TASK-018).
	if r.Date != nil {
		return out, reconciliation.EnqueueRun(ctx, p.queue, tx, company.id, competence.FromDate(*r.Date))
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// HandleReceipt é o handler do job receipt_extract (handle_extracao_comprovante da origem): o
// extrator lê o comprovante e o código aplica as regras de campo, sinaliza a duplicata e grava.
func (p *Processor) HandleReceipt(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	id, err := documentID(job)
	if err != nil {
		return err
	}
	q := store.New(tx)
	doc, err := lockDocument(ctx, q, id)
	if err != nil {
		return err
	}
	if doc.Status != StatusTriaged || doc.Kind == nil || *doc.Kind != statement.KindReceipt {
		return nil // já processado, em revisão ou não é comprovante
	}
	src, err := p.load(ctx, doc)
	if err != nil {
		return err
	}
	return p.extractReceipt(ctx, tx, doc, src, job.ID)
}

// receiptText é o texto do comprovante: o de todas as páginas do PDF (extrair_texto_pdf, juntas
// por "\n" como na origem) ou o do arquivo de texto.
func receiptText(src source) string {
	switch {
	case src.isPDF() && src.hasText():
		return strings.Join(src.pages, "\n")
	case src.isText():
		return src.text
	}
	return ""
}

// receiptStrategies é o roteamento da M3.3: o PDF com texto vai primeiro como texto e, se a
// leitura sair fora do schema ou com campo a revisar, vai renderizado (com o texto junto); o PDF
// sem texto vai renderizado; a imagem, como está; o arquivo de texto, como texto.
func receiptStrategies(src source) []string {
	switch {
	case src.isPDF() && src.hasText():
		return []string{strategyNativeText, strategyRender}
	case src.isPDF():
		return []string{strategyRender}
	case src.isImage():
		return []string{strategyImage}
	}
	return []string{strategyTextFile}
}

// receiptExtractor guarda o estado da corrida do extrator de comprovante.
type receiptExtractor struct {
	p          *Processor
	tx         pgx.Tx
	q          *store.Queries
	run        *tracer.Run
	doc        store.ReconciliationDocument
	src        source
	text       string
	jobID      string
	model      ai.Resolved
	system     string
	ref        promptRef
	company    *receiptCompany
	candidates int
}

// receiptAttempt é a leitura de uma tentativa que passou pelo schema, já com as regras de campo.
type receiptAttempt struct {
	receipt  receipt.Receipt
	weak     []string
	strategy string
	attempt  int
}

// extractReceipt é o extrair_comprovante da origem com o roteamento da M3.3 (até 2 chamadas). A
// origem fazia uma chamada só, com o texto ou com o aviso "[imagem N bytes]" no lugar da imagem.
func (p *Processor) extractReceipt(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument, src source, jobID string) error {
	q := store.New(tx)
	if src.pdfErr != nil {
		return p.setState(ctx, q, doc.ID, StatusExtractReview, diagnosis(ReasonUnreadable, nil, map[string]any{"erro": pdfReason(src.pdfErr)}), nil)
	}
	model, err := p.resolver.Resolve(ctx, tx, ai.Extrator)
	if agentUnavailable(err) {
		return p.setState(ctx, q, doc.ID, StatusExtractReview, diagnosis(ReasonAgentDisabled, nil, map[string]any{"alias": string(ai.Extrator)}), nil)
	}
	if err != nil {
		return err
	}
	text := receiptText(src)
	company, candidates, err := findReceiptCompany(ctx, q, doc, text)
	if err != nil {
		return err
	}
	companyID := doc.CompanyID
	if company != nil {
		companyID = &company.id
	}
	system, ref := systemPrompt(prompts.Get(receiptPromptName))
	run, err := p.tracer.Begin(ctx, tx, tracer.Spec{
		Agent: agentReceipt, Trigger: triggerUpload, UserID: doc.UploadedBy, CompanyID: companyID,
		Competence: docCompetence(doc), Model: &model, PromptVersion: ref.version,
		Input:    map[string]any{"mime": src.mime, "tamanho_bytes": len(src.data), "paginas": src.pageCount, "tem_texto": text != ""},
		Metadata: map[string]string{"limiar_campo": p.cfg.FieldThreshold.String(), "tentativas_max": strconv.Itoa(len(receiptStrategies(src)))},
	})
	if err != nil {
		return err
	}
	e := &receiptExtractor{p: p, tx: tx, q: q, run: run, doc: doc, src: src, text: text, jobID: jobID, model: model,
		system: system, ref: ref, company: company, candidates: candidates}
	if err := e.loop(ctx); err != nil {
		return run.Fail(err)
	}
	return nil
}

// loop faz as tentativas da M3.3: a 2ª só quando a 1ª não trouxe leitura completa. Vale a leitura
// com menos campos a revisar (a mais recente, no empate).
func (e *receiptExtractor) loop(ctx context.Context) error {
	var best *receiptAttempt
	attempts := 0
	for i, st := range receiptStrategies(e.src) {
		if best != nil && len(best.weak) == 0 {
			break
		}
		attempts = i + 1
		res, err := e.attempt(ctx, attempts, st)
		if err != nil {
			return err
		}
		if res != nil && (best == nil || len(res.weak) <= len(best.weak)) {
			best = res
		}
	}
	return e.decide(ctx, best, attempts)
}

// attempt chama o extrator na estratégia, valida o schema e as regras de campo e grava a linha do
// log de extração.
func (e *receiptExtractor) attempt(ctx context.Context, n int, st string) (*receiptAttempt, error) {
	var out *receiptAttempt
	err := e.run.Span(tracer.SpanParse, map[string]any{"tentativa": n, "estrategia": st, "alias": string(e.model.Alias)}, func(s tracer.SpanHandle) error {
		s.Meta("prompt_version", e.ref.version)
		entry := logEntry{alias: string(e.model.Alias), attempt: n, resolvedModel: e.model.Model.String(), prompt: e.ref, traceID: e.run.TraceID()}
		user, images, err := e.input(ctx, st)
		if err != nil {
			return err
		}
		if user == "" {
			// Nem texto nem imagem: não há o que mandar ao modelo.
			entry.errText = "render indisponível e PDF sem texto"
			s.Warn(ReasonUnreadable)
			return e.p.writeLog(ctx, e.q, e.doc, e.jobID, entry)
		}
		reply, err := e.p.call(ctx, e.run, llmCall{
			name: "extrair_comprovante", model: e.model, system: e.system, user: user, images: images, attempt: n,
			schema: &litellm.JSONSchema{Name: "comprovante_extraido", Schema: receiptSchema},
			input:  map[string]any{"estrategia": st, "imagens": len(images), "caracteres": utf8.RuneCountInString(user)},
		})
		if err != nil {
			return err
		}
		var parsed receipt.Receipt
		perr := receipt.ErrInvalidOutput
		if !reply.unusable {
			entry.rawOutput, entry.latency = reply.resp.Content, reply.resp.Latency
			entry.tokensIn, entry.tokensOut, entry.costUSD = usage(reply.resp)
			if reply.resp.Model != "" {
				entry.resolvedModel = reply.resp.Model
			}
			parsed, perr = receipt.Parse(reply.resp.Content)
		}
		if perr != nil {
			entry.errText = perr.Error()
			s.Warn(ReasonInvalidOutput)
			s.Output(map[string]any{"schema": false})
			return e.p.writeLog(ctx, e.q, e.doc, e.jobID, entry)
		}
		checked, weak := receipt.Check(parsed, e.p.cfg.FieldThreshold)
		// As marcas que o código lê do texto somam-se às do agente (só as do vocabulário da origem).
		checked.Alerts = receipt.MergeFlags(receipt.KnownFlags(parsed.Alerts), receipt.TextFlags(e.text))
		entry.check = receiptCheck(weak, e.p.cfg.FieldThreshold)
		out = &receiptAttempt{receipt: checked, weak: weak, strategy: st, attempt: n}
		s.Output(map[string]any{"schema": true, "campos_a_revisar": weak})
		if len(weak) > 0 {
			s.Warn(ReasonWeakFields)
		}
		return e.p.writeLog(ctx, e.q, e.doc, e.jobID, entry)
	})
	return out, err
}

// input monta a mensagem da estratégia: a empresa do documento (para o sentido), o texto e as
// imagens. Vazia quando não há o que mandar.
func (e *receiptExtractor) input(ctx context.Context, st string) (string, []litellm.Image, error) {
	company := "não identificada"
	if e.company != nil {
		company = e.company.label
	}
	lines := []string{"estrategia=" + st, "EMPRESA DO DOCUMENTO: " + company}
	var images []litellm.Image
	switch st {
	case strategyImage:
		images = []litellm.Image{{MIME: e.src.mime, Data: e.src.data, Detail: "high"}}
	case strategyRender:
		n := min(e.src.pageCount, maxReceiptPages)
		pages := make([]int, n)
		for i := range pages {
			pages[i] = i
		}
		var pngs [][]byte
		err := e.run.Span(tracer.SpanTool, map[string]any{"ferramenta": "renderizar_pagina", "paginas": n, "dpi": receiptDPI}, func(s tracer.SpanHandle) error {
			var err error
			pngs, err = e.p.pdf.Render(ctx, e.src.data, pages, receiptDPI)
			if err == nil {
				total := 0
				for _, b := range pngs {
					total += len(b)
				}
				s.Output(map[string]any{"bytes": total})
			}
			return err
		})
		switch {
		case errors.Is(err, pdftext.ErrUnreadable) || errors.Is(err, pdftext.ErrLimits):
			lines = append(lines, "render indisponível: "+pdfReason(err))
		case err != nil:
			return "", nil, err
		}
		for _, b := range pngs {
			images = append(images, litellm.Image{MIME: mimePNG, Data: b, Detail: "high"})
		}
		if e.src.pageCount > n {
			lines = append(lines, fmt.Sprintf("renderizadas as páginas 1 a %d de %d", n, e.src.pageCount))
		}
		if e.text != "" {
			lines = append(lines, "TEXTO:\n"+truncateRunes(e.text, maxReceiptChars))
		}
		if len(images) == 0 && e.text == "" {
			return "", nil, nil
		}
	default:
		lines = append(lines, "TEXTO:\n"+truncateRunes(e.text, maxReceiptChars))
	}
	return strings.Join(lines, "\n"), images, nil
}

// decide encerra a corrida: grava o lançamento ou deixa o documento em revisão.
func (e *receiptExtractor) decide(ctx context.Context, best *receiptAttempt, attempts int) error {
	var out receiptOutcome
	err := e.run.Span(tracer.SpanDecide, map[string]any{"tentativas": attempts}, func(s tracer.SpanHandle) error {
		var err error
		switch {
		case best == nil:
			out = receiptOutcome{status: StatusExtractReview, reason: ReasonInvalidOutput}
			err = e.p.setState(ctx, e.q, e.doc.ID, StatusExtractReview, diagnosis(ReasonInvalidOutput, nil, map[string]any{"tentativas": attempts}), nil)
		case e.company == nil:
			out = receiptOutcome{status: StatusExtractReview, reason: ReasonNoCompany, weak: best.weak, flags: best.receipt.Alerts}
			err = e.p.setState(ctx, e.q, e.doc.ID, StatusExtractReview, diagnosis(ReasonNoCompany, nil,
				map[string]any{"empresas_com_cnpj_no_documento": e.candidates}), nil)
		default:
			out, err = e.p.saveReceipt(ctx, e.tx, e.doc, e.company, best.receipt, best.weak, e.run.TraceID())
		}
		if err != nil {
			return err
		}
		if out.status != StatusImported {
			s.Warn(out.reason)
		}
		return nil
	})
	if err != nil {
		return err
	}
	decision := out.asMap()
	decision["document_id"] = e.doc.ID
	decision["tentativas"] = attempts
	if best != nil {
		decision["estrategia"] = best.strategy
		decision["tentativa_usada"] = best.attempt
	}
	if err := e.run.RecordDecision(ctx, "extracao_comprovante", decision); err != nil {
		return err
	}
	text := receiptRenderText(out, attempts)
	if err := e.run.Span(tracer.SpanRender, nil, func(s tracer.SpanHandle) error {
		s.Output(map[string]any{"texto": text})
		return nil
	}); err != nil {
		return err
	}
	status := tracer.StatusOK
	if out.status != StatusImported {
		status = tracer.StatusNeedsReview
	}
	return e.run.Finish(ctx, tracer.Result{Status: status, Decision: decision, Text: text})
}

func receiptRenderText(out receiptOutcome, attempts int) string {
	var text string
	switch {
	case out.status == StatusImported:
		text = "Comprovante lido: o lançamento foi gravado com todos os campos essenciais acima do limiar."
	case out.status == StatusClosedPeriod:
		return "O Contábil está fechado na competência do comprovante: nada foi gravado (pendência de período fechado)."
	case out.reason == ReasonWeakFields:
		text = fmt.Sprintf("Comprovante lido com campo a revisar (%s): o lançamento ficou parcial, fora do razão, na fila de revisão.",
			strings.Join(out.weak, ", "))
	case out.reason == ReasonNoCompany:
		return "Comprovante lido, mas a empresa não foi identificada (o envio não indicou e o documento não traz um CNPJ cadastrado único): vai para a revisão humana."
	default:
		return fmt.Sprintf("Nenhuma das %d tentativas trouxe um comprovante válido: vai para a revisão humana.", attempts)
	}
	if out.duplicate {
		text += " Possível duplicata de outro comprovante (mesmo valor, data e contraparte): sinalizada na fila, não descartada."
	}
	return text
}

// receiptSchema é o schema estrito da saída do extrator de comprovante (ComprovanteExtraido):
// dinheiro como texto decimal; confiança e origem para cada campo do prompt.
var receiptSchema = func() json.RawMessage {
	fields := []string{receipt.FieldDate, receipt.FieldAmount, receipt.FieldDirection, receipt.FieldCounterparty,
		receipt.FieldCounterpartyDoc, receipt.FieldDocumentRef, receipt.FieldOperationType}
	nullableString := map[string]any{"type": []string{"string", "null"}}
	origin := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pagina": map[string]any{"type": "integer"},
			"trecho": map[string]any{"type": "string"},
			"bbox":   map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "number"}},
		},
		"required":             []string{"pagina", "trecho", "bbox"},
		"additionalProperties": false,
	}
	conf, orig := map[string]any{}, map[string]any{}
	for _, f := range fields {
		conf[f] = map[string]any{"type": "number"}
		orig[f] = origin
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			receipt.FieldDate:            nullableString,
			receipt.FieldAmount:          nullableString,
			receipt.FieldDirection:       map[string]any{"type": []string{"string", "null"}, "enum": []any{receipt.Credit, receipt.Debit, nil}},
			receipt.FieldCounterparty:    nullableString,
			receipt.FieldCounterpartyDoc: nullableString,
			receipt.FieldDocumentRef:     nullableString,
			receipt.FieldOperationType: map[string]any{"type": []string{"string", "null"},
				"enum": []any{"pix", "boleto", "ted", "doc", "darf", "guia", "tarifa", "outro", nil}},
			"alertas": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"confianca_por_campo": map[string]any{"type": "object", "properties": conf, "required": fields,
				"additionalProperties": false},
			"origem_por_campo": map[string]any{"type": "object", "properties": orig, "required": fields,
				"additionalProperties": false},
		},
		"required":             append(append([]string{}, fields...), "alertas", "confianca_por_campo", "origem_por_campo"),
		"additionalProperties": false,
	}
	return jsonBytes(schema)
}()
