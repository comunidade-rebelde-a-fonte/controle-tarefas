-- A ficha da empresa como hub (TASK-025, D-039): a Visão geral da empresa na competência e os
-- indicadores compactos da listagem de empresas.

-- name: CompanyModuleProgress :many
-- Por módulo da empresa na competência: o estado do período e as contagens das obrigações, com
-- as classes do Dashboard (aplicáveis no progresso, concluídas, pendentes e atrasadas).
SELECT p.module_code AS module, p.state,
       count(v.id) FILTER (WHERE v.in_denominator)::int                                        AS applicable,
       count(v.id) FILTER (WHERE v.in_denominator AND v.is_final)::int                         AS done,
       count(v.id) FILTER (WHERE v.in_denominator AND v.counts_as_operational_progress)::int   AS operational_done,
       count(v.id) FILTER (WHERE v.in_denominator AND v.is_blocking)::int                      AS pending,
       count(v.id) FILTER (WHERE v.applicable AND NOT v.is_final
                             AND v.due_date_effective < sqlc.arg('today')::date)::int          AS overdue
FROM company_module_periods p
LEFT JOIN v_obligation_instances v
       ON v.company_id = p.company_id AND v.competence_id = p.competence_id AND v.primary_module = p.module_code
WHERE p.company_id = sqlc.arg('company_id') AND p.competence_id = sqlc.arg('competence_id')
GROUP BY p.module_code, p.state
ORDER BY CASE p.module_code WHEN 'FISCAL' THEN 1 WHEN 'DECLARATIONS' THEN 2 WHEN 'PAYROLL' THEN 3 WHEN 'ACCOUNTING' THEN 4 ELSE 5 END;

-- name: CompanyNextDue :many
-- As obrigações da empresa na competência que ainda vencem ou já venceram: aplicáveis e não
-- finais, a mais urgente primeiro (as atrasadas aparecem antes).
SELECT v.id, v.primary_module AS module, t.name AS type_name, v.status_label, v.due_date_effective,
       COALESCE(v.due_date_effective < sqlc.arg('today')::date, false)::boolean AS overdue
FROM v_obligation_instances v
JOIN obligation_types t ON t.id = v.obligation_type_id
WHERE v.company_id = sqlc.arg('company_id') AND v.competence_id = sqlc.arg('competence_id')
  AND v.applicable AND NOT v.is_final
ORDER BY v.due_date_effective NULLS LAST, v.type_sort_order, v.id
LIMIT sqlc.arg('row_limit');

-- name: CompanyOverdueCounts :many
-- As obrigações atrasadas de cada empresa na competência (o alerta crítico da listagem).
SELECT v.company_id, count(*)::int AS overdue
FROM v_obligation_instances v
WHERE v.competence_id = sqlc.arg('competence_id') AND v.applicable AND NOT v.is_final
  AND v.due_date_effective < sqlc.arg('today')::date
GROUP BY v.company_id;

-- name: GetCompanyCompetenceSnapshot :one
-- O snapshot da empresa na competência: o agrupamento do regime leva à aba certa da grade Fiscal.
SELECT s.regime_group, s.tax_regime
FROM company_competence_snapshots s
WHERE s.company_id = sqlc.arg('company_id') AND s.competence_id = sqlc.arg('competence_id');

-- name: GetCompanyRef :one
-- A identidade da empresa na ficha (o código leva às grades filtradas nela).
SELECT id, internal_code, legal_name FROM companies WHERE id = sqlc.arg('id');
