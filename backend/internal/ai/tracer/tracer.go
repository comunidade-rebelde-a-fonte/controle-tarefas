// Package tracer é a trilha local das corridas de agente [AC-029, 11 §3, TASK-009]: porte do
// tracer.run do Fator R de origem (tracing/tracer.py, commit 9dd7a74).
//
//   - A corrida nasce na transação do agente (ai_runs): sem trace, nada do agente grava. Decisão
//     de agente só se grava por Run.RecordDecision, e o banco exige trace_id [FR-16].
//   - Spans só plan|parse|tool|decide|render, como na origem, mais as chamadas ao LLM
//     (gerações, com modelo, tokens e custo do gateway).
//   - Falha do agente desfaz a transação e grava a corrida de erro à parte, com o código do erro
//     (nunca a mensagem crua, que pode trazer dado do documento).
//   - Ao terminar, a corrida entra na fila de sincronização com o Langfuse na mesma transação.
//     O Langfuse fora do ar não desfaz nem atrasa o domínio [REQ-042, AC-030].
//
// Uso numa requisição: Tracer.Do. Num job do worker: Begin na transação do job; em erro, devolva
// Run.Fail(err), que o runner grava depois do rollback junto com a falha do job.
package tracer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai"
	"controle/internal/ai/langfuse"
	"controle/internal/ai/litellm"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/store"
)

// Spans do contrato da origem (CLAUDE.md §10.4 do Fator R).
const (
	SpanPlan   = "plan"
	SpanParse  = "parse"
	SpanTool   = "tool"
	SpanDecide = "decide"
	SpanRender = "render"
)

// Status da corrida, como na origem.
type Status string

const (
	StatusOK          Status = "ok"
	StatusError       Status = "error"
	StatusNeedsReview Status = "needs_review"
)

// KindLangfuseSync é o job que envia a corrida e as notas do trace ao Langfuse.
const KindLangfuseSync = "langfuse_sync"

// Códigos gravados na corrida de erro.
const (
	CodeNotFinished = "NOT_FINISHED" // corrida que terminou sem Finish
	CodeAgentError  = "AGENT_ERROR"  // erro sem código próprio
	CodeLLM         = "LLM_UNAVAILABLE"
	CodeTimeout     = "TIMEOUT"
	CodeCanceled    = "CANCELED"
)

var (
	// ErrSpanNotAllowed: span fora de plan|parse|tool|decide|render.
	ErrSpanNotAllowed = errors.New("tracer: span não permitido; use plan, parse, tool, decide ou render")
	// ErrRunClosed: a corrida já terminou.
	ErrRunClosed = errors.New("tracer: corrida já encerrada")
	// ErrDecisionWithoutRun: decisão de agente fora de uma corrida aberta.
	ErrDecisionWithoutRun = errors.New("tracer: decisão de agente só pode ser gravada dentro de uma corrida")
)

var (
	nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,39}$`)
	kindRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	codeRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
)

// Spec descreve a corrida.
type Spec struct {
	Agent      string // ex.: consultor, parser_pgdas, triador
	Trigger    string // ex.: pergunta, upload, rotina
	UserID     *string
	CompanyID  *string
	Competence *competence.Competence // competência da Conciliação ou PA do Fator R
	// Model é o snapshot da resolução do alias (aliases.Resolver), nulo nos agentes
	// determinísticos, como o parser_pgdas (ADR-006).
	Model         *ai.Resolved
	PromptVersion string
	Input         map[string]any
	// Metadata vai para o Langfuse como metadados do trace (parser_version, tabela_vigencia,
	// politica_cpp…), como na origem.
	Metadata map[string]string
}

// Result é o resultado da corrida.
type Result struct {
	Status     Status
	Decision   map[string]any
	Text       string         // texto redigido para o usuário (render), em output.texto
	Output     map[string]any // demais campos da saída
	Confidence *decimal.Decimal
}

// Tracer abre e grava corridas.
type Tracer struct {
	clock clock.Clock
	queue *jobs.Queue
}

// New cria o tracer. clk é o tempo operacional (relógio real), como o da fila de jobs.
func New(clk clock.Clock, queue *jobs.Queue) *Tracer { return &Tracer{clock: clk, queue: queue} }

// Run é uma corrida aberta.
type Run struct {
	t         *Tracer
	tx        pgx.Tx
	traceID   string
	spec      Spec
	start     time.Time
	spans     []SpanRecord
	open      []int // pilha dos spans abertos (índices em spans)
	tokensIn  *int64
	tokensOut *int64
	cost      *decimal.Decimal
	active    bool
}

// SpanRecord é um span guardado na trilha local (ai_runs.spans) e reenviado ao Langfuse.
type SpanRecord struct {
	ID            string            `json:"id"`
	ParentID      string            `json:"parent_id,omitempty"` // vazio = filho da raiz
	Name          string            `json:"name"`
	Type          string            `json:"type"` // chain | tool | generation
	Start         time.Time         `json:"start"`
	End           time.Time         `json:"end"`
	Level         string            `json:"level,omitempty"`
	StatusMessage string            `json:"status_message,omitempty"`
	Input         any               `json:"input,omitempty"`
	Output        any               `json:"output,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Model         string            `json:"model,omitempty"`
	ResponseModel string            `json:"response_model,omitempty"`
	TokensIn      *int              `json:"tokens_in,omitempty"`
	TokensOut     *int              `json:"tokens_out,omitempty"`
	CostUSD       string            `json:"cost_usd,omitempty"`
}

