-- Configuração dos modelos de IA por alias [05 §8, TASK-008].

-- name: ListAIModelAliases :many
SELECT * FROM ai_model_aliases ORDER BY alias;

-- name: GetAIModelAlias :one
SELECT * FROM ai_model_aliases WHERE alias = sqlc.arg('alias');

-- name: UpdateAIModelAlias :one
-- Concorrência otimista: só grava sobre a versão lida; sem linha devolvida = conflito.
UPDATE ai_model_aliases
SET provider   = sqlc.narg('provider'),
    model      = sqlc.narg('model'),
    parameters = sqlc.arg('parameters'),
    enabled    = sqlc.arg('enabled'),
    version    = version + 1,
    updated_by = sqlc.arg('updated_by'),
    updated_at = sqlc.arg('updated_at')
WHERE alias = sqlc.arg('alias') AND version = sqlc.arg('version')
RETURNING *;

-- name: InsertAIModelAliasHistory :exec
INSERT INTO ai_model_alias_history (alias, version, provider, model, parameters, enabled, changed_by, changed_at)
VALUES (sqlc.arg('alias'), sqlc.arg('version'), sqlc.narg('provider'), sqlc.narg('model'), sqlc.arg('parameters'),
        sqlc.arg('enabled'), sqlc.narg('changed_by'), sqlc.arg('changed_at'));

-- name: ListAIModelAliasHistory :many
SELECT * FROM ai_model_alias_history WHERE alias = sqlc.arg('alias') ORDER BY version;
