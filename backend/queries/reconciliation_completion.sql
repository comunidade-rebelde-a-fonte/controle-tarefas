-- Conclusão da Conciliação por empresa e competência (TASK-022, D-036; F-008) e o que ela precisa
-- da obrigação Conciliação do Contábil, sempre lida aqui e alterada pelo serviço de obrigações.

-- name: LockReconciliationCompanyShared :exec
-- A trava da empresa em modo compartilhado, na checagem da trava da competência: as gravações da
-- Conciliação esperam a conclusão (que pega a trava exclusiva) e não se esperam entre si.
SELECT pg_advisory_xact_lock_shared(hashtextextended('conciliacao:' || sqlc.arg('company_id')::text, 0));

-- name: GetActiveCompletion :one
-- A conclusão em vigor da empresa na competência (no máximo uma).
SELECT * FROM reconciliation_completions
WHERE company_id = $1 AND competence = $2 AND reopened_at IS NULL;

-- name: ListCompletions :many
-- As conclusões da competência, da mais recente para a mais antiga, com as reaberturas e os nomes.
SELECT r.id, r.company_id, r.competence, r.completed_by, cu.name AS completed_by_name, r.completed_at, r.obligation_instance_id,
       r.obligation_status_before, r.obligation_sync, r.reopened_by, ru.name AS reopened_by_name, r.reopened_at, r.reopen_reason,
       r.reopen_obligation_sync
FROM reconciliation_completions r
JOIN users cu ON cu.id = r.completed_by
LEFT JOIN users ru ON ru.id = r.reopened_by
WHERE r.company_id = $1 AND r.competence = $2
ORDER BY r.completed_at DESC, r.seq DESC;

-- name: InsertCompletion :one
INSERT INTO reconciliation_completions (company_id, competence, completed_by, completed_at, obligation_instance_id,
                                        obligation_status_before, obligation_sync)
VALUES (sqlc.arg('company_id'), sqlc.arg('competence'), sqlc.arg('completed_by'), sqlc.arg('now'), sqlc.narg('obligation_instance_id'),
        sqlc.narg('obligation_status_before'), sqlc.arg('obligation_sync'))
RETURNING *;

-- name: ReopenCompletion :execrows
UPDATE reconciliation_completions
SET reopened_by = sqlc.arg('reopened_by'), reopened_at = sqlc.arg('now'), reopen_reason = sqlc.arg('reason'),
    reopen_obligation_sync = sqlc.arg('obligation_sync')
WHERE id = sqlc.arg('id') AND reopened_at IS NULL;

-- name: FindObligationInstance :one
-- A instância canônica da obrigação do tipo na empresa e competência (05 §9), com o status em texto.
SELECT i.id, i.status_code, d.label AS status_label, d.is_final, i.applicable
FROM obligation_instances i
JOIN obligation_types t ON t.id = i.obligation_type_id
JOIN competencies c ON c.id = i.competence_id
JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code
WHERE i.company_id = sqlc.arg('company_id') AND c.first_day = sqlc.arg('competence') AND t.code = sqlc.arg('type_code');

-- name: GetLastStatusChange :one
-- A última mudança de status da instância, pela ordem de inserção: a reabertura só desfaz o OK
-- da integração se ninguém mudou o status depois.
SELECT h.change_source, h.new_value, h.created_at
FROM obligation_status_history h
WHERE h.obligation_instance_id = $1 AND h.field = 'STATUS'
ORDER BY h.seq DESC
LIMIT 1;

-- name: CountCompetenceStatements :one
-- Os extratos importados da empresa que cobrem a competência (o critério pede ao menos um).
SELECT count(*)::bigint FROM statement_imports i
WHERE i.company_id = sqlc.arg('company_id')
  AND i.period_start < (sqlc.arg('competence')::date + interval '1 month')
  AND i.period_end >= sqlc.arg('competence')::date;

-- name: GetReconciliationClosingGuard :one
-- A guarda do fechamento do Contábil (TASK-023, D-037; decisão do usuário): a empresa usa a
-- Conciliação na competência se tem extrato importado que a cobre ou documento dela. Usando, a
-- obrigação Conciliação precisa estar final (ou não se aplicar) para o Contábil fechar.
SELECT c.first_day,
       (EXISTS (SELECT 1 FROM statement_imports i
                WHERE i.company_id = sqlc.arg('company_id')
                  AND i.period_start < (c.first_day + interval '1 month') AND i.period_end >= c.first_day)
        OR EXISTS (SELECT 1 FROM reconciliation_documents d
                   WHERE d.company_id = sqlc.arg('company_id')
                     AND (d.competence = c.first_day
                          OR (d.period_start < (c.first_day + interval '1 month')
                              AND COALESCE(d.period_end, d.period_start) >= c.first_day))))::boolean AS uses_reconciliation
FROM competencies c
WHERE c.id = sqlc.arg('competence_id');

-- name: FindModulePeriod :one
-- O período do módulo da empresa na competência (o fechamento visto pela Conciliação, 06 §5).
SELECT p.id, p.competence_id
FROM company_module_periods p
JOIN competencies c ON c.id = p.competence_id
WHERE p.company_id = sqlc.arg('company_id') AND c.first_day = sqlc.arg('competence') AND p.module_code = sqlc.arg('module_code');
