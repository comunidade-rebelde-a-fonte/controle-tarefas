package tracer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai/langfuse"
	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/platform/redact"
	"controle/internal/store"
)

// Status de sincronização com o Langfuse (ai_runs, ai_gold_evals e ai_human_evals).
const (
	SyncPending = "pending"
	SyncOK      = "ok"
	SyncFailed  = "failed"
)

// Nomes dos scores, como na origem (CLAUDE.md §10.4 do Fator R).
const (
	ScoreGoldPrefix = "gold_"
	ScoreGoldTotal  = "gold_acerto"
	ScoreHuman      = "human_eval"
)

// agentModule é a etiqueta do módulo no Langfuse.
var agentModule = map[string]string{
	"parser_pgdas": "fator_r", "consultor": "fator_r", "priorizador": "fator_r",
	"triador": "conciliacao", "extrator": "conciliacao", "resolvedor": "conciliacao", "classificador": "conciliacao",
}

// Syncer envia ao Langfuse as corridas e notas ainda não sincronizadas [REQ-041, AC-030].
type Syncer struct {
	client *langfuse.Client
	clock  clock.Clock
	queue  *jobs.Queue
	logger *slog.Logger
}

// NewSyncer cria o sincronizador.
func NewSyncer(client *langfuse.Client, clk clock.Clock, queue *jobs.Queue, logger *slog.Logger) *Syncer {
	return &Syncer{client: client, clock: clk, queue: queue, logger: logger}
}

// Configured indica se há Langfuse configurado.
func (s *Syncer) Configured() bool { return s.client.Configured() }

