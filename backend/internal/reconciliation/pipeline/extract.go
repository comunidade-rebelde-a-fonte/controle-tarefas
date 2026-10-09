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
	"controle/internal/platform/decimal"
	"controle/internal/platform/pdftext"
	"controle/internal/reconciliation/statement"
	"controle/internal/store"
)

var hundredPct = decimal.FromInt(100)

// MaxAttempts são as tentativas do extrator (contrato congelado da origem).
const MaxAttempts = 3

// strategy é a estratégia de cada tentativa (ESTRATEGIAS da origem): texto nativo, render a
// 150 DPI e render a 300 DPI com o alias extrator_forte.
type strategy struct {
	name string
	dpi  int
}

var strategies = map[int]strategy{1: {"texto_nativo", 72}, 2: {"render_150", 150}, 3: {"render_300", 300}}

// aliasFor é o alias_para_tentativa da origem: a 3ª tentativa escala para o extrator_forte.
func aliasFor(attempt int) ai.Alias {
	if attempt >= MaxAttempts {
		return ai.ExtratorForte
	}
	return ai.Extrator
}

// HandleExtract é o handler do job statement_extract (handle_extracao_extrato da origem): o
// leitor determinístico do CSV ou o extrator, e a importação só com o portão aprovado.
func (p *Processor) HandleExtract(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	id, err := documentID(job)
	if err != nil {
		return err
	}
	q := store.New(tx)
	doc, err := lockDocument(ctx, q, id)
	if err != nil {
		return err
	}
	if doc.Status != StatusTriaged || doc.Kind == nil || *doc.Kind != statement.KindStatement {
		return nil // já processado, em revisão ou não é extrato
	}
	src, err := p.load(ctx, doc)
	if err != nil {
		return err
	}
	if !p.cfg.AgentOnly {
		handled, _, err := p.localCSV(ctx, tx, doc, src, job.ID)
		if err != nil || handled {
			return err
		}
	}
	return p.extract(ctx, tx, doc, src, job.ID)
}

// attemptResult é o que uma tentativa deixou: o extrato que passou pelo schema e o validador.
type attemptResult struct {
	statement *statement.Statement
	result    *statement.Result
}

// extractor guarda o estado da corrida do extrator.
type extractor struct {
	p        *Processor
	tx       pgx.Tx
	q        *store.Queries
	run      *tracer.Run
	doc      store.ReconciliationDocument
	src      source
	jobID    string
	models   map[ai.Alias]ai.Resolved
	playbook *store.BankPlaybook
	// O que aconteceu com o playbook: usado na 1ª tentativa e invalidado.
	playbookUsed, playbookInvalidated string
}

