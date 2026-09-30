-- +goose Up
-- Histórico de mudanças da instância: status, vencimento, aplicabilidade, transmissão e
-- protocolo, com usuário, origem e motivo de exceção [PRD §23, §38.12].
CREATE TABLE obligation_status_history (
    id                     uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    obligation_instance_id uuid        NOT NULL REFERENCES obligation_instances (id) ON DELETE RESTRICT,
    field                  text        NOT NULL CHECK (field IN ('STATUS', 'DUE_DATE', 'APPLICABLE', 'TRANSMISSION_DATE', 'PROTOCOL')),
    old_value              text        NULL,
    new_value              text        NULL,
    user_id                uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    change_source          text        NOT NULL CHECK (change_source IN ('INLINE', 'BULK', 'IMPORT', 'SYSTEM')),
    -- Motivo de exceção (DCTFWeb com pendências) ou da alteração de vencimento.
    override_reason        text        NULL,
    created_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX obligation_status_history_instance_idx ON obligation_status_history (obligation_instance_id, created_at DESC);

-- +goose Down
DROP TABLE obligation_status_history;
