-- Simulador e agentes consultor/priorizador do Fator R [05 §4, 06 §3, TASK-013].

-- name: InsertFactorRSimulation :one
INSERT INTO factor_r_simulations (
    company_id, pa, meta, fracao_pro_labore, inss, irrf, horizonte_meses, piso_economia_anual,
    status, veredito, reforco_12m, reforco_mensal, custo_total, economia_12m, liquido,
    parametros_json, resultado_json, trace_id, created_by, created_at
) VALUES (
    sqlc.arg('company_id'), sqlc.arg('pa'), sqlc.arg('meta'), sqlc.arg('fracao_pro_labore'),
    sqlc.arg('inss'), sqlc.arg('irrf'), sqlc.arg('horizonte_meses'), sqlc.arg('piso_economia_anual'),
    sqlc.arg('status'), sqlc.narg('veredito'), sqlc.arg('reforco_12m'), sqlc.arg('reforco_mensal'),
    sqlc.arg('custo_total'), sqlc.narg('economia_12m'), sqlc.narg('liquido'),
    sqlc.arg('parametros_json'), sqlc.arg('resultado_json'), sqlc.arg('trace_id'),
    sqlc.narg('created_by'), sqlc.arg('created_at')::timestamptz
)
RETURNING id;

-- name: ListFactorRSimulations :many
-- Histórico da empresa, mais recentes primeiro, com o texto da corrida que fez cada simulação.
SELECT s.*, COALESCE(r.output ->> 'texto', '')::text AS texto
FROM factor_r_simulations s
LEFT JOIN ai_runs r ON r.trace_id = s.trace_id
WHERE s.company_id = sqlc.arg('company_id')
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetFactorRSimulationVerdict :one
-- Guarda do "corrigir": a simulação citada pela decisão existe e qual é o veredito.
SELECT veredito FROM factor_r_simulations WHERE id = $1;

-- name: ListActiveCompanyNames :many
-- Empresas ativas, para achar a empresa citada no pedido do analista sem passar pelo LLM.
SELECT id, legal_name, trade_name FROM companies WHERE active ORDER BY legal_name, internal_code LIMIT 500;

-- name: SearchActiveCompanies :many
-- Busca pelo nome que o LLM leu no pedido (company_ref), como a busca de empresas da origem: nome
-- com ILIKE (os curingas do texto valem) ou, se o texto tem dígitos, parte do CNPJ.
SELECT id, legal_name, trade_name FROM companies
WHERE active
  AND (legal_name ILIKE '%' || sqlc.arg('q')::text || '%'
       OR trade_name ILIKE '%' || sqlc.arg('q')::text || '%'
       OR (sqlc.arg('digits')::text <> '' AND cnpj LIKE '%' || sqlc.arg('digits')::text || '%'))
ORDER BY legal_name, internal_code
LIMIT 5;

-- name: CountAIRunsSince :one
-- Corridas de um agente e gatilho desde um instante (a rotina semanal do priorizador roda uma vez
-- por semana).
SELECT count(*) FROM ai_runs
WHERE agent = sqlc.arg('agent') AND trigger = sqlc.arg('trigger') AND created_at >= sqlc.arg('since')::timestamptz;
