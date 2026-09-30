-- +goose Up
-- Configurações da empresa com vigência por competência [PRD §7.4, §9, §38.4, §38.10].
-- Alterações valem a partir da competência informada e não reescrevem competências já
-- geradas [RN-004]. Os intervalos de uma mesma chave não se sobrepõem.

-- Módulos contratados/atendidos. DCTFWeb não é módulo (é visão) [PRD §7.4].
CREATE TABLE company_module_settings (
    id                        uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    module_code               text        NOT NULL CHECK (module_code IN ('FISCAL', 'DECLARATIONS', 'ACCOUNTING', 'PAYROLL', 'CREDENTIALS')),
    enabled                   boolean     NOT NULL,
    effective_from_competence date        NOT NULL CHECK (extract(day FROM effective_from_competence) = 1),
    effective_to_competence   date        NULL CHECK (effective_to_competence IS NULL OR extract(day FROM effective_to_competence) = 1),
    created_by                uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at                timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT company_module_settings_period_ck CHECK (effective_to_competence IS NULL OR effective_to_competence >= effective_from_competence),
    CONSTRAINT company_module_settings_no_overlap EXCLUDE USING gist (
        company_id WITH =,
        module_code WITH =,
        daterange(effective_from_competence, effective_to_competence, '[]') WITH &&
    )
);

-- Override de aplicabilidade por empresa × tipo de obrigação (DEFAULT volta à regra padrão).
-- A FK para obligation_types é criada junto com o catálogo (0007).
CREATE TABLE company_obligation_overrides (
    id                        uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    obligation_type_id        uuid        NOT NULL,
    applicability             text        NOT NULL CHECK (applicability IN ('DEFAULT', 'APPLICABLE', 'NOT_APPLICABLE')),
    effective_from_competence date        NOT NULL CHECK (extract(day FROM effective_from_competence) = 1),
    effective_to_competence   date        NULL CHECK (effective_to_competence IS NULL OR extract(day FROM effective_to_competence) = 1),
    created_by                uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at                timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT company_obligation_overrides_period_ck CHECK (effective_to_competence IS NULL OR effective_to_competence >= effective_from_competence),
    CONSTRAINT company_obligation_overrides_no_overlap EXCLUDE USING gist (
        company_id WITH =,
        obligation_type_id WITH =,
        daterange(effective_from_competence, effective_to_competence, '[]') WITH &&
    )
);

-- +goose Down
DROP TABLE company_obligation_overrides;
DROP TABLE company_module_settings;
