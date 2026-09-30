-- +goose Up
-- Calendários, feriados e regras de vencimento [PRD §21, §38.16-§38.18].
-- O calendário efetivo de uma empresa é a união: nacional + UF + município [plan M2].
CREATE TABLE business_calendars (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL,
    scope      text        NOT NULL CHECK (scope IN ('NATIONAL', 'STATE', 'MUNICIPAL')),
    state      char(2)     NULL CHECK (state IS NULL OR state ~ '^[A-Z]{2}$'),
    city       text        NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT business_calendars_scope_ck CHECK (
        (scope = 'NATIONAL' AND state IS NULL AND city IS NULL)
        OR (scope = 'STATE' AND state IS NOT NULL AND city IS NULL)
        OR (scope = 'MUNICIPAL' AND state IS NOT NULL AND city IS NOT NULL)
    )
);

CREATE UNIQUE INDEX business_calendars_uq ON business_calendars (scope, coalesce(state, ''), coalesce(lower(city), ''));

CREATE TABLE holidays (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_id uuid        NOT NULL REFERENCES business_calendars (id) ON DELETE RESTRICT,
    date        date        NOT NULL,
    name        text        NOT NULL CHECK (btrim(name) <> ''),
    scope       text        NOT NULL CHECK (scope IN ('NATIONAL', 'STATE', 'MUNICIPAL', 'CUSTOM')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT holidays_calendar_date_uq UNIQUE (calendar_id, date)
);

CREATE INDEX holidays_date_idx ON holidays (date);

-- Parâmetros do motor de vencimento [PRD §21.1]:
--   month_offset  meses após a competência (0 = no próprio mês);
--   base_day      dia N (CALENDAR) ou N-ésimo dia útil (BUSINESS);
--   ajuste quando o dia cai em dia não útil (PREVIOUS | NEXT | NONE).
CREATE TABLE due_date_rules (
    id                          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name                        text        NOT NULL CHECK (btrim(name) <> ''),
    month_offset                integer     NOT NULL CHECK (month_offset BETWEEN 0 AND 12),
    base_day                    integer     NOT NULL CHECK (base_day BETWEEN 1 AND 31),
    day_type                    text        NOT NULL CHECK (day_type IN ('CALENDAR', 'BUSINESS')),
    non_business_day_adjustment text        NOT NULL CHECK (non_business_day_adjustment IN ('PREVIOUS', 'NEXT', 'NONE')),
    calendar_id                 uuid        NULL REFERENCES business_calendars (id) ON DELETE RESTRICT,
    notes                       text        NULL,
    version                     integer     NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT due_date_rules_name_uq UNIQUE (name)
);

-- +goose Down
DROP TABLE due_date_rules;
DROP TABLE holidays;
DROP TABLE business_calendars;
