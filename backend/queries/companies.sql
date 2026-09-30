-- Empresas [PRD §7, §41.1]. Regime e módulos "vigentes" são resolvidos na competência
-- de referência (sqlc.arg('ref')), normalmente a competência corrente do relógio.

-- name: ListCompanies :many
SELECT
    c.id, c.internal_code, c.legal_name, c.trade_name, c.cnpj, c.city, c.state,
    c.service_start_date, c.service_end_date, c.active, c.payroll_type, c.factor_r, c.version,
    c.fiscal_responsible_user_id, c.accounting_responsible_user_id,
    c.payroll_responsible_user_id, c.declarations_responsible_user_id,
    COALESCE(r.tax_regime, '')::text AS current_tax_regime,
    COALESCE(m.modules, '{}')::text[] AS active_modules
FROM companies c
LEFT JOIN LATERAL (
    SELECT h.tax_regime
    FROM company_tax_regime_history h
    WHERE h.company_id = c.id
      AND h.effective_from_competence <= GREATEST(sqlc.arg('ref')::date, date_trunc('month', c.service_start_date)::date)
      AND (h.effective_to_competence IS NULL
           OR h.effective_to_competence >= GREATEST(sqlc.arg('ref')::date, date_trunc('month', c.service_start_date)::date))
) r ON true
LEFT JOIN LATERAL (
    SELECT array_agg(s.module_code ORDER BY s.module_code) AS modules
    FROM company_module_settings s
    WHERE s.company_id = c.id AND s.enabled
      AND s.effective_from_competence <= GREATEST(sqlc.arg('ref')::date, date_trunc('month', c.service_start_date)::date)
      AND (s.effective_to_competence IS NULL
           OR s.effective_to_competence >= GREATEST(sqlc.arg('ref')::date, date_trunc('month', c.service_start_date)::date))
) m ON true
WHERE (sqlc.narg('q')::text IS NULL
       OR c.legal_name ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\'
       OR c.trade_name ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\'
       OR c.internal_code ILIKE '%' || sqlc.narg('q')::text || '%' ESCAPE '\'
       OR (sqlc.narg('q_digits')::text IS NOT NULL AND c.cnpj LIKE '%' || sqlc.narg('q_digits')::text || '%'))
  AND (sqlc.narg('cnpj')::text IS NULL OR c.cnpj = sqlc.narg('cnpj')::text)
  AND (sqlc.narg('code')::text IS NULL OR c.internal_code = sqlc.narg('code')::text)
  AND (sqlc.narg('tax_regime')::text IS NULL OR r.tax_regime = sqlc.narg('tax_regime')::text)
  AND (sqlc.narg('active')::boolean IS NULL OR c.active = sqlc.narg('active')::boolean)
  AND (sqlc.narg('module')::text IS NULL OR sqlc.narg('module')::text = ANY (COALESCE(m.modules, '{}')))
ORDER BY c.legal_name, c.internal_code;

-- name: GetCompany :one
SELECT * FROM companies WHERE id = $1;

-- name: GetCompanyForUpdate :one
SELECT * FROM companies WHERE id = $1 FOR UPDATE;

-- name: FindCompanyByCnpj :one
SELECT id, internal_code, legal_name FROM companies WHERE cnpj = $1;

-- name: FindCompanyByCode :one
SELECT id, internal_code, legal_name FROM companies WHERE internal_code = $1;

-- name: InsertCompany :one
INSERT INTO companies (
    internal_code, legal_name, trade_name, cnpj, city, state, state_registration, municipal_registration,
    service_start_date, service_end_date, active, payroll_type, factor_r,
    fiscal_responsible_user_id, accounting_responsible_user_id, payroll_responsible_user_id,
    declarations_responsible_user_id, notes, created_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true, $11, $12, $13, $14, $15, $16, $17, $18, $18
) RETURNING *;

-- name: UpdateCompany :one
-- Concorrência otimista: só atualiza se a versão bater [plan D8].
UPDATE companies SET
    internal_code = $3, legal_name = $4, trade_name = $5, cnpj = $6, city = $7, state = $8,
    state_registration = $9, municipal_registration = $10, service_start_date = $11,
    service_end_date = $12, active = $13, payroll_type = $14, factor_r = $15,
    fiscal_responsible_user_id = $16, accounting_responsible_user_id = $17,
    payroll_responsible_user_id = $18, declarations_responsible_user_id = $19,
    notes = $20, updated_by = $21, updated_at = now(), version = version + 1
WHERE id = $1 AND version = $2
RETURNING *;

-- name: CompanyCurrentRegime :one
SELECT tax_regime FROM company_tax_regime_history
WHERE company_id = $1
  AND effective_from_competence <= sqlc.arg('ref')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('ref')::date);

-- name: CompanyActiveModules :many
SELECT module_code FROM company_module_settings
WHERE company_id = $1 AND enabled
  AND effective_from_competence <= sqlc.arg('ref')::date
  AND (effective_to_competence IS NULL OR effective_to_competence >= sqlc.arg('ref')::date)
ORDER BY module_code;

-- name: InsertRegime :one
INSERT INTO company_tax_regime_history (company_id, tax_regime, effective_from_competence, effective_to_competence, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: InsertModuleSetting :one
INSERT INTO company_module_settings (company_id, module_code, enabled, effective_from_competence, effective_to_competence, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UserNames :many
SELECT id, name FROM users WHERE id = ANY (sqlc.arg('ids')::uuid[]);

-- name: CountActiveUsers :one
SELECT count(*) FROM users WHERE id = ANY (sqlc.arg('ids')::uuid[]) AND active;
