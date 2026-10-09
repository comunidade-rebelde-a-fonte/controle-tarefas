-- Lançamentos do cliente (TASK-017, D-028): o comprovante lido (leitor determinístico ou extrator),
-- a duplicata de leiaute diferente e os lançamentos manuais (formulário e CSV).

-- name: InsertClientEntry :one
INSERT INTO client_entries (
    company_id, document_id, entry_date, amount, direction, counterparty, counterparty_doc, document_ref,
    operation_type, status, field_confidence, field_origin, duplicate_flagged, flags, trace_id, created_by,
    created_at, updated_at
) VALUES (
    sqlc.arg('company_id'), sqlc.narg('document_id'), sqlc.narg('entry_date'), sqlc.narg('amount'),
    sqlc.narg('direction'), sqlc.narg('counterparty'), sqlc.narg('counterparty_doc'), sqlc.narg('document_ref'),
    sqlc.narg('operation_type'), sqlc.arg('status'), sqlc.narg('field_confidence'), sqlc.narg('field_origin'),
    sqlc.arg('duplicate_flagged'), sqlc.arg('flags'), sqlc.narg('trace_id'), sqlc.narg('created_by'),
    sqlc.arg('now'), sqlc.arg('now')
)
RETURNING id;

-- name: ListClientEntryCounterparties :many
-- Lançamentos da empresa com o mesmo valor e a mesma data: é duplicata de leiaute diferente o que
-- tiver também a contraparte normalizada igual (M3.5 da origem; a normalização é a do código).
SELECT counterparty FROM client_entries
WHERE company_id = sqlc.arg('company_id') AND amount = sqlc.arg('amount') AND entry_date = sqlc.arg('entry_date');

-- name: GetClientEntryByDocument :one
SELECT * FROM client_entries WHERE document_id = $1;

-- name: GetClientEntry :one
SELECT * FROM client_entries WHERE id = $1;

-- name: SaveReconciliationReceiptDocument :exec
-- Comprovante lido: o documento fica com a empresa e o estado (importado ou revisao_extracao).
UPDATE reconciliation_documents
SET status = sqlc.arg('status'), kind = 'comprovante', company_id = sqlc.arg('company_id'),
    diagnosis = sqlc.narg('diagnosis'), updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id');

-- name: GetCompanyIdentity :one
-- Nome e CNPJ da empresa: o leitor do comprovante procura a empresa pelo nome.
SELECT id, legal_name, trade_name, cnpj FROM companies WHERE id = $1;

-- name: ListCompaniesByCnpj :many
-- Empresas cadastradas entre os CNPJs impressos no comprovante.
SELECT id, legal_name, trade_name, cnpj FROM companies WHERE cnpj = ANY(sqlc.arg('cnpjs')::text[]) ORDER BY cnpj;

-- name: ListClientEntries :many
-- Por data (sem data por último). O período filtra pela data do lançamento.
SELECT * FROM client_entries
WHERE (sqlc.narg('company_id')::uuid IS NULL OR company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('date_from')::date IS NULL OR entry_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR entry_date <= sqlc.narg('date_to')::date)
ORDER BY entry_date NULLS LAST, created_at, id
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountClientEntries :one
SELECT count(*) FROM client_entries
WHERE (sqlc.narg('company_id')::uuid IS NULL OR company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('date_from')::date IS NULL OR entry_date >= sqlc.narg('date_from')::date)
  AND (sqlc.narg('date_to')::date IS NULL OR entry_date <= sqlc.narg('date_to')::date);
