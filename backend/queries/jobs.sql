-- Fila de jobs [04 §7, 05 §7, TASK-005].

-- name: EnqueueJob :one
-- Novo job. Com dedupe_key, conflito com um job ativo do mesmo tipo = nada inserido.
INSERT INTO jobs (kind, payload, dedupe_key, max_attempts, run_after, created_at, updated_at)
VALUES (sqlc.arg('kind'), sqlc.arg('payload'), sqlc.narg('dedupe_key'), sqlc.arg('max_attempts'),
        sqlc.arg('run_after')::timestamptz, sqlc.arg('now')::timestamptz, sqlc.arg('now')::timestamptz)
ON CONFLICT (kind, dedupe_key) WHERE dedupe_key IS NOT NULL AND status IN ('pending', 'running', 'retry') DO NOTHING
RETURNING *;

-- name: GetActiveJobByDedupeKey :one
SELECT * FROM jobs
WHERE kind = sqlc.arg('kind') AND dedupe_key = sqlc.arg('dedupe_key') AND status IN ('pending', 'running', 'retry');

-- name: ClaimJob :one
-- Aquisição atômica: o próximo job pronto dos tipos que o worker sabe executar, em ordem de
-- chegada, travado com SKIP LOCKED (workers concorrentes nunca pegam o mesmo job). A tentativa
-- conta aqui.
UPDATE jobs
SET status = 'running', attempts = attempts + 1, locked_at = sqlc.arg('now')::timestamptz, locked_by = sqlc.arg('worker_id')::text,
    updated_at = sqlc.arg('now')::timestamptz
WHERE id = (
    SELECT j.id FROM jobs j
    WHERE j.status IN ('pending', 'retry')
      AND j.run_after <= sqlc.arg('now')::timestamptz
      AND j.kind = ANY (sqlc.arg('kinds')::text[])
    ORDER BY j.created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING *;

-- name: ExtendJobLease :execrows
-- Renova a posse enquanto o worker executa. 0 linhas = o worker perdeu o job.
UPDATE jobs SET locked_at = sqlc.arg('now')::timestamptz, updated_at = sqlc.arg('now')::timestamptz
WHERE id = sqlc.arg('id') AND status = 'running' AND locked_by = sqlc.arg('worker_id')::text;

-- name: CompleteJob :execrows
-- Conclusão, na mesma transação dos efeitos do job: só vale se o worker ainda tem a posse.
UPDATE jobs
SET status = 'done', locked_at = NULL, locked_by = NULL, last_error_code = NULL, last_error_message = NULL,
    finished_at = sqlc.arg('now')::timestamptz, updated_at = sqlc.arg('now')::timestamptz
WHERE id = sqlc.arg('id') AND status = 'running' AND locked_by = sqlc.arg('worker_id')::text;

-- name: FailJob :execrows
-- Falha de uma tentativa: retry com backoff ou dead (decidido no Go). Só com a posse.
UPDATE jobs
SET status = sqlc.arg('status')::text, run_after = sqlc.arg('run_after')::timestamptz, locked_at = NULL, locked_by = NULL,
    last_error_code = sqlc.arg('error_code')::text, last_error_message = sqlc.arg('error_message')::text,
    finished_at = sqlc.narg('finished_at')::timestamptz, updated_at = sqlc.arg('now')::timestamptz
WHERE id = sqlc.arg('id') AND status = 'running' AND locked_by = sqlc.arg('worker_id')::text;

-- name: ListExpiredJobLeases :many
-- Jobs cujo worker parou de renovar a posse.
SELECT * FROM jobs
WHERE status = 'running' AND locked_at < sqlc.arg('cutoff')::timestamptz
ORDER BY locked_at
FOR UPDATE SKIP LOCKED;

-- name: ReleaseExpiredJob :execrows
UPDATE jobs
SET status = sqlc.arg('status')::text, run_after = sqlc.arg('run_after')::timestamptz, locked_at = NULL, locked_by = NULL,
    last_error_code = sqlc.arg('error_code')::text, last_error_message = sqlc.arg('error_message')::text,
    finished_at = sqlc.narg('finished_at')::timestamptz, updated_at = sqlc.arg('now')::timestamptz
WHERE id = sqlc.arg('id') AND status = 'running' AND locked_at < sqlc.arg('cutoff')::timestamptz;

-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1;

-- name: RetryDeadJob :one
-- Reenvio manual de um job em estado terminal: volta para a fila com as tentativas zeradas.
UPDATE jobs
SET status = 'pending', attempts = 0, run_after = sqlc.arg('now')::timestamptz, finished_at = NULL,
    retried_by = sqlc.arg('retried_by')::uuid, retried_at = sqlc.arg('now')::timestamptz, updated_at = sqlc.arg('now')::timestamptz
WHERE id = sqlc.arg('id') AND status = 'dead'
RETURNING *;

-- name: TouchWorkerHeartbeat :exec
INSERT INTO worker_heartbeats (worker_id, started_at, last_seen_at)
VALUES (sqlc.arg('worker_id')::text, sqlc.arg('started_at')::timestamptz, sqlc.arg('now')::timestamptz)
ON CONFLICT (worker_id) DO UPDATE SET started_at = EXCLUDED.started_at, last_seen_at = EXCLUDED.last_seen_at;

-- name: LatestWorkerHeartbeat :one
SELECT * FROM worker_heartbeats ORDER BY last_seen_at DESC LIMIT 1;
