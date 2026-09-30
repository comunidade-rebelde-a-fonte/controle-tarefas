-- +goose Up
-- Fechamento independente por empresa + módulo + competência [PRD §10.3, §38.6-§38.7, RN-006].
-- Procurações/Certificados não é mensal: não tem período.
CREATE TABLE company_module_periods (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id    uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    competence_id uuid        NOT NULL REFERENCES competencies (id) ON DELETE RESTRICT,
    module_code   text        NOT NULL CHECK (module_code IN ('FISCAL', 'DECLARATIONS', 'ACCOUNTING', 'PAYROLL')),
    state         text        NOT NULL DEFAULT 'OPEN' CHECK (state IN ('OPEN', 'CLOSED')),
    closed_at     timestamptz NULL,
    closed_by     uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    -- Observação geral da empresa no módulo e competência [RN-016].
    general_note  text        NULL,
    version       integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT company_module_periods_uq UNIQUE (company_id, competence_id, module_code),
    CONSTRAINT company_module_periods_closed_ck CHECK ((state = 'CLOSED') = (closed_at IS NOT NULL AND closed_by IS NOT NULL))
);

CREATE INDEX company_module_periods_competence_module_state_idx ON company_module_periods (competence_id, module_code, state);

-- Histórico de abertura, fechamento e reabertura (reabertura exige motivo) [RN-007].
CREATE TABLE company_module_period_events (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_module_period_id uuid        NOT NULL REFERENCES company_module_periods (id) ON DELETE RESTRICT,
    event_type               text        NOT NULL CHECK (event_type IN ('OPENED', 'CLOSED', 'REOPENED')),
    reason                   text        NULL,
    user_id                  uuid        NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT company_module_period_events_reason_ck CHECK (event_type <> 'REOPENED' OR (reason IS NOT NULL AND btrim(reason) <> ''))
);

CREATE INDEX company_module_period_events_period_idx ON company_module_period_events (company_module_period_id, created_at);

-- +goose Down
DROP TABLE company_module_period_events;
DROP TABLE company_module_periods;
