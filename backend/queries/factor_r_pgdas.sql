-- Inbox PGDAS-D do Fator R [05 §4, 06 §3, TASK-012].

-- name: InsertPgdasDocument :one
-- Documento recebido. Conflito no sha256 = o arquivo já foi enviado: nada inserido.
INSERT INTO factor_r_pgdas_documents (sha256, file_ref, mime, size_bytes, original_name, uploaded_by, created_at, updated_at)
VALUES (sqlc.arg('sha256'), sqlc.arg('file_ref'), sqlc.arg('mime'), sqlc.arg('size_bytes'),
        sqlc.narg('original_name'), sqlc.narg('uploaded_by'), sqlc.arg('now')::timestamptz, sqlc.arg('now')::timestamptz)
ON CONFLICT (sha256) DO NOTHING
RETURNING id;

-- name: SetPgdasDocumentJob :exec
UPDATE factor_r_pgdas_documents SET job_id = sqlc.arg('job_id') WHERE id = sqlc.arg('id');

-- name: GetPgdasDocumentIDBySha :one
SELECT id FROM factor_r_pgdas_documents WHERE sha256 = $1;

-- name: GetPgdasDocument :one
-- Documento com o estado do job de processamento [AC-014] e o texto da última corrida do agente.
SELECT d.*,
       j.status AS job_status, j.attempts AS job_attempts, j.max_attempts AS job_max_attempts,
       j.last_error_code AS job_last_error_code, COALESCE(r.output ->> 'texto', '')::text AS texto_agente
FROM factor_r_pgdas_documents d
LEFT JOIN jobs j ON j.id = d.job_id
LEFT JOIN ai_runs r ON r.trace_id = d.trace_id
WHERE d.id = $1;

-- name: GetPgdasDocumentForUpdate :one
SELECT * FROM factor_r_pgdas_documents WHERE id = $1 FOR UPDATE;

-- name: ListPgdasDocuments :many
-- Mais recentes primeiro (listar da origem), com o estado do job e o texto do agente. O filtro
-- por empresa traz os extratos vinculados a ela (aba Fator R da ficha, TASK-014b).
SELECT d.*,
       j.status AS job_status, j.attempts AS job_attempts, j.max_attempts AS job_max_attempts,
       j.last_error_code AS job_last_error_code, COALESCE(r.output ->> 'texto', '')::text AS texto_agente
FROM factor_r_pgdas_documents d
LEFT JOIN jobs j ON j.id = d.job_id
LEFT JOIN ai_runs r ON r.trace_id = d.trace_id
WHERE (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text)
  AND (sqlc.narg('company_id')::uuid IS NULL OR d.company_id = sqlc.narg('company_id')::uuid)
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountPgdasDocuments :one
SELECT count(*) FROM factor_r_pgdas_documents
WHERE (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text)
  AND (sqlc.narg('company_id')::uuid IS NULL OR company_id = sqlc.narg('company_id')::uuid);

-- name: CountPgdasDocumentsByStatus :many
SELECT status, count(*) AS total FROM factor_r_pgdas_documents GROUP BY status ORDER BY status;

-- name: SavePgdasParse :exec
-- Resultado do parse e a decisão do agente no upload (job pgdas_parse).
UPDATE factor_r_pgdas_documents
SET status = sqlc.arg('status'), motivo = sqlc.narg('motivo'), company_id = sqlc.narg('company_id'),
    cnpj_extraido = sqlc.narg('cnpj_extraido'), pa = sqlc.narg('pa'), confidence = sqlc.arg('confidence'),
    parser_version = sqlc.arg('parser_version'), parsed_json = sqlc.arg('parsed_json'),
    extracted_text = sqlc.narg('extracted_text'), trace_id = sqlc.arg('trace_id'),
    updated_at = sqlc.arg('now')::timestamptz, version = version + 1
WHERE id = sqlc.arg('id');

-- name: SavePgdasDecision :exec
-- Vínculo, rejeição ou cadastro pelo extrato, decididos pelo analista numa corrida do agente.
UPDATE factor_r_pgdas_documents
SET status = sqlc.arg('status'), motivo = sqlc.narg('motivo'), company_id = sqlc.narg('company_id'),
    trace_id = sqlc.arg('trace_id'), updated_at = sqlc.arg('now')::timestamptz, version = version + 1
WHERE id = sqlc.arg('id');

-- name: InsertFactorRMovementFromPgdas :one
-- Mês lido do extrato, só em competência vazia: nunca sobrescreve (criar_se_ausente da origem).
-- Nada inserido = a competência já tinha lançamento.
INSERT INTO factor_r_movements (
    company_id, competence, receita_bruta, folha_total_declarada_pgdas, origem, observacao,
    pgdas_document_id, updated_by, updated_at
) VALUES (
    sqlc.arg('company_id'), sqlc.arg('competence'), sqlc.arg('receita_bruta'),
    sqlc.narg('folha_total_declarada_pgdas'), 'pgdas', sqlc.arg('observacao'),
    sqlc.arg('pgdas_document_id'), sqlc.narg('updated_by'), sqlc.arg('now')::timestamptz
)
ON CONFLICT (company_id, competence) DO NOTHING
RETURNING id;
