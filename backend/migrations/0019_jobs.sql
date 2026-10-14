-- +goose Up
-- Fila de jobs em PostgreSQL [04 §7, 05 §7, TASK-005]: porte da fila do Conciliador (tabela job:
-- aquisição com SELECT ... FOR UPDATE SKIP LOCKED, 3 tentativas, backoff de 2^n segundos e estado
-- terminal), com os nomes do 05 §7. A posse do job tem prazo (lease) renovado pelo worker: se o
-- worker cair, o job volta para a fila (D-019).
CREATE TABLE jobs (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    kind               text        NOT NULL CHECK (kind ~ '^[a-z][a-z0-9_]{0,63}$'),
    status             text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'retry', 'done', 'dead')),
    -- Payload referencial (IDs), nunca blobs nem conteúdo de documento [04 §7, 09].
    payload            jsonb       NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(payload) = 'object'),
    -- Idempotência do enfileiramento: um job ativo por (kind, dedupe_key).
    dedupe_key         text        NULL CHECK (dedupe_key IS NULL OR btrim(dedupe_key) <> ''),
    -- Tentativas iniciadas: conta na aquisição, então uma queda do worker também conta.
    attempts           integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts       integer     NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 20),
    run_after          timestamptz NOT NULL DEFAULT now(),
    locked_at          timestamptz NULL,
    locked_by          text        NULL,
    last_error_code    text        NULL,
    -- Mensagem sanitizada: sem chaves, tokens ou conteúdo extenso.
    last_error_message text        NULL,
    finished_at        timestamptz NULL,
    retried_by         uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    retried_at         timestamptz NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT jobs_lock_ck CHECK ((status = 'running') = (locked_at IS NOT NULL AND locked_by IS NOT NULL)),
    CONSTRAINT jobs_finished_ck CHECK ((status IN ('done', 'dead')) = (finished_at IS NOT NULL)),
    CONSTRAINT jobs_attempts_ck CHECK (attempts <= max_attempts)
);

-- Aquisição: os jobs prontos em ordem de chegada (origem: ORDER BY created_at).
CREATE INDEX jobs_ready_idx ON jobs (created_at) WHERE status IN ('pending', 'retry');
-- Retomada de posse vencida.
CREATE INDEX jobs_running_idx ON jobs (locked_at) WHERE status = 'running';
CREATE UNIQUE INDEX jobs_active_dedupe_uq ON jobs (kind, dedupe_key)
    WHERE dedupe_key IS NOT NULL AND status IN ('pending', 'running', 'retry');

-- Batimento de cada worker: a interface mostra "fila parada" quando nenhum worker deu sinal
-- recente (runbook "Worker parado", 11 §9).
CREATE TABLE worker_heartbeats (
    worker_id    text        PRIMARY KEY CHECK (btrim(worker_id) <> ''),
    started_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE worker_heartbeats;
DROP TABLE jobs;
