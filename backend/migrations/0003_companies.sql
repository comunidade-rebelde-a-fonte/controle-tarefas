-- +goose Up
-- Cadastro único de empresas [PRD §7, §38.2] e histórico de regime tributário [§8, §38.3].
-- Sem exclusão física: FKs ON DELETE RESTRICT e nenhum endpoint DELETE [RN-008].
CREATE TABLE companies (
    id                               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    internal_code                    text        NOT NULL CHECK (btrim(internal_code) <> ''),
    legal_name                       text        NOT NULL CHECK (btrim(legal_name) <> ''),
    trade_name                       text        NULL,
    cnpj                             char(14)    NOT NULL CHECK (cnpj ~ '^[0-9]{14}$'),
    city                             text        NULL,
    state                            char(2)     NULL CHECK (state IS NULL OR state ~ '^[A-Z]{2}$'),
    state_registration               text        NULL,
    municipal_registration           text        NULL,
    service_start_date               date        NOT NULL,
    service_end_date                 date        NULL,
    active                           boolean     NOT NULL DEFAULT true,
    payroll_type                     text        NOT NULL CHECK (payroll_type IN ('NO_PAYROLL', 'PRO_LABORE', 'PAYROLL')),
    factor_r                         boolean     NULL,
    fiscal_responsible_user_id       uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    accounting_responsible_user_id   uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    payroll_responsible_user_id      uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    declarations_responsible_user_id uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    notes                            text        NULL,
    version                          integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                       uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    updated_by                       uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at                       timestamptz NOT NULL DEFAULT now(),
    updated_at                       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT companies_cnpj_uq UNIQUE (cnpj),
    CONSTRAINT companies_internal_code_uq UNIQUE (internal_code),
    CONSTRAINT companies_service_period_ck CHECK (service_end_date IS NULL OR service_end_date >= service_start_date)
);

-- Índices de filtro/busca [PRD §51] + pg_trgm para a busca global por nome [plan §4].
CREATE INDEX companies_legal_name_idx ON companies (legal_name);
CREATE INDEX companies_legal_name_trgm_idx ON companies USING gin (legal_name gin_trgm_ops);
CREATE INDEX companies_trade_name_trgm_idx ON companies USING gin (trade_name gin_trgm_ops);

-- Regime vigente por competência. Competências são armazenadas como DATE no dia 1;
-- effective_to_competence é inclusivo e NULL significa vigência aberta.
CREATE TABLE company_tax_regime_history (
    id                        uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    tax_regime                text        NOT NULL CHECK (tax_regime IN ('SIMPLES_NACIONAL', 'LUCRO_PRESUMIDO', 'LUCRO_REAL', 'LUCRO_ARBITRADO', 'IMUNE_ISENTA')),
    effective_from_competence date        NOT NULL CHECK (extract(day FROM effective_from_competence) = 1),
    effective_to_competence   date        NULL CHECK (effective_to_competence IS NULL OR extract(day FROM effective_to_competence) = 1),
    created_by                uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at                timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT company_tax_regime_period_ck CHECK (effective_to_competence IS NULL OR effective_to_competence >= effective_from_competence),
    -- Intervalos da mesma empresa não podem se sobrepor [PRD §38.3].
    CONSTRAINT company_tax_regime_no_overlap EXCLUDE USING gist (
        company_id WITH =,
        daterange(effective_from_competence, effective_to_competence, '[]') WITH &&
    )
);

CREATE INDEX company_tax_regime_company_from_idx ON company_tax_regime_history (company_id, effective_from_competence);

-- +goose Down
DROP TABLE company_tax_regime_history;
DROP TABLE companies;
