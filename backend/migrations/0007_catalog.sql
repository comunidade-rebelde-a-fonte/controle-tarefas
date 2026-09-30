-- +goose Up
-- Catálogo configurável de obrigações [PRD §20, §38.8-§38.9, §38.15; plan D1-D3, D5, D7].
-- Obrigações não são colunas fixas: novas declarações entram por cadastro, sem migration [§15.2].
CREATE TABLE obligation_types (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    code                     text        NOT NULL CHECK (code ~ '^[A-Z][A-Z0-9_]*$'),
    name                     text        NOT NULL CHECK (btrim(name) <> ''),
    -- Módulo dono da instância (fechamento e ativação). DCTFWeb é só contexto de exibição (R1, D5).
    primary_module           text        NOT NULL CHECK (primary_module IN ('FISCAL', 'DECLARATIONS', 'ACCOUNTING', 'PAYROLL')),
    status_set_id            uuid        NOT NULL REFERENCES status_sets (id) ON DELETE RESTRICT,
    periodicity              text        NOT NULL DEFAULT 'MONTHLY' CHECK (periodicity IN ('MONTHLY')),
    active                   boolean     NOT NULL DEFAULT true,
    sort_order               integer     NOT NULL,
    -- Padrão do tipo; o valor efetivo vem de obligation_type_regime_rules (D2).
    counts_in_progress       boolean     NOT NULL DEFAULT true,
    due_date_rule_id         uuid        NULL REFERENCES due_date_rules (id) ON DELETE RESTRICT,
    allows_transmission_date boolean     NOT NULL DEFAULT false,
    allows_protocol          boolean     NOT NULL DEFAULT false,
    initial_status_code      text        NOT NULL DEFAULT 'NOT_STARTED',
    -- Status aplicado na geração quando a empresa é "Sem folha" (D7).
    status_when_no_payroll   text        NULL CHECK (status_when_no_payroll IN ('NO_MOVEMENT', 'NOT_APPLICABLE')),
    -- Coluna "Fechamento" do Fiscal: estágio, fora dos percentuais e do "pronto para fechar" (D1, R2, R3).
    is_closing_stage         boolean     NOT NULL DEFAULT false,
    version                  integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT obligation_types_code_uq UNIQUE (code),
    -- Os status configurados precisam existir no conjunto do tipo.
    CONSTRAINT obligation_types_initial_status_fk FOREIGN KEY (status_set_id, initial_status_code)
        REFERENCES status_definitions (status_set_id, code) ON DELETE RESTRICT,
    CONSTRAINT obligation_types_no_payroll_status_fk FOREIGN KEY (status_set_id, status_when_no_payroll)
        REFERENCES status_definitions (status_set_id, code) ON DELETE RESTRICT
);

-- Telas em que o tipo aparece como coluna (a mesma instância em vários contextos) [§38.9, RN-009].
CREATE TABLE obligation_display_contexts (
    obligation_type_id uuid    NOT NULL REFERENCES obligation_types (id) ON DELETE RESTRICT,
    module_code        text    NOT NULL CHECK (module_code IN ('FISCAL', 'DECLARATIONS', 'ACCOUNTING', 'PAYROLL', 'DCTFWEB')),
    sort_order         integer NOT NULL,
    active             boolean NOT NULL DEFAULT true,
    PRIMARY KEY (obligation_type_id, module_code)
);

-- Aplicabilidade padrão por regime (D2/D3): tipo sem regra ativa para o regime não é gerado.
CREATE TABLE obligation_type_regime_rules (
    obligation_type_id uuid    NOT NULL REFERENCES obligation_types (id) ON DELETE RESTRICT,
    tax_regime         text    NOT NULL CHECK (tax_regime IN ('SIMPLES_NACIONAL', 'LUCRO_PRESUMIDO', 'LUCRO_REAL', 'LUCRO_ARBITRADO', 'IMUNE_ISENTA')),
    default_applicable boolean NOT NULL DEFAULT true,
    counts_in_progress boolean NOT NULL DEFAULT true,
    active             boolean NOT NULL DEFAULT true,
    PRIMARY KEY (obligation_type_id, tax_regime)
);

-- Dependências entre tipos: HARD (Contábil, bloqueia) e SOFT (DCTFWeb, alerta) [§16.4, §18.2].
CREATE TABLE obligation_dependencies (
    id                              uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    obligation_type_id              uuid    NOT NULL REFERENCES obligation_types (id) ON DELETE RESTRICT,
    prerequisite_obligation_type_id uuid    NOT NULL REFERENCES obligation_types (id) ON DELETE RESTRICT,
    dependency_mode                 text    NOT NULL CHECK (dependency_mode IN ('HARD', 'SOFT')),
    active                          boolean NOT NULL DEFAULT true,
    CONSTRAINT obligation_dependencies_uq UNIQUE (obligation_type_id, prerequisite_obligation_type_id),
    CONSTRAINT obligation_dependencies_self_ck CHECK (obligation_type_id <> prerequisite_obligation_type_id)
);

ALTER TABLE company_obligation_overrides
    ADD CONSTRAINT company_obligation_overrides_type_fk
    FOREIGN KEY (obligation_type_id) REFERENCES obligation_types (id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE company_obligation_overrides DROP CONSTRAINT company_obligation_overrides_type_fk;
DROP TABLE obligation_dependencies;
DROP TABLE obligation_type_regime_rules;
DROP TABLE obligation_display_contexts;
DROP TABLE obligation_types;