// Begin abre a corrida na transação do agente: grava ai_runs com status provisório de erro
// (NOT_FINISHED), corrigido por Finish. Linhas de domínio do agente podem referenciar o
// trace_id na mesma transação.
func (t *Tracer) Begin(ctx context.Context, tx pgx.Tx, spec Spec) (*Run, error) {
	if !nameRe.MatchString(spec.Agent) || !nameRe.MatchString(spec.Trigger) {
		return nil, fmt.Errorf("tracer: agente %q ou gatilho %q fora do formato", spec.Agent, spec.Trigger)
	}
	c, err := spec.columns()
	if err != nil {
		return nil, err
	}
	run := &Run{t: t, tx: tx, traceID: NewTraceID(), spec: spec, start: t.clock.Now(), active: true}
	if err := store.New(tx).InsertAIRun(ctx, store.InsertAIRunParams{
		TraceID: run.traceID, Agent: spec.Agent, Trigger: spec.Trigger, UserID: spec.UserID, CompanyID: spec.CompanyID,
		Competence: c.competence, Alias: c.alias, Provider: c.provider, Model: c.model, ModelConfigVersion: c.version,
		PromptVersion: c.prompt, Metadata: c.metadata, Input: c.input, CreatedAt: run.start,
	}); err != nil {
		return nil, fmt.Errorf("tracer: abrir a corrida: %w", err)
	}
	return run, nil
}

// columns são os campos da corrida que vêm da Spec, iguais na abertura e na corrida de erro.
type columns struct {
	competence             *time.Time
	alias, provider, model *string
	version                *int32
	prompt                 *string
	input, metadata        []byte
}

func (s Spec) columns() (columns, error) {
	input, err := objectJSON(s.Input)
	if err != nil {
		return columns{}, err
	}
	meta, err := json.Marshal(nonNilMap(s.Metadata))
	if err != nil {
		return columns{}, err
	}
	c := columns{input: input, metadata: meta}
	if s.Competence != nil {
		d := s.Competence.FirstDay()
		c.competence = &d
	}
	if m := s.Model; m != nil {
		alias, provider, name, version := string(m.Alias), m.Model.Provider, m.Model.Name, m.Version
		c.alias, c.provider, c.model, c.version = &alias, &provider, &name, &version
	}
	if s.PromptVersion != "" {
		p := s.PromptVersion
		c.prompt = &p
	}
	return c, nil
}

// TraceID é o trace_id da corrida, o mesmo no Langfuse.
func (r *Run) TraceID() string { return r.traceID }

// SpanHandle registra a saída e os avisos de um span em andamento.
type SpanHandle struct {
	run *Run
	idx int
}

// Output grava a saída do span (sanitizada).
func (s SpanHandle) Output(v any) {
	if clean, err := sanitize(v); err == nil {
		s.run.spans[s.idx].Output = clean
	}
}

// Meta grava um metadado do span.
func (s SpanHandle) Meta(key, value string) {
	rec := &s.run.spans[s.idx]
	if rec.Metadata == nil {
		rec.Metadata = map[string]string{}
	}
	rec.Metadata[key] = value
}

