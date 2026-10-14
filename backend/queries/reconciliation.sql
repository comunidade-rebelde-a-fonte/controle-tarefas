-- Conciliação [05 §5, TASK-007].

-- name: GetAccountingPeriodState :one
-- Estado do fechamento do módulo Contábil da empresa na competência: a única trava da
-- Conciliação [REQ-033, 05 §6]. Sem período gerado, não há trava.
SELECT p.state
FROM company_module_periods p
JOIN competencies c ON c.id = p.competence_id
WHERE p.company_id = sqlc.arg('company_id') AND p.module_code = 'ACCOUNTING' AND c.first_day = sqlc.arg('competence')::date;
