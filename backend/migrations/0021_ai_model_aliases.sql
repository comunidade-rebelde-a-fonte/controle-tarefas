-- +goose Up
-- Configuração dos modelos de IA por alias [docs/ai-spec/05 §8, 06 §7, TASK-008]. Guarda só
-- configuração não secreta: as chaves dos provedores existem apenas no ambiente do LiteLLM
-- [REQ-047, 10 §9]. Uma linha por alias do ai.Catalog (um teste confere).
--
-- provider/model NULL = vale o padrão do .env (<ALIAS>_MODEL). A escolha feita na tela vale para
-- as novas corridas; a corrida em andamento mantém o modelo resolvido no início [F-010].
CREATE TABLE ai_model_aliases (
    alias       text        PRIMARY KEY CHECK (alias ~ '^[a-z][a-z0-9_]{1,39}$'),
    provider    text        NULL CHECK (provider ~ '^[a-z][a-z0-9_-]{1,39}$'),
    -- Mesmo formato do ai.ParseModel; nunca algo com cara de chave (sk-…, pk-…, rk-…).
    model       text        NULL CHECK (model ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$' AND model !~* '^(sk|pk|rk)-'),
    -- Parâmetros de chamada permitidos (lista fechada); ausente = padrão do provedor.
    parameters  jsonb       NOT NULL DEFAULT '{}',
    enabled     boolean     NOT NULL DEFAULT true,
    version     integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by  uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ai_model_aliases_model_pair_ck CHECK ((provider IS NULL) = (model IS NULL)),
    CONSTRAINT ai_model_aliases_parameters_ck CHECK (
        jsonb_typeof(parameters) = 'object'
        AND parameters - ARRAY['temperature', 'max_tokens'] = '{}'::jsonb
        AND CASE
            WHEN parameters -> 'temperature' IS NULL THEN true
            WHEN jsonb_typeof(parameters -> 'temperature') <> 'number' THEN false
            ELSE (parameters ->> 'temperature')::numeric BETWEEN 0 AND 2
        END
        AND CASE
            WHEN parameters -> 'max_tokens' IS NULL THEN true
            WHEN jsonb_typeof(parameters -> 'max_tokens') <> 'number' THEN false
            ELSE (parameters ->> 'max_tokens')::numeric BETWEEN 1 AND 1000000
                AND (parameters ->> 'max_tokens')::numeric = trunc((parameters ->> 'max_tokens')::numeric)
        END
    )
);

-- Histórico de versões (quem trocou o quê e quando), gravado pelo serviço na mesma transação da
-- alteração, como os demais históricos do Controle.
CREATE TABLE ai_model_alias_history (
    alias       text        NOT NULL REFERENCES ai_model_aliases (alias) ON DELETE RESTRICT,
    version     integer     NOT NULL CHECK (version >= 1),
    provider    text        NULL,
    model       text        NULL,
    parameters  jsonb       NOT NULL CHECK (parameters - ARRAY['temperature', 'max_tokens'] = '{}'::jsonb),
    enabled     boolean     NOT NULL,
    changed_by  uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    changed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (alias, version),
    CONSTRAINT ai_model_alias_history_model_pair_ck CHECK ((provider IS NULL) = (model IS NULL))
);

-- Aliases do ai.Catalog, todos começando pelo padrão do .env (versão 1).
INSERT INTO ai_model_aliases (alias)
VALUES ('triador'), ('extrator'), ('extrator_forte'), ('resolvedor'), ('classificador'), ('consultor'), ('priorizador');

INSERT INTO ai_model_alias_history (alias, version, parameters, enabled)
SELECT alias, version, parameters, enabled FROM ai_model_aliases;

-- +goose Down
DROP TABLE ai_model_alias_history;
DROP TABLE ai_model_aliases;
