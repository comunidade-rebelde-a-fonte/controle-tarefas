package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/clock"
	"controle/internal/store"
)

// HeartbeatInterval é a frequência do batimento do worker em worker_heartbeats.
const HeartbeatInterval = 10 * time.Second

// Claimed é o job adquirido, entregue ao handler.
type Claimed struct {
	ID          string
	Kind        string
	Payload     json.RawMessage
	Attempt     int32
	MaxAttempts int32
}

// Handler executa um job. Tudo o que ele grava em tx só vale se o job for concluído: a conclusão
// é gravada na mesma transação e exige que o worker ainda tenha a posse do job [06 §11]. Chamadas
// externas (LLM, storage) devem ser idempotentes ou toleradas na repetição: um job só repete se a
// tentativa anterior não foi concluída.
type Handler func(ctx context.Context, tx pgx.Tx, job Claimed) error

// RunnerConfig configura o consumo da fila.
type RunnerConfig struct {
	WorkerID string
	// Concurrency é o número de jobs executados ao mesmo tempo (padrão 1, como a origem).
	Concurrency int
	// PollInterval é a espera com a fila vazia (JOB_POLL_SECONDS; padrão 1 s, como a origem).
	PollInterval time.Duration
	// Lease é o prazo da posse de um job, renovado a cada Lease/3 enquanto o handler roda.
	Lease time.Duration
	// ShutdownGrace é quanto os jobs em andamento podem levar para terminar no encerramento.
	ShutdownGrace time.Duration
}

// Runner consome a fila: adquire, executa, conclui ou registra a falha, renova a posse dos jobs
// em andamento, retoma os jobs de workers que caíram e registra o batimento do worker.
type Runner struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	logger   *slog.Logger
	cfg      RunnerConfig
	handlers map[string]Handler
	kinds    []string
}

var errLeaseLost = errors.New("jobs: o worker perdeu a posse do job")

// NewRunner cria o consumidor com os handlers por tipo de job.
func NewRunner(pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger, cfg RunnerConfig, handlers map[string]Handler) *Runner {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.Lease <= 0 {
		cfg.Lease = 5 * time.Minute
	}
	if cfg.ShutdownGrace <= 0 {
		cfg.ShutdownGrace = 30 * time.Second
	}
	kinds := make([]string, 0, len(handlers))
	for k := range handlers {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	return &Runner{pool: pool, clock: clk, logger: logger, cfg: cfg, handlers: handlers, kinds: kinds}
}

// Kinds são os tipos de job que este worker adquire.
func (r *Runner) Kinds() []string { return slices.Clone(r.kinds) }

// Run consome a fila até ctx ser cancelado. Os jobs em andamento têm ShutdownGrace para terminar;
// depois disso, o contexto deles é cancelado e a tentativa é registrada como falha.
func (r *Runner) Run(ctx context.Context) error {
	hard, cancelHard := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelHard()
	go func() {
		select {
		case <-ctx.Done():
		case <-hard.Done():
			return
		}
		t := time.NewTimer(r.cfg.ShutdownGrace)
		defer t.Stop()
		select {
		case <-t.C:
			cancelHard()
		case <-hard.Done():
		}
	}()

	r.logger.Info("fila de jobs: consumindo", "worker_id", r.cfg.WorkerID, "kinds", r.kinds,
		"concurrency", r.cfg.Concurrency, "lease", r.cfg.Lease.String())
	started := r.clock.Now()
	var wg sync.WaitGroup
	wg.Go(func() {
		r.every(ctx, HeartbeatInterval, func() { r.logErr("batimento do worker", r.Heartbeat(ctx, started)) })
	})
	wg.Go(func() {
		r.every(ctx, max(r.cfg.Lease/2, time.Second), func() {
			_, err := r.ReclaimExpired(ctx)
			r.logErr("retomada de jobs com posse vencida", err)
		})
	})
	for range r.cfg.Concurrency {
		wg.Go(func() { r.consume(ctx, hard) })
	}
	wg.Wait()
	r.logger.Info("fila de jobs: encerrada", "worker_id", r.cfg.WorkerID)
	return nil
}

func (r *Runner) logErr(what string, err error) {
	if err != nil && !errors.Is(err, context.Canceled) {
		r.logger.Error("fila de jobs: "+what, "worker_id", r.cfg.WorkerID, "error", err)
	}
}

// every executa fn agora e a cada intervalo, até ctx ser cancelado.
func (r *Runner) every(ctx context.Context, interval time.Duration, fn func()) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		fn()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r *Runner) consume(ctx, run context.Context) {
	for ctx.Err() == nil {
		worked, err := r.processOne(ctx, run)
		r.logErr("aquisição", err)
		if worked && err == nil {
			continue
		}
		t := time.NewTimer(r.cfg.PollInterval)
		select {
		case <-ctx.Done():
		case <-t.C:
		}
		t.Stop()
	}
}

// ProcessOne adquire e executa um job pronto; false se não havia nenhum.
func (r *Runner) ProcessOne(ctx context.Context) (bool, error) { return r.processOne(ctx, ctx) }

