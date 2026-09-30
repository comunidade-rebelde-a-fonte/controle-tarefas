-- name: ListCredentialCompanies :many
-- Empresas ativas com o módulo Procurações/Certificados habilitado na competência de referência.
SELECT
    c.id, c.internal_code, c.legal_name, c.trade_name, c.cnpj,
    COALESCE(r.tax_regime, '')::text AS tax_regime
FROM companies c
LEFT JOIN LATERAL (
    SELECT h.tax_regime
    FROM company_tax_regime_history h
    WHERE h.company_id = c.id
      AND h.effective_from_competence <= sqlc.arg('ref')::date
      AND (h.effective_to_competence IS NULL OR h.effective_to_competence >= sqlc.arg('ref')::date)
) r ON true
WHERE c.active
  AND EXISTS (
      SELECT 1 FROM company_module_settings s
      WHERE s.company_id = c.id AND s.module_code = 'CREDENTIALS' AND s.enabled
        AND s.effective_from_competence <= sqlc.arg('ref')::date
        AND (s.effective_to_competence IS NULL OR s.effective_to_competence >= sqlc.arg('ref')::date)
  )
ORDER BY c.legal_name, c.internal_code;

-- name: GetCredentialCompany :one
SELECT
    c.id, c.internal_code, c.legal_name, c.trade_name, c.cnpj,
    COALESCE(r.tax_regime, '')::text AS tax_regime
FROM companies c
LEFT JOIN LATERAL (
    SELECT h.tax_regime
    FROM company_tax_regime_history h
    WHERE h.company_id = c.id
      AND h.effective_from_competence <= sqlc.arg('ref')::date
      AND (h.effective_to_competence IS NULL OR h.effective_to_competence >= sqlc.arg('ref')::date)
) r ON true
WHERE c.id = sqlc.arg('id');

-- name: ListCredentialControls :many
SELECT * FROM credential_controls WHERE company_id = ANY (sqlc.arg('company_ids')::uuid[]);

-- name: InsertCredentialControl :one
-- Primeiro registro do tipo para a empresa (version 1); conflito = alguém criou antes.
INSERT INTO credential_controls (company_id, type, expires_at, renewal_status, notes, updated_by, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (company_id, type) DO NOTHING
RETURNING *;

-- name: UpdateCredentialControl :one
UPDATE credential_controls
SET expires_at = $4, renewal_status = $5, notes = $6, updated_by = $7, updated_at = $8, version = version + 1
WHERE company_id = $1 AND type = $2 AND version = $3
RETURNING *;
