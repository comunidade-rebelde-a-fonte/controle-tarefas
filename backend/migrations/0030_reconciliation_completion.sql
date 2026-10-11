-- +goose Up
-- Conclusão da Conciliação por empresa e competência (TASK-022, D-035; F-008). O contador conclui
-- quando o extrato está todo resolvido, e a obrigação Conciliação do Contábil vai para OK pelo
-- serviço de obrigações; reaberta com motivo, volta para Pendente. Não é um segundo fechamento: o
-- fechamento do Contábil continua a única autoridade (ADR-003). Cada conclusão é uma linha; a
-- reabertura completa a linha, e o histórico de conclusões e reaberturas fica.

-- A origem das mudanças automáticas entre módulos (PRD §6.6): o histórico da obrigação passa a
-- aceitar SYSTEM_INTEGRATION. Mudança aditiva: os valores existentes continuam válidos.
ALTER TABLE obligation_status_history DROP CONSTRAINT obligation_status_history_change_source_check;
ALTER TABLE obligation_status_history ADD CONSTRAINT obligation_status_history_change_source_check
    CHECK (change_source IN ('INLINE', 'BULK', 'IMPORT', 'SYSTEM', 'SYSTEM_INTEGRATION'));

CREATE TABLE reconciliation_completions (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    competence               date        NOT NULL CHECK (extract(day FROM competence) = 1),
    completed_by             uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    completed_at             timestamptz NOT NULL,
    -- A obrigação Conciliação no momento da conclusão e o que a sincronização fez com ela:
    -- concluida (foi para OK), ja_final (já estava final, nada mudou), nao_aplicavel ou
    -- sem_obrigacao (a empresa não tem a obrigação na competência).
    obligation_instance_id   uuid        NULL REFERENCES obligation_instances (id) ON DELETE RESTRICT,
    obligation_status_before text        NULL,
    obligation_sync          text        NOT NULL CHECK (obligation_sync IN ('concluida', 'ja_final', 'nao_aplicavel', 'sem_obrigacao')),
    -- A reabertura, sempre com motivo: pendente (a obrigação voltou para Pendente) ou mantida
    -- (alguém mudou a obrigação depois da conclusão, ou ela já era final; nada mudou).
    reopened_by              uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    reopened_at              timestamptz NULL,
    reopen_reason            text        NULL CHECK (reopen_reason IS NULL OR btrim(reopen_reason) <> ''),
    reopen_obligation_sync   text        NULL CHECK (reopen_obligation_sync IN ('pendente', 'mantida', 'nao_aplicavel', 'sem_obrigacao')),
    -- Ordem de inserção (como no histórico das obrigações, 0015): desempata conclusões no mesmo instante.
    seq                      bigint      GENERATED ALWAYS AS IDENTITY,
    CONSTRAINT reconciliation_completions_obligation_ck CHECK ((obligation_instance_id IS NULL) = (obligation_sync = 'sem_obrigacao')),
    CONSTRAINT reconciliation_completions_reopen_ck CHECK (
        (reopened_at IS NULL) = (reopened_by IS NULL)
        AND (reopened_at IS NULL) = (reopen_reason IS NULL)
        AND (reopened_at IS NULL) = (reopen_obligation_sync IS NULL)
    )
);

-- Uma conclusão em vigor por empresa e competência: a concorrência acaba aqui, além da trava da
-- empresa (LockReconciliationCompany).
CREATE UNIQUE INDEX reconciliation_completions_active_uq ON reconciliation_completions (company_id, competence)
    WHERE reopened_at IS NULL;
CREATE INDEX reconciliation_completions_history_idx ON reconciliation_completions (company_id, competence, completed_at DESC);

-- +goose Down
-- Falha de propósito se houver histórico com SYSTEM_INTEGRATION: o histórico não é apagado.
DROP TABLE reconciliation_completions;
ALTER TABLE obligation_status_history DROP CONSTRAINT obligation_status_history_change_source_check;
ALTER TABLE obligation_status_history ADD CONSTRAINT obligation_status_history_change_source_check
    CHECK (change_source IN ('INLINE', 'BULK', 'IMPORT', 'SYSTEM'));
