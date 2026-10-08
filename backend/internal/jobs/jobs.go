// Package jobs é a fila de processamento assíncrono em PostgreSQL [REQ-046, 04 §7, TASK-005].
//
// Porte da fila do Conciliador (app/servicos/fila.py e app/workers/runner.py, commit 2bc6e26):
// aquisição com SELECT ... FOR UPDATE SKIP LOCKED em ordem de chegada, 3 tentativas por padrão,
// backoff de 2^n segundos e estado terminal. Os nomes seguem o 05 §7 (dead, e não dead_letter).
// Diferenças, todas para não perder job (D-019):
//   - a tentativa conta na aquisição e a posse tem prazo (lease), renovado enquanto o handler roda:
//     se o worker cair, o job volta para a fila em vez de ficar em running para sempre;
//   - o handler grava na mesma transação que marca o job como concluído, e a conclusão só vale se o
//     worker ainda tem a posse: o efeito acontece uma vez por aquisição [06 §11];
//   - enfileirar com dedupe_key é idempotente: um job ativo por (kind, dedupe_key);
//   - o worker só adquire os tipos que sabe executar; os demais esperam na fila.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/db"
	"controle/internal/platform/redact"
	"controle/internal/store"
)

// Estados do job [05 §7].
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusRetry   = "retry"
	StatusDone    = "done"
	StatusDead    = "dead"
)

// Códigos de erro gravados pela fila (os handlers podem usar os seus, ver Coded).
const (
	CodeHandlerError = "HANDLER_ERROR"
	CodePanic        = "HANDLER_PANIC"
	CodeWorkerLost   = "WORKER_LOST"
	CodeShutdown     = "WORKER_SHUTDOWN"
)

// DefaultMaxAttempts é o padrão da origem (JOB_MAX_TENTATIVAS=3).
const DefaultMaxAttempts = 3

// MaxPayloadBytes limita o payload: ele é referencial (IDs), nunca documento [04 §7, 09].
const MaxPayloadBytes = 8 << 10

// maxBackoff limita a espera entre tentativas.
const maxBackoff = time.Hour

var kindRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Backoff é a espera depois da n-ésima tentativa falhar: 2^n segundos, como na origem
// (fila.backoff), limitada a uma hora.
func Backoff(attempt int32) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	secs := math.Pow(2, float64(attempt))
	if secs > maxBackoff.Seconds() {
		return maxBackoff
	}
	return time.Duration(secs) * time.Second
}

// Next é o próximo estado de um job depois de uma tentativa que falhou.
type Next struct {
	Status   string
	RunAfter time.Time
}

// NextAfterFailure decide entre nova tentativa e estado terminal (proximo_estado_falha da origem):
// a tentativa já contada é attempts; se chegou ao máximo, ou se o erro é permanente, o job morre.
func NextAfterFailure(attempts, maxAttempts int32, now time.Time, permanent bool) Next {
	if permanent || attempts >= maxAttempts {
		return Next{Status: StatusDead, RunAfter: now}
	}
	return Next{Status: StatusRetry, RunAfter: now.Add(Backoff(attempts))}
}

// codedError é um erro com código estável e, opcionalmente, permanente (sem nova tentativa).
type codedError struct {
	code      string
	permanent bool
	err       error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// Coded marca err com um código estável (ex.: "LLM_UNAVAILABLE"), gravado em last_error_code.
func Coded(code string, err error) error { return &codedError{code: code, err: err} }

// Permanent marca uma falha que não adianta repetir: o job vai direto para dead.
func Permanent(code string, err error) error {
	return &codedError{code: code, permanent: true, err: err}
}

// classify devolve o código, se a falha é permanente e a mensagem sanitizada.
// CodeOf devolve o código dado por Coded ou Permanent, se houver.
func CodeOf(err error) (string, bool) {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code, true
	}
	return "", false
}

func classify(err error) (code string, permanent bool, message string) {
	code = CodeHandlerError
	var ce *codedError
	if errors.As(err, &ce) {
		code, permanent = ce.code, ce.permanent
	}
	return code, permanent, redact.Message(err.Error())
}