// Handle é o job langfuse_sync: envia a corrida (se ainda não foi) e as notas pendentes do
// trace. Falha do Langfuse fica registrada (failed, com o erro sanitizado) e o job termina: o
// reenvio periódico tenta de novo, como na origem [AC-030]. Sem Langfuse configurado, não faz
// nada e a corrida fica pending.
func (s *Syncer) Handle(ctx context.Context, tx pgx.Tx, job jobs.Claimed) error {
	if !s.client.Configured() {
		return nil
	}
	var p struct {
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil || !ValidTraceID(p.TraceID) {
		return jobs.Permanent("INVALID_PAYLOAD", errors.New("payload sem trace_id válido"))
	}
	q := store.New(tx)
	run, err := q.LockAIRunForSync(ctx, p.TraceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if run.LangfuseSyncStatus != SyncOK {
		trace, err := toTrace(run)
		if err != nil {
			return jobs.Permanent("INVALID_RUN", err)
		}
		if err := s.client.Export(ctx, trace); err != nil {
			msg := redact.Message(err.Error())
			if err := q.MarkAIRunSyncFailed(ctx, store.MarkAIRunSyncFailedParams{LastError: &msg, Now: s.clock.Now(), TraceID: run.TraceID}); err != nil {
				return err
			}
			s.logger.Warn("Langfuse: trace não enviado; o reenvio periódico tenta de novo", "trace_id", run.TraceID, "error", msg)
			return nil
		}
		now := s.clock.Now()
		if err := q.MarkAIRunSynced(ctx, store.MarkAIRunSyncedParams{Now: &now, TraceID: run.TraceID}); err != nil {
			return err
		}
	}
	return s.syncScores(ctx, q, run.TraceID)
}

// syncScores envia as notas pendentes do trace: gold_<campo> (BOOLEAN) e gold_acerto
// (NUMERIC, entre os campos comparáveis) da nota ouro, e human_eval (CATEGORICAL) da humana.
// O ID do score é o da nota (ou derivado, no agregado): reenviar não duplica.
func (s *Syncer) syncScores(ctx context.Context, q *store.Queries, traceID string) error {
	gold, err := q.ListAIGoldEvals(ctx, traceID)
	if err != nil {
		return err
	}
	human, err := q.ListAIHumanEvals(ctx, traceID)
	if err != nil {
		return err
	}
	var goldOK, goldFailed []string
	pending, hits, comparable := false, 0, 0
	for _, g := range gold {
		if g.Status != GoldUnavailable {
			comparable++
			if g.Status == GoldOK {
				hits++
			}
		}
		if g.LangfuseSyncStatus == SyncOK {
			continue
		}
		pending = true
		if g.Status == GoldUnavailable { // a origem não manda score para campo sem ouro
			goldOK = append(goldOK, g.ID)
			continue
		}
		value := 0.0
		if g.Status == GoldOK {
			value = 1
		}
		if s.send(ctx, langfuse.Score{ID: g.ID, TraceID: traceID, Name: ScoreGoldPrefix + g.Field, Type: langfuse.ScoreBoolean, Value: value}) {
			goldOK = append(goldOK, g.ID)
		} else {
			goldFailed = append(goldFailed, g.ID)
		}
	}
	if pending && comparable > 0 {
		total := langfuse.Score{
			ID: derivedScoreID(traceID, ScoreGoldTotal), TraceID: traceID, Name: ScoreGoldTotal,
			Type: langfuse.ScoreNumeric, Value: float64(hits) / float64(comparable),
		}
		if !s.send(ctx, total) { // reenvia junto com as notas do trace
			goldFailed, goldOK = append(goldFailed, goldOK...), nil
		}
	}
	var humanOK, humanFailed []string
	for _, h := range human {
		if h.LangfuseSyncStatus == SyncOK {
			continue
		}
		sc := langfuse.Score{ID: h.ID, TraceID: traceID, Name: ScoreHuman, Type: langfuse.ScoreCategorical, Value: h.Rating}
		if h.Comment != nil {
			sc.Comment = *h.Comment
		}
		if s.send(ctx, sc) {
			humanOK = append(humanOK, h.ID)
		} else {
			humanFailed = append(humanFailed, h.ID)
		}
	}
	for _, m := range []struct {
		ids    []string
		status string
		mark   func(context.Context, []string, string) error
	}{
		{goldOK, SyncOK, markGold(q)}, {goldFailed, SyncFailed, markGold(q)},
		{humanOK, SyncOK, markHuman(q)}, {humanFailed, SyncFailed, markHuman(q)},
	} {
		if len(m.ids) == 0 {
			continue
		}
		if err := m.mark(ctx, m.ids, m.status); err != nil {
			return err
		}
	}
	return nil
}

func markGold(q *store.Queries) func(context.Context, []string, string) error {
	return func(ctx context.Context, ids []string, status string) error {
		return q.MarkAIGoldEvalsSync(ctx, store.MarkAIGoldEvalsSyncParams{Status: status, Ids: ids})
	}
}

func markHuman(q *store.Queries) func(context.Context, []string, string) error {
	return func(ctx context.Context, ids []string, status string) error {
		return q.MarkAIHumanEvalsSync(ctx, store.MarkAIHumanEvalsSyncParams{Status: status, Ids: ids})
	}
}

func (s *Syncer) send(ctx context.Context, sc langfuse.Score) bool {
	if err := s.client.SendScore(ctx, sc); err != nil {
		s.logger.Warn("Langfuse: score não enviado; o reenvio periódico tenta de novo", "trace_id", sc.TraceID, "score", sc.Name, "error", redact.Message(err.Error()))
		return false
	}
	return true
}

// Requeue enfileira o reenvio das corridas e notas com falha e das pendentes há mais de
// staleAfter (job perdido), no máximo limit por vez: o reenvio periódico da origem (de 5 em 5
// minutos, 50 por vez). Devolve quantos jobs novos entraram na fila.
func (s *Syncer) Requeue(ctx context.Context, db store.DBTX, staleAfter time.Duration, limit int) (int, error) {
	if !s.client.Configured() {
		return 0, nil
	}
	ids, err := store.New(db).ListAIRunsToResync(ctx, store.ListAIRunsToResyncParams{
		StaleBefore: s.clock.Now().Add(-staleAfter), Lim: int32(min(max(limit, 1), 1000)),
	})
	if err != nil {
		return 0, err
	}
	created := 0
	for _, id := range ids {
		_, isNew, err := s.queue.Enqueue(ctx, db, jobs.Spec{
			Kind: KindLangfuseSync, Payload: map[string]string{"trace_id": id}, DedupeKey: id,
		})
		if err != nil {
			return created, err
		}
		if isNew {
			created++
		}
	}
	return created, nil
}

// toTrace monta o trace do Langfuse a partir da corrida local: span raiz com o agente (nome,
// etiquetas e metadados da origem) e os spans guardados.
func toTrace(run store.AiRun) (langfuse.Trace, error) {
	var input, output, decision map[string]any
	var meta map[string]string
	var spans []SpanRecord
	for _, f := range []struct {
		raw  []byte
		dest any
	}{{run.Input, &input}, {run.Output, &output}, {run.Decision, &decision}, {run.Metadata, &meta}, {run.Spans, &spans}} {
		dec := json.NewDecoder(bytes.NewReader(f.raw))
		dec.UseNumber()
		if err := dec.Decode(f.dest); err != nil {
			return langfuse.Trace{}, fmt.Errorf("corrida %s com JSON inválido: %w", run.TraceID, err)
		}
	}
	end := run.UpdatedAt
	if run.LatencyMs != nil {
		end = run.CreatedAt.Add(time.Duration(*run.LatencyMs) * time.Millisecond)
	}
	t := langfuse.Trace{
		ID: run.TraceID, RootID: RootSpanID(run.TraceID), Name: run.Agent, Start: run.CreatedAt, End: end,
		Level: langfuse.LevelDefault, Input: input, Tags: []string{run.Agent, run.Trigger},
	}
	if m, ok := agentModule[run.Agent]; ok {
		t.Tags = append(t.Tags, m)
	}
	switch Status(run.Status) {
	case StatusError:
		t.Level = langfuse.LevelError
		if run.ErrorCode != nil {
			t.StatusMessage = *run.ErrorCode
		}
	case StatusNeedsReview:
		t.Level, t.StatusMessage = langfuse.LevelWarning, string(StatusNeedsReview)
	}
	if output == nil {
		output = map[string]any{}
	}
	output["decisao"] = decision
	t.Output = output
	md := map[string]any{"status": run.Status, "gatilho": run.Trigger}
	for k, v := range meta {
		md[k] = v
	}
	if run.CompanyID != nil {
		md["company_id"] = *run.CompanyID
		t.SessionID = *run.CompanyID
	}
	if run.Competence != nil {
		md["competencia"] = run.Competence.Format("2006-01")
	}
	if run.Provider != nil && run.Model != nil {
		md["modelo"] = *run.Provider + "/" + *run.Model
	}
	if run.Alias != nil {
		md["alias"] = *run.Alias
	}
	if run.ModelConfigVersion != nil {
		md["versao_config_modelo"] = *run.ModelConfigVersion
	}
	if run.PromptVersion != nil {
		md["prompt_version"] = *run.PromptVersion
	}
	if run.Confidence != nil {
		md["confianca"] = run.Confidence.Text()
	}
	t.Metadata = md
	if run.UserID != nil {
		t.UserID = *run.UserID
	}
	for _, sp := range spans {
		ls := langfuse.Span{
			ID: sp.ID, ParentID: sp.ParentID, Name: sp.Name, Type: sp.Type, Start: sp.Start, End: sp.End,
			Level: sp.Level, StatusMessage: sp.StatusMessage, Input: sp.Input, Output: sp.Output,
			Model: sp.Model, ResponseModel: sp.ResponseModel, TokensIn: sp.TokensIn, TokensOut: sp.TokensOut, CostUSD: sp.CostUSD,
		}
		if len(sp.Metadata) > 0 {
			ls.Metadata = map[string]any{}
			for k, v := range sp.Metadata {
				ls.Metadata[k] = v
			}
		}
		t.Spans = append(t.Spans, ls)
	}
	return t, nil
}
