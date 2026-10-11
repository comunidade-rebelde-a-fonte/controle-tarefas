-- Métricas da Conciliação (TASK-021c, D-034): o painel do 11-observability §7, os playbooks por
-- banco, o resumo da competência na ficha da empresa e as contas bancárias (CC-11). Tudo é
-- consulta sobre as tabelas da Conciliação; não há tabela de métrica. Nos filtros, empresa e
-- competência são opcionais.

-- name: ListBankPlaybooks :many
-- Os playbooks por banco, todas as versões (no máximo uma válida por banco), como a tela da origem.
SELECT * FROM bank_playbooks ORDER BY bank, version DESC;

-- name: ListPlaybookStats :many
-- O sucesso dos playbooks de cada banco, somando as versões, e a versão vigente.
SELECT s.bank, s.versions, s.successes, s.failures, v.version AS valid_version
FROM (
    SELECT p.bank, count(*)::bigint AS versions, COALESCE(sum(p.successes), 0)::bigint AS successes,
           COALESCE(sum(p.failures), 0)::bigint AS failures
    FROM bank_playbooks p
    GROUP BY p.bank
) s
LEFT JOIN bank_playbooks v ON v.bank = s.bank AND v.valid
ORDER BY s.bank;

-- name: GetMatchingMetrics :one
-- As linhas do extrato do filtro, as conciliadas (ativas) pela origem e as automáticas desfeitas.
WITH lines AS (
    SELECT t.id FROM statement_transactions t
    WHERE (sqlc.narg('company_id')::uuid IS NULL OR t.company_id = sqlc.narg('company_id')::uuid)
      AND (sqlc.narg('competence')::date IS NULL OR t.competence = sqlc.narg('competence')::date)
)
SELECT (SELECT count(*) FROM lines)::bigint AS lines,
       count(r.id) FILTER (WHERE r.undone_at IS NULL AND r.origin = 'auto')::bigint AS auto,
       count(r.id) FILTER (WHERE r.undone_at IS NULL AND r.origin = 'manual')::bigint AS manual,
       count(r.id) FILTER (WHERE r.undone_at IS NOT NULL AND r.origin = 'auto')::bigint AS undone_auto
FROM lines l
JOIN reconciliations r ON r.statement_transaction_id = l.id;

-- name: ListMatchingByMethod :many
-- As conciliações ativas por método: passe_1 a passe_4 (automáticas), manual, sem_comprovante e
-- resolvedor (o "por passe" da origem).
SELECT r.method, count(*)::bigint AS total
FROM reconciliations r
JOIN statement_transactions t ON t.id = r.statement_transaction_id
WHERE r.undone_at IS NULL
  AND (sqlc.narg('company_id')::uuid IS NULL OR t.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('competence')::date IS NULL OR t.competence = sqlc.narg('competence')::date)
GROUP BY r.method
ORDER BY r.method;

-- name: ListExtractionMetrics :many
-- A extração por tipo de documento, como o make eval-extrator (D-027): as tentativas são as do
-- agente (extrator e extrator_forte); aprovado é o extrato importado ou o comprovante completo sem
-- campo corrigido à mão; o leitor determinístico (sem agente) fica à parte; o custo soma todas as
-- chamadas de modelo do documento (triagem e extração) do log de extração. O documento entra na
-- competência pela informada no envio, pelo período do extrato ou pela data do comprovante.
WITH docs AS (
    SELECT d.id, d.kind
    FROM reconciliation_documents d
    WHERE d.kind IN ('extrato', 'comprovante')
      AND (sqlc.narg('company_id')::uuid IS NULL OR d.company_id = sqlc.narg('company_id')::uuid)
      AND (sqlc.narg('competence')::date IS NULL OR d.competence = sqlc.narg('competence')::date
           OR (d.period_start < (sqlc.narg('competence')::date + interval '1 month')
               AND COALESCE(d.period_end, d.period_start) >= sqlc.narg('competence')::date)
           OR EXISTS (SELECT 1 FROM client_entries e WHERE e.document_id = d.id
                      AND date_trunc('month', e.entry_date)::date = sqlc.narg('competence')::date))
), per_doc AS (
    SELECT d.kind,
           COALESCE((SELECT max(l.attempt) FROM extraction_logs l
                     WHERE l.document_id = d.id AND l.alias IN ('extrator', 'extrator_forte')), 0) AS agent_attempts,
           (SELECT sum(l.cost_usd) FROM extraction_logs l WHERE l.document_id = d.id) AS cost,
           CASE WHEN d.kind = 'extrato'
                THEN EXISTS (SELECT 1 FROM statement_imports i WHERE i.document_id = d.id)
                ELSE EXISTS (SELECT 1 FROM client_entries e WHERE e.document_id = d.id AND e.status = 'extraido'
                             AND NOT EXISTS (SELECT 1 FROM jsonb_each(COALESCE(e.field_origin, '{}'::jsonb)) f
                                             WHERE jsonb_typeof(f.value) = 'object' AND f.value ->> 'origem' = 'humano'))
           END AS approved,
           EXISTS (SELECT 1 FROM client_entries e WHERE e.document_id = d.id) AS has_entry
    FROM docs d
)
SELECT kind::text AS kind,
       count(*) FILTER (WHERE approved OR has_entry OR agent_attempts > 0)::bigint AS documents,
       count(*) FILTER (WHERE (approved OR has_entry) AND agent_attempts = 0)::bigint AS local_reader,
       count(*) FILTER (WHERE agent_attempts > 0)::bigint AS agent,
       count(*) FILTER (WHERE agent_attempts = 1 AND approved)::bigint AS first_attempt,
       COALESCE(sum(agent_attempts) FILTER (WHERE agent_attempts > 0), 0)::bigint AS agent_attempts,
       count(*) FILTER (WHERE agent_attempts > 0 AND NOT approved)::bigint AS not_approved,
       COALESCE(sum(cost), 0)::numeric(18,8) AS cost_usd
