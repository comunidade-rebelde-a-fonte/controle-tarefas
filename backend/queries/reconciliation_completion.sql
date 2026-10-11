-- Conclusão da Conciliação por empresa e competência (TASK-022, D-035; F-008) e o que ela precisa
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
