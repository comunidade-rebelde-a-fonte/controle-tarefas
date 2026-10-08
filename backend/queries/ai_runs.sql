-- Auditoria local das corridas de agente [05 §8, 11 §3, TASK-009].

-- name: InsertAIRun :exec
-- Abre a corrida na transação do agente com status provisório 'error' (NOT_FINISHED): o status
-- real só é gravado no fim (FinishAIRun), e a corrida que terminar sem ele fica como erro, como
-- na origem. Se a transação desfizer, a corrida some junto (a de erro é regravada à parte, por
-- InsertAIRunFull).
INSERT INTO ai_runs (trace_id, agent, trigger, user_id, company_id, competence, alias, provider, model,
                     model_config_version, prompt_version, metadata, input, status, error_code, created_at, updated_at)
VALUES (sqlc.arg('trace_id'), sqlc.arg('agent'), sqlc.arg('trigger'), sqlc.narg('user_id'), sqlc.narg('company_id'),
        sqlc.narg('competence'), sqlc.narg('alias'), sqlc.narg('provider'), sqlc.narg('model'),
        sqlc.narg('model_config_version'), sqlc.narg('prompt_version'), sqlc.arg('metadata'), sqlc.arg('input'),
        'error', 'NOT_FINISHED', sqlc.arg('created_at'), sqlc.arg('created_at'));

-- name: FinishAIRun :execrows
UPDATE ai_runs
SET output = sqlc.arg('output'), decision = sqlc.arg('decision'), spans = sqlc.arg('spans'),
    status = sqlc.arg('status'), confidence = sqlc.narg('confidence'), latency_ms = sqlc.arg('latency_ms'),
    tokens_in = sqlc.narg('tokens_in'), tokens_out = sqlc.narg('tokens_out'), cost_usd = sqlc.narg('cost_usd'),
    error_code = sqlc.narg('error_code'), updated_at = sqlc.arg('updated_at')
WHERE trace_id = sqlc.arg('trace_id');

-- name: InsertAIRunFull :exec
-- Corrida inteira de uma vez: a de erro, gravada fora da transação desfeita do agente.
INSERT INTO ai_runs (trace_id, agent, trigger, user_id, company_id, competence, alias, provider, model,
                     model_config_version, prompt_version, metadata, input, output, decision, spans, status, confidence,
                     latency_ms, tokens_in, tokens_out, cost_usd, error_code, created_at, updated_at)
VALUES (sqlc.arg('trace_id'), sqlc.arg('agent'), sqlc.arg('trigger'), sqlc.narg('user_id'), sqlc.narg('company_id'),
        sqlc.narg('competence'), sqlc.narg('alias'), sqlc.narg('provider'), sqlc.narg('model'),
        sqlc.narg('model_config_version'), sqlc.narg('prompt_version'), sqlc.arg('metadata'), sqlc.arg('input'), sqlc.arg('output'),
        sqlc.arg('decision'), sqlc.arg('spans'), sqlc.arg('status'), sqlc.narg('confidence'), sqlc.arg('latency_ms'),
        sqlc.narg('tokens_in'), sqlc.narg('tokens_out'), sqlc.narg('cost_usd'), sqlc.narg('error_code'),
        sqlc.arg('created_at'), sqlc.arg('updated_at'));

-- name: InsertAIDecision :exec
INSERT INTO ai_decisions (trace_id, agent, kind, data, created_at)
VALUES (sqlc.arg('trace_id'), sqlc.arg('agent'), sqlc.arg('kind'), sqlc.arg('data'), sqlc.arg('created_at'));

-- name: InsertAIGoldEval :one
INSERT INTO ai_gold_evals (trace_id, field, expected, obtained, within_tolerance, status, created_at)
VALUES (sqlc.arg('trace_id'), sqlc.arg('field'), sqlc.narg('expected'), sqlc.narg('obtained'),
        sqlc.narg('within_tolerance'), sqlc.arg('status'), sqlc.arg('created_at'))
RETURNING *;

-- name: InsertAIHumanEval :one
INSERT INTO ai_human_evals (trace_id, user_id, rating, comment, created_at)
VALUES (sqlc.arg('trace_id'), sqlc.arg('user_id'), sqlc.arg('rating'), sqlc.narg('comment'), sqlc.arg('created_at'))
RETURNING *;

-- name: AIRunExists :one
SELECT EXISTS (SELECT 1 FROM ai_runs WHERE trace_id = sqlc.arg('trace_id'));

-- name: GetAIRun :one
SELECT * FROM ai_runs WHERE trace_id = sqlc.arg('trace_id');

-- name: ListAIRuns :many
SELECT r.trace_id, r.agent, r.trigger, r.status, r.confidence, r.latency_ms, r.company_id, r.competence,
       r.alias, r.provider, r.model, r.tokens_in, r.tokens_out, r.cost_usd, r.langfuse_sync_status, r.created_at,
       EXISTS (SELECT 1 FROM ai_human_evals h WHERE h.trace_id = r.trace_id) AS has_human_eval
FROM ai_runs r
WHERE (sqlc.narg('agent')::text IS NULL OR r.agent = sqlc.narg('agent')::text)
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status')::text)
  AND (sqlc.narg('company_id')::uuid IS NULL OR r.company_id = sqlc.narg('company_id')::uuid)
  AND (NOT sqlc.arg('without_human_eval')::bool OR NOT EXISTS (SELECT 1 FROM ai_human_evals h WHERE h.trace_id = r.trace_id))
