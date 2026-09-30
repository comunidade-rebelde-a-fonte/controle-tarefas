-- +goose Up
-- Instância de obrigação: tabela central do controle mensal [PRD §38.11].
-- UNIQUE (empresa, competência, tipo) impede duplicação entre telas: as telas são visões
-- desta tabela [RN-009, ADR-005].
CREATE TABLE obligation_instances (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    competence_id            uuid        NOT NULL REFERENCES competencies (id) ON DELETE RESTRICT,
    obligation_type_id       uuid        NOT NULL REFERENCES obligation_types (id) ON DELETE RESTRICT,
    status_code              text        NOT NULL,
    applicable               boolean     NOT NULL,
    -- Snapshot da regra tipo × regime na geração (D2): ICMS no Simples e Fechamento = false.
    counts_in_progress       boolean     NOT NULL,
    regime_snapshot          text        NOT NULL CHECK (regime_snapshot IN ('SIMPLES_NACIONAL', 'LUCRO_PRESUMIDO', 'LUCRO_REAL', 'LUCRO_ARBITRADO', 'IMUNE_ISENTA')),
    due_date_original        date        NULL,
    due_date_effective       date        NULL,
    due_date_override_reason text        NULL,
    due_date_overridden_by   uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    due_date_overridden_at   timestamptz NULL,
    transmission_date        date        NULL,
    protocol                 text        NULL,
    version                  integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    last_changed_by          uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    last_changed_at          timestamptz NULL,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT obligation_instances_uq UNIQUE (company_id, competence_id, obligation_type_id),
    CONSTRAINT obligation_instances_due_override_ck CHECK (
        due_date_override_reason IS NULL OR (due_date_overridden_by IS NOT NULL AND due_date_overridden_at IS NOT NULL)
    )
);

-- Índices de filtro [PRD §51].
CREATE INDEX obligation_instances_competence_company_idx ON obligation_instances (competence_id, company_id);
CREATE INDEX obligation_instances_competence_type_status_idx ON obligation_instances (competence_id, obligation_type_id, status_code);
CREATE INDEX obligation_instances_due_status_idx ON obligation_instances (due_date_effective, status_code);

-- +goose Down
DROP TABLE obligation_instances;
