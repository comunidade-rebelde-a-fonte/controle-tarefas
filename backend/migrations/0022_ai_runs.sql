-- +goose Up
-- Auditoria local das corridas de agente [docs/ai-spec/05 §8, 11 §3, TASK-009]: porte de
-- agent_traces, agent_decisions, evals_gold e evals_human do Fator R (tracing/, commit 9dd7a74),
-- com os nomes do 05 §8. A trilha local é a fonte de verdade; o Langfuse é espelho
-- sincronizado depois, e falha dele não desfaz nada aqui [REQ-042, AC-030]. Nada é apagado:
-- FKs ON DELETE RESTRICT e nenhum job remove traces (FR-22).

-- Uma linha por corrida. O trace_id é o do Langfuse (32 hex, padrão OpenTelemetry), o mesmo
-- nos dois lados. A linha nasce na transação do agente: sem trace, nada do agente grava.
CREATE TABLE ai_runs (
    trace_id             text          PRIMARY KEY CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    agent                text          NOT NULL CHECK (agent ~ '^[a-z][a-z0-9_]{1,39}$'),
    trigger              text          NOT NULL CHECK (trigger ~ '^[a-z][a-z0-9_]{1,39}$'),
    user_id              uuid          NULL REFERENCES users (id) ON DELETE RESTRICT,
    company_id           uuid          NULL REFERENCES companies (id) ON DELETE RESTRICT,
    -- Competência da Conciliação ou PA do Fator R (dia 1).
    competence           date          NULL CHECK (competence IS NULL OR extract(day FROM competence) = 1),
    -- Snapshot da resolução do alias no início da corrida (D-014, D-021). Nulo nos agentes
    -- determinísticos, como o parser_pgdas (ADR-006).
    alias                text          NULL,
    provider             text          NULL,
    model                text          NULL,
    model_config_version integer       NULL CHECK (model_config_version IS NULL OR model_config_version >= 0),
    prompt_version       text          NULL,
    -- Metadados do trace no Langfuse (ex.: parser_version, tabela_vigencia, politica_cpp).
    metadata             jsonb         NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(metadata) = 'object'),
    -- Entrada, saída e decisão já sanitizadas (sem texto bruto de documento nem segredo).
    input                jsonb         NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(input) = 'object'),
    output               jsonb         NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(output) = 'object'),
    decision             jsonb         NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(decision) = 'object'),
    -- Spans plan|parse|tool|decide|render e gerações do LLM: o detalhe da corrida não depende
    -- do Langfuse, e o reenvio manda o trace completo.
    spans                jsonb         NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(spans) = 'array'),
    status               text          NOT NULL CHECK (status IN ('ok', 'error', 'needs_review')),
    confidence           numeric(5,4)  NULL CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1),
    latency_ms           integer       NULL CHECK (latency_ms IS NULL OR latency_ms >= 0),
    tokens_in            integer       NULL CHECK (tokens_in IS NULL OR tokens_in >= 0),
    tokens_out           integer       NULL CHECK (tokens_out IS NULL OR tokens_out >= 0),
    cost_usd             numeric(18,8) NULL CHECK (cost_usd IS NULL OR cost_usd >= 0),
    -- Código do erro (nunca a mensagem crua, que pode trazer dado do documento).
    error_code           text          NULL CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{1,63}$'),
    langfuse_sync_status text          NOT NULL DEFAULT 'pending' CHECK (langfuse_sync_status IN ('pending', 'ok', 'failed')),
    langfuse_attempts    integer       NOT NULL DEFAULT 0 CHECK (langfuse_attempts >= 0),
    langfuse_last_error  text          NULL,
    langfuse_synced_at   timestamptz   NULL,
    created_at           timestamptz   NOT NULL,
    updated_at           timestamptz   NOT NULL,
    CONSTRAINT ai_runs_model_pair_ck CHECK ((provider IS NULL) = (model IS NULL)),
    CONSTRAINT ai_runs_error_code_ck CHECK (error_code IS NULL OR status = 'error'),
    CONSTRAINT ai_runs_synced_ck CHECK ((langfuse_sync_status = 'ok') = (langfuse_synced_at IS NOT NULL))
);