func (r *Runner) processOne(claimCtx, runCtx context.Context) (bool, error) {
	if len(r.kinds) == 0 {
		return false, nil
	}
	row, err := store.New(r.pool).ClaimJob(claimCtx, store.ClaimJobParams{
		Now: r.clock.Now(), WorkerID: r.cfg.WorkerID, Kinds: r.kinds,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("adquirir job: %w", err)
	}
	r.execute(runCtx, row)
	return true, nil
}

func (r *Runner) execute(runCtx context.Context, row store.Job) {
	log := r.logger.With("job_id", row.ID, "kind", row.Kind, "attempt", row.Attempts, "max_attempts", row.MaxAttempts,
		"worker_id", r.cfg.WorkerID)
	jobCtx, cancel := context.WithCancel(runCtx)
	defer cancel()
	stop := r.keepLease(jobCtx, row.ID, cancel, log)
	start := time.Now()
	err := r.runHandler(jobCtx, row)
	stop()
	switch {
	case err == nil:
		log.Info("job concluído", "duration_ms", time.Since(start).Milliseconds())
	case errors.Is(err, errLeaseLost):
		log.Warn("job abandonado: a posse venceu e outro worker o retomou; os efeitos desta tentativa foram desfeitos")
	default:
		r.fail(row, err, runCtx.Err() != nil, log)
	}
}

// runHandler executa o handler e conclui o job na mesma transação, só se a posse ainda é deste
// worker. Pânico no handler vira falha da tentativa, sem derrubar o worker.
func (r *Runner) runHandler(ctx context.Context, row store.Job) (err error) {
	h := r.handlers[row.Kind]
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transação do job: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			err = Coded(CodePanic, fmt.Errorf("pânico no handler %s: %v", row.Kind, p))
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	claimed := Claimed{ID: row.ID, Kind: row.Kind, Payload: row.Payload, Attempt: row.Attempts, MaxAttempts: row.MaxAttempts}
	if err = h(ctx, tx, claimed); err != nil {
		return err
	}
	n, err := store.New(tx).CompleteJob(ctx, store.CompleteJobParams{Now: r.clock.Now(), ID: row.ID, WorkerID: r.cfg.WorkerID})
	if err != nil {
		return fmt.Errorf("concluir job: %w", err)
	}
	if n == 0 {
		return errLeaseLost
	}
	return tx.Commit(ctx)
}

// keepLease renova a posse a cada Lease/3; se o job foi retomado por outro worker, cancela a
// execução. stop encerra a renovação.
func (r *Runner) keepLease(ctx context.Context, id string, cancel context.CancelFunc, log *slog.Logger) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		t := time.NewTicker(max(r.cfg.Lease/3, 10*time.Millisecond))
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				n, err := store.New(r.pool).ExtendJobLease(ctx, store.ExtendJobLeaseParams{Now: r.clock.Now(), ID: id, WorkerID: r.cfg.WorkerID})
				if err != nil {
					if ctx.Err() == nil {
						log.Warn("não foi possível renovar a posse do job", "error", err)
					}
					continue
				}
				if n == 0 {
					log.Warn("posse do job perdida durante a execução")
					cancel()
					return
				}
			}
		}
	})
	return func() {
		close(done)
		wg.Wait()
	}
}

// fail registra a tentativa que falhou: nova tentativa com backoff ou dead.
func (r *Runner) fail(row store.Job, err error, shuttingDown bool, log *slog.Logger) {
	code, permanent, message := classify(err)
	if shuttingDown && errors.Is(err, context.Canceled) {
		code = CodeShutdown
	}
	now := r.clock.Now()
	next := NextAfterFailure(row.Attempts, row.MaxAttempts, now, permanent)
	var finished *time.Time
	if next.Status == StatusDead {
		finished = &now
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, dbErr := store.New(r.pool).FailJob(ctx, store.FailJobParams{
		Status: next.Status, RunAfter: next.RunAfter, ErrorCode: code, ErrorMessage: message, FinishedAt: finished,
		Now: now, ID: row.ID, WorkerID: r.cfg.WorkerID,
	})
	switch {
	case dbErr != nil:
		log.Error("não foi possível registrar a falha do job; a posse vai vencer e ele volta para a fila", "error", dbErr)
	case n == 0:
		log.Warn("falha do job não registrada: a posse já era de outro worker", "error_code", code)
	case next.Status == StatusDead:
		log.Error("job encerrado sem sucesso (dead)", "error_code", code, "error", message)
	default:
		log.Warn("tentativa do job falhou; nova tentativa agendada", "error_code", code, "error", message, "run_after", next.RunAfter)
	}
}

// ReclaimExpired devolve à fila (ou encerra) os jobs cujo worker parou de renovar a posse.
func (r *Runner) ReclaimExpired(ctx context.Context) (int, error) {
	now := r.clock.Now()
	cutoff := now.Add(-r.cfg.Lease)
	count := 0
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		rows, err := q.ListExpiredJobLeases(ctx, cutoff)
		if err != nil {
			return err
		}
		for _, j := range rows {
			next := NextAfterFailure(j.Attempts, j.MaxAttempts, now, false)
			var finished *time.Time
			if next.Status == StatusDead {
				finished = &now
			}
			owner := ""
			if j.LockedBy != nil {
				owner = *j.LockedBy
			}
			n, err := q.ReleaseExpiredJob(ctx, store.ReleaseExpiredJobParams{
				Status: next.Status, RunAfter: next.RunAfter, ErrorCode: CodeWorkerLost,
				ErrorMessage: fmt.Sprintf("O worker %s parou de renovar a posse do job (prazo de %s).", owner, r.cfg.Lease),
				FinishedAt:   finished, Now: now, ID: j.ID, Cutoff: cutoff,
			})
			if err != nil {
				return err
			}
			if n == 1 {
				count++
				r.logger.Warn("job retomado de um worker que parou", "job_id", j.ID, "kind", j.Kind, "previous_worker", owner,
					"status", next.Status, "attempt", j.Attempts)
			}
		}
		return nil
	})
	return count, err
}

// Heartbeat registra que este worker está ativo.
func (r *Runner) Heartbeat(ctx context.Context, started time.Time) error {
	return store.New(r.pool).TouchWorkerHeartbeat(ctx, store.TouchWorkerHeartbeatParams{
		WorkerID: r.cfg.WorkerID, StartedAt: started, Now: r.clock.Now(),
	})
}
