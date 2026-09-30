-- name: InsertImportPreview :one
INSERT INTO import_previews (file_name, rows, summary, created_by, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: GetImportPreviewForUpdate :one
SELECT * FROM import_previews WHERE id = $1 FOR UPDATE;

-- name: MarkImportCommitted :exec
UPDATE import_previews SET committed_at = $2, committed_by = $3, result = $4 WHERE id = $1;

-- name: ListCompaniesForImport :many
-- Cadastro atual para conciliação: CNPJ e código interno.
SELECT id, internal_code, cnpj, legal_name FROM companies;

-- name: GetCompanyForImportUpdate :one
SELECT * FROM companies WHERE id = $1 FOR UPDATE;
