-- Dashboard [PRD §24]: agregados da competência com filtros de regime, agrupamento e
-- responsável (o do módulo da obrigação, no snapshot da competência).

-- name: DashboardModuleCounts :many
-- Contagens por módulo principal. As quatro classes (concluídas, em andamento, não iniciadas e
-- pendentes) particionam as obrigações que contam no progresso; atraso segue a grade
-- (aplicável, não final, vencimento efetivo < hoje).
SELECT v.primary_module AS module,
       count(*) FILTER (WHERE v.in_denominator)::int                                                              AS applicable,
       count(*) FILTER (WHERE v.in_denominator AND v.is_final)::int                                               AS done,
       count(*) FILTER (WHERE v.in_denominator AND v.counts_as_operational_progress)::int                         AS operational_done,
       count(*) FILTER (WHERE v.in_denominator AND v.counts_as_started AND NOT v.is_final AND NOT v.is_blocking)::int AS in_progress,
       count(*) FILTER (WHERE v.in_denominator AND NOT v.counts_as_started AND NOT v.is_final AND NOT v.is_blocking)::int AS not_started,
       count(*) FILTER (WHERE v.in_denominator AND v.is_blocking)::int                                            AS pending,
       count(*) FILTER (WHERE v.applicable AND NOT v.is_final AND v.due_date_effective < sqlc.arg('today')::date)::int AS overdue
FROM v_obligation_instances v
JOIN company_competence_snapshots s ON s.company_id = v.company_id AND s.competence_id = v.competence_id
WHERE v.competence_id = sqlc.arg('competence_id')
  AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
  AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
  AND (sqlc.narg('responsible_id')::uuid IS NULL OR sqlc.narg('responsible_id')::uuid = CASE v.primary_module
        WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
        WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END)
GROUP BY v.primary_module;

-- name: DashboardCompanyModules :many
-- Empresa × módulo: estado do período e agregado do fechamento (pronto para fechar, plan R2).
SELECT p.company_id, p.module_code, p.state,
       count(v.id) FILTER (WHERE v.in_denominator)::int              AS denominator,
       count(v.id) FILTER (WHERE v.in_denominator AND v.is_final)::int AS final_done
FROM company_module_periods p
JOIN company_competence_snapshots s ON s.company_id = p.company_id AND s.competence_id = p.competence_id
LEFT JOIN v_obligation_instances v
       ON v.company_id = p.company_id AND v.competence_id = p.competence_id AND v.primary_module = p.module_code
WHERE p.competence_id = sqlc.arg('competence_id')
  AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
  AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
  AND (sqlc.narg('responsible_id')::uuid IS NULL OR sqlc.narg('responsible_id')::uuid = CASE p.module_code
        WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
        WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END)
GROUP BY p.company_id, p.module_code, p.state;

-- name: DashboardPreviousOpenCompanies :one
-- Empresas com algum módulo ainda aberto em competências anteriores à selecionada (plan R8).
SELECT count(DISTINCT p.company_id)::int
FROM company_module_periods p
JOIN competencies k ON k.id = p.competence_id
JOIN company_competence_snapshots s ON s.company_id = p.company_id AND s.competence_id = p.competence_id
WHERE k.competence_key < sqlc.arg('competence_key') AND p.state = 'OPEN'
  AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
  AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
  AND (sqlc.narg('responsible_id')::uuid IS NULL OR sqlc.narg('responsible_id')::uuid = CASE p.module_code
        WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
        WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END);

-- name: DashboardResponsibles :many
-- Progresso por responsável e módulo (responsável do snapshot) [PRD §24.5].
SELECT u.id AS user_id, u.name AS user_name, v.primary_module AS module,
       count(*) FILTER (WHERE v.in_denominator)::int                                      AS applicable,
       count(*) FILTER (WHERE v.in_denominator AND v.is_final)::int                       AS done,
       count(*) FILTER (WHERE v.in_denominator AND v.counts_as_operational_progress)::int AS operational_done,
       count(*) FILTER (WHERE v.in_denominator AND v.is_blocking)::int                    AS pending,
       count(*) FILTER (WHERE v.applicable AND NOT v.is_final AND v.due_date_effective < sqlc.arg('today')::date)::int AS overdue
FROM v_obligation_instances v
JOIN company_competence_snapshots s ON s.company_id = v.company_id AND s.competence_id = v.competence_id
LEFT JOIN users u ON u.id = CASE v.primary_module
        WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
        WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END
WHERE v.competence_id = sqlc.arg('competence_id')
  AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
  AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
  AND (sqlc.narg('responsible_id')::uuid IS NULL OR u.id = sqlc.narg('responsible_id')::uuid)
GROUP BY u.id, u.name, v.primary_module
ORDER BY u.name NULLS LAST, v.primary_module;