// Warn marca o span com nível WARNING (ex.: confiança baixa).
func (s SpanHandle) Warn(message string) {
	rec := &s.run.spans[s.idx]
	rec.Level, rec.StatusMessage = langfuse.LevelWarning, message
}

// Span executa fn como um passo da corrida. Spans podem ser aninhados. Erro de fn marca o span
// com nível ERROR e o código do erro, e é devolvido.
func (r *Run) Span(name string, input any, fn func(s SpanHandle) error) error {
	typ := ""
	switch name {
	case SpanPlan, SpanDecide, SpanRender:
		typ = langfuse.TypeChain
	case SpanParse, SpanTool:
		typ = langfuse.TypeTool
	default:
		return fmt.Errorf("%w: %q", ErrSpanNotAllowed, name)
	}
	if !r.active {
		return ErrRunClosed
	}
	in, err := sanitize(input)
	if err != nil {
		return err
	}
	r.spans = append(r.spans, SpanRecord{ID: newSpanID(), ParentID: r.parent(), Name: name, Type: typ, Start: r.t.clock.Now(), Input: in})
	idx := len(r.spans) - 1
	r.open = append(r.open, idx)
	err = fn(SpanHandle{run: r, idx: idx})
	r.open = r.open[:len(r.open)-1]
	rec := &r.spans[idx]
	rec.End = r.t.clock.Now()
	if err != nil {
		rec.Level, rec.StatusMessage = langfuse.LevelError, errorCode(err)
	}
	return err
}

func (r *Run) parent() string {
	if len(r.open) == 0 {
		return ""
	}
	return r.spans[r.open[len(r.open)-1]].ID
}

// Generation é uma chamada ao LLM pelo gateway.
type Generation struct {
	Name     string
	Model    ai.Model          // modelo pedido (o snapshot da corrida)
	Response *litellm.Response // nil se a chamada falhou
	Err      error
	// Input e Output são opcionais: só o necessário para auditar, nunca o documento inteiro.
	// No envio ao Langfuse ainda passam pelo mascaramento.
	Input, Output any
}

// Generation registra a chamada ao LLM dentro do span em andamento e soma tokens e custo à
// corrida.
func (r *Run) Generation(g Generation) error {
	if !r.active {
		return ErrRunClosed
	}
	if !nameRe.MatchString(g.Name) {
		return fmt.Errorf("tracer: nome de geração fora do formato: %q", g.Name)
	}
	in, err := sanitize(g.Input)
	if err != nil {
		return err
	}
	out, err := sanitize(g.Output)
	if err != nil {
		return err
	}
	end := r.t.clock.Now()
	rec := SpanRecord{
		ID: newSpanID(), ParentID: r.parent(), Name: g.Name, Type: langfuse.TypeGeneration,
		Start: end, End: end, Input: in, Output: out, Model: g.Model.String(),
	}
	if g.Err != nil {
		rec.Level, rec.StatusMessage = langfuse.LevelError, errorCode(g.Err)
	}
	if resp := g.Response; resp != nil {
		rec.Start = end.Add(-resp.Latency)
		rec.ResponseModel = resp.Model
		in, out := resp.Usage.PromptTokens, resp.Usage.CompletionTokens
		rec.TokensIn, rec.TokensOut = &in, &out
		r.tokensIn = addInt(r.tokensIn, in)
		r.tokensOut = addInt(r.tokensOut, out)
		if c, err := decimal.ParseNumber(resp.CostUSD); err == nil && c.Sign() >= 0 {
			rec.CostUSD = resp.CostUSD
			if r.cost == nil {
				r.cost = &c
			} else {
				sum := r.cost.Add(c)
				r.cost = &sum
			}
		}
	}
	r.spans = append(r.spans, rec)
	return nil
}

func addInt(acc *int64, v int) *int64 {
	n := int64(v)
	if acc != nil {
		n += *acc
	}
	return &n
}

// RecordDecision grava uma decisão do agente na transação da corrida: o único caminho de
// gravação de decisão de agente [FR-16].
func (r *Run) RecordDecision(ctx context.Context, kind string, data map[string]any) error {
	if r == nil || !r.active {
		return ErrDecisionWithoutRun
	}
	if !kindRe.MatchString(kind) {
		return fmt.Errorf("tracer: tipo de decisão fora do formato: %q", kind)
	}
	b, err := objectJSON(data)
	if err != nil {
		return err
	}
	return store.New(r.tx).InsertAIDecision(ctx, store.InsertAIDecisionParams{
		TraceID: r.traceID, Agent: r.spec.Agent, Kind: kind, Data: b, CreatedAt: r.t.clock.Now(),
	})
}