ORDER BY r.created_at DESC, r.trace_id
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: CountAIRuns :one
SELECT count(*)
FROM ai_runs r
WHERE (sqlc.narg('agent')::text IS NULL OR r.agent = sqlc.narg('agent')::text)
  AND (sqlc.narg('status')::text IS NULL OR r.status = sqlc.narg('status')::text)
  AND (sqlc.narg('company_id')::uuid IS NULL OR r.company_id = sqlc.narg('company_id')::uuid)
  AND (NOT sqlc.arg('without_human_eval')::bool OR NOT EXISTS (SELECT 1 FROM ai_human_evals h WHERE h.trace_id = r.trace_id));

-- name: ListAIDecisions :many
SELECT * FROM ai_decisions WHERE trace_id = sqlc.arg('trace_id') ORDER BY created_at, id;

-- name: ListAIGoldEvals :many
SELECT * FROM ai_gold_evals WHERE trace_id = sqlc.arg('trace_id') ORDER BY field, created_at;

-- name: ListAIHumanEvals :many
SELECT * FROM ai_human_evals WHERE trace_id = sqlc.arg('trace_id') ORDER BY created_at DESC, id;

-- name: AIRunTotals :one
-- Painel (FR-18): corridas totais e nas últimas 24 h, pendências sem nota humana, custo e
-- tokens do LLM nos últimos 30 dias (locais, do gateway).
-- As contagens *_count dizem se houve dado (sem nenhum, a API devolve null, não zero).
SELECT count(*)                                                                         AS total,
       count(*) FILTER (WHERE r.created_at >= sqlc.arg('since_24h')::timestamptz)       AS last_24h,
       count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM ai_human_evals h WHERE h.trace_id = r.trace_id)) AS without_human_eval,
       coalesce(sum(r.cost_usd) FILTER (WHERE r.created_at >= sqlc.arg('since_30d')::timestamptz), 0)::numeric(18,8) AS cost_30d,
       count(r.cost_usd) FILTER (WHERE r.created_at >= sqlc.arg('since_30d')::timestamptz) AS cost_count,
       coalesce(sum(coalesce(r.tokens_in, 0) + coalesce(r.tokens_out, 0))
           FILTER (WHERE r.created_at >= sqlc.arg('since_30d')::timestamptz), 0)::bigint AS tokens_30d,
       count(*) FILTER (WHERE r.created_at >= sqlc.arg('since_30d')::timestamptz
                          AND (r.tokens_in IS NOT NULL OR r.tokens_out IS NOT NULL)) AS tokens_count
FROM ai_runs r;

-- name: AIGoldTotals :one
SELECT count(*) FILTER (WHERE status = 'ok') AS ok, count(*) FILTER (WHERE status = 'erro') AS erro
FROM ai_gold_evals;

-- name: AIHumanTotals :one
SELECT count(*) FILTER (WHERE rating = 'acerto') AS acerto, count(*) AS total
FROM ai_human_evals;

-- name: AIRunsByAgent :many
SELECT agent,
       count(*)                                              AS runs,
       coalesce(round(avg(latency_ms)), 0)::bigint           AS avg_latency_ms,
       count(latency_ms)                                     AS latency_count,
       coalesce(round(avg(confidence), 4), 0)::numeric(5,4)  AS avg_confidence,
       count(confidence)                                     AS confidence_count,
       count(*) FILTER (WHERE status = 'error')              AS errors,
       count(*) FILTER (WHERE status = 'needs_review')       AS reviews
FROM ai_runs
GROUP BY agent
ORDER BY agent;

-- name: LockAIRunForSync :one
-- O job de sincronização trava a corrida: um reenvio por vez para o mesmo trace.
SELECT * FROM ai_runs WHERE trace_id = sqlc.arg('trace_id') FOR UPDATE;

-- name: MarkAIRunSynced :exec
UPDATE ai_runs
SET langfuse_sync_status = 'ok', langfuse_attempts = langfuse_attempts + 1, langfuse_last_error = NULL,
    langfuse_synced_at = sqlc.arg('now'), updated_at = sqlc.arg('now')
WHERE trace_id = sqlc.arg('trace_id');

-- name: MarkAIRunSyncFailed :exec
UPDATE ai_runs
SET langfuse_sync_status = 'failed', langfuse_attempts = langfuse_attempts + 1,
    langfuse_last_error = sqlc.arg('last_error'), langfuse_synced_at = NULL, updated_at = sqlc.arg('now')
WHERE trace_id = sqlc.arg('trace_id');

-- name: MarkAIGoldEvalsSync :exec
UPDATE ai_gold_evals SET langfuse_sync_status = sqlc.arg('status')
WHERE id = ANY (sqlc.arg('ids')::uuid[]);

-- name: MarkAIHumanEvalsSync :exec
UPDATE ai_human_evals SET langfuse_sync_status = sqlc.arg('status')
WHERE id = ANY (sqlc.arg('ids')::uuid[]);

-- name: ListAIRunsToResync :many
-- Reenvio periódico (mecanismo da origem): corridas e notas com falha, e as pendentes há mais
-- tempo que o limite (job perdido). Mais antigas primeiro.
SELECT trace_id
FROM (
    SELECT trace_id, created_at FROM ai_runs
    WHERE langfuse_sync_status = 'failed'
       OR (langfuse_sync_status = 'pending' AND created_at < sqlc.arg('stale_before')::timestamptz)
    UNION
    SELECT trace_id, created_at FROM ai_gold_evals
    WHERE langfuse_sync_status = 'failed'
       OR (langfuse_sync_status = 'pending' AND created_at < sqlc.arg('stale_before')::timestamptz)
    UNION
    SELECT trace_id, created_at FROM ai_human_evals
    WHERE langfuse_sync_status = 'failed'
       OR (langfuse_sync_status = 'pending' AND created_at < sqlc.arg('stale_before')::timestamptz)
) pendentes
GROUP BY trace_id
ORDER BY min(created_at)
LIMIT sqlc.arg('lim');
