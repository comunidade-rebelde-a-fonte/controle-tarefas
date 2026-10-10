-- Classificação contábil e razão da Conciliação (TASK-020, D-031): plano de contas e regras por
-- empresa, a classificação da linha conciliada (regra → histórico → agente), a sugestão do agente
-- e o razão como consulta sobre as conciliações ativas [REQ-026 a REQ-028].

-- name: ListChartAccounts :many
SELECT * FROM chart_accounts
WHERE company_id = sqlc.arg('company_id') AND (NOT sqlc.arg('only_active')::boolean OR active)
ORDER BY code;

-- name: GetChartAccount :one
SELECT * FROM chart_accounts WHERE id = sqlc.arg('id') AND company_id = sqlc.arg('company_id');

-- name: InsertChartAccount :one
INSERT INTO chart_accounts (company_id, code, name, created_at, updated_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('now'), sqlc.arg('now'))
RETURNING *;

-- name: UpdateChartAccount :one
UPDATE chart_accounts SET name = sqlc.arg('name'), active = sqlc.arg('active'), updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id') AND company_id = sqlc.arg('company_id')
RETURNING *;

-- name: UpsertChartAccount :one
-- Importação do plano: conta nova entra; a que já existe ganha o nome do arquivo e volta a ativa.
INSERT INTO chart_accounts (company_id, code, name, created_at, updated_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('code'), sqlc.arg('name'), sqlc.arg('now'), sqlc.arg('now'))
ON CONFLICT (company_id, code) DO UPDATE SET name = EXCLUDED.name, active = true, updated_at = EXCLUDED.updated_at
RETURNING (xmax = 0) AS inserted;

-- name: CopyChartAccounts :many
-- Copiar de um padrão (D-020): as contas ativas da outra empresa que esta ainda não tem.
INSERT INTO chart_accounts (company_id, code, name, created_at, updated_at)
SELECT sqlc.arg('company_id'), s.code, s.name, sqlc.arg('now'), sqlc.arg('now')
FROM chart_accounts s
WHERE s.company_id = sqlc.arg('source_company_id') AND s.active
ORDER BY s.code
ON CONFLICT (company_id, code) DO NOTHING
RETURNING id;

-- name: CountActiveChartAccounts :one
SELECT count(*) FROM chart_accounts WHERE company_id = sqlc.arg('company_id') AND active;

-- name: ListActiveAccountCodes :many
SELECT code FROM chart_accounts WHERE company_id = sqlc.arg('company_id') AND active ORDER BY code;

-- name: ListActiveChartForAgent :many
SELECT code, name FROM chart_accounts WHERE company_id = sqlc.arg('company_id') AND active ORDER BY code;

-- name: ListClassificationRules :many
-- Da mais nova para a mais antiga: a ordem em que a classificação as testa (a mais nova vale).
SELECT * FROM classification_rules
WHERE company_id = sqlc.arg('company_id') AND (NOT sqlc.arg('only_active')::boolean OR active)
ORDER BY created_at DESC, id DESC;

-- name: ListActiveRulesOfType :many
SELECT * FROM classification_rules
WHERE company_id = sqlc.arg('company_id') AND rule_type = sqlc.arg('rule_type') AND active
ORDER BY created_at DESC, id DESC;

-- name: GetClassificationRule :one
SELECT * FROM classification_rules WHERE id = sqlc.arg('id') AND company_id = sqlc.arg('company_id');

-- name: InsertClassificationRule :one
INSERT INTO classification_rules (company_id, rule_type, pattern, account_code, source_statement_transaction_id, created_by,
                                  created_at, updated_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('rule_type'), sqlc.arg('pattern'), sqlc.arg('account_code'),
        sqlc.narg('source_statement_transaction_id'), sqlc.arg('created_by'), sqlc.arg('now'), sqlc.arg('now'))
RETURNING *;

-- name: DeactivateClassificationRule :one
UPDATE classification_rules SET active = false, deactivated_by = sqlc.arg('deactivated_by'), deactivated_at = sqlc.arg('now'),
                                updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id') AND company_id = sqlc.arg('company_id') AND active
RETURNING *;

