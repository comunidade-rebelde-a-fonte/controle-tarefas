-- Processamento dos documentos da Conciliação (TASK-016, D-027): triagem, extração do extrato,
-- importação atômica, playbooks por banco e o log de extração por tentativa.

-- name: GetReconciliationDocumentForUpdate :one
-- O job trava o documento: uma tentativa por vez.
SELECT * FROM reconciliation_documents WHERE id = $1 FOR UPDATE;

-- name: SaveReconciliationTriage :exec
-- Resultado do triador (e o estado que ele decide: triado ou revisao_triagem).
UPDATE reconciliation_documents
SET kind = sqlc.arg('kind'), bank = sqlc.narg('bank'), branch = sqlc.narg('branch'), account = sqlc.narg('account'),
    period_start = sqlc.narg('period_start'), period_end = sqlc.narg('period_end'),
    triage_confidence = sqlc.narg('triage_confidence'), triage_trace_id = sqlc.narg('triage_trace_id'),
    status = sqlc.arg('status'), diagnosis = sqlc.narg('diagnosis'), updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id');

-- name: SetReconciliationDocumentState :exec
-- Muda o estado e o diagnóstico (revisão, pendência) e aponta o job que processa agora.
UPDATE reconciliation_documents
SET status = sqlc.arg('status'), diagnosis = sqlc.narg('diagnosis'), job_id = COALESCE(sqlc.narg('job_id'), job_id),
    updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id');

-- name: SaveReconciliationImportedDocument :exec
-- Extrato importado: o documento fica com a empresa e os dados do extrato aprovado.
UPDATE reconciliation_documents
SET status = 'importado', kind = 'extrato', company_id = sqlc.arg('company_id'), bank = sqlc.arg('bank'),
    branch = sqlc.arg('branch'), account = sqlc.arg('account'), period_start = sqlc.arg('period_start'),
    period_end = sqlc.arg('period_end'), diagnosis = NULL, updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id');

-- name: FindBankAccountByIdentity :one
-- A conta é única no sistema (D-020): banco, agência e conta identificam a empresa.
SELECT id, company_id FROM bank_accounts WHERE bank = $1 AND branch = $2 AND account = $3;

-- name: InsertStatementImport :one
INSERT INTO statement_imports (
    company_id, document_id, bank_account_id, bank, branch, account, period_start, period_end,
    opening_balance, closing_balance, validation_ok, validation, attempts, trace_id, created_at
) VALUES (
    sqlc.arg('company_id'), sqlc.arg('document_id'), sqlc.narg('bank_account_id'), sqlc.arg('bank'),
    sqlc.arg('branch'), sqlc.arg('account'), sqlc.arg('period_start'), sqlc.arg('period_end'),
    sqlc.arg('opening_balance'), sqlc.arg('closing_balance'), true, sqlc.arg('validation'),
    sqlc.arg('attempts'), sqlc.narg('trace_id'), sqlc.arg('now')
)
RETURNING id;

-- name: InsertStatementTransaction :execrows
-- Transação já importada (mesma chave de dedupe) é ignorada, como na origem.
INSERT INTO statement_transactions (
    company_id, statement_import_id, position, txn_date, description, document_ref, amount, direction,
    balance_after, dedupe_key, confidence, created_at
) VALUES (
    sqlc.arg('company_id'), sqlc.arg('statement_import_id'), sqlc.arg('position'), sqlc.arg('txn_date'),
    sqlc.arg('description'), sqlc.narg('document_ref'), sqlc.arg('amount'), sqlc.arg('direction'),
    sqlc.narg('balance_after'), sqlc.arg('dedupe_key'), sqlc.narg('confidence'), sqlc.arg('now')
)
ON CONFLICT (dedupe_key) DO NOTHING;

-- name: GetValidBankPlaybook :one
-- Playbook vigente do banco (no máximo um válido por banco).
SELECT * FROM bank_playbooks WHERE bank = $1 AND valid;

-- name: InvalidateBankPlaybooks :execrows
-- A extração com o playbook falhou na 1ª tentativa: invalida, e a próxima redescobre.
UPDATE bank_playbooks SET valid = false, failures = failures + 1, updated_at = sqlc.arg('now')
WHERE bank = sqlc.arg('bank') AND valid;

-- name: AddBankPlaybookSuccess :exec
UPDATE bank_playbooks SET successes = successes + 1, updated_at = sqlc.arg('now') WHERE id = sqlc.arg('id');

-- name: NextBankPlaybookVersion :one
SELECT (COALESCE(max(version), 0) + 1)::integer AS version FROM bank_playbooks WHERE bank = $1;

-- name: InsertBankPlaybook :one
-- Só depois do portão aprovar (M1.5.2 da origem).
INSERT INTO bank_playbooks (
    bank, version, table_start_page, ignored_pages, date_format, decimal_separator, sign_representation,
    balance_location, winning_strategy, validated_at, valid, successes, failures, created_at, updated_at
) VALUES (
    sqlc.arg('bank'), sqlc.arg('version'), sqlc.arg('table_start_page'), sqlc.arg('ignored_pages'),
    sqlc.arg('date_format'), sqlc.arg('decimal_separator'), sqlc.arg('sign_representation'),
    sqlc.arg('balance_location'), sqlc.arg('winning_strategy'), sqlc.arg('now'), true, 1, 0,
    sqlc.arg('now'), sqlc.arg('now')
)
RETURNING id;

-- name: InsertExtractionLog :one
-- Uma linha por tentativa de extração (ou leitura determinística, tentativa 0) e por intervenção
-- humana (alias "humano").
INSERT INTO extraction_logs (
    company_id, document_id, file_sha256, alias, resolved_model, prompt_hash, prompt_version, raw_output,
    validator_result, attempt, tokens_in, tokens_out, cost_usd, latency_ms, trace_id, job_id, actor_id,
    change, error, created_at
) VALUES (
    sqlc.narg('company_id'), sqlc.narg('document_id'), sqlc.narg('file_sha256'), sqlc.arg('alias'),
    sqlc.narg('resolved_model'), sqlc.narg('prompt_hash'), sqlc.narg('prompt_version'), sqlc.narg('raw_output'),
    sqlc.narg('validator_result'), sqlc.arg('attempt'), sqlc.narg('tokens_in'), sqlc.narg('tokens_out'),
    sqlc.narg('cost_usd'), sqlc.narg('latency_ms'), sqlc.narg('trace_id'), sqlc.narg('job_id'),
    sqlc.narg('actor_id'), sqlc.narg('change'), sqlc.narg('error'), sqlc.arg('now')
)
RETURNING id;

-- name: GetStatementImportByDocument :one
-- A importação do documento, com o número de transações gravadas por ela.
SELECT i.*, (SELECT count(*) FROM statement_transactions t WHERE t.statement_import_id = i.id) AS transactions
FROM statement_imports i
WHERE i.document_id = $1;

-- name: ListExtractionLogsByDocument :many
-- As tentativas e intervenções do documento, na ordem em que aconteceram (sem a saída bruta).
SELECT id, alias, attempt, resolved_model, prompt_version, validator_result, tokens_in, tokens_out, cost_usd,
       latency_ms, trace_id, job_id, actor_id, change, error, created_at
FROM extraction_logs
WHERE document_id = $1
ORDER BY created_at, id;
