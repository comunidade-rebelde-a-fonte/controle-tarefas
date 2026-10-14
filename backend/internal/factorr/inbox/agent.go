package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai/tracer"
	"controle/internal/factorr"
	"controle/internal/factorr/engine"
	"controle/internal/factorr/pgdas"
	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/pdftext"
	"controle/internal/store"
)

// Códigos de falha do job (permanentes: repetir não adianta).
const (
	CodeInvalidPayload   = "INVALID_PAYLOAD"
	CodeDocumentNotFound = "PGDAS_DOCUMENT_NOT_FOUND"
	CodeFileMissing      = "PGDAS_FILE_MISSING"
	CodeInvalidPA        = "PGDAS_PA_INVALIDO"
)

// Motivo de revisão quando o CNPJ lido não está na carteira.
const reasonCNPJNotFound = "cnpj_nao_encontrado"

// movement é o que o vínculo fez com a competência do PA (Movimento da origem).
type movement string

const (
	movCreated       movement = "criado"
	movExisting      movement = "existente"
	movNoRPA         movement = "sem_rpa"
	movNotApplicable movement = "nao_aplicavel"
)

// Observações dos meses lidos do extrato, como na origem.
const (
	obsPA                  = "Receita do PA lida do extrato PGDAS-D"
	obsPriorWithPayroll    = "Receita e folha lidas do extrato PGDAS-D do PA %s. Folha = total declarado no PGDAS-D, sem divisão entre pró-labore, salários, CPP e FGTS."
	obsPriorWithoutPayroll = "Receita lida do extrato PGDAS-D do PA %s. Atividade sem fator r: folha não se aplica."
)

// priorReasons explica por que os meses anteriores não foram lançados (MOTIVOS_ANTERIORES).
var priorReasons = map[string]string{
	pgdas.ReceitasNaoConferem: "as receitas mês a mês não somam o RBT12 declarado",
	pgdas.FolhasNaoConferem:   "a folha mês a mês não soma a FS12 declarada",
}

// prior é o que o vínculo fez com os 12 meses anteriores ao PA (MesesAnteriores).
type prior struct {
	created, existing int
	withPayroll       bool
	reason            string
}

func (p prior) asMap() map[string]any {
	var reason any
	if p.reason != "" {
		reason = p.reason
	}
	return map[string]any{"criados": p.created, "existentes": p.existing, "com_folha": p.withPayroll, "motivo": reason}
}

// writer grava os meses lidos do extrato (criar_se_ausente): só em competência vazia, nunca
// sobrescreve, folha só o total declarado e conferido, sem inventar a divisão [AC-008].
type writer struct {
	q         *store.Queries
	now       time.Time
	userID    *string
	docID     string
	companyID string
}