-- name: ListClassificationLines :many
-- Linhas da empresa com conciliação ativa, com a classificação e os comprovantes conciliados
-- (contraparte e documento; vazio é nulo): a entrada da classificação e do histórico. Só as linhas
-- da competência dada (date_from..date_to) ou, para o histórico, as confirmadas antes dela.
SELECT t.id, t.txn_date, t.position, t.description, t.account_code, t.classification_origin, t.classification_pending,
       COALESCE(array_agg(COALESCE(e.counterparty, '') ORDER BY e.id) FILTER (WHERE e.id IS NOT NULL), '{}')::text[] AS counterparties,
       COALESCE(array_agg(COALESCE(e.counterparty_doc, '') ORDER BY e.id) FILTER (WHERE e.id IS NOT NULL), '{}')::text[] AS docs
FROM statement_transactions t
JOIN reconciliations r ON r.statement_transaction_id = t.id AND r.undone_at IS NULL
LEFT JOIN reconciliation_items i ON i.reconciliation_id = r.id AND i.released_at IS NULL
LEFT JOIN client_entries e ON e.id = i.client_entry_id
WHERE t.company_id = sqlc.arg('company_id') AND t.txn_date BETWEEN sqlc.arg('date_from') AND sqlc.arg('date_to')
  AND (NOT sqlc.arg('only_confirmed')::boolean OR (t.account_code IS NOT NULL AND NOT t.classification_pending))
  AND (NOT sqlc.arg('only_unconfirmed')::boolean OR t.classification_origin IS NULL OR t.classification_pending)
  AND (sqlc.narg('line_id')::uuid IS NULL OR t.id = sqlc.narg('line_id')::uuid)
GROUP BY t.id
ORDER BY t.txn_date DESC, t.position DESC, t.id DESC;

-- name: SetAutoLineClassification :execrows
-- A classificação automática (regra, histórico ou à espera do agente): só na linha ainda sem
-- classificação confirmada, então nunca troca a do contador, mesmo em corrida.
UPDATE statement_transactions
SET account_code = sqlc.narg('account_code'), classification_origin = sqlc.narg('origin'), classification_pending = sqlc.arg('pending'),
    classification_rule_id = sqlc.narg('rule_id'), classification_history_id = sqlc.narg('history_id'),
    classification_suggestion_id = NULL, classified_by = NULL, classified_at = sqlc.narg('classified_at')
WHERE id = sqlc.arg('id') AND company_id = sqlc.arg('company_id')
  AND (classification_origin IS NULL OR classification_pending);

-- name: SetLineClassification :exec
-- A classificação humana (correção ou sugestão aceita) e o que a produziu (o CHECK da 0029
-- confere a coerência).
UPDATE statement_transactions
SET account_code = sqlc.narg('account_code'), classification_origin = sqlc.narg('origin'), classification_pending = sqlc.arg('pending'),
    classification_rule_id = sqlc.narg('rule_id'), classification_history_id = sqlc.narg('history_id'),
    classification_suggestion_id = sqlc.narg('suggestion_id'), classified_by = sqlc.narg('classified_by'),
    classified_at = sqlc.narg('classified_at')
WHERE id = sqlc.arg('id') AND company_id = sqlc.arg('company_id');

-- name: GetLineClassification :one
-- A linha, se está conciliada e a classificação atual.
SELECT t.id, t.company_id, t.txn_date, t.description, t.amount, t.direction, t.account_code, t.classification_origin,
       t.classification_pending, t.classification_rule_id, t.classification_history_id, t.classification_suggestion_id,
       t.classified_by, t.classified_at,
       EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL) AS reconciled
FROM statement_transactions t
WHERE t.id = sqlc.arg('id');

-- name: AccountIsActive :one
SELECT EXISTS (SELECT 1 FROM chart_accounts WHERE company_id = sqlc.arg('company_id') AND code = sqlc.arg('code') AND active);

-- name: InsertClassificationSuggestion :one
INSERT INTO classification_suggestions (company_id, statement_transaction_id, payload, account_code, trace_id, created_by, created_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('statement_transaction_id'), sqlc.arg('payload'), sqlc.narg('account_code'), sqlc.arg('trace_id'),
        sqlc.narg('created_by'), sqlc.arg('now'))
RETURNING id;

-- name: GetClassificationSuggestionForUpdate :one
SELECT * FROM classification_suggestions WHERE id = $1 FOR UPDATE;

-- name: ListClassificationSuggestionsByLine :many
SELECT * FROM classification_suggestions WHERE statement_transaction_id = $1 ORDER BY created_at DESC, id DESC;

-- name: DecideClassificationSuggestion :exec
UPDATE classification_suggestions SET status = sqlc.arg('status'), decided_by = sqlc.arg('decided_by'), decided_at = sqlc.arg('now')
WHERE id = sqlc.arg('id') AND status = 'pendente';

