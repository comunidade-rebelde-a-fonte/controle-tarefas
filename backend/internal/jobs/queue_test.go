package jobs_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/jobs"
	"controle/internal/platform/clock"
	"controle/internal/store"
	"controle/internal/testutil"
)

// TEST-005: fila PostgreSQL — concorrência com SKIP LOCKED, retry, estado terminal e
// idempotência, contra Postgres real.

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func t0() *clock.Fixed {
	return clock.NewFixed(time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC), time.UTC)
}

func runner(pool *pgxpool.Pool, clk clock.Clock, worker string, handlers map[string]jobs.Handler, lease time.Duration) *jobs.Runner {
	return jobs.NewRunner(pool, clk, quiet, jobs.RunnerConfig{
		WorkerID: worker, Concurrency: 2, PollInterval: 10 * time.Millisecond, Lease: lease,
	}, handlers)
}

func enqueue(t *testing.T, pool *pgxpool.Pool, clk clock.Clock, s jobs.Spec) jobs.Job {
	t.Helper()
	j, _, err := jobs.NewQueue(clk, 3).Enqueue(context.Background(), pool, s)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func get(t *testing.T, pool *pgxpool.Pool, id string) store.Job {
	t.Helper()
	j, err := store.New(pool).GetJob(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func str(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestTEST005_EnqueueWithDedupeKeyIsIdempotent(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := t0()
	q := jobs.NewQueue(clk, 3)
	ctx := context.Background()
	spec := jobs.Spec{Kind: "pgdas_parse", Payload: map[string]string{"document_id": "d1"}, DedupeKey: "doc:d1"}
	first, created, err := q.Enqueue(ctx, pool, spec)
	if err != nil || !created || first.Status != jobs.StatusPending || first.MaxAttempts != 3 {
		t.Fatalf("1º = %+v, %v, %v", first, created, err)
	}
	again, created, err := q.Enqueue(ctx, pool, spec)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("repetido = %+v, %v, %v (esperado o mesmo job)", again, created, err)
	}
	// Sem a chave, cada chamada é um job novo.
	a, _, _ := q.Enqueue(ctx, pool, jobs.Spec{Kind: "ping"})
	b, _, _ := q.Enqueue(ctx, pool, jobs.Spec{Kind: "ping"})
	if a.ID == b.ID {
		t.Fatal("jobs sem chave não podem ser deduplicados")
	}
	// Concluído o job, a mesma chave enfileira de novo.
	r := runner(pool, clk, "w1", map[string]jobs.Handler{"pgdas_parse": func(context.Context, pgx.Tx, jobs.Claimed) error { return nil }}, time.Minute)
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatalf("processar = %v, %v", ok, err)
	}
	next, created, err := q.Enqueue(ctx, pool, spec)
	if err != nil || !created || next.ID == first.ID {
		t.Fatalf("depois de concluído = %+v, %v, %v", next, created, err)
	}
	// Enfileirar na transação de quem chama: some junto no rollback.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inTx, _, err := q.Enqueue(ctx, tx, jobs.Spec{Kind: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if _, err := store.New(pool).GetJob(ctx, inTx.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("job enfileirado numa transação desfeita continua existindo: %v", err)
	}
}

func TestEnqueue_RejectsInvalidKindAndPayload(t *testing.T) {
	pool := testutil.NewDB(t)
	q := jobs.NewQueue(t0(), 3)
	for name, s := range map[string]jobs.Spec{
		"tipo com maiúscula": {Kind: "PGDAS"},
		"payload lista":      {Kind: "ping", Payload: []int{1}},
		"payload grande":     {Kind: "ping", Payload: map[string]string{"blob": strings.Repeat("x", jobs.MaxPayloadBytes)}},
	} {
		if _, _, err := q.Enqueue(context.Background(), pool, s); err == nil {
			t.Errorf("%s: deveria ser recusado", name)
		}
	}
}

// Vários workers concorrentes: cada job executa exatamente uma vez (SKIP LOCKED).
func TestTEST005_ConcurrentWorkersExecuteEachJobOnce(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := clock.NewSystem(time.UTC)
	const total = 60
	for i := range total {
		enqueue(t, pool, clk, jobs.Spec{Kind: "work", Payload: map[string]int{"n": i}})
	}
	var mu sync.Mutex
	runs := map[string]int{}
	handler := func(_ context.Context, _ pgx.Tx, j jobs.Claimed) error {
		mu.Lock()
		runs[j.ID]++
		mu.Unlock()
		time.Sleep(2 * time.Millisecond)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for w := range 6 {
		r := runner(pool, clk, fmt.Sprintf("w%d", w), map[string]jobs.Handler{"work": handler}, time.Minute)
		wg.Go(func() { _ = r.Run(ctx) })
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		var done int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM jobs WHERE status = 'done'`).Scan(&done); err != nil {
			t.Fatal(err)
		}
		if done == total {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("só %d de %d jobs concluídos", done, total)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(runs) != total {
		t.Fatalf("%d jobs executados; esperado %d", len(runs), total)
	}
	for id, n := range runs {
		if n != 1 {
			t.Fatalf("job %s executou %d vezes", id, n)
		}
	}
	var badAttempts int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM jobs WHERE attempts <> 1 OR locked_by IS NOT NULL`).Scan(&badAttempts); err != nil || badAttempts != 0 {
		t.Fatalf("jobs com tentativas ou posse inesperadas: %d, %v", badAttempts, err)
	}
}

// Falha → retry com backoff de 2^n s (origem) → sucesso na 3ª tentativa.
func TestTEST005_RetryWithBackoffThenSuccess(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := t0()
	ctx := context.Background()
	j := enqueue(t, pool, clk, jobs.Spec{Kind: "flaky"})
	calls := 0
	r := runner(pool, clk, "w1", map[string]jobs.Handler{"flaky": func(context.Context, pgx.Tx, jobs.Claimed) error {
		calls++
		if calls < 3 {
			return jobs.Coded("LLM_UNAVAILABLE", errors.New("gateway fora do ar"))
		}
		return nil
	}}, time.Minute)

	mustProcess := func(want bool) {
		t.Helper()
		ok, err := r.ProcessOne(ctx)
		if err != nil || ok != want {
			t.Fatalf("ProcessOne = %v, %v; esperado %v", ok, err, want)
		}
	}
	mustProcess(true)
	got := get(t, pool, j.ID)
	if got.Status != jobs.StatusRetry || got.Attempts != 1 || str(got.LastErrorCode) != "LLM_UNAVAILABLE" ||
		!got.RunAfter.Equal(clk.Now().Add(2*time.Second)) || got.LockedBy != nil {
		t.Fatalf("depois da 1ª falha = %+v", got)
	}
	mustProcess(false) // ainda no backoff
	clk.Advance(2 * time.Second)
	mustProcess(true)
	if got = get(t, pool, j.ID); got.Status != jobs.StatusRetry || got.Attempts != 2 || !got.RunAfter.Equal(clk.Now().Add(4*time.Second)) {
		t.Fatalf("depois da 2ª falha = %+v", got)
	}
	clk.Advance(4 * time.Second)
	mustProcess(true)
	if got = get(t, pool, j.ID); got.Status != jobs.StatusDone || got.Attempts != 3 || got.LastErrorCode != nil || got.FinishedAt == nil {
		t.Fatalf("depois do sucesso = %+v", got)
	}
}

// Esgotadas as tentativas, o job morre com o erro sanitizado e não volta sozinho.
func TestTEST005_DeadAfterMaxAttemptsWithSanitizedError(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := t0()
	ctx := context.Background()
	j := enqueue(t, pool, clk, jobs.Spec{Kind: "broken"})
	r := runner(pool, clk, "w1", map[string]jobs.Handler{"broken": func(context.Context, pgx.Tx, jobs.Claimed) error {
		return errors.New("chamada com Bearer sk-live-abcdef0123 falhou\n" + strings.Repeat("detalhe ", 100))
	}}, time.Minute)
	for range 3 {
		if ok, err := r.ProcessOne(ctx); !ok || err != nil {
			t.Fatalf("ProcessOne = %v, %v", ok, err)
		}
		clk.Advance(time.Hour)
	}
	got := get(t, pool, j.ID)
	msg := str(got.LastErrorMessage)
	if got.Status != jobs.StatusDead || got.Attempts != 3 || got.FinishedAt == nil || str(got.LastErrorCode) != jobs.CodeHandlerError {
		t.Fatalf("job = %+v", got)
	}
	if strings.Contains(msg, "sk-live") || strings.Contains(msg, "\n") || len([]rune(msg)) > 301 || !strings.Contains(msg, "Bearer [oculto]") {
		t.Fatalf("mensagem não sanitizada: %q", msg)
	}
	if ok, _ := r.ProcessOne(ctx); ok {
		t.Fatal("job dead não pode voltar sozinho")
	}
}

func TestPermanentErrorAndPanicAndUnknownKind(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := t0()
	ctx := context.Background()
	perm := enqueue(t, pool, clk, jobs.Spec{Kind: "permanent"})
	r := runner(pool, clk, "w1", map[string]jobs.Handler{
		"permanent": func(context.Context, pgx.Tx, jobs.Claimed) error {
			return jobs.Permanent("DOCUMENT_UNREADABLE", errors.New("PDF sem camada de texto"))
		},
		"panics": func(context.Context, pgx.Tx, jobs.Claimed) error { panic("bug no handler") },
	}, time.Minute)
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if got := get(t, pool, perm.ID); got.Status != jobs.StatusDead || got.Attempts != 1 || str(got.LastErrorCode) != "DOCUMENT_UNREADABLE" {
		t.Fatalf("permanente = %+v", got)
	}
	pan := enqueue(t, pool, clk, jobs.Spec{Kind: "panics"})
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if got := get(t, pool, pan.ID); got.Status != jobs.StatusRetry || str(got.LastErrorCode) != jobs.CodePanic {
		t.Fatalf("pânico = %+v", got)
	}
	// Tipo sem handler neste worker: fica na fila, sem gastar tentativa.
	other := enqueue(t, pool, clk, jobs.Spec{Kind: "outro_tipo"})
	clk.Advance(time.Hour)
	for {
		ok, err := r.ProcessOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
	}
	if got := get(t, pool, other.ID); got.Status != jobs.StatusPending || got.Attempts != 0 {
		t.Fatalf("tipo desconhecido = %+v", got)
	}
}

func effectsTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `CREATE TABLE job_effects (job_id uuid PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
}

func effects(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM job_effects`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Idempotência por aquisição: o efeito do handler e a conclusão do job são uma transação só.
func TestTEST005_EffectsCommitOnlyWithCompletion(t *testing.T) {
	pool := testutil.NewDB(t)
	effectsTable(t, pool)
	clk := t0()
	ctx := context.Background()
	fail := true
	r := runner(pool, clk, "w1", map[string]jobs.Handler{"write": func(ctx context.Context, tx pgx.Tx, j jobs.Claimed) error {
		if _, err := tx.Exec(ctx, `INSERT INTO job_effects (job_id) VALUES ($1)`, j.ID); err != nil {
			return err
		}
		if fail {
			return errors.New("falhou depois de gravar")
		}
		return nil
	}}, time.Minute)
	j := enqueue(t, pool, clk, jobs.Spec{Kind: "write"})
	if _, err := r.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	if n := effects(t, pool); n != 0 {
		t.Fatalf("efeito de tentativa que falhou ficou gravado: %d", n)
	}
	fail = false
	clk.Advance(time.Minute)
	if _, err := r.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	if n := effects(t, pool); n != 1 || get(t, pool, j.ID).Status != jobs.StatusDone {
		t.Fatalf("efeitos = %d, status %s", n, get(t, pool, j.ID).Status)
	}
}

// recordOnRollback é o erro de handler que deixa algo gravado depois do rollback (a corrida de
// erro do agente, na TASK-009).
type recordOnRollback struct {
	err   error
	id    string
	fails bool
}

func (e recordOnRollback) Error() string { return e.err.Error() }
func (e recordOnRollback) Unwrap() error { return e.err }
func (e recordOnRollback) Record(ctx context.Context, tx pgx.Tx) error {
	if e.fails {
		return errors.New("registro falhou")
	}
	_, err := tx.Exec(ctx, `INSERT INTO job_effects (job_id) VALUES ($1)`, e.id)
	return err
}

// O registro da tentativa que falhou sobrevive ao rollback, junto com a falha do job; o código do
// erro embrulhado continua valendo. Se o registro falhar, a falha do job é registrada mesmo assim.
func TestAfterRollback_RecordsWithJobFailure(t *testing.T) {
	pool := testutil.NewDB(t)
	effectsTable(t, pool)
	clk := t0()
	ctx := context.Background()
	recordID := "00000000-0000-4000-8000-000000000001"
	failRecord := false
	r := runner(pool, clk, "w1", map[string]jobs.Handler{"agent": func(ctx context.Context, tx pgx.Tx, j jobs.Claimed) error {
		if _, err := tx.Exec(ctx, `INSERT INTO job_effects (job_id) VALUES ($1)`, j.ID); err != nil {
			return err
		}
		return recordOnRollback{err: jobs.Coded("LLM_UNAVAILABLE", errors.New("gateway fora")), id: recordID, fails: failRecord}
	}}, time.Minute)
	j := enqueue(t, pool, clk, jobs.Spec{Kind: "agent"})
	if _, err := r.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	var ids []string
	rows, err := pool.Query(ctx, `SELECT job_id::text FROM job_effects`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	got := get(t, pool, j.ID)
	if len(ids) != 1 || ids[0] != recordID {
		t.Fatalf("gravado = %v; want só o registro da tentativa (o efeito do handler foi desfeito)", ids)
	}
	if got.Status != jobs.StatusRetry || str(got.LastErrorCode) != "LLM_UNAVAILABLE" {
		t.Fatalf("job = %s / %s", got.Status, str(got.LastErrorCode))
	}

	failRecord = true
	recordID = "00000000-0000-4000-8000-000000000002"
	clk.Advance(time.Minute)
	if _, err := r.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	if got := get(t, pool, j.ID); got.Attempts != 2 || got.Status != jobs.StatusRetry || effects(t, pool) != 1 {
		t.Fatalf("com o registro falhando, a falha do job deve ser registrada mesmo assim: %+v, efeitos %d", got, effects(t, pool))
	}
}

// Se outro worker retomou o job durante a execução, a conclusão não vale e os efeitos são desfeitos.
func TestTEST005_LostLeaseRollsBackEffects(t *testing.T) {
	pool := testutil.NewDB(t)
	effectsTable(t, pool)
	clk := t0()
	ctx := context.Background()
	j := enqueue(t, pool, clk, jobs.Spec{Kind: "write"})
	r := runner(pool, clk, "w1", map[string]jobs.Handler{"write": func(ctx context.Context, tx pgx.Tx, c jobs.Claimed) error {
		if _, err := tx.Exec(ctx, `INSERT INTO job_effects (job_id) VALUES ($1)`, c.ID); err != nil {
			return err
		}
		// Outro worker toma o job (posse vencida retomada e readquirida).
		_, err := pool.Exec(ctx, `UPDATE jobs SET locked_by = 'w2' WHERE id = $1`, c.ID)
		return err
	}}, time.Minute)
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	got := get(t, pool, j.ID)
	if n := effects(t, pool); n != 0 || got.Status != jobs.StatusRunning || str(got.LockedBy) != "w2" {
		t.Fatalf("efeitos %d, job %+v: o worker sem posse não pode concluir", n, got)
	}
}

// Worker que caiu: a posse vence e o job volta para a fila (a tentativa conta).
func TestTEST005_ExpiredLeaseIsReclaimed(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := t0()
	ctx := context.Background()
	j := enqueue(t, pool, clk, jobs.Spec{Kind: "work", MaxAttempts: 2})
	// O worker "morto" adquire e some sem concluir.
	if _, err := store.New(pool).ClaimJob(ctx, store.ClaimJobParams{Now: clk.Now(), WorkerID: "morto", Kinds: []string{"work"}}); err != nil {
		t.Fatal(err)
	}
	r := runner(pool, clk, "vivo", map[string]jobs.Handler{"work": func(context.Context, pgx.Tx, jobs.Claimed) error { return nil }}, time.Minute)
	if n, err := r.ReclaimExpired(ctx); n != 0 || err != nil {
		t.Fatalf("posse ainda válida foi retomada: %d, %v", n, err)
	}
	clk.Advance(2 * time.Minute)
	if n, err := r.ReclaimExpired(ctx); n != 1 || err != nil {
		t.Fatalf("retomados = %d, %v", n, err)
	}
	got := get(t, pool, j.ID)
	if got.Status != jobs.StatusRetry || got.Attempts != 1 || str(got.LastErrorCode) != jobs.CodeWorkerLost || got.LockedBy != nil ||
		!strings.Contains(str(got.LastErrorMessage), "morto") {
		t.Fatalf("retomado = %+v", got)
	}
	// O worker vivo executa depois do backoff.
	clk.Advance(2 * time.Second)
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if got = get(t, pool, j.ID); got.Status != jobs.StatusDone || got.Attempts != 2 {
		t.Fatalf("concluído = %+v", got)
	}
	// Na última tentativa, a posse vencida encerra o job.
	last := enqueue(t, pool, clk, jobs.Spec{Kind: "work", MaxAttempts: 1})
	if _, err := store.New(pool).ClaimJob(ctx, store.ClaimJobParams{Now: clk.Now(), WorkerID: "morto", Kinds: []string{"work"}}); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * time.Minute)
	if _, err := r.ReclaimExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if got = get(t, pool, last.ID); got.Status != jobs.StatusDead || got.FinishedAt == nil {
		t.Fatalf("última tentativa = %+v", got)
	}
}

// Enquanto o handler roda, o worker renova a posse: a retomada não pega um job vivo.
func TestTEST005_LeaseIsRenewedWhileRunning(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := clock.NewSystem(time.UTC)
	ctx := context.Background()
	const lease = 300 * time.Millisecond
	j := enqueue(t, pool, clk, jobs.Spec{Kind: "slow"})
	r := runner(pool, clk, "w1", map[string]jobs.Handler{"slow": func(ctx context.Context, _ pgx.Tx, _ jobs.Claimed) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1200 * time.Millisecond):
			return nil
		}
	}}, lease)
	reaper := runner(pool, clk, "w2", map[string]jobs.Handler{}, lease)
	stop := make(chan struct{})
	var reclaimed int
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				n, _ := reaper.ReclaimExpired(ctx)
				reclaimed += n
			}
		}
	})
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	close(stop)
	wg.Wait()
	if got := get(t, pool, j.ID); reclaimed != 0 || got.Status != jobs.StatusDone || got.Attempts != 1 {
		t.Fatalf("retomados %d, job %+v: a renovação da posse falhou", reclaimed, got)
	}
}