// Job é o estado de um job na API [06 §8]. O payload não é exposto.
type Job struct {
	ID               string     `json:"id"`
	Kind             string     `json:"kind"`
	Status           string     `json:"status"`
	Attempts         int32      `json:"attempts"`
	MaxAttempts      int32      `json:"max_attempts"`
	RunAfter         time.Time  `json:"run_after"`
	LockedAt         *time.Time `json:"locked_at"`
	LockedBy         *string    `json:"locked_by"`
	LastErrorCode    *string    `json:"last_error_code"`
	LastErrorMessage *string    `json:"last_error_message"`
	FinishedAt       *time.Time `json:"finished_at"`
	RetriedAt        *time.Time `json:"retried_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// WorkerOnline: algum worker deu sinal recente. Job pendente sem worker = fila parada.
	WorkerOnline bool `json:"worker_online"`
}

func toJob(r store.Job) Job {
	return Job{
		ID: r.ID, Kind: r.Kind, Status: r.Status, Attempts: r.Attempts, MaxAttempts: r.MaxAttempts,
		RunAfter: r.RunAfter, LockedAt: r.LockedAt, LockedBy: r.LockedBy, LastErrorCode: r.LastErrorCode,
		LastErrorMessage: r.LastErrorMessage, FinishedAt: r.FinishedAt, RetriedAt: r.RetriedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// Spec descreve um job a enfileirar.
type Spec struct {
	Kind string
	// Payload é serializado como objeto JSON com referências (IDs), nunca conteúdo.
	Payload any
	// DedupeKey torna o enfileiramento idempotente; vazio = sem deduplicação.
	DedupeKey string
	// MaxAttempts: 0 = o padrão da fila.
	MaxAttempts int32
	// RunAfter: zero = agora.
	RunAfter time.Time
}

// Queue enfileira jobs.
type Queue struct {
	clock       clock.Clock
	maxAttempts int32
}

// NewQueue cria a fila; maxAttempts é o padrão (JOB_MAX_TENTATIVAS).
func NewQueue(clk clock.Clock, maxAttempts int32) *Queue {
	if maxAttempts < 1 {
		maxAttempts = DefaultMaxAttempts
	}
	return &Queue{clock: clk, maxAttempts: maxAttempts}
}

// Enqueue grava o job na transação ou conexão de quem chama, para o job nascer junto com o dado
// que o motivou. Com DedupeKey e um job ativo igual, devolve o existente e created = false.
func (q *Queue) Enqueue(ctx context.Context, conn store.DBTX, s Spec) (Job, bool, error) {
	if !kindRe.MatchString(s.Kind) {
		return Job{}, false, fmt.Errorf("jobs: tipo inválido %q", s.Kind)
	}
	payload := []byte("{}")
	if s.Payload != nil {
		b, err := json.Marshal(s.Payload)
		if err != nil {
			return Job{}, false, fmt.Errorf("jobs: payload: %w", err)
		}
		payload = b
	}
	if len(payload) > MaxPayloadBytes || len(payload) == 0 || payload[0] != '{' {
		return Job{}, false, fmt.Errorf("jobs: o payload deve ser um objeto JSON referencial de até %d bytes", MaxPayloadBytes)
	}
	maxAttempts := s.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = q.maxAttempts
	}
	now := q.clock.Now()
	runAfter := s.RunAfter
	if runAfter.IsZero() {
		runAfter = now
	}
	var dedupe *string
	if s.DedupeKey != "" {
		dedupe = &s.DedupeKey
	}
	st := store.New(conn)
	row, err := st.EnqueueJob(ctx, store.EnqueueJobParams{
		Kind: s.Kind, Payload: payload, DedupeKey: dedupe, MaxAttempts: maxAttempts, RunAfter: runAfter, Now: now,
	})
	if errors.Is(err, pgx.ErrNoRows) && dedupe != nil {
		existing, err := st.GetActiveJobByDedupeKey(ctx, store.GetActiveJobByDedupeKeyParams{Kind: s.Kind, DedupeKey: dedupe})
		if err != nil {
			return Job{}, false, fmt.Errorf("jobs: job ativo com a mesma chave: %w", err)
		}
		return toJob(existing), false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("jobs: enfileirar %s: %w", s.Kind, err)
	}
	return toJob(row), true, nil
}

const (
	jobNotFound = "JOB_NOT_FOUND"
	jobMissing  = "Job não encontrado."
)

// Service consulta e reenvia jobs pela API [06 §8].
type Service struct {
	db    store.DBTX
	clock clock.Clock
	// onlineWindow: um worker é considerado ativo se deu sinal dentro desta janela.
	onlineWindow time.Duration
}

// NewService cria o serviço da API de jobs.
func NewService(conn store.DBTX, clk clock.Clock) *Service {
	return &Service{db: conn, clock: clk, onlineWindow: 3 * HeartbeatInterval}
}

func (s *Service) workerOnline(ctx context.Context, q *store.Queries) (bool, error) {
	hb, err := q.LatestWorkerHeartbeat(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.clock.Now().Sub(hb.LastSeenAt) <= s.onlineWindow, nil
}

// Get devolve o estado do job.
func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	q := store.New(s.db)
	row, err := q.GetJob(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.NotFound(jobNotFound, jobMissing)
	}
	if err != nil {
		return Job{}, err
	}
	j := toJob(row)
	if j.WorkerOnline, err = s.workerOnline(ctx, q); err != nil {
		return Job{}, err
	}
	return j, nil
}

// Retry devolve à fila um job em estado terminal, com as tentativas zeradas. Só vale para dead: o
// reenvio não pula portões de domínio, que ficam no handler [06 §8].
func (s *Service) Retry(ctx context.Context, id, userID string) (Job, error) {
	q := store.New(s.db)
	row, err := q.RetryDeadJob(ctx, store.RetryDeadJobParams{Now: s.clock.Now(), RetriedBy: userID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		cur, getErr := q.GetJob(ctx, id)
		if errors.Is(getErr, pgx.ErrNoRows) {
			return Job{}, apperr.NotFound(jobNotFound, jobMissing)
		}
		if getErr != nil {
			return Job{}, getErr
		}
		return Job{}, apperr.Conflict("JOB_NOT_RETRYABLE", "Só um job encerrado com falha (dead) pode ser reenviado.").
			WithDetails(map[string]any{"status": cur.Status})
	}
	if db.IsConstraint(err, db.UniqueViolation, "jobs_active_dedupe_uq") {
		return Job{}, apperr.Conflict("JOB_ALREADY_ACTIVE", "Já existe um job ativo para o mesmo item.")
	}
	if err != nil {
		return Job{}, err
	}
	j := toJob(row)
	if j.WorkerOnline, err = s.workerOnline(ctx, q); err != nil {
		return Job{}, err
	}
	return j, nil
}