FROM per_doc
GROUP BY kind
ORDER BY kind;

-- name: GetCorrectionMetrics :one
-- As correções humanas depois da leitura: campo do comprovante corrigido e "que documento é
-- esse?" (o log de extração com o alias humano), nos documentos do filtro.
SELECT count(*)::bigint AS actions, count(DISTINCT l.document_id)::bigint AS documents
FROM extraction_logs l
JOIN reconciliation_documents d ON d.id = l.document_id
WHERE l.alias = 'humano'
  AND (CASE WHEN l.change IS JSON OBJECT THEN l.change::jsonb ->> 'acao' END) IN ('corrigir_campos', 'identificar')
  AND (sqlc.narg('company_id')::uuid IS NULL OR d.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('competence')::date IS NULL OR d.competence = sqlc.narg('competence')::date
       OR (d.period_start < (sqlc.narg('competence')::date + interval '1 month')
           AND COALESCE(d.period_end, d.period_start) >= sqlc.narg('competence')::date)
       OR EXISTS (SELECT 1 FROM client_entries e WHERE e.document_id = d.id
                  AND date_trunc('month', e.entry_date)::date = sqlc.narg('competence')::date));

-- name: GetAfterCloseMetrics :one
-- Depois do fechamento (decisão do usuário, D-034): as reaberturas do Contábil (sempre com
-- motivo) nas competências em que a empresa tem extrato ou documento na Conciliação e, à parte,
-- os documentos que chegaram com a competência fechada (pendência de período fechado).
SELECT
    (SELECT count(*) FROM company_module_period_events ev
     JOIN company_module_periods p ON p.id = ev.company_module_period_id
     JOIN competencies c ON c.id = p.competence_id
     WHERE p.module_code = 'ACCOUNTING' AND ev.event_type = 'REOPENED'
       AND (sqlc.narg('company_id')::uuid IS NULL OR p.company_id = sqlc.narg('company_id')::uuid)
       AND (sqlc.narg('competence')::date IS NULL OR c.first_day = sqlc.narg('competence')::date)
       AND (EXISTS (SELECT 1 FROM statement_transactions t WHERE t.company_id = p.company_id AND t.competence = c.first_day)
            OR EXISTS (SELECT 1 FROM reconciliation_documents d WHERE d.company_id = p.company_id
                       AND (d.competence = c.first_day
                            OR (d.period_start < (c.first_day + interval '1 month')
                                AND COALESCE(d.period_end, d.period_start) >= c.first_day))))
    )::bigint AS reopenings,
    (SELECT count(*) FROM reconciliation_documents d
     WHERE d.status = 'pendencia_periodo_fechado'
       AND (sqlc.narg('company_id')::uuid IS NULL OR d.company_id = sqlc.narg('company_id')::uuid)
       AND (sqlc.narg('competence')::date IS NULL OR d.competence = sqlc.narg('competence')::date
            OR (d.period_start < (sqlc.narg('competence')::date + interval '1 month')
                AND COALESCE(d.period_end, d.period_start) >= sqlc.narg('competence')::date))
    )::bigint AS late_documents;

