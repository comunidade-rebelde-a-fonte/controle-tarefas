-- name: SessionTimezone :one
-- Fuso da sessão; o pool força timezone=UTC [PRD §45.2].
SELECT current_setting('TimeZone')::text AS timezone;