// extract é o extrair_com_loop da origem: até 3 tentativas, cada uma com a sua estratégia; da
// 2ª em diante, com o extrato anterior, o resultado do validador e as dicas. Saída fora do schema
// conta como tentativa. O playbook do banco vai na 1ª tentativa; se ela falhar, ele é invalidado
// e as seguintes redescobrem o leiaute [AC-017].
func (p *Processor) extract(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument, src source, jobID string) error {
	q := store.New(tx)
	if src.pdfErr != nil {
		return p.setState(ctx, q, doc.ID, StatusExtractReview, diagnosis(ReasonUnreadable, nil, map[string]any{"erro": pdfReason(src.pdfErr)}), nil)
	}
	// Os dois aliases são resolvidos no início: a corrida não muda de modelo no meio [D-014].
	models := map[ai.Alias]ai.Resolved{}
	for _, a := range []ai.Alias{ai.Extrator, ai.ExtratorForte} {
		m, err := p.resolver.Resolve(ctx, tx, a)
		if agentUnavailable(err) {
			return p.setState(ctx, q, doc.ID, StatusExtractReview, diagnosis(ReasonAgentDisabled, nil, map[string]any{"alias": string(a)}), nil)
		}
		if err != nil {
			return err
		}
		models[a] = m
	}
	var playbook *store.BankPlaybook
	if doc.Bank != nil {
		pb, err := q.GetValidBankPlaybook(ctx, *doc.Bank)
		switch {
		case err == nil:
			playbook = &pb
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
	}
	_, first := systemPrompt(prompts.Get(extractPromptName))
	_, correction := systemPrompt(prompts.Get(correctionPromptName))
	meta := map[string]string{"modelo_forte": models[ai.ExtratorForte].Model.String(), "tentativas_max": strconv.Itoa(MaxAttempts)}
	if playbook != nil {
		meta["playbook"] = fmt.Sprintf("%s v%d", playbook.Bank, playbook.Version)
	}
	extratorModel := models[ai.Extrator]
	run, err := p.tracer.Begin(ctx, tx, tracer.Spec{
		Agent: agentExtrator, Trigger: triggerUpload, UserID: doc.UploadedBy, CompanyID: doc.CompanyID,
		Competence: docCompetence(doc), Model: &extratorModel, PromptVersion: first.version + ";" + correction.version,
		Input:    map[string]any{"mime": src.mime, "tamanho_bytes": len(src.data), "paginas": src.pageCount},
		Metadata: meta,
	})
	if err != nil {
		return err
	}
	e := &extractor{p: p, tx: tx, q: q, run: run, doc: doc, src: src, jobID: jobID, models: models, playbook: playbook}
	if err := e.loop(ctx); err != nil {
		return run.Fail(err)
	}
	return nil
}

func (e *extractor) loop(ctx context.Context) error {
	var last attemptResult
	succeeded := 0
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		res, err := e.attempt(ctx, attempt, last)
		if err != nil {
			return err
		}
		if res.statement != nil {
			last = res
		}
		if res.result != nil && res.result.OK {
			succeeded = attempt
			break
		}
		// O playbook usado na 1ª tentativa falhou (schema ou portão): invalida e redescobre.
		if attempt == 1 && e.playbook != nil {
			if _, err := e.q.InvalidateBankPlaybooks(ctx, store.InvalidateBankPlaybooksParams{Now: e.p.clock.Now(), Bank: e.playbook.Bank}); err != nil {
				return err
			}
			e.playbookInvalidated = fmt.Sprintf("%s v%d", e.playbook.Bank, e.playbook.Version)
			e.playbook = nil
		}
	}
	return e.decide(ctx, last, succeeded)
}

// attempt executa uma tentativa: monta a entrada da estratégia, chama o modelo, valida o schema e
// o saldo, e grava a linha do log de extração.
func (e *extractor) attempt(ctx context.Context, attempt int, last attemptResult) (attemptResult, error) {
	st := strategies[attempt]
	model := e.models[aliasFor(attempt)]
	parts := []prompts.Prompt{prompts.Get(extractPromptName)}
	if attempt > 1 {
		parts[0] = prompts.Get(correctionPromptName)
	}
	usingPlaybook := e.playbook != nil
	if usingPlaybook {
		parts = append(parts, prompts.Get(playbookPromptName))
		e.playbookUsed = fmt.Sprintf("%s v%d", e.playbook.Bank, e.playbook.Version)
	}
	system, ref := systemPrompt(parts...)
	var out attemptResult
	err := e.run.Span(tracer.SpanParse, map[string]any{"tentativa": attempt, "estrategia": st.name, "alias": string(model.Alias)}, func(s tracer.SpanHandle) error {
		s.Meta("prompt_version", ref.version)
		user, images, err := e.input(ctx, attempt, st, last)
		if err != nil {
			return err
		}
		reply, err := e.p.call(ctx, e.run, llmCall{
			name: "extrair_extrato", model: model, system: system, user: user, images: images, attempt: attempt,
			schema: &litellm.JSONSchema{Name: "extrato_extraido", Schema: statementSchema},
			input:  map[string]any{"estrategia": st.name, "dpi": st.dpi, "imagens": len(images), "caracteres": utf8.RuneCountInString(user), "playbook": usingPlaybook},
		})
		if err != nil {
			return err
		}
		entry := logEntry{alias: string(model.Alias), attempt: attempt, resolvedModel: model.Model.String(), prompt: ref, traceID: e.run.TraceID()}
		var parsed statement.Statement
		perr := statement.ErrInvalidOutput
		if !reply.unusable {
			entry.rawOutput, entry.latency = reply.resp.Content, reply.resp.Latency
			entry.tokensIn, entry.tokensOut, entry.costUSD = usage(reply.resp)
			if reply.resp.Model != "" {
				entry.resolvedModel = reply.resp.Model
			}
			parsed, perr = statement.ParseStatement(reply.resp.Content)
		}
		if perr != nil {
			// JSON inválido ou fora do schema: a tentativa é perdida (TentativaParseError).
			entry.errText = perr.Error()
			s.Warn(ReasonInvalidOutput)
			s.Output(map[string]any{"schema": false})
			return e.p.writeLog(ctx, e.q, e.doc, e.jobID, entry)
		}
		// validar_saldo: o mesmo validador do portão.
		r := statement.Validate(parsed)
		entry.validator = &r
		if err := e.run.Span(tracer.SpanTool, map[string]any{"ferramenta": "validar_saldo"}, func(v tracer.SpanHandle) error {
			v.Output(map[string]any{"ok": r.OK, "diferenca": r.Difference.String(), "transacoes": len(parsed.Transactions)})
			return nil
		}); err != nil {
			return err
		}
		out = attemptResult{statement: &parsed, result: &r}
		s.Output(map[string]any{"schema": true, "ok": r.OK, "diferenca": r.Difference.String()})
		if !r.OK {
			s.Warn(ReasonBalanceMismatch)
		}
		return e.p.writeLog(ctx, e.q, e.doc, e.jobID, entry)
	})
	return out, err
}

