// Package runs é a consulta da auditoria local de IA [06 §9, FR-18, AC-029, AC-031]: lista e
// detalhe das corridas de agente (com spans, decisões e notas, sem depender do Langfuse), nota
// humana e o resumo do painel de observabilidade da origem (corridas, pendências, acerto ouro e
// humano, métricas por agente, custo e tokens do LLM).
//
// Os números do painel vêm da trilha local, a fonte de verdade: o custo e os tokens são os do
// gateway, gravados em cada corrida (a origem os buscava na API do Langfuse).
package runs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/ai/tracer"
	"controle/internal/platform/clock"
	"controle/internal/platform/decimal"
	"controle/internal/store"
	"controle/internal/user"
)

// Run é a corrida na lista.
type Run struct {
	TraceID            string           `json:"trace_id"`
	Agent              string           `json:"agent"`
	Trigger            string           `json:"trigger"`
	Status             string           `json:"status"`
	Confidence         *decimal.Decimal `json:"confidence"`
	LatencyMs          *int32           `json:"latency_ms"`
	CompanyID          *string          `json:"company_id"`
	Competence         *string          `json:"competence"`
	Alias              *string          `json:"alias"`
	Provider           *string          `json:"provider"`
	Model              *string          `json:"model"`
	TokensIn           *int32           `json:"tokens_in"`
	TokensOut          *int32           `json:"tokens_out"`
	CostUSD            *decimal.Decimal `json:"cost_usd"`
	LangfuseSyncStatus string           `json:"langfuse_sync_status"`
	HasHumanEval       bool             `json:"has_human_eval"`
	CreatedAt          time.Time        `json:"created_at"`
}

// List é uma página de corridas.
type List struct {
	Items []Run `json:"items"`
	Total int64 `json:"total"`
}

// Decision é uma decisão gravada pelo agente.
type Decision struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// GoldEval é a nota ouro de um campo.
type GoldEval struct {
	Field           string  `json:"field"`
	Expected        *string `json:"expected"`
	Obtained        *string `json:"obtained"`
	WithinTolerance *bool   `json:"within_tolerance"`
	Status          string  `json:"status"`
}

