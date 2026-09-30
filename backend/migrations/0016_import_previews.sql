-- +goose Up
-- Prévias de importação de empresas [PRD §35.3, §41.12]: o resultado da validação fica
-- guardado com token e validade (1 h) até a confirmação. Não guarda o arquivo enviado.
CREATE TABLE import_previews (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         text        NOT NULL DEFAULT 'COMPANIES' CHECK (kind IN ('COMPANIES')),
    file_name    text        NOT NULL,
    rows         jsonb       NOT NULL,
    summary      jsonb       NOT NULL,
    created_by   uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at   timestamptz NOT NULL,
    expires_at   timestamptz NOT NULL,
    committed_at timestamptz NULL,
    committed_by uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    result       jsonb       NULL,
    CONSTRAINT import_previews_expiry_ck CHECK (expires_at > created_at)
);

CREATE INDEX import_previews_expires_at_idx ON import_previews (expires_at);

-- +goose Down
DROP TABLE import_previews;
