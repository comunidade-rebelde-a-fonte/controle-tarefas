package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai"
	"controle/internal/ai/litellm"
	"controle/internal/ai/prompts"
	"controle/internal/ai/tracer"
	"controle/internal/jobs"
	"controle/internal/platform/competence"
	"controle/internal/platform/pdftext"
	"controle/internal/reconciliation/statement"
	"controle/internal/store"
)

// HandleTriage é o handler do job reconciliation_triage (handle_triagem da origem): os leitores
// determinísticos do PDF (extrato e comprovante) e, se eles não resolverem, o triador. O documento
// triado como extrato ganha o job statement_extract; como comprovante, o receipt_extract.
func (p *Processor) HandleTriage(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	id, err := documentID(job)
	if err != nil {
		return err
	}
	q := store.New(tx)
	doc, err := lockDocument(ctx, q, id)
	if err != nil {
		return err
	}
	if doc.Status != StatusPending {
		return nil // já processado, em revisão ou parado por período fechado
	}
	src, err := p.load(ctx, doc)
	if err != nil {
		return err
	}
	if !p.cfg.AgentOnly {
		handled, _, err := p.localPDF(ctx, tx, doc, src, job.ID)
		if err != nil || handled {
			return err
		}
		handled, _, err = p.localReceipt(ctx, tx, doc, src, job.ID)
		if err != nil || handled {
			return err
		}
	}
	return p.triage(ctx, tx, doc, src, job.ID)
}

func docCompetence(doc store.ReconciliationDocument) *competence.Competence {
	if doc.Competence == nil {
		return nil
	}
	c := competence.FromDate(*doc.Competence)
	return &c
}

// agentUnavailable é o alias desligado na tela de IA ou sem modelo: o documento vai para a
// revisão humana em vez de gastar as tentativas do job.
func agentUnavailable(err error) bool {
	return errors.Is(err, ai.ErrAliasDisabled) || errors.Is(err, ai.ErrNotConfigured)
}

// triage é o triar da origem: classifica o documento pelo texto das primeiras páginas, pela
// imagem da primeira página (PDF sem texto, foto ou print) ou pelo conteúdo do arquivo de texto.
// Abaixo do limiar, ou tipo desconhecido, o documento vai para revisao_triagem.
func (p *Processor) triage(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument, src source, jobID string) error {
	q := store.New(tx)
	if src.pdfErr != nil {
		return p.setState(ctx, q, doc.ID, StatusTriageReview, diagnosis(ReasonUnreadable, nil, map[string]any{"erro": pdfReason(src.pdfErr)}), nil)
	}
	model, err := p.resolver.Resolve(ctx, tx, ai.Triador)
	if agentUnavailable(err) {
		return p.setState(ctx, q, doc.ID, StatusTriageReview, diagnosis(ReasonAgentDisabled, nil, map[string]any{"alias": string(ai.Triador)}), nil)
	}
	if err != nil {
		return err
	}
	system, ref := systemPrompt(prompts.Get(triagePromptName))
	run, err := p.tracer.Begin(ctx, tx, tracer.Spec{
		Agent: agentTriador, Trigger: triggerUpload, UserID: doc.UploadedBy, CompanyID: doc.CompanyID,
		Competence: docCompetence(doc), Model: &model, PromptVersion: ref.version,
		Input:    map[string]any{"mime": src.mime, "tamanho_bytes": len(src.data), "paginas": src.pageCount},
		Metadata: map[string]string{"limiar": p.cfg.TriageThreshold.String()},
	})
	if err != nil {
		return err
	}
	if err := p.triageRun(ctx, tx, run, doc, src, model, system, ref, jobID); err != nil {
		return run.Fail(err)
	}
	return nil
}

// pdfReason é o motivo do PDF que o PDFium não leu (os motivos do extrator do PGDAS-D).
func pdfReason(err error) string {
	if errors.Is(err, pdftext.ErrLimits) {
		return pdftext.ReasonLimits
	}
	return pdftext.ReasonUnreadable
}

