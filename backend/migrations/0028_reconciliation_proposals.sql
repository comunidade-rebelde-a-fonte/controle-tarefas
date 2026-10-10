-- +goose Up
-- TASK-019 (D-030): o resolvedor grava só a proposta, com a corrida que a produziu (a chave para
-- ai_runs veio na 0022); a conciliação nasce do aceite humano (AC-019).

-- As propostas da linha, da mais recente para a mais antiga (a fila mostra o histórico).
CREATE INDEX resolver_proposals_txn_idx ON resolver_proposals (statement_transaction_id, created_at);

-- O resolvedor automático pede uma só vez por linha: procura qualquer job da linha, não só o ativo.
CREATE INDEX jobs_kind_dedupe_idx ON jobs (kind, dedupe_key) WHERE dedupe_key IS NOT NULL;

-- +goose Down
DROP INDEX jobs_kind_dedupe_idx;
DROP INDEX resolver_proposals_txn_idx;
