-- Resolvedor de exceções (TASK-019, D-030): a linha e o contexto que o agente recebe, a proposta e o
-- aceite humano, que é o único caminho até a conciliação [AC-019].

-- name: GetStatementLine :one
-- A linha do extrato, com a competência e se está conciliada.
SELECT t.id, t.company_id, t.txn_date, t.competence, t.amount, t.direction, t.description, t.flags,
       EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL) AS reconciled
FROM statement_transactions t
WHERE t.id = $1;

-- name: ListStatementLinesNear :many
-- Linhas da empresa no período, com o sinal de conciliada: as irmãs da linha.
SELECT t.id, t.txn_date, t.amount, t.direction, t.description,
       EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL) AS reconciled
FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.txn_date BETWEEN sqlc.arg('date_from') AND sqlc.arg('date_to')
ORDER BY t.txn_date, t.statement_import_id, t.position, t.id;

-- name: InsertResolverProposal :one
-- Só a proposta: a hipótese, a justificativa e os pares, pendentes da decisão humana.
INSERT INTO resolver_proposals (company_id, statement_transaction_id, payload, trace_id, created_by, created_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('statement_transaction_id'), sqlc.arg('payload'), sqlc.arg('trace_id'),
        sqlc.narg('created_by'), sqlc.arg('now'))
RETURNING id;

-- name: GetResolverProposalForUpdate :one
SELECT * FROM resolver_proposals WHERE id = $1 FOR UPDATE;

-- name: ListResolverProposalsByLine :many
SELECT * FROM resolver_proposals WHERE statement_transaction_id = $1 ORDER BY created_at DESC, id DESC;

-- name: DecideResolverProposal :exec
UPDATE resolver_proposals SET status = sqlc.arg('status'), decided_by = sqlc.arg('decided_by'), decided_at = sqlc.arg('now')
WHERE id = sqlc.arg('id') AND status = 'pendente';

-- name: ListFreeStatementLinesByIDs :many
-- Das linhas citadas, as da empresa sem conciliação ativa (o aceite ignora as outras).
SELECT t.id, t.txn_date FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.id = ANY(sqlc.arg('ids')::uuid[])
  AND NOT EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL);

-- name: ListFreeClientEntriesByIDs :many
-- Dos lançamentos citados, os da empresa sem conciliação ativa.
SELECT e.id FROM client_entries e
WHERE e.company_id = sqlc.arg('company_id') AND e.id = ANY(sqlc.arg('ids')::uuid[])
  AND NOT EXISTS (
      SELECT 1 FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
      WHERE i.client_entry_id = e.id AND i.released_at IS NULL AND r.undone_at IS NULL
  );

-- name: InsertManualReconciliation :one
-- Conciliação do aceite humano: origem manual, com o autor e a proposta [AC-019].
INSERT INTO reconciliations (company_id, statement_transaction_id, method, origin, proposal_id, created_by, created_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('statement_transaction_id'), sqlc.arg('method'), 'manual', sqlc.arg('proposal_id'),
        sqlc.arg('created_by'), sqlc.arg('now'))
RETURNING id;

-- name: ListFreeLinesWithoutProposal :many
-- Linhas livres da competência que nunca tiveram proposta nem pedido ao resolvedor: o automático
-- pede uma só vez por linha (a rejeitada e a de job com falha não voltam sozinhas; o contador pede
-- de novo na fila).
SELECT t.id FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.competence = sqlc.arg('competence')
  AND NOT EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
  AND NOT EXISTS (SELECT 1 FROM resolver_proposals p WHERE p.statement_transaction_id = t.id)
  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.kind = sqlc.arg('job_kind')::text AND j.dedupe_key = t.id::text)
ORDER BY t.txn_date, t.statement_import_id, t.position, t.id;
