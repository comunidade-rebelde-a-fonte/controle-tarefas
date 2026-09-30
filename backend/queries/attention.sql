-- "Exige atenção" [PRD §24.6]: contagem e amostra (5 primeiros) por categoria.

-- name: AttentionObligations :many
-- Atrasadas, pendências, documentos faltantes e erros na competência, com os filtros do
-- dashboard. Amostra: vencimento mais antigo primeiro.
SELECT y.category::text AS category, y.total::int AS total, y.instance_id, y.company_id, y.internal_code, y.legal_name,
       y.module, y.type_name, y.status_label, y.due_date_effective
FROM (
    SELECT x.*,
           row_number() OVER (PARTITION BY x.category ORDER BY x.due_date_effective NULLS LAST, x.internal_code, x.type_sort_order) AS rn,
           count(*) OVER (PARTITION BY x.category) AS total
    FROM (
        SELECT cat AS category, v.id AS instance_id, c.id AS company_id, c.internal_code, c.legal_name,
               v.primary_module AS module, t.name AS type_name, v.status_label, v.due_date_effective, v.type_sort_order
        FROM v_obligation_instances v
        JOIN obligation_types t ON t.id = v.obligation_type_id
        JOIN companies c ON c.id = v.company_id
        JOIN company_competence_snapshots s ON s.company_id = v.company_id AND s.competence_id = v.competence_id
        CROSS JOIN LATERAL unnest(ARRAY[
            CASE WHEN v.applicable AND NOT v.is_final AND v.due_date_effective < sqlc.arg('today')::date THEN 'OVERDUE' END,
            CASE WHEN v.applicable AND v.is_blocking THEN 'PENDING' END,
            CASE WHEN v.applicable AND v.status_code = 'MISSING_DOCUMENTS' THEN 'MISSING_DOCUMENTS' END,
            CASE WHEN v.applicable AND v.status_code = 'ERROR' THEN 'ERROR' END
        ]::text[]) AS cat
        WHERE v.competence_id = sqlc.arg('competence_id') AND cat IS NOT NULL
          AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
          AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
          AND (sqlc.narg('responsible_id')::uuid IS NULL OR sqlc.narg('responsible_id')::uuid = CASE v.primary_module
                WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
                WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END)
    ) x
) y
WHERE y.rn <= 5
ORDER BY y.category, y.rn;

-- name: AttentionPreviousOpen :many
-- Empresas com módulo aberto em competência anterior (plan R8): a mais antiga primeiro.
SELECT y.total::int AS total, y.company_id, y.internal_code, y.legal_name, y.oldest_key::text AS oldest_key, y.open_periods::int AS open_periods
FROM (
    SELECT x.*, row_number() OVER (ORDER BY x.oldest_key, x.internal_code) AS rn, count(*) OVER () AS total
    FROM (
        SELECT c.id AS company_id, c.internal_code, c.legal_name, min(k.competence_key) AS oldest_key, count(*) AS open_periods
        FROM company_module_periods p
        JOIN competencies k ON k.id = p.competence_id
        JOIN companies c ON c.id = p.company_id
        JOIN company_competence_snapshots s ON s.company_id = p.company_id AND s.competence_id = p.competence_id
        WHERE k.competence_key < sqlc.arg('competence_key') AND p.state = 'OPEN'
          AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
          AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
          AND (sqlc.narg('responsible_id')::uuid IS NULL OR sqlc.narg('responsible_id')::uuid = CASE p.module_code
                WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
                WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END)
        GROUP BY c.id, c.internal_code, c.legal_name
    ) x
) y
WHERE y.rn <= 5
ORDER BY y.rn;

-- name: AttentionBlockedDependencies :many
-- Dependências bloqueando, por empresa e modo [PRD §24.6]:
--   SOFT (DCTFWeb): obrigação aplicável não final com componente aplicável não final;
--   HARD (Contábil): etapa aplicável não final atrás de uma etapa anterior Pendente.
SELECT y.mode, y.total::int AS total, y.company_id, y.internal_code, y.legal_name, y.dependent_name::text AS dependent_name, y.pending_names::text AS pending_names
FROM (
    SELECT x.*, row_number() OVER (PARTITION BY x.mode ORDER BY x.internal_code) AS rn, count(*) OVER (PARTITION BY x.mode) AS total
    FROM (
        SELECT dep.dependency_mode AS mode, c.id AS company_id, c.internal_code, c.legal_name,
               min(dt.name) AS dependent_name,
               string_agg(DISTINCT pt.name, ', ') AS pending_names
        FROM obligation_dependencies dep
        JOIN v_obligation_instances d ON d.obligation_type_id = dep.obligation_type_id
        JOIN obligation_types dt ON dt.id = d.obligation_type_id
        JOIN v_obligation_instances p
          ON p.company_id = d.company_id AND p.competence_id = d.competence_id AND p.obligation_type_id = dep.prerequisite_obligation_type_id
        JOIN obligation_types pt ON pt.id = p.obligation_type_id
        JOIN companies c ON c.id = d.company_id
        JOIN company_competence_snapshots s ON s.company_id = d.company_id AND s.competence_id = d.competence_id
        WHERE dep.active AND d.competence_id = sqlc.arg('competence_id')
          AND d.applicable AND NOT d.is_final AND p.applicable AND NOT p.is_final
          AND (dep.dependency_mode = 'SOFT' OR p.is_blocking)
          AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
          AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
          AND (sqlc.narg('responsible_id')::uuid IS NULL OR sqlc.narg('responsible_id')::uuid = CASE d.primary_module
                WHEN 'FISCAL' THEN s.fiscal_responsible_user_id WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
                WHEN 'ACCOUNTING' THEN s.accounting_responsible_user_id WHEN 'PAYROLL' THEN s.payroll_responsible_user_id END)
        GROUP BY dep.dependency_mode, c.id, c.internal_code, c.legal_name
    ) x
) y
WHERE y.rn <= 5
ORDER BY y.mode, y.rn;