-- name: ListLinesAwaitingAgent :many
-- Linhas da competência à espera do agente que nunca tiveram sugestão nem pedido: o classificador
-- automático pede uma só vez por linha.
SELECT t.id FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.competence = sqlc.arg('competence')
  AND t.classification_origin = 'agente' AND t.classification_pending
  AND EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
  AND NOT EXISTS (SELECT 1 FROM classification_suggestions s WHERE s.statement_transaction_id = t.id)
  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.kind = sqlc.arg('job_kind')::text AND j.dedupe_key = t.id::text)
ORDER BY t.txn_date, t.statement_import_id, t.position, t.id;

-- name: ListLedgerLines :many
-- O razão (M5.6): uma linha por conciliação ativa, com a conta bancária, a classificação, os
-- comprovantes e os ids do drill-down. Nunca tabela escrita: reclassificar muda a origem.
SELECT r.id AS reconciliation_id, r.method, r.origin AS reconciliation_origin, r.pass, r.created_at AS reconciled_at,
       t.id AS statement_transaction_id, t.txn_date, t.competence, t.position, t.description, t.amount, t.direction,
       t.account_code, t.classification_origin, t.classification_pending,
       a.name AS account_name,
       si.id AS statement_import_id, si.document_id AS statement_document_id, si.bank, si.branch, si.account AS bank_account,
       COALESCE(array_agg(e.id ORDER BY e.id) FILTER (WHERE e.id IS NOT NULL), '{}')::uuid[] AS entry_ids,
       COALESCE(array_agg(COALESCE(e.counterparty, '') ORDER BY e.id) FILTER (WHERE e.id IS NOT NULL), '{}')::text[] AS counterparties,
       COALESCE(array_agg(COALESCE(e.counterparty_doc, '') ORDER BY e.id) FILTER (WHERE e.id IS NOT NULL), '{}')::text[] AS docs,
       COALESCE(array_agg(e.document_id ORDER BY e.id) FILTER (WHERE e.document_id IS NOT NULL), '{}')::uuid[] AS receipt_document_ids
FROM reconciliations r
JOIN statement_transactions t ON t.id = r.statement_transaction_id
JOIN statement_imports si ON si.id = t.statement_import_id
LEFT JOIN chart_accounts a ON a.company_id = t.company_id AND a.code = t.account_code
LEFT JOIN reconciliation_items i ON i.reconciliation_id = r.id AND i.released_at IS NULL
LEFT JOIN client_entries e ON e.id = i.client_entry_id
WHERE r.company_id = sqlc.arg('company_id') AND r.undone_at IS NULL
  AND t.competence BETWEEN sqlc.arg('competence_from') AND sqlc.arg('competence_to')
GROUP BY r.id, t.id, a.name, si.id
ORDER BY t.txn_date, si.bank, si.branch, si.account, t.position, t.id;

-- name: GetReconciliationDetail :one
-- A conciliação para o drill-down (AC-021): a linha, a conta bancária, a classificação e a regra.
SELECT r.id, r.company_id, r.method, r.origin, r.pass, r.score, r.proposal_id, r.created_by, r.created_at, r.undone_at,
       t.id AS statement_transaction_id, t.txn_date, t.competence, t.description, t.amount, t.direction, t.account_code,
       t.classification_origin, t.classification_pending, t.classification_rule_id, t.classification_history_id,
       t.classification_suggestion_id, t.classified_by, t.classified_at,
       a.name AS account_name, cr.rule_type, cr.pattern AS rule_pattern,
       si.id AS statement_import_id, si.document_id AS statement_document_id, si.bank, si.branch, si.account AS bank_account
FROM reconciliations r
JOIN statement_transactions t ON t.id = r.statement_transaction_id
JOIN statement_imports si ON si.id = t.statement_import_id
LEFT JOIN chart_accounts a ON a.company_id = t.company_id AND a.code = t.account_code
LEFT JOIN classification_rules cr ON cr.id = t.classification_rule_id
WHERE r.id = sqlc.arg('id');

-- name: ListReconciliationEntries :many
-- Os comprovantes da conciliação (os itens ativos; desfeita, os da época).
SELECT e.id, e.entry_date, e.amount, e.direction, e.counterparty, e.counterparty_doc, e.document_ref, e.status, e.document_id
FROM reconciliation_items i
JOIN client_entries e ON e.id = i.client_entry_id
WHERE i.reconciliation_id = sqlc.arg('reconciliation_id')
ORDER BY e.entry_date NULLS LAST, e.id;