-- name: ListAgentRunMetrics :many
-- As corridas e o custo do resolvedor e do classificador (M4.7 da origem: o custo por item).
SELECT agent::text AS agent, count(*)::bigint AS runs, COALESCE(sum(cost_usd), 0)::numeric(18,8) AS cost_usd
FROM ai_runs
WHERE agent IN ('resolvedor', 'classificador')
  AND (sqlc.narg('company_id')::uuid IS NULL OR company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('competence')::date IS NULL OR competence = sqlc.narg('competence')::date)
GROUP BY agent
ORDER BY agent;

-- name: ListCompetenceStatements :many
-- Os extratos importados da empresa que cobrem a competência, com as linhas de cada um.
SELECT i.id, i.document_id, d.file_name, i.bank, i.branch, i.account, i.period_start, i.period_end,
       i.opening_balance, i.closing_balance, i.attempts,
       (SELECT count(*) FROM statement_transactions t WHERE t.statement_import_id = i.id)::bigint AS transactions
FROM statement_imports i
JOIN reconciliation_documents d ON d.id = i.document_id
WHERE i.company_id = sqlc.arg('company_id')
  AND i.period_start < (sqlc.arg('competence')::date + interval '1 month')
  AND i.period_end >= sqlc.arg('competence')::date
ORDER BY i.bank, i.branch, i.account, i.period_start;

-- name: GetCompetenceClassification :one
-- As linhas conciliadas da competência: classificadas (confirmadas) e à espera de conta.
SELECT count(*) FILTER (WHERE t.account_code IS NOT NULL AND NOT t.classification_pending)::bigint AS classified,
       count(*) FILTER (WHERE t.account_code IS NULL OR t.classification_pending)::bigint AS pending
FROM statement_transactions t
WHERE t.company_id = sqlc.arg('company_id') AND t.competence = sqlc.arg('competence')::date
  AND EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL);

-- name: GetCompetenceEntries :one
-- Os lançamentos do cliente da competência e os parciais (a revisar).
SELECT count(*)::bigint AS total, count(*) FILTER (WHERE e.status = 'parcial')::bigint AS partial
FROM client_entries e
WHERE e.company_id = sqlc.arg('company_id') AND date_trunc('month', e.entry_date)::date = sqlc.arg('competence')::date;

-- name: ListCompanyBankAccounts :many
-- As contas bancárias cadastradas da empresa, com os extratos já importados de cada uma.
SELECT b.id, b.company_id, b.bank, b.branch, b.account, b.active, b.created_by, b.created_at,
       (SELECT count(*) FROM statement_imports i
        WHERE i.company_id = b.company_id AND i.bank = b.bank AND i.branch = b.branch AND i.account = b.account)::bigint AS statements,
       -- Texto vazio sem extrato (o sqlc não vê o nulo do LEFT JOIN LATERAL).
       COALESCE(to_char(last.period_end, 'YYYY-MM-DD'), '')::text AS last_period_end
FROM bank_accounts b
LEFT JOIN LATERAL (
    SELECT i.period_end FROM statement_imports i
    WHERE i.company_id = b.company_id AND i.bank = b.bank AND i.branch = b.branch AND i.account = b.account
    ORDER BY i.period_end DESC
    LIMIT 1
) last ON true
WHERE b.company_id = $1
ORDER BY b.bank, b.branch, b.account;

-- name: ListReadBankAccounts :many
-- As contas que os extratos importados da empresa leram e que ninguém cadastrou: o cadastro com um
-- clique grava banco, agência e conta exatamente como a leitura (D-034).
SELECT i.bank, i.branch, i.account, count(*)::bigint AS statements, max(i.period_end)::date AS last_period_end
FROM statement_imports i
WHERE i.company_id = $1
  AND NOT EXISTS (SELECT 1 FROM bank_accounts b WHERE b.bank = i.bank AND b.branch = i.branch AND b.account = i.account)
GROUP BY i.bank, i.branch, i.account
ORDER BY i.bank, i.branch, i.account;

-- name: InsertBankAccount :one
INSERT INTO bank_accounts (company_id, bank, branch, account, created_by, created_at, updated_at)
VALUES (sqlc.arg('company_id'), sqlc.arg('bank'), sqlc.arg('branch'), sqlc.arg('account'), sqlc.narg('created_by'),
        sqlc.arg('now'), sqlc.arg('now'))
ON CONFLICT (bank, branch, account) DO NOTHING
RETURNING *;

-- name: GetBankAccountOwner :one
-- A empresa dona da conta (a identidade é única no sistema).
SELECT b.id, b.company_id, c.internal_code, c.legal_name
FROM bank_accounts b
JOIN companies c ON c.id = b.company_id
WHERE b.bank = $1 AND b.branch = $2 AND b.account = $3;
