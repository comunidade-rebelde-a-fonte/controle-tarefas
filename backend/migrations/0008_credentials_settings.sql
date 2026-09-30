-- +goose Up
-- Procurações RFB e certificados digitais: um registro ativo por empresa e tipo [PRD §19, §38.19, RN-017].
-- Nunca armazena senha, chave privada ou segredo [RN-018]: só metadados e arquivo público validado.
CREATE TABLE credential_controls (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id       uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    type             text        NOT NULL CHECK (type IN ('RFB_POWER_OF_ATTORNEY', 'DIGITAL_CERTIFICATE')),
    expires_at       date        NULL,
    renewal_status   text        NOT NULL DEFAULT 'CURRENT' CHECK (renewal_status IN ('CURRENT', 'RENEWAL_REQUESTED', 'AWAITING_CLIENT', 'RENEWED')),
    file_storage_key text        NULL,
    file_name        text        NULL,
    file_mime_type   text        NULL,
    file_size_bytes  bigint      NULL CHECK (file_size_bytes IS NULL OR file_size_bytes >= 0),
    file_uploaded_at timestamptz NULL,
    notes            text        NULL,
    version          integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by       uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT credential_controls_company_type_uq UNIQUE (company_id, type),
    CONSTRAINT credential_controls_file_ck CHECK ((file_storage_key IS NULL) = (file_name IS NULL))
);

CREATE INDEX credential_controls_expires_at_idx ON credential_controls (expires_at);

-- Configurações chave/valor (JSON), ex.: faixas de validade de credenciais [§19.3].
CREATE TABLE settings (
    key        text        PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]*$'),
    value      jsonb       NOT NULL,
    updated_by uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO settings (key, value) VALUES ('credential_thresholds', '{"days": [7, 15, 30, 60]}');

-- +goose Down
DROP TABLE settings;
DROP TABLE credential_controls;