// input monta a mensagem da tentativa (_montar_user da origem). A 1ª vai com o texto nativo (ou,
// sem texto, com as páginas renderizadas); a 2ª e a 3ª, com as páginas no DPI da estratégia, o
// extrato anterior, o resultado do validador e as dicas. Imagem enviada vai como está; arquivo
// de texto vai como texto em todas.
func (e *extractor) input(ctx context.Context, attempt int, st strategy, last attemptResult) (string, []litellm.Image, error) {
	lines := []string{fmt.Sprintf("estrategia=%s dpi=%d", st.name, st.dpi)}
	if e.playbook != nil {
		lines = append(lines, "PLAYBOOK VALIDADO="+string(jsonBytes(playbookData(*e.playbook))))
	}
	src := e.src
	if attempt > 1 {
		lines = append(lines, "páginas="+strconv.Itoa(max(src.pageCount, 1)))
		if last.statement != nil && last.result != nil {
			lines = append(lines,
				"anterior="+string(jsonBytes(last.statement)),
				"validacao="+string(jsonBytes(last.result)),
				"dicas="+strings.Join(statement.Hints(*last.statement, *last.result), ","))
		}
	}
	var images []litellm.Image
	switch {
	case src.isImage():
		images = []litellm.Image{{MIME: src.mime, Data: src.data, Detail: "high"}}
	case src.isText():
		lines = append(lines, "TEXTO:\n"+truncateRunes(src.text, maxExtractChars))
	case attempt == 1 && src.hasText():
		lines = append(lines, "TEXTO:\n"+truncateRunes(strings.Join(src.pages, "\n\n"), maxExtractChars))
	default:
		n := min(src.pageCount, maxRenderPages)
		pages := make([]int, n)
		for i := range pages {
			pages[i] = i
		}
		var pngs [][]byte
		err := e.run.Span(tracer.SpanTool, map[string]any{"ferramenta": "renderizar_pagina", "paginas": n, "dpi": st.dpi}, func(s tracer.SpanHandle) error {
			var err error
			pngs, err = e.p.pdf.Render(ctx, src.data, pages, st.dpi)
			if err == nil {
				total := 0
				for _, b := range pngs {
					total += len(b)
				}
				s.Output(map[string]any{"bytes": total})
			}
			return err
		})
		if errors.Is(err, pdftext.ErrUnreadable) || errors.Is(err, pdftext.ErrLimits) {
			lines = append(lines, "render indisponível: "+pdfReason(err))
			break
		}
		if err != nil {
			return "", nil, err
		}
		if src.pageCount > n {
			lines = append(lines, fmt.Sprintf("renderizadas as páginas 1 a %d de %d", n, src.pageCount))
		}
		for _, b := range pngs {
			images = append(images, litellm.Image{MIME: mimePNG, Data: b, Detail: "high"})
		}
	}
	return strings.Join(lines, "\n"), images, nil
}

