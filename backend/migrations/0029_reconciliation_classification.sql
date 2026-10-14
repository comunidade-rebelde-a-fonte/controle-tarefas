-- +goose Up
-- TASK-020 (D-031): classificação contábil na linha do extrato conciliada, com a precedência
-- regra → histórico → agente e o rastreio do que classificou; a sugestão do agente fica à parte,
-- pendente da confirmação humana [REQ-026, REQ-027, AC-020]. O razão é consulta, não tabela
-- (REQ-028).

-- Vínculos por empresa, como o resto da Conciliação (D-020).
ALTER TABLE classification_rules
    ADD CONSTRAINT classification_rules_id_company_uq UNIQUE (id, company_id),
    -- A conta da regra existe no plano da empresa.
    ADD CONSTRAINT classification_rules_account_fk FOREIGN KEY (company_id, account_code)
        REFERENCES chart_accounts (company_id, code) ON DELETE RESTRICT,
    -- A linha cuja correção originou a regra (M5.5), quando houver.
    ADD COLUMN source_statement_transaction_id uuid NULL,
    ADD CONSTRAINT classification_rules_source_fk FOREIGN KEY (source_statement_transaction_id, company_id)
        REFERENCES statement_transactions (id, company_id) ON DELETE RESTRICT,
    ADD COLUMN deactivated_by uuid NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD COLUMN deactivated_at timestamptz NULL,
    ADD CONSTRAINT classification_rules_deactivation_ck CHECK (active = (deactivated_at IS NULL));

-- As regras ativas da empresa, da mais nova para a mais antiga (a mais nova vale; D-031).
DROP INDEX classification_rules_company_idx;
CREATE INDEX classification_rules_company_idx ON classification_rules (company_id, created_at DESC, id DESC) WHERE active;

-- sugestao de classificação do agente: só a sugestão; a conta da linha muda no aceite humano.
CREATE TABLE classification_suggestions (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               uuid        NOT NULL,
    statement_transaction_id uuid        NOT NULL,
    -- SugestaoClassificacao como a origem a grava (model_dump): conta, nome, justificativa,
    -- regra sugerida e confiança.
    payload                  jsonb       NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    -- A conta sugerida, só quando está no plano ativo da empresa; fora dele, nula.
    account_code             text        NULL,
    status                   text        NOT NULL DEFAULT 'pendente' CHECK (status IN ('pendente', 'aceita', 'rejeitada')),
    trace_id                 text        NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT,
    created_by               uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at               timestamptz NOT NULL DEFAULT now(),
    decided_by               uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at               timestamptz NULL,
    CONSTRAINT classification_suggestions_id_company_uq UNIQUE (id, company_id),
    CONSTRAINT classification_suggestions_txn_fk FOREIGN KEY (statement_transaction_id, company_id)
        REFERENCES statement_transactions (id, company_id) ON DELETE RESTRICT,
    CONSTRAINT classification_suggestions_account_fk FOREIGN KEY (company_id, account_code)
        REFERENCES chart_accounts (company_id, code) ON DELETE RESTRICT,
    CONSTRAINT classification_suggestions_decision_ck CHECK ((status = 'pendente') = (decided_at IS NULL)),
    -- Só se aceita sugestão com conta.
    CONSTRAINT classification_suggestions_accept_ck CHECK (status <> 'aceita' OR account_code IS NOT NULL)
);

CREATE INDEX classification_suggestions_txn_idx ON classification_suggestions (statement_transaction_id, created_at);

-- A classificação da linha, com o que a produziu: a regra, a linha do histórico ou a sugestão
-- aceita, e quem confirmou (manual ou sugestão aceita).
ALTER TABLE statement_transactions
    ADD COLUMN classification_rule_id uuid NULL,
    ADD COLUMN classification_history_id uuid NULL,
    ADD COLUMN classification_suggestion_id uuid NULL,
    ADD COLUMN classified_by uuid NULL REFERENCES users (id) ON DELETE RESTRICT,
    ADD COLUMN classified_at timestamptz NULL,
    ADD CONSTRAINT statement_transactions_account_fk FOREIGN KEY (company_id, account_code)
        REFERENCES chart_accounts (company_id, code) ON DELETE RESTRICT,
    ADD CONSTRAINT statement_transactions_rule_fk FOREIGN KEY (classification_rule_id, company_id)
        REFERENCES classification_rules (id, company_id) ON DELETE RESTRICT,
    ADD CONSTRAINT statement_transactions_history_fk FOREIGN KEY (classification_history_id, company_id)
        REFERENCES statement_transactions (id, company_id) ON DELETE RESTRICT,
    ADD CONSTRAINT statement_transactions_suggestion_fk FOREIGN KEY (classification_suggestion_id, company_id)
        REFERENCES classification_suggestions (id, company_id) ON DELETE RESTRICT,
    -- Classificação coerente: sem classificação; regra ou histórico com o que classificou; à espera
    -- do agente (pendente, sem conta); sugestão aceita ou correção manual, com quem confirmou.
    ADD CONSTRAINT statement_transactions_classification_ck CHECK (
        (classification_origin IS NULL AND account_code IS NULL AND NOT classification_pending)
        OR (classification_origin = 'regra' AND account_code IS NOT NULL AND classification_rule_id IS NOT NULL AND NOT classification_pending)
        OR (classification_origin = 'historico' AND account_code IS NOT NULL AND classification_history_id IS NOT NULL AND NOT classification_pending)
        OR (classification_origin = 'agente' AND classification_pending AND account_code IS NULL)
        OR (classification_origin = 'agente' AND NOT classification_pending AND account_code IS NOT NULL
            AND classification_suggestion_id IS NOT NULL AND classified_by IS NOT NULL)
        OR (classification_origin = 'manual' AND NOT classification_pending AND account_code IS NOT NULL AND classified_by IS NOT NULL)
    );

-- O histórico da empresa: as linhas classificadas e confirmadas, da mais recente para a mais antiga.
CREATE INDEX statement_transactions_classified_idx ON statement_transactions (company_id, txn_date DESC)
    WHERE account_code IS NOT NULL AND NOT classification_pending;

-- +goose Down
DROP INDEX statement_transactions_classified_idx;
ALTER TABLE statement_transactions
    DROP CONSTRAINT statement_transactions_classification_ck,
    DROP CONSTRAINT statement_transactions_suggestion_fk,
    DROP CONSTRAINT statement_transactions_history_fk,
    DROP CONSTRAINT statement_transactions_rule_fk,
    DROP CONSTRAINT statement_transactions_account_fk,
    DROP COLUMN classified_at,
    DROP COLUMN classified_by,
    DROP COLUMN classification_suggestion_id,
    DROP COLUMN classification_history_id,
    DROP COLUMN classification_rule_id;
DROP TABLE classification_suggestions;
DROP INDEX classification_rules_company_idx;
CREATE INDEX classification_rules_company_idx ON classification_rules (company_id) WHERE active;
ALTER TABLE classification_rules
    DROP CONSTRAINT classification_rules_deactivation_ck,
    DROP COLUMN deactivated_at,
    DROP COLUMN deactivated_by,
    DROP CONSTRAINT classification_rules_source_fk,
    DROP COLUMN source_statement_transaction_id,
    DROP CONSTRAINT classification_rules_account_fk,
    DROP CONSTRAINT classification_rules_id_company_uq;