// Finish grava o resultado e enfileira a sincronização com o Langfuse, na transação da corrida.
// A corrida só vale com o commit de quem chama.
func (r *Run) Finish(ctx context.Context, res Result) error {
	if !r.active {
		return ErrRunClosed
	}
	switch res.Status {
	case StatusOK, StatusNeedsReview, StatusError:
	default:
		return fmt.Errorf("tracer: status inválido %q", res.Status)
	}
	if c := res.Confidence; c != nil && (c.Sign() < 0 || c.Cmp(decimal.FromInt(1)) > 0) {
		return fmt.Errorf("tracer: confiança fora de 0 a 1: %s", c.Text())
	}
	output := map[string]any{"texto": res.Text}
	for k, v := range res.Output {
		output[k] = v
	}
	rec, err := r.record(output, res.Decision, r.t.clock.Now())
	if err != nil {
		return err
	}
	params := store.FinishAIRunParams{
		Output: rec.output, Decision: rec.decision, Spans: rec.spans, Status: string(res.Status),
		LatencyMs: &rec.latency, TokensIn: rec.tokensIn, TokensOut: rec.tokensOut, CostUsd: rec.cost,
		UpdatedAt: rec.end, TraceID: r.traceID,
	}
	if c := res.Confidence; c != nil {
		q := c.Quantize(-4)
		params.Confidence = &q
	}
	n, err := store.New(r.tx).FinishAIRun(ctx, params)
	if err != nil {
		return fmt.Errorf("tracer: gravar o resultado: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("tracer: corrida %s não encontrada na transação", r.traceID)
	}
	r.active = false
	return r.t.enqueueSync(ctx, r.tx, r.traceID)
}

// Fail encerra a corrida com erro. Num job do worker, devolva o resultado como erro do handler:
// depois do rollback da tentativa, o runner grava a corrida de erro junto com a falha do job.
func (r *Run) Fail(err error) error {
	r.active = false
	return &failedRun{run: r, err: err, end: r.t.clock.Now()}
}

type failedRun struct {
	run *Run
	err error
	end time.Time
}

func (f *failedRun) Error() string { return f.err.Error() }
func (f *failedRun) Unwrap() error { return f.err }

// Record grava a corrida de erro (implementa jobs.AfterRollback).
func (f *failedRun) Record(ctx context.Context, tx pgx.Tx) error {
	return f.run.insertError(ctx, tx, f.err, f.end)
}

var _ jobs.AfterRollback = (*failedRun)(nil)

// insertError grava a corrida de erro numa transação nova (a do agente foi desfeita): sem
// decisões, com o código do erro e os spans e o consumo do LLM até a falha.
func (r *Run) insertError(ctx context.Context, tx pgx.Tx, cause error, end time.Time) error {
	code := errorCode(cause)
	rec, err := r.record(map[string]any{"erro": code}, nil, end)
	if err != nil {
		return err
	}
	c, err := r.spec.columns()
	if err != nil {
		return err
	}
	if err := store.New(tx).InsertAIRunFull(ctx, store.InsertAIRunFullParams{
		TraceID: r.traceID, Agent: r.spec.Agent, Trigger: r.spec.Trigger, UserID: r.spec.UserID, CompanyID: r.spec.CompanyID,
		Competence: c.competence, Alias: c.alias, Provider: c.provider, Model: c.model, ModelConfigVersion: c.version,
		PromptVersion: c.prompt, Metadata: c.metadata, Input: c.input, Output: rec.output, Decision: rec.decision,
		Spans: rec.spans, Status: string(StatusError), LatencyMs: &rec.latency, TokensIn: rec.tokensIn,
		TokensOut: rec.tokensOut, CostUsd: rec.cost, ErrorCode: &code, CreatedAt: r.start, UpdatedAt: rec.end,
	}); err != nil {
		return fmt.Errorf("tracer: gravar a corrida de erro: %w", err)
	}
	return r.t.enqueueSync(ctx, tx, r.traceID)
}
