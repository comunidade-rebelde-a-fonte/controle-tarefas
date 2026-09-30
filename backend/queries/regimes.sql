-- Histórico de regime tributário por vigência [PRD §8, §41.2, RN-003].

-- name: ListRegimes :many
SELECT h.id, h.tax_regime, h.effective_from_competence, h.effective_to_competence,
       h.created_by, u.name AS created_by_name, h.created_at
FROM company_tax_regime_history h
JOIN users u ON u.id = h.created_by
WHERE h.company_id = $1
ORDER BY h.effective_from_competence;

-- name: OpenRegimeForUpdate :one
SELECT * FROM company_tax_regime_history
WHERE company_id = $1 AND effective_to_competence IS NULL
FOR UPDATE;

-- name: CloseRegime :exec
UPDATE company_tax_regime_history SET effective_to_competence = $2 WHERE id = $1;

-- name: RegimeAt :one
SELECT tax_regime FROM company_tax_regime_history
WHERE company_id = $1
  AND effective_from_competence <= sqlc.arg('competence')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('competence')::date);

-- name: LockCompany :one
SELECT id FROM companies WHERE id = $1 FOR UPDATE;
