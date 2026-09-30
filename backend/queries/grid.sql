-- Grades dos módulos [PRD §26, §41.6]: no máximo 3 consultas por grade (colunas, linhas, células).

-- name: GridColumns :many
-- Colunas: tipos exibidos no módulo (catálogo, sort_order) com regra ativa para os regimes da
-- grade, ou com instâncias já geradas na competência para esses regimes (competências antigas
-- não perdem colunas). Traz os status do conjunto, todas as dependências ativas e o id da
-- competência (NULL se ainda não gerada), para a grade caber em 3 consultas.
SELECT k.id AS competence_id, t.id, t.code, t.name, t.primary_module, dc.sort_order, t.is_closing_stage,
       t.allows_transmission_date, t.allows_protocol, ss.code AS status_set_code,
       (SELECT jsonb_agg(jsonb_build_object(
                   'code', d.code, 'label', d.label, 'sort_order', d.sort_order,
                   'counts_as_started', d.counts_as_started,
                   'counts_as_operational_progress', d.counts_as_operational_progress,
                   'is_final', d.is_final, 'is_blocking', d.is_blocking,
                   'excluded_from_denominator', d.excluded_from_denominator) ORDER BY d.sort_order)
        FROM status_definitions d WHERE d.status_set_id = t.status_set_id)::jsonb AS statuses,
       (SELECT coalesce(jsonb_agg(jsonb_build_object(
                   'type_id', x.obligation_type_id, 'prereq_id', x.prerequisite_obligation_type_id,
                   'mode', x.dependency_mode)), '[]'::jsonb)
        FROM obligation_dependencies x WHERE x.active)::jsonb AS dependencies
FROM obligation_display_contexts dc
JOIN obligation_types t ON t.id = dc.obligation_type_id
JOIN status_sets ss ON ss.id = t.status_set_id
LEFT JOIN competencies k ON k.competence_key = sqlc.arg('competence_key')
WHERE dc.module_code = sqlc.arg('module') AND dc.active
  AND (
        (t.active AND EXISTS (
            SELECT 1 FROM obligation_type_regime_rules r
            WHERE r.obligation_type_id = t.id AND r.active AND r.tax_regime = ANY (sqlc.arg('regimes')::text[])))
     OR EXISTS (
            SELECT 1 FROM obligation_instances i
            JOIN company_competence_snapshots s ON s.company_id = i.company_id AND s.competence_id = i.competence_id
            WHERE i.competence_id = k.id AND i.obligation_type_id = t.id
              AND s.tax_regime = ANY (sqlc.arg('regimes')::text[]))
  )
ORDER BY dc.sort_order, t.code;

-- name: GridRows :many
-- Linhas: empresas com o módulo na competência (período gerado), com o snapshot usado na
-- geração (regime, folha, responsável do módulo), o período e a observação geral.
SELECT c.id AS company_id, c.internal_code, c.legal_name, c.trade_name, c.cnpj,
       s.tax_regime, s.regime_group, s.payroll_type, s.factor_r,
       ru.id AS responsible_id, ru.name AS responsible_name,
       p.id AS period_id, p.state AS period_state, p.closed_at, cu.id AS closed_by_id, cu.name AS closed_by_name,
       p.general_note, p.version AS period_version
FROM company_competence_snapshots s
JOIN companies c ON c.id = s.company_id
JOIN company_module_periods p
  ON p.company_id = s.company_id AND p.competence_id = s.competence_id AND p.module_code = sqlc.arg('period_module')
LEFT JOIN users ru ON ru.id = CASE sqlc.arg('period_module')::text
        WHEN 'FISCAL'       THEN s.fiscal_responsible_user_id
        WHEN 'DECLARATIONS' THEN s.declarations_responsible_user_id
        WHEN 'ACCOUNTING'   THEN s.accounting_responsible_user_id
        WHEN 'PAYROLL'      THEN s.payroll_responsible_user_id
    END
LEFT JOIN users cu ON cu.id = p.closed_by
WHERE s.competence_id = sqlc.arg('competence_id')
  AND (sqlc.narg('regime_group')::text IS NULL OR s.regime_group = sqlc.narg('regime_group')::text)
  AND (sqlc.narg('tax_regime')::text IS NULL OR s.tax_regime = sqlc.narg('tax_regime')::text)
  AND (sqlc.narg('responsible_id')::uuid IS NULL OR ru.id = sqlc.narg('responsible_id')::uuid)
  AND (sqlc.narg('period_state')::text IS NULL OR p.state = sqlc.narg('period_state')::text)
  AND (sqlc.narg('has_note')::boolean IS NULL OR (p.general_note IS NOT NULL) = sqlc.narg('has_note')::boolean)
ORDER BY c.legal_name, c.internal_code;

-- name: GridCells :many
-- Células: instâncias das empresas da grade para os tipos das colunas, seus pré-requisitos e
-- todas as obrigações do módulo da linha (agregado de progresso, como no fechamento, plan R1).
-- period_state é o do módulo principal da instância (na visão DCTFWeb, cada célula tem o seu).
SELECT v.id, v.company_id, v.obligation_type_id, t.code AS type_code, t.name AS type_name, t.primary_module,
       v.type_sort_order, v.status_code, v.status_label, v.applicable, v.counts_in_progress, v.is_final,
       v.counts_as_started, v.counts_as_operational_progress, v.is_blocking, v.excluded_from_denominator,
       v.due_date_original, v.due_date_effective, v.due_date_override_reason, v.transmission_date, v.protocol,
       v.version, v.last_changed_at, u.name AS last_changed_by_name,
       COALESCE(p.state, 'OPEN')::text AS period_state,
       COALESCE(hh.has, false)::boolean AS has_history
FROM v_obligation_instances v
JOIN obligation_types t ON t.id = v.obligation_type_id
LEFT JOIN users u ON u.id = v.last_changed_by
LEFT JOIN company_module_periods p
       ON p.company_id = v.company_id AND p.competence_id = v.competence_id AND p.module_code = t.primary_module
-- Indicador de histórico por célula via índice (um EXISTS na lista de colunas vira uma
-- "hashed SubPlan" que lê o histórico inteiro) [T-M7-02].
LEFT JOIN LATERAL (
    SELECT true AS has FROM obligation_status_history h WHERE h.obligation_instance_id = v.id LIMIT 1
) hh ON true
WHERE v.competence_id = sqlc.arg('competence_id')
  AND v.company_id = ANY (sqlc.arg('company_ids')::uuid[])
  AND (v.obligation_type_id = ANY (sqlc.arg('type_ids')::uuid[]) OR t.primary_module = sqlc.arg('period_module'));
