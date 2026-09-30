-- name: CompanyHistoryPeriods :many
-- Matriz competência × módulo da empresa: estado do período, regime do snapshot e o agregado
-- de conclusão do módulo [PRD §27.3 aba 8].
SELECT k.competence_key, s.tax_regime, p.id AS period_id, p.module_code, p.state, p.closed_at,
       count(v.id) FILTER (WHERE v.in_denominator)::int                AS denominator,
       count(v.id) FILTER (WHERE v.in_denominator AND v.is_final)::int AS final_done,
       count(v.id) FILTER (WHERE v.in_denominator AND v.counts_as_operational_progress)::int AS operational_done,
       count(v.id) FILTER (WHERE v.in_denominator AND v.is_blocking)::int AS blocking
FROM company_module_periods p
JOIN competencies k ON k.id = p.competence_id
JOIN company_competence_snapshots s ON s.company_id = p.company_id AND s.competence_id = p.competence_id
LEFT JOIN v_obligation_instances v
       ON v.company_id = p.company_id AND v.competence_id = p.competence_id AND v.primary_module = p.module_code
WHERE p.company_id = $1
GROUP BY k.competence_key, s.tax_regime, p.id, p.module_code, p.state, p.closed_at
ORDER BY k.competence_key DESC, p.module_code;

-- name: CompanyHistoryEvents :many
-- Eventos de abertura, fechamento e reabertura dos períodos da empresa, em ordem.
SELECT e.company_module_period_id, e.event_type, e.reason, e.created_at, u.name AS user_name
FROM company_module_period_events e
JOIN company_module_periods p ON p.id = e.company_module_period_id
JOIN users u ON u.id = e.user_id
WHERE p.company_id = $1
ORDER BY e.seq;