func TestJobsAPI_GetRetryAndWorkerOnline(t *testing.T) {
	pool := testutil.NewDB(t)
	srv := testutil.NewServer(t, pool, testutil.FixedClock(t, "2026-10-08T12:00:00Z"))
	ctx := context.Background()
	sys := clock.NewSystem(time.UTC)

	var j jobs.Job
	pending := enqueue(t, pool, sys, jobs.Spec{Kind: "ping"})
	srv.Get("/api/v1/jobs/"+pending.ID).MustStatus(t, http.StatusOK).JSON(t, &j)
	if j.Status != jobs.StatusPending || j.Kind != "ping" || j.WorkerOnline {
		t.Fatalf("pendente = %+v", j)
	}
	if code := srv.Post("/api/v1/jobs/"+pending.ID+"/retry", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "JOB_NOT_RETRYABLE" {
		t.Fatalf("reenvio de pendente: %s", code)
	}
	srv.Get("/api/v1/jobs/00000000-0000-0000-0000-00000000abcd").MustStatus(t, http.StatusNotFound)
	srv.Get("/api/v1/jobs/nao-e-uuid").MustStatus(t, http.StatusNotFound)

	// Um worker dá sinal: a fila não está parada.
	r := runner(pool, sys, "w1", map[string]jobs.Handler{"fails": func(context.Context, pgx.Tx, jobs.Claimed) error {
		return errors.New("falha")
	}}, time.Minute)
	if err := r.Heartbeat(ctx, sys.Now()); err != nil {
		t.Fatal(err)
	}
	srv.Get("/api/v1/jobs/"+pending.ID).MustStatus(t, http.StatusOK).JSON(t, &j)
	if !j.WorkerOnline {
		t.Fatal("worker com batimento recente deveria aparecer ativo")
	}

	// Job dead pode ser reenviado: volta pendente, tentativas zeradas.
	dead := enqueue(t, pool, sys, jobs.Spec{Kind: "fails", MaxAttempts: 1, DedupeKey: "item:1"})
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	srv.Post("/api/v1/jobs/"+dead.ID+"/retry", nil).MustStatus(t, http.StatusOK).JSON(t, &j)
	if j.Status != jobs.StatusPending || j.Attempts != 0 || j.RetriedAt == nil || j.FinishedAt != nil {
		t.Fatalf("reenviado = %+v", j)
	}
	// Com outro job ativo para o mesmo item, o reenvio é recusado.
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	enqueue(t, pool, sys, jobs.Spec{Kind: "fails", DedupeKey: "item:1"})
	if code := srv.Post("/api/v1/jobs/"+dead.ID+"/retry", nil).MustStatus(t, http.StatusConflict).ErrorCode(t); code != "JOB_ALREADY_ACTIVE" {
		t.Fatalf("reenvio com job ativo: %s", code)
	}
}

// Encerramento ordenado: o job em andamento termina dentro do prazo; passado o prazo, a
// tentativa é registrada como WORKER_SHUTDOWN e o job volta para a fila.
func TestRunner_GracefulShutdown(t *testing.T) {
	pool := testutil.NewDB(t)
	clk := clock.NewSystem(time.UTC)
	started := make(chan string, 2)
	slow := func(d time.Duration) jobs.Handler {
		return func(ctx context.Context, _ pgx.Tx, j jobs.Claimed) error {
			started <- j.ID
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		}
	}
	run := func(h jobs.Handler, grace time.Duration) (store.Job, time.Duration) {
		t.Helper()
		j := enqueue(t, pool, clk, jobs.Spec{Kind: "slow"})
		r := jobs.NewRunner(pool, clk, quiet, jobs.RunnerConfig{
			WorkerID: "w-" + j.ID[:8], PollInterval: 10 * time.Millisecond, Lease: time.Minute, ShutdownGrace: grace,
		}, map[string]jobs.Handler{"slow": h})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { _ = r.Run(ctx); close(done) }()
		<-started
		begin := time.Now()
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Run não terminou depois do cancelamento")
		}
		return get(t, pool, j.ID), time.Since(begin)
	}
	if got, _ := run(slow(300*time.Millisecond), 5*time.Second); got.Status != jobs.StatusDone {
		t.Fatalf("job dentro do prazo de encerramento = %+v", got)
	}
	got, took := run(slow(time.Minute), 200*time.Millisecond)
	if got.Status != jobs.StatusRetry || str(got.LastErrorCode) != jobs.CodeShutdown || took > 5*time.Second {
		t.Fatalf("job além do prazo = %+v (levou %s)", got, took)
	}
}