func (p *Processor) triageRun(ctx context.Context, tx pgx.Tx, run *tracer.Run, doc store.ReconciliationDocument, src source,
	model ai.Resolved, system string, ref promptRef, jobID string) error {
	q := store.New(tx)
	var user string
	var images []litellm.Image
	unreadable := ""
	err := run.Span(tracer.SpanParse, map[string]any{"mime": src.mime, "paginas": src.pageCount}, func(s tracer.SpanHandle) error {
		switch {
		case src.isPDF() && src.hasText():
			pages := src.pages[:min(maxTriagePages, len(src.pages))]
			user = truncateRunes(strings.Join(pages, "\n"), maxTriageChars)
			s.Meta("ferramenta", "extrair_texto_pdf")
		case src.isPDF():
			png, err := p.pdf.Render(ctx, src.data, []int{0}, 72)
			if errors.Is(err, pdftext.ErrUnreadable) || errors.Is(err, pdftext.ErrLimits) {
				unreadable = pdfReason(err)
				return nil
			}
			if err != nil {
				return err
			}
			user = "PDF sem camada de texto: a imagem da primeira página vai anexa."
			images = []litellm.Image{{MIME: mimePNG, Data: png[0]}}
			s.Meta("ferramenta", "renderizar_pagina")
		case src.isImage():
			user = "Imagem do documento anexa."
			images = []litellm.Image{{MIME: src.mime, Data: src.data}}
		default:
			user = truncateRunes(src.text, maxTriageChars)
		}
		s.Output(map[string]any{"caracteres": utf8.RuneCountInString(user), "imagens": len(images)})
		return nil
	})
	if err != nil {
		return err
	}
	if unreadable != "" {
		return p.finishTriageReview(ctx, q, run, doc, diagnosis(ReasonUnreadable, nil, map[string]any{"erro": unreadable}))
	}
	var t statement.Triage
	err = run.Span(tracer.SpanPlan, map[string]any{"alias": string(model.Alias)}, func(s tracer.SpanHandle) error {
		s.Meta("prompt_version", ref.version)
		reply, err := p.call(ctx, run, llmCall{
			name: "triar_documento", model: model, system: system, user: user, images: images, attempt: 1,
			schema: &litellm.JSONSchema{Name: "resultado_triagem", Schema: triageSchema},
			input:  map[string]any{"caracteres": utf8.RuneCountInString(user), "imagens": len(images)},
		})
		if err != nil {
			return err
		}
		entry := logEntry{alias: string(ai.Triador), attempt: 1, resolvedModel: model.Model.String(), prompt: ref, traceID: run.TraceID()}
		var perr error
		if reply.unusable {
			perr = statement.ErrInvalidOutput
		} else {
			entry.rawOutput, entry.latency = reply.resp.Content, reply.resp.Latency
			entry.tokensIn, entry.tokensOut, entry.costUSD = usage(reply.resp)
			if reply.resp.Model != "" {
				entry.resolvedModel = reply.resp.Model
			}
			t, perr = statement.ParseTriage(reply.resp.Content)
		}
		if perr != nil {
			// Como na origem (TentativaParseError não tratado no triar), a tentativa do job falha e a
			// fila tenta de novo.
			return jobs.Coded(CodeTriageOutput, fmt.Errorf("saída do triador fora do schema: %w", perr))
		}
		s.Output(map[string]any{"tipo": t.Kind, "banco": t.Bank, "confianca": t.Confidence.String()})
		return p.writeLog(ctx, q, doc, jobID, entry)
	})
	if err != nil {
		return err
	}
	return p.decideTriage(ctx, tx, run, doc, t)
}