CREATE INDEX ai_runs_created_idx ON ai_runs (created_at DESC);
CREATE INDEX ai_runs_agent_idx ON ai_runs (agent, created_at DESC);
CREATE INDEX ai_runs_company_idx ON ai_runs (company_id, created_at DESC) WHERE company_id IS NOT NULL;
CREATE INDEX ai_runs_sync_idx ON ai_runs (langfuse_sync_status, created_at) WHERE langfuse_sync_status <> 'ok';

-- Único caminho de gravação de decisão de agente (record_decision da origem): sem corrida,
-- não há decisão [FR-16].
CREATE TABLE ai_decisions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id   text        NOT NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT,
    agent      text        NOT NULL,
    kind       text        NOT NULL CHECK (kind ~ '^[a-z][a-z0-9_]{0,63}$'),
    data       jsonb       NOT NULL CHECK (jsonb_typeof(data) = 'object'),
    created_at timestamptz NOT NULL
);

CREATE INDEX ai_decisions_trace_idx ON ai_decisions (trace_id);

-- Nota ouro por campo (evals_gold da origem), com o vocabulário da origem.
CREATE TABLE ai_gold_evals (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id             text        NOT NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT,
    field                text        NOT NULL CHECK (field ~ '^[a-z][a-z0-9_]{0,63}$'),
    expected             text        NULL,
    obtained             text        NULL,
    within_tolerance     boolean     NULL,
    status               text        NOT NULL CHECK (status IN ('ok', 'erro', 'ouro_indisponivel')),
    langfuse_sync_status text        NOT NULL DEFAULT 'pending' CHECK (langfuse_sync_status IN ('pending', 'ok', 'failed')),
    created_at           timestamptz NOT NULL
);

CREATE INDEX ai_gold_evals_trace_idx ON ai_gold_evals (trace_id);

-- Nota humana (evals_human da origem): acerto, parcial ou erro; erro exige comentário.
CREATE TABLE ai_human_evals (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id             text        NOT NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT,
    user_id              uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    rating               text        NOT NULL CHECK (rating IN ('acerto', 'parcial', 'erro')),
    comment              text        NULL CHECK (comment IS NULL OR char_length(comment) <= 2000),
    langfuse_sync_status text        NOT NULL DEFAULT 'pending' CHECK (langfuse_sync_status IN ('pending', 'ok', 'failed')),
    created_at           timestamptz NOT NULL,
    CONSTRAINT ai_human_evals_comment_on_error_ck CHECK (rating <> 'erro' OR (comment IS NOT NULL AND btrim(comment) <> ''))
);

CREATE INDEX ai_human_evals_trace_idx ON ai_human_evals (trace_id, created_at DESC);

-- Os trace_id da Conciliação (TASK-007) passam a apontar para a corrida local. Continuam
-- opcionais até as TASKs dos agentes da Conciliação (016 a 019) definirem quando a corrida
-- existe; nunca apontam para um trace inexistente.
ALTER TABLE resolver_proposals
    ADD CONSTRAINT resolver_proposals_trace_fk FOREIGN KEY (trace_id) REFERENCES ai_runs (trace_id) ON DELETE RESTRICT;
ALTER TABLE extraction_logs
    ADD CONSTRAINT extraction_logs_trace_fk FOREIGN KEY (trace_id) REFERENCES ai_runs (trace_id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE extraction_logs DROP CONSTRAINT extraction_logs_trace_fk;
ALTER TABLE resolver_proposals DROP CONSTRAINT resolver_proposals_trace_fk;
DROP TABLE ai_human_evals;
DROP TABLE ai_gold_evals;
DROP TABLE ai_decisions;
DROP TABLE ai_runs;
