package tracer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/ai/litellm"
	"controle/internal/jobs"
	"controle/internal/platform/apperr"
	"controle/internal/platform/decimal"
	"controle/internal/store"
)

// Beginner abre transações (o pool do banco).
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Do executa fn como uma corrida completa numa transação própria, como o tracer.run da origem:
// grava o resultado e enfileira a sincronização; se fn falhar, desfaz tudo e grava a corrida de
// erro à parte. Devolve o trace_id (também na falha, se a corrida chegou a abrir).
func (t *Tracer) Do(ctx context.Context, db Beginner, spec Spec, fn func(ctx context.Context, tx pgx.Tx, run *Run) (Result, error)) (string, error) {
	var run *Run
	var end time.Time
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		var err error
		if run, err = t.Begin(ctx, tx, spec); err != nil {
			return err
		}
		res, err := fn(ctx, tx, run)
		if err != nil {
			end = t.clock.Now()
			return err
		}
		return run.Finish(ctx, res)
	})
	if err == nil {
		return run.traceID, nil
	}
	if run == nil {
		return "", err
	}
	if end.IsZero() {
		end = t.clock.Now()
	}
	rec := pgx.BeginFunc(context.WithoutCancel(ctx), db, func(tx pgx.Tx) error {
		return run.insertError(context.WithoutCancel(ctx), tx, err, end)
	})
	if rec != nil {
		return run.traceID, errors.Join(err, rec)
	}
	return run.traceID, err
}

// errorCode é o código gravado no lugar da mensagem do erro: o código do domínio ou da fila,
// LLM_UNAVAILABLE para falha do gateway, TIMEOUT e CANCELED para o contexto, e AGENT_ERROR no
// resto. A mensagem crua nunca é gravada: pode trazer dado do documento.
func errorCode(err error) string {
	var ae *apperr.Error
	var gw *litellm.Error
	switch {
	case errors.As(err, &ae) && codeRe.MatchString(ae.Code):
		return ae.Code
	case errors.Is(err, context.DeadlineExceeded):
		return CodeTimeout
	case errors.Is(err, context.Canceled):
		return CodeCanceled
	case errors.As(err, &gw), errors.Is(err, litellm.ErrNotConfigured):
		return CodeLLM
	}
	if code, ok := jobs.CodeOf(err); ok && codeRe.MatchString(code) {
		return code
	}
	return CodeAgentError
}

// enqueueSync põe a sincronização do trace na fila, na transação dada (idempotente por trace).
func (t *Tracer) enqueueSync(ctx context.Context, tx store.DBTX, traceID string) error {
	_, _, err := t.queue.Enqueue(ctx, tx, jobs.Spec{
		Kind: KindLangfuseSync, Payload: map[string]string{"trace_id": traceID}, DedupeKey: traceID,
	})
	if err != nil {
		return fmt.Errorf("tracer: enfileirar a sincronização com o Langfuse: %w", err)
	}
	return nil
}

type recorded struct {
	output, decision, spans []byte
	latency                 int32
	tokensIn, tokensOut     *int32
	cost                    *decimal.Decimal
	end                     time.Time
}

// record serializa o que a corrida grava no fim: saída, decisão, spans, latência e consumo do
// LLM. Spans ainda abertos (falha no meio de um passo) são fechados em end.
func (r *Run) record(output, decision map[string]any, end time.Time) (recorded, error) {
	for _, idx := range r.open {
		if r.spans[idx].End.IsZero() {
			r.spans[idx].End = end
		}
	}
	out, err := objectJSON(output)
	if err != nil {
		return recorded{}, err
	}
	dec, err := objectJSON(decision)
	if err != nil {
		return recorded{}, err
	}
	spans := r.spans
	if spans == nil {
		spans = []SpanRecord{}
	}
	sp, err := json.Marshal(spans)
	if err != nil {
		return recorded{}, fmt.Errorf("tracer: spans: %w", err)
	}
	rec := recorded{output: out, decision: dec, spans: sp, end: end,
		latency: clamp32(end.Sub(r.start).Milliseconds()), tokensIn: to32(r.tokensIn), tokensOut: to32(r.tokensOut)}
	if r.cost != nil {
		// numeric(18,8): custo de uma corrida cabe com folga; fora disso, fica sem custo.
		if c := r.cost.Quantize(-8); c.Fits(10, 8) {
			rec.cost = &c
		}
	}
	return rec, nil
}

func clamp32(v int64) int32 {
	switch {
	case v < 0:
		return 0
	case v > math.MaxInt32:
		return math.MaxInt32
	}
	return int32(v)
}

func to32(v *int64) *int32 {
	if v == nil {
		return nil
	}
	n := clamp32(*v)
	return &n
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
