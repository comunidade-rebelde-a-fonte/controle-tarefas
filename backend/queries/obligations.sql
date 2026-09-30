-- Instâncias de obrigação: alteração de status, detalhes e histórico [PRD §23, §41.7].

-- name: GetInstanceForUpdate :one
SELECT i.id, i.company_id, i.competence_id, i.obligation_type_id, i.status_code, i.applicable,
       i.version, i.due_date_original, i.due_date_effective, i.transmission_date, i.protocol,
       t.code AS type_code, t.name AS type_name, t.primary_module, t.status_set_id,
       t.allows_transmission_date, t.allows_protocol,
       k.competence_key,
       COALESCE(p.state, 'OPEN')::text AS period_state
FROM obligation_instances i
JOIN obligation_types t ON t.id = i.obligation_type_id
JOIN competencies k ON k.id = i.competence_id
LEFT JOIN company_module_periods p
       ON p.company_id = i.company_id AND p.competence_id = i.competence_id AND p.module_code = t.primary_module
WHERE i.id = $1
FOR UPDATE OF i;

-- name: GetStatusDefinition :one
SELECT * FROM status_definitions WHERE status_set_id = $1 AND code = $2;

-- name: UpdateInstanceStatus :execrows
UPDATE obligation_instances SET
    status_code = $3, version = version + 1, last_changed_by = $4, last_changed_at = $5, updated_at = $5
WHERE id = $1 AND version = $2;

-- name: InsertStatusHistory :exec
INSERT INTO obligation_status_history (obligation_instance_id, field, old_value, new_value, user_id, change_source, override_reason, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetInstanceView :one
-- Instância para resposta da API, com metadados do status e indicadores.
SELECT v.id, v.company_id, k.competence_key, v.obligation_type_id, v.type_code, t.name AS type_name,
       v.primary_module, v.status_code, v.status_label, v.applicable, v.counts_in_progress,
       v.is_final, v.counts_as_started, v.is_blocking, v.excluded_from_denominator,
       v.due_date_original, v.due_date_effective, v.due_date_override_reason,
       v.transmission_date, v.protocol, v.version, v.last_changed_at,
       u.name AS last_changed_by_name,
       EXISTS (SELECT 1 FROM obligation_status_history h WHERE h.obligation_instance_id = v.id) AS has_history
FROM v_obligation_instances v
JOIN obligation_types t ON t.id = v.obligation_type_id
JOIN competencies k ON k.id = v.competence_id
LEFT JOIN users u ON u.id = v.last_changed_by
WHERE v.id = $1;

-- name: ListActiveDependencies :many
SELECT obligation_type_id, prerequisite_obligation_type_id, dependency_mode
FROM obligation_dependencies WHERE active;

-- name: CompanyCompetenceInstances :many
-- Instâncias da empresa na competência, para checar dependências.
SELECT v.obligation_type_id, v.type_code, t.name AS type_name, v.status_code, v.applicable, v.is_final, v.type_sort_order
FROM v_obligation_instances v
JOIN obligation_types t ON t.id = v.obligation_type_id
WHERE v.company_id = $1 AND v.competence_id = $2;
