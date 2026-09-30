-- Módulos e overrides de aplicabilidade com vigência [PRD §7.4, §9, §41.3-§41.4; plan R6].

-- name: ListModuleSettings :many
SELECT s.id, s.module_code, s.enabled, s.effective_from_competence, s.effective_to_competence,
       s.created_by, u.name AS created_by_name, s.created_at
FROM company_module_settings s JOIN users u ON u.id = s.created_by
WHERE s.company_id = $1
ORDER BY s.module_code, s.effective_from_competence;

-- name: OpenModuleSettingForUpdate :one
SELECT * FROM company_module_settings
WHERE company_id = $1 AND module_code = $2 AND effective_to_competence IS NULL
FOR UPDATE;

-- name: CloseModuleSetting :exec
UPDATE company_module_settings SET effective_to_competence = $2 WHERE id = $1;

-- name: SetModuleSettingEnabled :exec
UPDATE company_module_settings SET enabled = $2, created_by = $3, created_at = now() WHERE id = $1;

-- name: ModulesAt :many
-- Módulos habilitados na competência.
SELECT module_code FROM company_module_settings
WHERE company_id = $1 AND enabled
  AND effective_from_competence <= sqlc.arg('competence')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('competence')::date)
ORDER BY module_code;

-- name: ListOverrides :many
SELECT o.id, o.obligation_type_id, o.applicability, o.effective_from_competence, o.effective_to_competence,
       o.created_by, u.name AS created_by_name, o.created_at
FROM company_obligation_overrides o JOIN users u ON u.id = o.created_by
WHERE o.company_id = $1
ORDER BY o.obligation_type_id, o.effective_from_competence;

-- name: OpenOverrideForUpdate :one
SELECT * FROM company_obligation_overrides
WHERE company_id = $1 AND obligation_type_id = $2 AND effective_to_competence IS NULL
FOR UPDATE;

-- name: CloseOverride :exec
UPDATE company_obligation_overrides SET effective_to_competence = $2 WHERE id = $1;

-- name: SetOverrideApplicability :exec
UPDATE company_obligation_overrides SET applicability = $2, created_by = $3, created_at = now() WHERE id = $1;

-- name: InsertOverride :one
INSERT INTO company_obligation_overrides (company_id, obligation_type_id, applicability, effective_from_competence, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: OverridesAt :many
-- Overrides vigentes na competência (DEFAULT não altera a regra padrão).
SELECT obligation_type_id, applicability FROM company_obligation_overrides
WHERE company_id = $1
  AND effective_from_competence <= sqlc.arg('competence')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('competence')::date);

-- name: ListObligationTypesForCompany :many
-- Tipos ativos do catálogo com a regra padrão para o regime informado (NULL = não gerado no regime).
SELECT t.id, t.code, t.name, t.primary_module, t.sort_order,
       r.default_applicable, r.counts_in_progress
FROM obligation_types t
LEFT JOIN obligation_type_regime_rules r
       ON r.obligation_type_id = t.id AND r.tax_regime = sqlc.narg('tax_regime')::text AND r.active
WHERE t.active
ORDER BY t.primary_module, t.sort_order;

-- name: ObligationTypeExists :one
SELECT EXISTS (SELECT 1 FROM obligation_types WHERE id = $1);