// playbookData é o playbook preenchido do bloco do prompt (playbook.md da origem): os campos do
// schema congelado (M1.5.1), como dado.
func playbookData(pb store.BankPlaybook) map[string]any {
	var ignored any = []any{}
	if len(pb.IgnoredPages) > 0 {
		ignored = json.RawMessage(pb.IgnoredPages)
	}
	validated := ""
	if pb.ValidatedAt != nil {
		validated = pb.ValidatedAt.Format("2006-01-02")
	}
	return map[string]any{
		"banco": pb.Bank, "versao": pb.Version, "pagina_inicio_tabela": pb.TableStartPage, "paginas_ignorar": ignored,
		"formato_data": pb.DateFormat, "separador_decimal": pb.DecimalSeparator, "representacao_sinal": pb.SignRepresentation,
		"localizacao_saldos": pb.BalanceLocation, "estrategia_vencedora": pb.WinningStrategy, "validado_em": validated,
	}
}

// decide encerra a corrida: importa com o portão aprovado ou deixa o documento em revisão com o
// diagnóstico [AC-015].
func (e *extractor) decide(ctx context.Context, last attemptResult, succeeded int) error {
	attempts := succeeded
	if attempts == 0 {
		attempts = MaxAttempts
	}
	var out importOutcome
	err := e.run.Span(tracer.SpanDecide, map[string]any{"tentativas": attempts}, func(s tracer.SpanHandle) error {
		var err error
		switch {
		case last.statement == nil:
			out = importOutcome{status: StatusExtractReview, reason: ReasonInvalidOutput}
			err = e.p.setState(ctx, e.q, e.doc.ID, StatusExtractReview, diagnosis(ReasonInvalidOutput, nil, map[string]any{"tentativas": MaxAttempts}), nil)
		case succeeded == 0:
			out, err = e.p.importStatement(ctx, e.tx, e.doc, *last.statement, *last.result, MaxAttempts, e.run.TraceID())
		default:
			out, err = e.p.importStatement(ctx, e.tx, e.doc, *last.statement, *last.result, succeeded, e.run.TraceID())
		}
		if err != nil {
			return err
		}
		if out.companyID != "" && e.doc.CompanyID == nil {
			if err := e.run.LinkCompany(ctx, out.companyID); err != nil {
				return err
			}
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
	decision["playbook_usado"] = nilIfEmpty(e.playbookUsed)
	decision["playbook_invalidado"] = nilIfEmpty(e.playbookInvalidated)
	if last.result != nil {
		decision["ok"] = last.result.OK
		decision["diferenca"] = last.result.Difference.String()
	}
	if err := e.run.RecordDecision(ctx, "extracao_extrato", decision); err != nil {
		return err
	}
	text := extractText(out, attempts)
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

func extractText(out importOutcome, attempts int) string {
	switch out.status {
	case StatusImported:
		return fmt.Sprintf("Extrato importado na tentativa %d: %d transações gravadas (%d já existiam). O saldo fechou em R$ 0,00.", attempts, out.inserted, out.duplicates)
	case StatusClosedPeriod:
		return "O saldo fechou, mas o Contábil está fechado na competência do extrato: nada foi importado (pendência de período fechado)."
	}
	switch out.reason {
	case ReasonInvalidOutput:
		return fmt.Sprintf("Nenhuma das %d tentativas trouxe um extrato válido: vai para a revisão humana.", MaxAttempts)
	case ReasonBalanceMismatch:
		return fmt.Sprintf("O saldo não fechou em %d tentativas: nenhuma transação foi importada; vai para a revisão humana com o diagnóstico.", attempts)
	case ReasonNoCompany:
		return "O saldo fechou, mas a conta não está cadastrada e o envio não indicou a empresa: vai para a revisão humana."
	case ReasonAccountOtherFirm:
		return "O saldo fechou, mas a conta pertence a outra empresa: vai para a revisão humana."
	}
	return "Vai para a revisão humana."
}