// decideTriage aplica o limiar, grava a triagem e enfileira a extração.
func (p *Processor) decideTriage(ctx context.Context, tx pgx.Tx, run *tracer.Run, doc store.ReconciliationDocument, t statement.Triage) error {
	q := store.New(tx)
	status, reason := StatusTriaged, ""
	switch {
	case t.Kind == statement.KindUnknown:
		status, reason = StatusTriageReview, ReasonUnknownKind
	case t.Confidence.Cmp(p.cfg.TriageThreshold) < 0:
		status, reason = StatusTriageReview, ReasonLowConfidence
	}
	var decision map[string]any
	err := run.Span(tracer.SpanDecide, nil, func(s tracer.SpanHandle) error {
		var diag []byte
		if reason != "" {
			diag = jsonBytes(diagnosis(reason, nil, map[string]any{
				"tipo": t.Kind, "confianca": t.Confidence.String(), "limiar": p.cfg.TriageThreshold.String(),
				"justificativa": t.Justification,
			}))
			s.Warn(reason)
		}
		conf := t.Confidence.Quantize(-4)
		trace := run.TraceID()
		if err := q.SaveReconciliationTriage(ctx, store.SaveReconciliationTriageParams{
			Kind: &t.Kind, Bank: t.Bank, Branch: t.Branch, Account: t.Account, PeriodStart: t.PeriodStart,
			PeriodEnd: t.PeriodEnd, TriageConfidence: &conf, TriageTraceID: &trace, Status: status, Diagnosis: diag,
			Now: p.clock.Now(), ID: doc.ID,
		}); err != nil {
			return err
		}
		next := ""
		if status == StatusTriaged {
			next = KindStatementExtract
			if t.Kind == statement.KindReceipt {
				next = KindReceiptExtract
			}
			if _, err := p.enqueue(ctx, tx, q, doc.ID, next, StatusTriaged); err != nil {
				return err
			}
		}
		// Agência e conta ficam só no documento (banco local): a decisão também vai ao Langfuse.
		decision = map[string]any{
			"document_id": doc.ID, "status": status, "motivo": nilIfEmpty(reason), "tipo": t.Kind, "banco": t.Bank,
			"confianca": t.Confidence.String(), "limiar": p.cfg.TriageThreshold.String(), "proximo_job": nilIfEmpty(next),
		}
		if err := run.RecordDecision(ctx, "triagem_documento", decision); err != nil {
			return err
		}
		s.Output(decision)
		return nil
	})
	if err != nil {
		return err
	}
	text := triageText(t, status, reason)
	if err := run.Span(tracer.SpanRender, nil, func(s tracer.SpanHandle) error {
		s.Output(map[string]any{"texto": text})
		return nil
	}); err != nil {
		return err
	}
	runStatus := tracer.StatusOK
	if status != StatusTriaged {
		runStatus = tracer.StatusNeedsReview
	}
	conf := t.Confidence
	return run.Finish(ctx, tracer.Result{Status: runStatus, Decision: decision, Text: text, Confidence: &conf})
}

// finishTriageReview encerra a corrida com o documento em revisão sem classificação.
func (p *Processor) finishTriageReview(ctx context.Context, q *store.Queries, run *tracer.Run, doc store.ReconciliationDocument, diag map[string]any) error {
	if err := p.setState(ctx, q, doc.ID, StatusTriageReview, diag, nil); err != nil {
		return err
	}
	decision := map[string]any{"document_id": doc.ID, "status": StatusTriageReview, "motivo": diag["motivo"]}
	if err := run.RecordDecision(ctx, "triagem_documento", decision); err != nil {
		return err
	}
	return run.Finish(ctx, tracer.Result{Status: tracer.StatusNeedsReview, Decision: decision,
		Text: "Documento ilegível: vai para a revisão humana (que documento é esse?)."})
}

func triageText(t statement.Triage, status, reason string) string {
	desc := map[string]string{statement.KindStatement: "extrato", statement.KindReceipt: "comprovante", statement.KindUnknown: "documento desconhecido"}[t.Kind]
	if t.Bank != nil && *t.Bank != "" {
		desc += " do banco " + *t.Bank
	}
	conf := t.Confidence.Mul(hundredPct).Quantize(0).String() + "%"
	switch {
	case status == StatusTriaged && t.Kind == statement.KindStatement:
		return fmt.Sprintf("Classificado como %s (confiança %s). Segue para a extração do extrato.", desc, conf)
	case status == StatusTriaged:
		return fmt.Sprintf("Classificado como %s (confiança %s). Segue para a extração do comprovante.", desc, conf)
	case reason == ReasonUnknownKind:
		return "O triador não reconheceu o documento: vai para a revisão humana (que documento é esse?)."
	default:
		return fmt.Sprintf("Classificado como %s com confiança %s, abaixo do limiar: vai para a revisão humana.", desc, conf)
	}
}
