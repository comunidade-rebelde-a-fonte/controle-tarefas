-- Rodada do matching da Conciliação (TASK-018, D-029, ADR-008): as linhas livres da competência, os
-- vizinhos para as bordas, os lançamentos da empresa, a gravação das conciliações automáticas e as
-- marcas da fila.

-- name: LockReconciliationCompany :exec
-- Uma rodada por empresa de cada vez (o lançamento pode servir a linhas de competências vizinhas).
SELECT pg_advisory_xact_lock(hashtextextended('conciliacao:' || sqlc.arg('company_id')::text, 0));

-- name: ListFreeStatementTransactions :many
-- Linhas da competência sem conciliação ativa, na ordem do extrato.
SELECT t.id, t.txn_date, t.amount, t.direction, t.description, t.flags
FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.competence = sqlc.arg('competence')
  AND NOT EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
ORDER BY t.txn_date, t.statement_import_id, t.position, t.id;

-- name: ListStatementTransactionsBetween :many
-- Linhas da empresa no período, conciliadas ou não: o par de um estorno ou de uma transferência.
SELECT t.id, t.txn_date, t.amount, t.direction, t.description
FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.txn_date BETWEEN sqlc.arg('date_from') AND sqlc.arg('date_to')
ORDER BY t.txn_date, t.statement_import_id, t.position, t.id;

-- name: ListClientEntriesForMatching :many
-- Todos os lançamentos da empresa, com o sinal de livre (sem conciliação ativa).
SELECT e.id, e.entry_date, e.amount, e.direction, e.counterparty, e.document_ref, e.status, e.flags,
       NOT EXISTS (
           SELECT 1 FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
           WHERE i.client_entry_id = e.id AND i.released_at IS NULL AND r.undone_at IS NULL
       ) AS free
FROM client_entries e
WHERE e.company_id = sqlc.arg('company_id')
ORDER BY e.entry_date NULLS LAST, e.created_at, e.id;

-- name: InsertAutoReconciliation :one
-- Conciliação automática: só os passes determinísticos, sem autor [AC-018].
INSERT INTO reconciliations (company_id, statement_transaction_id, score, pass, method, origin, created_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('statement_transaction_id'), sqlc.arg('score'), sqlc.arg('pass'),
        sqlc.arg('method'), 'auto', sqlc.arg('now'))
RETURNING id;

-- name: InsertReconciliationItem :exec
INSERT INTO reconciliation_items (reconciliation_id, company_id, client_entry_id)
VALUES (sqlc.arg('reconciliation_id'), sqlc.arg('company_id'), sqlc.arg('client_entry_id'));

-- name: SetStatementTransactionFlags :exec
UPDATE statement_transactions SET flags = sqlc.arg('flags') WHERE id = sqlc.arg('id');

-- name: CountActiveReconciliationsByCompetence :one
-- Linhas da competência conciliadas (balde "conciliado").
SELECT count(*) FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.competence = sqlc.arg('competence')
  AND EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL);
