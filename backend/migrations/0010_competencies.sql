-- +goose Up
-- Competências mensais [PRD §38.5] e snapshot da configuração usada na geração [plan D4, ADR-006].
CREATE TABLE competencies (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    competence_key char(7)     NOT NULL CHECK (competence_key ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    year           integer     NOT NULL CHECK (year BETWEEN 2000 AND 2200),
    month          integer     NOT NULL CHECK (month BETWEEN 1 AND 12),
    first_day      date        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT competencies_key_uq UNIQUE (competence_key),
    CONSTRAINT competencies_first_day_uq UNIQUE (first_day),
    CONSTRAINT competencies_consistency_ck CHECK (
        first_day = make_date(year, month, 1)
        AND competence_key = lpad(year::text, 4, '0') || '-' || lpad(month::text, 2, '0')
    )
);

-- Configuração da empresa congelada na competência: regime, tipo de folha, fator R, módulos,
-- responsáveis e localidade (calendário). Mudanças posteriores de cadastro não a alteram [RN-004].
CREATE TABLE company_competence_snapshots (
    id                               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                       uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    competence_id                    uuid        NOT NULL REFERENCES competencies (id) ON DELETE RESTRICT,
    tax_regime                       text        NOT NULL CHECK (tax_regime IN ('SIMPLES_NACIONAL', 'LUCRO_PRESUMIDO', 'LUCRO_REAL', 'LUCRO_ARBITRADO', 'IMUNE_ISENTA')),
    regime_group                     text        NOT NULL CHECK (regime_group IN ('NORMAL', 'SIMPLES')),
    payroll_type                     text        NOT NULL CHECK (payroll_type IN ('NO_PAYROLL', 'PRO_LABORE', 'PAYROLL')),
    factor_r                         boolean     NULL,
    active_modules                   text[]      NOT NULL,
    fiscal_responsible_user_id       uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    accounting_responsible_user_id   uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    payroll_responsible_user_id      uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    declarations_responsible_user_id uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    city                             text        NULL,
    state                            char(2)     NULL,
    created_at                       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT company_competence_snapshots_uq UNIQUE (company_id, competence_id),
    CONSTRAINT company_competence_snapshots_group_ck CHECK ((regime_group = 'SIMPLES') = (tax_regime = 'SIMPLES_NACIONAL'))
);

CREATE INDEX company_competence_snapshots_competence_idx ON company_competence_snapshots (competence_id, regime_group);

-- +goose Down
DROP TABLE company_competence_snapshots;
DROP TABLE competencies;