// HumanEval é uma nota humana.
type HumanEval struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Rating    string    `json:"rating"`
	Comment   *string   `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

// Detail é a corrida com entrada, saída, decisão, spans, decisões e notas.
type Detail struct {
	Run
	ModelConfigVersion *int32            `json:"model_config_version"`
	PromptVersion      *string           `json:"prompt_version"`
	ErrorCode          *string           `json:"error_code"`
	Metadata           map[string]string `json:"metadata"`
	Input              json.RawMessage   `json:"input"`
	Output             json.RawMessage   `json:"output"`
	Decision           json.RawMessage   `json:"decision"`
	Spans              json.RawMessage   `json:"spans"`
	Decisions          []Decision        `json:"decisions"`
	GoldEvals          []GoldEval        `json:"gold_evals"`
	HumanEvals         []HumanEval       `json:"human_evals"`
	LangfuseAttempts   int32             `json:"langfuse_attempts"`
	LangfuseLastError  *string           `json:"langfuse_last_error"`
	LangfuseSyncedAt   *time.Time        `json:"langfuse_synced_at"`
	// LangfuseURL abre o trace no Langfuse (configuração não secreta); nulo sem Langfuse.
	LangfuseURL *string `json:"langfuse_url"`
}

// AgentSummary são as métricas de um agente.
type AgentSummary struct {
	Agent         string           `json:"agent"`
	Runs          int64            `json:"runs"`
	AvgLatencyMs  *int64           `json:"avg_latency_ms"`
	AvgConfidence *decimal.Decimal `json:"avg_confidence"`
	Errors        int64            `json:"errors"`
	Reviews       int64            `json:"reviews"`
}

// Summary é o painel de observabilidade (FR-18).
type Summary struct {
	RunsLast24h      int64            `json:"runs_24h"`
	RunsTotal        int64            `json:"runs_total"`
	PendingHumanEval int64            `json:"pending_human_eval"`
	GoldAccuracy     *decimal.Decimal `json:"gold_accuracy"`
	HumanAccuracy    *decimal.Decimal `json:"human_accuracy"`
	ByAgent          []AgentSummary   `json:"by_agent"`
	LLMCost30d       *decimal.Decimal `json:"llm_cost_30d"`
	LLMTokens30d     *int64           `json:"llm_tokens_30d"`
}

// Filter filtra a lista.
type Filter struct {
	Agent            *string
	Status           *string
	CompanyID        *string
	WithoutHumanEval bool
	Limit, Offset    int32
}

// EvaluationInput é a nota humana enviada pela tela.
type EvaluationInput struct {
	Rating  string  `json:"rating"`
	Comment *string `json:"comment"`
}

// Service consulta a trilha local e registra notas humanas.
type Service struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	tracer   *tracer.Tracer
	traceURL func(traceID string) *string
}

// NewService cria o serviço. clk é o tempo operacional (o mesmo das corridas); traceURL monta o
// link para o Langfuse (nil = sem link).
func NewService(pool *pgxpool.Pool, clk clock.Clock, tr *tracer.Tracer, traceURL func(string) *string) *Service {
	if traceURL == nil {
		traceURL = func(string) *string { return nil }
	}
	return &Service{pool: pool, clock: clk, tracer: tr, traceURL: traceURL}
}

// List devolve as corridas mais recentes primeiro.
func (s *Service) List(ctx context.Context, f Filter) (List, error) {
	q := store.New(s.pool)
	rows, err := q.ListAIRuns(ctx, store.ListAIRunsParams{
		Agent: f.Agent, Status: f.Status, CompanyID: f.CompanyID, WithoutHumanEval: f.WithoutHumanEval, Lim: f.Limit, Off: f.Offset,
	})
	if err != nil {
		return List{}, err
	}
	total, err := q.CountAIRuns(ctx, store.CountAIRunsParams{
		Agent: f.Agent, Status: f.Status, CompanyID: f.CompanyID, WithoutHumanEval: f.WithoutHumanEval,
	})
	if err != nil {
		return List{}, err
	}
	out := List{Items: make([]Run, 0, len(rows)), Total: total}
	for _, r := range rows {
		out.Items = append(out.Items, Run{
			TraceID: r.TraceID, Agent: r.Agent, Trigger: r.Trigger, Status: r.Status, Confidence: scaled(r.Confidence, -4),
			LatencyMs: r.LatencyMs, CompanyID: r.CompanyID, Competence: month(r.Competence), Alias: r.Alias,
			Provider: r.Provider, Model: r.Model, TokensIn: r.TokensIn, TokensOut: r.TokensOut, CostUSD: scaled(r.CostUsd, -8),
			LangfuseSyncStatus: r.LangfuseSyncStatus, HasHumanEval: r.HasHumanEval, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// Get devolve o detalhe da corrida. Os spans vêm da trilha local: o detalhe não depende do
// Langfuse estar no ar (na origem, ficavam "indisponíveis").
func (s *Service) Get(ctx context.Context, traceID string) (Detail, error) {
	if !tracer.ValidTraceID(traceID) {
		return Detail{}, tracer.ErrRunNotFound
	}
	q := store.New(s.pool)
	r, err := q.GetAIRun(ctx, traceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, tracer.ErrRunNotFound
	}
	if err != nil {
		return Detail{}, err
	}
	decisions, err := q.ListAIDecisions(ctx, traceID)
	if err != nil {
		return Detail{}, err
	}
	gold, err := q.ListAIGoldEvals(ctx, traceID)
	if err != nil {
		return Detail{}, err
	}
	human, err := q.ListAIHumanEvals(ctx, traceID)
	if err != nil {
		return Detail{}, err
	}
	var meta map[string]string
	if err := json.Unmarshal(r.Metadata, &meta); err != nil {
		return Detail{}, err
	}
	d := Detail{
		Run: Run{
			TraceID: r.TraceID, Agent: r.Agent, Trigger: r.Trigger, Status: r.Status, Confidence: scaled(r.Confidence, -4),
			LatencyMs: r.LatencyMs, CompanyID: r.CompanyID, Competence: month(r.Competence), Alias: r.Alias,
			Provider: r.Provider, Model: r.Model, TokensIn: r.TokensIn, TokensOut: r.TokensOut, CostUSD: scaled(r.CostUsd, -8),
			LangfuseSyncStatus: r.LangfuseSyncStatus, HasHumanEval: len(human) > 0, CreatedAt: r.CreatedAt,
		},
		ModelConfigVersion: r.ModelConfigVersion, PromptVersion: r.PromptVersion, ErrorCode: r.ErrorCode, Metadata: meta,
		Input: r.Input, Output: r.Output, Decision: r.Decision, Spans: r.Spans,
		Decisions: make([]Decision, 0, len(decisions)), GoldEvals: make([]GoldEval, 0, len(gold)), HumanEvals: make([]HumanEval, 0, len(human)),
		LangfuseAttempts: r.LangfuseAttempts, LangfuseLastError: r.LangfuseLastError, LangfuseSyncedAt: r.LangfuseSyncedAt,
		LangfuseURL: s.traceURL(r.TraceID),
	}
	for _, x := range decisions {
		d.Decisions = append(d.Decisions, Decision{ID: x.ID, Kind: x.Kind, Data: x.Data, CreatedAt: x.CreatedAt})
	}
	for _, g := range gold {
		d.GoldEvals = append(d.GoldEvals, GoldEval{Field: g.Field, Expected: g.Expected, Obtained: g.Obtained, WithinTolerance: g.WithinTolerance, Status: g.Status})
	}
	for _, h := range human {
		d.HumanEvals = append(d.HumanEvals, humanOut(h))
	}
	return d, nil
}

// Evaluate grava a nota humana e enfileira o envio ao Langfuse [AC-031].
func (s *Service) Evaluate(ctx context.Context, traceID string, in EvaluationInput) (HumanEval, error) {
	if !tracer.ValidTraceID(traceID) {
		return HumanEval{}, tracer.ErrRunNotFound
	}
	var ev tracer.HumanEval
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		ev, err = s.tracer.RecordHuman(ctx, tx, traceID, user.FromContext(ctx), in.Rating, in.Comment)
		return err
	})
	if err != nil {
		return HumanEval{}, err
	}
	return humanOut(ev), nil
}

// Summary devolve o painel. As taxas de acerto seguem a origem: ouro = ok ÷ (ok + erro) entre
// campos comparáveis; humana = acerto ÷ notas.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	q := store.New(s.pool)
	now := s.clock.Now()
	totals, err := q.AIRunTotals(ctx, store.AIRunTotalsParams{Since24h: now.Add(-24 * time.Hour), Since30d: now.AddDate(0, 0, -30)})
	if err != nil {
		return Summary{}, err
	}
	gold, err := q.AIGoldTotals(ctx)
	if err != nil {
		return Summary{}, err
	}
	human, err := q.AIHumanTotals(ctx)
	if err != nil {
		return Summary{}, err
	}
	agents, err := q.AIRunsByAgent(ctx)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{
		RunsLast24h: totals.Last24h, RunsTotal: totals.Total, PendingHumanEval: totals.WithoutHumanEval,
		GoldAccuracy:  ratio(gold.Ok, gold.Ok+gold.Erro),
		HumanAccuracy: ratio(human.Acerto, human.Total),
		ByAgent:       make([]AgentSummary, 0, len(agents)),
	}
	if totals.CostCount > 0 {
		out.LLMCost30d = scaled(&totals.Cost30d, -8)
	}
	if totals.TokensCount > 0 {
		out.LLMTokens30d = &totals.Tokens30d
	}
	for _, a := range agents {
		as := AgentSummary{Agent: a.Agent, Runs: a.Runs, Errors: a.Errors, Reviews: a.Reviews}
		if a.LatencyCount > 0 {
			lat := a.AvgLatencyMs
			as.AvgLatencyMs = &lat
		}
		if a.ConfidenceCount > 0 {
			as.AvgConfidence = scaled(&a.AvgConfidence, -4)
		}
		out.ByAgent = append(out.ByAgent, as)
	}
	return out, nil
}

func humanOut(h store.AiHumanEval) HumanEval {
	return HumanEval{ID: h.ID, UserID: h.UserID, Rating: h.Rating, Comment: h.Comment, CreatedAt: h.CreatedAt}
}

// ratio é a divisão exata (Decimal do Python), nula sem denominador.
func ratio(num, den int64) *decimal.Decimal {
	if den == 0 {
		return nil
	}
	r := decimal.FromInt(num).Quo(decimal.FromInt(den))
	return &r
}

// scaled reaplica a escala da coluna: o pgx perde a escala do zero (ADR-007).
func scaled(d *decimal.Decimal, exp int32) *decimal.Decimal {
	if d == nil {
		return nil
	}
	q := d.Quantize(exp)
	return &q
}

func month(d *time.Time) *string {
	if d == nil {
		return nil
	}
	s := d.Format("2006-01")
	return &s
}
