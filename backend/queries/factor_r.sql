-- Fator R [docs/ai-spec/05 §4, 06 §3]. "Sujeita ao Fator R" é companies.factor_r (D-017).

-- name: GetFactorRCompany :one
-- Empresa com o regime vigente na competência de referência (PA ou competência corrente);
-- tax_regime vazio quando não há regime vigente nela.
SELECT
    c.id, c.internal_code, c.legal_name, c.trade_name, c.cnpj, c.active, c.factor_r,
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

-- name: ListFactorRPortfolioCompanies :many
-- Carteira do PA: empresas ativas, sujeitas ao Fator R e no Simples Nacional no PA (D-017).
SELECT
    c.id, c.internal_code, c.legal_name, c.trade_name, c.cnpj,
    s.pacote_comercial,
    COALESCE(s.honorario_mensal, 0)::numeric(14,2) AS honorario_mensal,
    s.inicio_atividade
FROM companies c
JOIN company_tax_regime_history h
  ON h.company_id = c.id
 AND h.effective_from_competence <= sqlc.arg('pa')::date
 AND (h.effective_to_competence IS NULL OR h.effective_to_competence >= sqlc.arg('pa')::date)
LEFT JOIN company_factor_r_settings s ON s.company_id = c.id
WHERE c.active
  AND c.factor_r IS TRUE
  AND h.tax_regime = 'SIMPLES_NACIONAL'
ORDER BY c.legal_name, c.internal_code;

-- name: GetFactorRSettings :one
SELECT * FROM company_factor_r_settings WHERE company_id = $1;

-- name: InsertFactorRSettings :one
-- Primeira configuração da empresa (version 1); conflito = alguém criou antes.
INSERT INTO company_factor_r_settings (
    company_id, cnae, atividade, quantidade_socios, contato, pacote_comercial,
    honorario_mensal, inicio_atividade, notas, updated_by, updated_at
) VALUES (
    sqlc.arg('company_id'), sqlc.narg('cnae'), sqlc.narg('atividade'), sqlc.arg('quantidade_socios'),
    sqlc.narg('contato'), sqlc.narg('pacote_comercial'), sqlc.arg('honorario_mensal'),
    sqlc.narg('inicio_atividade'), sqlc.narg('notas'), sqlc.arg('updated_by'), sqlc.arg('updated_at')
)
ON CONFLICT (company_id) DO NOTHING
RETURNING *;

-- name: UpdateFactorRSettings :one
UPDATE company_factor_r_settings
SET cnae = sqlc.narg('cnae'), atividade = sqlc.narg('atividade'),
    quantidade_socios = sqlc.arg('quantidade_socios'), contato = sqlc.narg('contato'),
    pacote_comercial = sqlc.narg('pacote_comercial'), honorario_mensal = sqlc.arg('honorario_mensal'),
    inicio_atividade = sqlc.narg('inicio_atividade'), notas = sqlc.narg('notas'),
    updated_by = sqlc.arg('updated_by'), updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE company_id = sqlc.arg('company_id') AND version = sqlc.arg('version')
RETURNING *;

-- name: ListFactorRMovements :many
-- Movimentos de uma empresa entre duas competências (inclusive), em ordem.
SELECT * FROM factor_r_movements
WHERE company_id = sqlc.arg('company_id')
  AND competence >= sqlc.arg('from_competence')::date
  AND competence <= sqlc.arg('to_competence')::date
ORDER BY competence;

-- name: ListFactorRMovementsForCompanies :many
-- Movimentos de várias empresas numa janela, numa consulta só (carteira).
SELECT * FROM factor_r_movements
WHERE company_id = ANY (sqlc.arg('company_ids')::uuid[])
  AND competence >= sqlc.arg('from_competence')::date
  AND competence <= sqlc.arg('to_competence')::date;

-- name: GetFactorRMovement :one
SELECT * FROM factor_r_movements WHERE company_id = $1 AND competence = $2;

-- name: InsertFactorRMovementManual :one
-- Lançamento manual numa competência vazia (version 1); conflito = alguém lançou antes.
INSERT INTO factor_r_movements (
    company_id, competence, receita_bruta, pro_labore, salarios_13_ferias, cpp_recolhida,
    fgts_recolhido, origem, observacao, updated_by, updated_at
) VALUES (
    sqlc.arg('company_id'), sqlc.arg('competence'), sqlc.arg('receita_bruta'), sqlc.arg('pro_labore'),
    sqlc.arg('salarios_13_ferias'), sqlc.arg('cpp_recolhida'), sqlc.arg('fgts_recolhido'), 'manual',
    sqlc.narg('observacao'), sqlc.arg('updated_by'), sqlc.arg('updated_at')
)
ON CONFLICT (company_id, competence) DO NOTHING
RETURNING *;

-- name: UpdateFactorRMovementManual :one
-- Lançamento manual substitui a competência inteira e passa a origem a manual, como na
-- origem. A folha total do PGDAS-D dá lugar à divisão informada ("Detalhar folha").
UPDATE factor_r_movements
SET receita_bruta = sqlc.arg('receita_bruta'), pro_labore = sqlc.arg('pro_labore'),
    salarios_13_ferias = sqlc.arg('salarios_13_ferias'), cpp_recolhida = sqlc.arg('cpp_recolhida'),
    fgts_recolhido = sqlc.arg('fgts_recolhido'), folha_total_declarada_pgdas = NULL,
    origem = 'manual', observacao = sqlc.narg('observacao'),
    updated_by = sqlc.arg('updated_by'), updated_at = sqlc.arg('updated_at'), version = version + 1
WHERE company_id = sqlc.arg('company_id') AND competence = sqlc.arg('competence') AND version = sqlc.arg('version')
RETURNING *;

-- name: GetFactorRPolicy :one
SELECT * FROM factor_r_policy WHERE id;

-- name: ListSimplesTablesInForce :many
-- Faixas dos Anexos III e V vigentes numa data, a vigência mais recente primeiro. O serviço
-- monta cada anexo só com as faixas da vigência mais recente.
SELECT * FROM factor_r_simples_tables
WHERE vigencia_inicio <= sqlc.arg('ref')::date
  AND (vigencia_fim IS NULL OR vigencia_fim > sqlc.arg('ref')::date)
ORDER BY anexo, vigencia_inicio DESC, faixa;
