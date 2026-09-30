-- Derivados por empresa × módulo × competência [PRD §12, §10.5; plan R2-R4].

-- name: InstanceDerived :one
-- Uma instância com os derivados do status atual.
SELECT v.id, v.company_id, v.competence_id, v.obligation_type_id, v.type_code, v.primary_module,
       v.status_code, v.status_label, v.applicable, v.counts_in_progress, v.is_final, v.counts_as_started,
       v.is_blocking, v.due_date_original, v.due_date_effective, v.version,
       (v.applicable AND NOT v.is_final AND v.due_date_effective < sqlc.arg('today')::date) AS is_overdue
FROM v_obligation_instances v
WHERE v.id = sqlc.arg('id')::uuid;