func (w writer) insert(ctx context.Context, c competence.Competence, receita decimal.Decimal, folha *decimal.Decimal, obs string) (bool, error) {
	_, err := w.q.InsertFactorRMovementFromPgdas(ctx, store.InsertFactorRMovementFromPgdasParams{
		CompanyID: w.companyID, Competence: c.FirstDay(), ReceitaBruta: receita, FolhaTotalDeclaradaPgdas: folha,
		Observacao: &obs, PgdasDocumentID: &w.docID, UpdatedBy: w.userID, Now: w.now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// competenceOf lê a competência como parse_competencia da origem. Inválida (ex.: ano anterior a
// 1900) é falha permanente no job e 422 na API; a origem levantava exceção no meio do vínculo.
func competenceOf(s string) (competence.Competence, error) {
	y, m, err := pgdas.ParsePA(s)
	if err != nil {
		return competence.Competence{}, jobs.Permanent(CodeInvalidPA,
			apperr.Unprocessable(CodeInvalidPA, fmt.Sprintf("Competência %q lida do extrato é inválida.", s)))
	}
	return competence.New(y, time.Month(m)), nil
}

// apply é o _aplicar_vinculo da origem: os meses anteriores conferidos e a receita do PA.
func (w writer) apply(ctx context.Context, st pgdas.Stored) (movement, prior, error) {
	months, withPayroll, reason, err := st.PriorMonths()
	if err != nil {
		return "", prior{}, err
	}
	pa := ""
	if st.PA != nil {
		pa = *st.PA
	}
	obs := fmt.Sprintf(obsPriorWithoutPayroll, pa)
	if withPayroll {
		obs = fmt.Sprintf(obsPriorWithPayroll, pa)
	}
	keys := make([]string, 0, len(months))
	for k := range months {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	p := prior{reason: reason}
	for _, k := range keys {
		c, err := competenceOf(k)
		if err != nil {
			return "", prior{}, err
		}
		mv := months[k]
		var folha *decimal.Decimal
		if withPayroll {
			folha = &mv.Folha
		}
		created, err := w.insert(ctx, c, mv.Receita, folha, obs)
		if err != nil {
			return "", prior{}, err
		}
		if created {
			p.created++
		} else {
			p.existing++
		}
	}
	p.withPayroll = withPayroll && p.created > 0
	if st.PA == nil || st.RPA == nil {
		return movNoRPA, p, nil
	}
	c, err := competenceOf(*st.PA)
	if err != nil {
		return "", prior{}, err
	}
	rpa, err := decimal.Parse(*st.RPA)
	if err != nil {
		return "", prior{}, fmt.Errorf("inbox: RPA %q: %w", *st.RPA, err)
	}
	created, err := w.insert(ctx, c, rpa, nil, obsPA)
	if err != nil {
		return "", prior{}, err
	}
	if created {
		return movCreated, p, nil
	}
	return movExisting, p, nil
}

// --- Textos (render), sempre com o disclaimer ---

func linkedText(st pgdas.Stored, mov movement, companyName string, p prior) string {
	pa := "não identificado"
	if st.PA != nil && *st.PA != "" {
		pa = *st.PA
	}
	frases := map[movement]string{
		movCreated:       fmt.Sprintf("Receita do PA %s lançada na competência %s (origem pgdas).", pa, pa),
		movExisting:      fmt.Sprintf("A competência %s já tinha lançamento; nada foi alterado.", pa),
		movNoRPA:         "O extrato não trouxe a receita do PA; nenhum movimento foi criado.",
		movNotApplicable: "Nenhum movimento foi criado.",
	}
	final := " A folha não é alterada."
	if p.withPayroll && p.created > 0 {
		final = " Nenhum lançamento existente foi alterado."
	}
	return fmt.Sprintf("Extrato vinculado a %s. %s%s%s", companyName, frases[mov], priorSentence(p), final)
}

func priorSentence(p prior) string {
	if p.created > 0 {
		content := "receita"
		if p.withPayroll {
			content = "receita e folha total declarada"
		}
		return fmt.Sprintf(" %d meses anteriores lançados a partir do extrato (%s); competências que já existiam não foram alteradas.", p.created, content)
	}
	if p.reason != "" {
		return fmt.Sprintf(" Meses anteriores não lançados: %s.", priorReasons[p.reason])
	}
	return ""
}

func reviewText(st pgdas.Stored, motivo string) string {
	pa := "não identificado"
	if st.PA != nil && *st.PA != "" {
		pa = *st.PA
	}
	return fmt.Sprintf("Extrato do PA %s precisa de revisão: %s.", pa, motivo)
}

const rejectedText = "Extrato rejeitado pelo analista."

// --- Nota ouro (services/ouro_pgdas.py) ---

func okOrError(ok bool) string {
	if ok {
		return tracer.GoldOK
	}
	return tracer.GoldError
}

func abs(d decimal.Decimal) decimal.Decimal {
	if d.Sign() < 0 {
		return d.Neg()
	}
	return d
}

// within é _dentro da origem: com esperado zero, só zero vale.
func within(obtained, expected, tol decimal.Decimal) bool {
	if expected.IsZero() {
		return obtained.IsZero()
	}
	return abs(obtained.Sub(expected)).Cmp(tol.Mul(abs(expected))) <= 0
}

func compareNumber(field string, obtained *string, expected *decimal.Decimal, tol decimal.Decimal) (tracer.GoldResult, error) {
	if obtained == nil || expected == nil {
		var exp *string
		if expected != nil {
			s := expected.String()
			exp = &s
		}
		return tracer.GoldResult{Field: field, Expected: exp, Obtained: obtained, Status: tracer.GoldUnavailable}, nil
	}
	ob, err := decimal.Parse(*obtained)
	if err != nil {
		return tracer.GoldResult{}, fmt.Errorf("inbox: %s lido %q: %w", field, *obtained, err)
	}
	ok := within(ob, *expected, tol)
	exp := expected.String()
	return tracer.GoldResult{Field: field, Expected: &exp, Obtained: obtained, WithinTolerance: &ok, Status: okOrError(ok)}, nil
}

// gold avalia o extrato vinculado contra o motor sobre a série lançada (avaliar da origem): o
// CNPJ contra o da empresa; o PA não tem referência; RBT12, FS12, Fator R e anexo contra o
// cálculo da série sem movimentos do PGDAS-D, com a tolerância da política.
func gold(ctx context.Context, tr *tracer.Tracer, tx pgx.Tx, q *store.Queries, traceID string, st pgdas.Stored, companyID, companyCNPJ string) error {
	pol, err := q.GetFactorRPolicy(ctx)
	if err != nil {
		return err
	}
	tol := pol.ToleranciaOuroPct
	cnpjOK := st.CNPJ != nil && *st.CNPJ == companyCNPJ
	results := []tracer.GoldResult{
		{Field: "cnpj", Expected: &companyCNPJ, Obtained: st.CNPJ, WithinTolerance: &cnpjOK, Status: okOrError(cnpjOK)},
		// A série mensal não informa o PA: não há referência independente para ele.
		{Field: "pa", Obtained: st.PA, Status: tracer.GoldUnavailable},
	}
	var ref *engine.Result
	if st.PA != nil {
		pa, err := competenceOf(*st.PA)
		if err != nil {
			return err
		}
		if ref, err = factorr.GoldReference(ctx, q, companyID, pa); err != nil {
			return err
		}
	}
	if ref == nil {
		for _, f := range []string{"rbt12", "fs12", "fator_r", "anexo"} {
			results = append(results, tracer.GoldResult{Field: f, Obtained: st.Field(f), Status: tracer.GoldUnavailable})
		}
	} else {
		for _, c := range []struct {
			field    string
			obtained *string
			expected *decimal.Decimal
		}{{"rbt12", st.RBT12, &ref.RBT12}, {"fs12", st.FS12, &ref.FS12}, {"fator_r", st.FatorR, ref.FactorR}} {
			r, err := compareNumber(c.field, c.obtained, c.expected, tol)
			if err != nil {
				return err
			}
			results = append(results, r)
		}
		var expected *string
		if ref.Annex != nil {
			a := string(*ref.Annex)
			expected = &a
		}
		an := tracer.GoldResult{Field: "anexo", Expected: expected, Obtained: st.Anexo, Status: tracer.GoldUnavailable}
		if st.Anexo != nil {
			ok := expected != nil && *st.Anexo == *expected
			an.WithinTolerance, an.Status = &ok, okOrError(ok)
		}
		results = append(results, an)
	}
	_, err = tr.RecordGold(ctx, tx, traceID, results)
	return err
}

// --- Agente no job pgdas_parse (gatilho upload) ---

// Agent processa o documento enviado no worker: extrai o texto, faz o parse, procura a empresa
// pelo CNPJ e decide, numa corrida do parser_pgdas (receber_documento da origem).
type Agent struct {
	clock     clock.Clock
	tracer    *tracer.Tracer
	storage   Storage
	extractor *pdftext.Extractor
}

// NewAgent cria o agente do job. clk é o tempo operacional (o da fila).
func NewAgent(clk clock.Clock, tr *tracer.Tracer, storage Storage, extractor *pdftext.Extractor) *Agent {
	return &Agent{clock: clk, tracer: tr, storage: storage, extractor: extractor}
}

type parsePayload struct {
	DocumentID string `json:"document_id"`
}

func fieldsMap(r pgdas.Result) map[string]any {
	out := make(map[string]any, len(pgdas.Fields))
	for _, f := range pgdas.Fields {
		out[f] = r.Fields[f]
	}
	return out
}

// HandleParse é o handler do job pgdas_parse. Tudo é gravado na transação do job: documento,
// lançamentos, corrida, decisão e nota ouro; em erro, a tentativa é desfeita e a corrida de erro
// fica gravada com a falha do job.
func (a *Agent) HandleParse(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	var p parsePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil || p.DocumentID == "" {
		return jobs.Permanent(CodeInvalidPayload, errors.New("pgdas_parse: payload sem document_id"))
	}
	q := store.New(tx)
	doc, err := q.GetPgdasDocumentForUpdate(ctx, p.DocumentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.Permanent(CodeDocumentNotFound, errors.New("pgdas_parse: documento inexistente"))
	}
	if err != nil {
		return err
	}
	if doc.Status != StatusReceived {
		return nil // já processado, ou rejeitado antes do processamento
	}
	pol, err := q.GetFactorRPolicy(ctx)
	if err != nil {
		return err
	}
	data, err := a.storage.Read(doc.Sha256)
	if errors.Is(err, fs.ErrNotExist) {
		return jobs.Permanent(CodeFileMissing, errors.New("pgdas_parse: original ausente no armazenamento"))
	}
	if err != nil {
		return err
	}
	limiar := pol.LimiarConfiancaParser
	run, err := a.tracer.Begin(ctx, tx, tracer.Spec{
		Agent: AgentName, Trigger: TriggerUpload, UserID: doc.UploadedBy,
		Input:    map[string]any{"mime": doc.Mime, "tamanho_bytes": doc.SizeBytes},
		Metadata: map[string]string{"limiar": limiar.String(), "parser_version": pgdas.ParserVersion},
	})
	if err != nil {
		return err
	}
	if err := a.decideUpload(ctx, tx, q, run, doc, data, limiar); err != nil {
		return run.Fail(err)
	}
	return nil
}

func (a *Agent) decideUpload(ctx context.Context, tx pgx.Tx, q *store.Queries, run *tracer.Run, doc store.FactorRPgdasDocument, data []byte, limiar decimal.Decimal) error {
	var res pgdas.Result
	var extracted string
	reason := ""
	err := run.Span(tracer.SpanParse, map[string]any{"mime": doc.Mime, "tamanho": doc.SizeBytes}, func(s tracer.SpanHandle) error {
		ext, err := a.extractor.Extract(ctx, data, doc.Mime)
		if err != nil {
			return err // falha do extrator ou contexto cancelado: o job tenta de novo
		}
		if ext.Reason != "" {
			res, reason = pgdas.Empty(ext.Reason), ext.Reason
		} else {
			r, perr := pgdas.Parse(ext.Text)
			switch {
			case errors.Is(perr, pgdas.ErrParse):
				// Na origem a exceção derrubava o processo isolado: extracao_excedeu_limites.
				res, reason = pgdas.Empty(pdftext.ReasonLimits), pdftext.ReasonLimits
			case perr != nil:
				return perr
			default:
				res, extracted = r, ext.Text
			}
		}
		s.Output(map[string]any{
			"campos": fieldsMap(res), "confianca": res.Confidence.String(), "motivos": res.Reasons,
			"parser_version": res.ParserVersion,
		})
		return nil
	})
	if err != nil {
		return err
	}
	st := res.Stored()
	var companyID, companyName, companyCNPJ string
	err = run.Span(tracer.SpanTool, map[string]any{"cnpj": st.CNPJ}, func(s tracer.SpanHandle) error {
		if st.CNPJ != nil && pgdas.CNPJValido(*st.CNPJ) {
			row, err := q.FindCompanyByCnpj(ctx, *st.CNPJ)
			switch {
			case err == nil:
				companyID, companyName, companyCNPJ = row.ID, row.LegalName, *st.CNPJ
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
		}
		var out any
		if companyID != "" {
			out = companyID
		}
		s.Output(map[string]any{"company_id": out})
		return nil
	})
	if err != nil {
		return err
	}
	now := a.clock.Now()
	status, motivo := StatusNeedsReview, ""
	mov, p := movNotApplicable, prior{}
	var decision map[string]any
	err = run.Span(tracer.SpanDecide, nil, func(s tracer.SpanHandle) error {
		switch {
		case reason != "":
			motivo = reason
		case companyID == "":
			motivo = reasonCNPJNotFound
		case res.Confidence.Cmp(limiar) < 0:
			motivo = fmt.Sprintf("confianca_baixa (%s < %s)", res.Confidence, limiar)
		default:
			status = StatusLinked
			var err error
			w := writer{q: q, now: now, userID: doc.UploadedBy, docID: doc.ID, companyID: companyID}
			if mov, p, err = w.apply(ctx, st); err != nil {
				return err
			}
			if err := run.LinkCompany(ctx, companyID); err != nil {
				return err
			}
		}
		// A empresa só fica no documento vinculado; com confiança baixa ela foi achada, mas não vale.
		linked := ""
		if status == StatusLinked {
			linked = companyID
		}
		if err := saveParse(ctx, q, doc.ID, run.TraceID(), status, motivo, linked, res, extracted, now); err != nil {
			return err
		}
		decision = map[string]any{
			"document_id": doc.ID, "status": status, "motivo": nilIfEmpty(motivo), "company_id": nilIfEmpty(linked),
			"confianca": res.Confidence.String(), "limiar": limiar.String(), "movimento": string(mov),
			"competencia": st.PA, "meses_anteriores": p.asMap(),
		}
		if err := run.RecordDecision(ctx, "pgdas_documento", decision); err != nil {
			return err
		}
		s.Output(decision)
		return nil
	})
	if err != nil {
		return err
	}
	text := reviewText(st, motivo)
	if status == StatusLinked {
		text = linkedText(st, mov, companyName, p)
	}
	text = factorr.WithDisclaimer(text)
	if err := run.Span(tracer.SpanRender, nil, func(s tracer.SpanHandle) error {
		s.Output(map[string]any{"texto": text})
		return nil
	}); err != nil {
		return err
	}
	runStatus := tracer.StatusNeedsReview
	if status == StatusLinked {
		runStatus = tracer.StatusOK
	}
	conf := res.Confidence
	if err := run.Finish(ctx, tracer.Result{Status: runStatus, Decision: decision, Text: text, Confidence: &conf}); err != nil {
		return err
	}
	if status == StatusLinked {
		return gold(ctx, a.tracer, tx, q, run.TraceID(), st, companyID, companyCNPJ)
	}
	return nil
}

// saveParse grava o resultado do parse e a decisão no documento.
func saveParse(ctx context.Context, q *store.Queries, docID, traceID, status, motivo, companyID string, res pgdas.Result, text string, now time.Time) error {
	parsed, err := json.Marshal(res.Stored())
	if err != nil {
		return err
	}
	var pa *time.Time
	if f := res.Fields["pa"]; f != nil {
		if y, m, err := pgdas.ParsePA(*f); err == nil {
			d := competence.New(y, time.Month(m)).FirstDay()
			pa = &d
		}
	}
	conf := res.Confidence
	version := res.ParserVersion
	return q.SavePgdasParse(ctx, store.SavePgdasParseParams{
		Status: status, Motivo: nilIfEmptyPtr(motivo), CompanyID: nilIfEmptyPtr(companyID),
		CnpjExtraido: res.Fields["cnpj"], Pa: pa, Confidence: &conf, ParserVersion: &version,
		ParsedJson: parsed, ExtractedText: nilIfEmptyPtr(text), TraceID: &traceID, Now: now, ID: docID,
	})
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilIfEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
