-- name: GetPeriod :one
-- Período do módulo (empresa + módulo + competência) com quem fechou.
SELECT p.id, p.company_id, k.competence_key, p.module_code, p.state, p.closed_at, p.general_note, p.version,
       p.closed_by AS closed_by_id, u.name AS closed_by_name
FROM company_module_periods p
JOIN competencies k ON k.id = p.competence_id
LEFT JOIN users u ON u.id = p.closed_by
WHERE p.id = $1;

-- name: GetPeriodForUpdate :one
SELECT p.id, p.company_id, p.competence_id, p.module_code, p.state, p.version
FROM company_module_periods p
WHERE p.id = $1
FOR UPDATE;

-- name: ClosePeriod :execrows
UPDATE company_module_periods SET
    state = 'CLOSED', closed_at = $3, closed_by = $4, version = version + 1, updated_at = $3
WHERE id = $1 AND version = $2 AND state = 'OPEN';

-- name: ReopenPeriod :execrows
UPDATE company_module_periods SET
    state = 'OPEN', closed_at = NULL, closed_by = NULL, version = version + 1, updated_at = $3
WHERE id = $1 AND version = $2 AND state = 'CLOSED';

-- name: UpdatePeriodNote :execrows
UPDATE company_module_periods SET general_note = $3, version = version + 1, updated_at = $4
WHERE id = $1 AND version = $2;

-- name: InsertPeriodEvent :exec
INSERT INTO company_module_period_events (company_module_period_id, event_type, reason, user_id, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: ListPeriodEvents :many
-- Eventos de abertura, fechamento e reabertura, mais recente primeiro.
SELECT e.id, e.event_type, e.reason, e.created_at, u.id AS user_id, u.name AS user_name
FROM company_module_period_events e
JOIN users u ON u.id = e.user_id
WHERE e.company_module_period_id = $1
ORDER BY e.created_at DESC, e.seq DESC;

-- name: ClosingStageInstances :many
-- Estágio de fechamento do módulo (ex.: coluna Fechamento do Fiscal) e o status final do seu conjunto.
SELECT i.id, i.status_code, i.version, i.applicable, t.id AS obligation_type_id, t.name AS type_name,
       cur.label AS status_label, fin.code AS final_code, fin.label AS final_label
FROM obligation_instances i
JOIN obligation_types t ON t.id = i.obligation_type_id
JOIN status_definitions cur ON cur.status_set_id = t.status_set_id AND cur.code = i.status_code
JOIN LATERAL (
    SELECT d.code, d.label FROM status_definitions d
    WHERE d.status_set_id = t.status_set_id AND d.is_final
    ORDER BY d.sort_order DESC
    LIMIT 1
) fin ON true
WHERE i.company_id = $1 AND i.competence_id = $2 AND t.primary_module = $3 AND t.is_closing_stage
ORDER BY t.sort_order
FOR UPDATE OF i;
