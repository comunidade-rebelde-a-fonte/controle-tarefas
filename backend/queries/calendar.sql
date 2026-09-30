-- Calendários, feriados e regras de vencimento [PRD §21, §41.10].

-- name: ListCalendars :many
SELECT c.id, c.name, c.scope, c.state, c.city, count(h.id) AS holiday_count
FROM business_calendars c LEFT JOIN holidays h ON h.calendar_id = c.id
GROUP BY c.id
ORDER BY CASE c.scope WHEN 'NATIONAL' THEN 0 WHEN 'STATE' THEN 1 ELSE 2 END, c.state, c.city;

-- name: FindCalendar :one
SELECT * FROM business_calendars
WHERE scope = $1 AND coalesce(state, '') = coalesce(sqlc.narg('state')::text, '')
  AND coalesce(lower(city), '') = coalesce(lower(sqlc.narg('city')::text), '');

-- name: InsertCalendar :one
INSERT INTO business_calendars (name, scope, state, city) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListHolidays :many
SELECT h.id, h.calendar_id, h.date, h.name, h.scope, c.name AS calendar_name
FROM holidays h JOIN business_calendars c ON c.id = h.calendar_id
WHERE (sqlc.narg('calendar_id')::uuid IS NULL OR h.calendar_id = sqlc.narg('calendar_id')::uuid)
  AND (sqlc.narg('year')::int IS NULL OR extract(year FROM h.date)::int = sqlc.narg('year')::int)
ORDER BY h.date, c.name;

-- name: InsertHoliday :one
INSERT INTO holidays (calendar_id, date, name, scope) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetHoliday :one
SELECT * FROM holidays WHERE id = $1;

-- name: DeleteHoliday :exec
-- Exclusão permitida apenas para feriados configuráveis (estaduais, municipais e custom).
DELETE FROM holidays WHERE id = $1 AND scope <> 'NATIONAL';

-- name: HolidaysBetween :many
-- Feriados de um conjunto de calendários num intervalo (motor de vencimento).
SELECT DISTINCT date FROM holidays
WHERE calendar_id = ANY (sqlc.arg('calendar_ids')::uuid[]) AND date BETWEEN sqlc.arg('from_date')::date AND sqlc.arg('to_date')::date
ORDER BY date;

-- name: ListDueDateRules :many
SELECT * FROM due_date_rules ORDER BY name;

-- name: GetDueDateRule :one
SELECT * FROM due_date_rules WHERE id = $1;

-- name: InsertDueDateRule :one
INSERT INTO due_date_rules (name, month_offset, base_day, day_type, non_business_day_adjustment, calendar_id, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: UpdateDueDateRule :one
UPDATE due_date_rules SET
    name = $3, month_offset = $4, base_day = $5, day_type = $6, non_business_day_adjustment = $7,
    calendar_id = $8, notes = $9, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING *;
