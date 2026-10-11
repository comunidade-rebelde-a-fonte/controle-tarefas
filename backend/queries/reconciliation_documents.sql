-- Documentos da Conciliação (TASK-015): o original no MinIO, o hash e o estado [REQ-018].

-- name: GetReconciliationDocumentIDBySha :one
SELECT id FROM reconciliation_documents WHERE sha256 = $1;

-- name: InsertReconciliationDocument :one
-- Sem linha (ON CONFLICT): outro envio do mesmo arquivo gravou antes (dedupe por sha256).
INSERT INTO reconciliation_documents (
    company_id, competence, bucket, object_key, object_version_id, sha256, file_name, content_type,
    size_bytes, status, diagnosis, uploaded_by, created_at, updated_at
) VALUES (
    sqlc.narg('company_id'), sqlc.narg('competence'), sqlc.arg('bucket'), sqlc.arg('object_key'),
    sqlc.narg('object_version_id'), sqlc.arg('sha256'), sqlc.arg('file_name'), sqlc.arg('content_type'),
    sqlc.arg('size_bytes'), sqlc.arg('status'), sqlc.narg('diagnosis'), sqlc.narg('uploaded_by'), sqlc.arg('now'), sqlc.arg('now')
)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: SetReconciliationDocumentJob :exec
UPDATE reconciliation_documents SET job_id = $1 WHERE id = $2;

-- name: GetReconciliationDocument :one
-- Documento com o estado do job que o processa [AC-014].
SELECT d.*,
       j.status AS job_status, j.attempts AS job_attempts, j.max_attempts AS job_max_attempts,
       j.last_error_code AS job_last_error_code
FROM reconciliation_documents d
LEFT JOIN jobs j ON j.id = d.job_id
WHERE d.id = $1;

-- name: ListReconciliationDocuments :many
-- Mais recentes primeiro. A competência casa com a informada no envio ou com o período do
-- extrato (lido na triagem) que encosta no mês.
SELECT d.*,
       j.status AS job_status, j.attempts AS job_attempts, j.max_attempts AS job_max_attempts,
       j.last_error_code AS job_last_error_code
FROM reconciliation_documents d
LEFT JOIN jobs j ON j.id = d.job_id
WHERE (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text)
  AND (sqlc.narg('company_id')::uuid IS NULL OR d.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('kind')::text IS NULL OR d.kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('bank')::text IS NULL OR lower(d.bank) = lower(sqlc.narg('bank')::text))
  AND (sqlc.narg('competence')::date IS NULL OR d.competence = sqlc.narg('competence')::date
       OR (d.period_start < (sqlc.narg('competence')::date + interval '1 month')
           AND COALESCE(d.period_end, d.period_start) >= sqlc.narg('competence')::date))
ORDER BY d.created_at DESC, d.id DESC
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountReconciliationDocuments :one
SELECT count(*) FROM reconciliation_documents d
WHERE (sqlc.narg('status')::text IS NULL OR d.status = sqlc.narg('status')::text)
  AND (sqlc.narg('company_id')::uuid IS NULL OR d.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('kind')::text IS NULL OR d.kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('bank')::text IS NULL OR lower(d.bank) = lower(sqlc.narg('bank')::text))
  AND (sqlc.narg('competence')::date IS NULL OR d.competence = sqlc.narg('competence')::date
       OR (d.period_start < (sqlc.narg('competence')::date + interval '1 month')
           AND COALESCE(d.period_end, d.period_start) >= sqlc.narg('competence')::date));

-- name: CountReconciliationDocumentsByStatus :many
SELECT status, count(*) AS total FROM reconciliation_documents GROUP BY status ORDER BY status;
