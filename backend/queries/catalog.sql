-- Catálogo de obrigações [PRD §20, §41.10].

-- name: ListObligationTypes :many
SELECT t.*, s.code AS status_set_code
FROM obligation_types t JOIN status_sets s ON s.id = t.status_set_id
ORDER BY t.primary_module, t.sort_order, t.code;

-- name: GetObligationType :one
SELECT t.*, s.code AS status_set_code
FROM obligation_types t JOIN status_sets s ON s.id = t.status_set_id
WHERE t.id = $1;

-- name: ListDisplayContexts :many
SELECT obligation_type_id, module_code, sort_order FROM obligation_display_contexts
WHERE active ORDER BY module_code, sort_order;

-- name: ListRegimeRules :many
SELECT obligation_type_id, tax_regime, default_applicable, counts_in_progress FROM obligation_type_regime_rules
WHERE active ORDER BY obligation_type_id, tax_regime;

-- name: ListDependencies :many
SELECT d.id, d.obligation_type_id, d.prerequisite_obligation_type_id, p.code AS prerequisite_code, d.dependency_mode
FROM obligation_dependencies d JOIN obligation_types p ON p.id = d.prerequisite_obligation_type_id
WHERE d.active ORDER BY d.obligation_type_id, p.sort_order;

-- name: GetStatusSetByCode :one
SELECT * FROM status_sets WHERE code = $1;

-- name: StatusExistsInSet :one
SELECT EXISTS (SELECT 1 FROM status_definitions WHERE status_set_id = $1 AND code = $2);

-- name: DueDateRuleExists :one
SELECT EXISTS (SELECT 1 FROM due_date_rules WHERE id = $1);

-- name: InsertObligationType :one
INSERT INTO obligation_types (
    code, name, primary_module, status_set_id, active, sort_order, counts_in_progress, due_date_rule_id,
    allows_transmission_date, allows_protocol, initial_status_code, status_when_no_payroll
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: UpdateObligationType :one
UPDATE obligation_types SET
    name = $3, active = $4, sort_order = $5, counts_in_progress = $6, due_date_rule_id = $7,
    allows_transmission_date = $8, allows_protocol = $9, initial_status_code = $10,
    status_when_no_payroll = $11, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING id;

-- name: DeactivateDisplayContexts :exec
UPDATE obligation_display_contexts SET active = false WHERE obligation_type_id = $1;

-- name: UpsertDisplayContext :exec
INSERT INTO obligation_display_contexts (obligation_type_id, module_code, sort_order, active)
VALUES ($1, $2, $3, true)
ON CONFLICT (obligation_type_id, module_code) DO UPDATE SET sort_order = EXCLUDED.sort_order, active = true;

-- name: DeactivateRegimeRules :exec
UPDATE obligation_type_regime_rules SET active = false WHERE obligation_type_id = $1;

-- name: UpsertRegimeRule :exec
INSERT INTO obligation_type_regime_rules (obligation_type_id, tax_regime, default_applicable, counts_in_progress, active)
VALUES ($1, $2, $3, $4, true)
ON CONFLICT (obligation_type_id, tax_regime) DO UPDATE SET
    default_applicable = EXCLUDED.default_applicable, counts_in_progress = EXCLUDED.counts_in_progress, active = true;

-- name: DeactivateDependencies :exec
UPDATE obligation_dependencies SET active = false WHERE obligation_type_id = $1;

-- name: UpsertDependency :exec
INSERT INTO obligation_dependencies (obligation_type_id, prerequisite_obligation_type_id, dependency_mode, active)
VALUES ($1, $2, $3, true)
ON CONFLICT (obligation_type_id, prerequisite_obligation_type_id) DO UPDATE SET
    dependency_mode = EXCLUDED.dependency_mode, active = true;

-- name: ListStatusSets :many
SELECT id, code, name FROM status_sets ORDER BY code;

-- name: ListStatusDefinitions :many
SELECT * FROM status_definitions ORDER BY status_set_id, sort_order;
