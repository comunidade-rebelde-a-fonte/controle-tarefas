-- Motor de geração de competência [PRD §10.1-§10.2, §39; plan M2].

-- name: LockCompetence :exec
-- Serializa gerações concorrentes da mesma competência (boot + agendador + acesso).
SELECT pg_advisory_xact_lock(hashtextextended('competence:' || sqlc.arg('competence_key')::text, 0));

-- name: InsertCompetence :exec
INSERT INTO competencies (competence_key, year, month, first_day)
VALUES (sqlc.arg('competence_key'), extract(year FROM sqlc.arg('first_day')::date)::int,
        extract(month FROM sqlc.arg('first_day')::date)::int, sqlc.arg('first_day')::date)
ON CONFLICT (competence_key) DO NOTHING;

-- name: GetCompetenceByKey :one
SELECT * FROM competencies WHERE competence_key = $1;

-- name: EligibleCompaniesWithoutSnapshot :many
-- Empresas ativas, dentro do período de atendimento e ainda sem snapshot na competência
-- (R5: empresas já geradas nunca são reprocessadas) [RN-005, CA-005].
SELECT c.id, c.payroll_type, c.factor_r, c.city, c.state,
       c.fiscal_responsible_user_id, c.accounting_responsible_user_id,
       c.payroll_responsible_user_id, c.declarations_responsible_user_id
FROM companies c
WHERE c.active
  AND c.service_start_date <= sqlc.arg('last_day')::date
  AND (c.service_end_date IS NULL OR c.service_end_date >= sqlc.arg('first_day')::date)
  AND NOT EXISTS (
      SELECT 1 FROM company_competence_snapshots s
      WHERE s.company_id = c.id AND s.competence_id = sqlc.arg('competence_id')::uuid
  )
ORDER BY c.internal_code;

-- name: RegimesAtCompetence :many
SELECT company_id, tax_regime FROM company_tax_regime_history
WHERE effective_from_competence <= sqlc.arg('competence')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('competence')::date);

-- name: ModulesAtCompetence :many
SELECT company_id, module_code FROM company_module_settings
WHERE enabled
  AND effective_from_competence <= sqlc.arg('competence')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('competence')::date)
ORDER BY company_id, module_code;

-- name: OverridesAtCompetence :many
SELECT company_id, obligation_type_id, applicability FROM company_obligation_overrides
WHERE effective_from_competence <= sqlc.arg('competence')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('competence')::date);

-- name: EngineObligationTypes :many
SELECT id, code, primary_module, active, sort_order, initial_status_code, status_when_no_payroll,
       due_date_rule_id, status_set_id
FROM obligation_types
ORDER BY primary_module, sort_order, code;

-- name: EngineStatusCodes :many
SELECT status_set_id, code FROM status_definitions ORDER BY status_set_id, sort_order;

-- name: EngineRegimeRules :many
SELECT obligation_type_id, tax_regime, default_applicable, counts_in_progress, active
FROM obligation_type_regime_rules;

-- name: EngineCalendars :many
SELECT id, scope, state, city FROM business_calendars;

-- name: EngineHolidaysInRange :many
SELECT calendar_id, date FROM holidays
WHERE date BETWEEN sqlc.arg('from_date')::date AND sqlc.arg('to_date')::date;

-- name: LatestCompetence :one
SELECT competence_key FROM competencies ORDER BY first_day DESC LIMIT 1;

-- name: CompetenceExists :one
SELECT EXISTS (SELECT 1 FROM competencies WHERE competence_key = $1);

-- name: ListCompetenciesWithCounts :many
-- Competências geradas com totais para navegação e histórico.
SELECT k.competence_key, k.created_at,
       (SELECT count(*) FROM company_competence_snapshots s WHERE s.competence_id = k.id) AS companies,
       (SELECT count(*) FROM obligation_instances i WHERE i.competence_id = k.id) AS instances,
       (SELECT count(*) FROM company_module_periods p WHERE p.competence_id = k.id AND p.state = 'OPEN') AS open_periods,
       (SELECT count(*) FROM company_module_periods p WHERE p.competence_id = k.id AND p.state = 'CLOSED') AS closed_periods
FROM competencies k
ORDER BY k.first_day DESC;
