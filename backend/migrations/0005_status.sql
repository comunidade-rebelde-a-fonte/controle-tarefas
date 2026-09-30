-- +goose Up
-- Conjuntos de status por tipo de processo e seus metadados [PRD §11, §38.13-§38.14].
-- Os metadados permitem calcular dashboard e progresso sem regra na interface.
CREATE TABLE status_sets (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text        NOT NULL CHECK (code ~ '^[A-Z][A-Z0-9_]*$'),
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT status_sets_code_uq UNIQUE (code)
);

CREATE TABLE status_definitions (
    id                             uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    status_set_id                  uuid    NOT NULL REFERENCES status_sets (id) ON DELETE RESTRICT,
    code                           text    NOT NULL CHECK (code ~ '^[A-Z][A-Z0-9_]*$'),
    label                          text    NOT NULL,
    sort_order                     integer NOT NULL,
    counts_as_started              boolean NOT NULL,
    counts_as_operational_progress boolean NOT NULL,
    is_final                       boolean NOT NULL,
    -- Pendência: bloqueia o fechamento e entra em "pendentes"/"Exige atenção".
    is_blocking                    boolean NOT NULL,
    -- Status que retira o item do denominador dos percentuais (ex.: Fiscal "Não se aplica").
    excluded_from_denominator      boolean NOT NULL DEFAULT false,
    CONSTRAINT status_definitions_set_code_uq UNIQUE (status_set_id, code)
);

-- +goose Down
DROP TABLE status_definitions;
DROP TABLE status_sets;
