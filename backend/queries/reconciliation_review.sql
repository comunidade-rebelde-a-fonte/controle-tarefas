-- Fila de revisão da Conciliação (TASK-021a, D-032): os itens por balde, a linha com os
-- candidatos e as ações humanas (conciliar à mão, sem comprovante, desfazer, corrigir o campo do
-- comprovante e dizer que documento é) [REQ-019, REQ-023, REQ-024, REQ-030; 08 §7].

-- name: ListReviewQueue :many
-- Os itens da fila, de todos os baldes ou de um: documentos e comprovantes parciais em revisão,
-- linhas livres marcadas (divergência, borda, desfeita) ou não (extrato sem comprovante),
-- comprovantes completos sem extrato e linhas conciliadas à espera da classificação.
WITH items AS (
    SELECT 'documento'::text AS kind, d.id, 'revisao'::text AS bucket, d.company_id, d.status AS state,
           NULL::date AS item_date, NULL::numeric AS amount, NULL::text AS direction, d.file_name AS summary,
           COALESCE(d.diagnosis->>'motivo', '')::text AS reason,
           COALESCE(d.competence, date_trunc('month', d.period_start)::date) AS competence, d.created_at
    FROM reconciliation_documents d
    WHERE d.status IN ('revisao_triagem', 'revisao_extracao', 'pendencia_periodo_fechado')
    UNION ALL
    SELECT 'comprovante', e.id, CASE WHEN e.status = 'parcial' THEN 'revisao' ELSE 'comprovante_sem' END, e.company_id, e.status,
           e.entry_date, e.amount, e.direction, COALESCE(e.counterparty, ''),
           CASE WHEN e.status = 'parcial' THEN 'campos_a_revisar' ELSE '' END,
           date_trunc('month', e.entry_date)::date, e.created_at
    FROM client_entries e
    WHERE e.status IN ('parcial', 'extraido', 'manual')
      AND NOT EXISTS (
          SELECT 1 FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
          WHERE i.client_entry_id = e.id AND i.released_at IS NULL AND r.undone_at IS NULL)
    UNION ALL
    SELECT 'linha', t.id,
           CASE WHEN m.reason IS NOT NULL OR m.kind IS NOT NULL THEN 'divergencia' ELSE 'extrato_sem' END,
           t.company_id, COALESCE(m.kind, 'livre'), t.txn_date, t.amount, t.direction::text, t.description,
           COALESCE(m.reason, ''), t.competence, t.created_at
    FROM statement_transactions t
    LEFT JOIN LATERAL (
        SELECT x->>'tipo' AS kind, COALESCE(x->>'motivo', x->>'tipo') AS reason
        FROM jsonb_array_elements(t.flags) x
        WHERE jsonb_typeof(x) = 'object' AND x->>'tipo' IN ('divergencia', 'borda', 'desfeita')
        LIMIT 1
    ) m ON true
    WHERE NOT EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
    UNION ALL
    SELECT 'linha', t.id, 'classificacao', t.company_id, 'conciliada', t.txn_date, t.amount, t.direction::text, t.description,
           'sem_regra_nem_historico', t.competence, t.created_at
    FROM statement_transactions t
    WHERE (t.classification_origin IS NULL OR t.classification_pending)
      AND EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
)
SELECT i.kind, i.id, i.bucket, i.company_id, c.internal_code AS company_code, c.legal_name AS company_name, i.state,
       i.item_date, i.amount, i.direction, i.summary, i.reason, i.competence
FROM items i
LEFT JOIN companies c ON c.id = i.company_id
WHERE (sqlc.narg('company_id')::uuid IS NULL OR i.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('competence')::date IS NULL OR i.competence = sqlc.narg('competence')::date OR i.competence IS NULL)
  AND (sqlc.narg('bucket')::text IS NULL OR i.bucket = sqlc.narg('bucket')::text)
ORDER BY CASE i.bucket WHEN 'revisao' THEN 1 WHEN 'divergencia' THEN 2 WHEN 'extrato_sem' THEN 3 WHEN 'comprovante_sem' THEN 4 ELSE 5 END,
         i.item_date NULLS FIRST, i.created_at, i.id
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountReviewQueue :many
-- As contagens por balde, com os mesmos filtros de empresa e competência.
WITH items AS (
    SELECT 'revisao'::text AS bucket, d.company_id, COALESCE(d.competence, date_trunc('month', d.period_start)::date) AS competence
    FROM reconciliation_documents d
    WHERE d.status IN ('revisao_triagem', 'revisao_extracao', 'pendencia_periodo_fechado')
    UNION ALL
    SELECT CASE WHEN e.status = 'parcial' THEN 'revisao' ELSE 'comprovante_sem' END, e.company_id, date_trunc('month', e.entry_date)::date
    FROM client_entries e
    WHERE e.status IN ('parcial', 'extraido', 'manual')
      AND NOT EXISTS (
          SELECT 1 FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
          WHERE i.client_entry_id = e.id AND i.released_at IS NULL AND r.undone_at IS NULL)
    UNION ALL
    SELECT CASE WHEN EXISTS (
               SELECT 1 FROM jsonb_array_elements(t.flags) x
               WHERE jsonb_typeof(x) = 'object' AND x->>'tipo' IN ('divergencia', 'borda', 'desfeita'))
           THEN 'divergencia' ELSE 'extrato_sem' END, t.company_id, t.competence
    FROM statement_transactions t
    WHERE NOT EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
    UNION ALL
    SELECT 'classificacao', t.company_id, t.competence
    FROM statement_transactions t
    WHERE (t.classification_origin IS NULL OR t.classification_pending)
      AND EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
)
SELECT i.bucket, count(*) AS total
FROM items i
WHERE (sqlc.narg('company_id')::uuid IS NULL OR i.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('competence')::date IS NULL OR i.competence = sqlc.narg('competence')::date OR i.competence IS NULL)
GROUP BY i.bucket;

-- name: GetReviewLine :one
-- A linha do extrato na fila: a empresa, o extrato (conta bancária e original), as marcas e a
-- conciliação ativa, se houver.
SELECT t.id, t.company_id, c.internal_code AS company_code, c.legal_name AS company_name, t.txn_date, t.competence, t.position,
       t.description, t.document_ref, t.amount, t.direction, t.balance_after, t.flags, t.account_code, t.classification_origin,
       t.classification_pending,
       si.id AS statement_import_id, si.document_id AS statement_document_id, si.bank, si.branch, si.account AS bank_account,
       r.id AS reconciliation_id, r.method AS reconciliation_method, r.origin AS reconciliation_origin, r.pass AS reconciliation_pass,
       r.created_at AS reconciled_at, r.created_by AS reconciled_by
FROM statement_transactions t
JOIN companies c ON c.id = t.company_id
JOIN statement_imports si ON si.id = t.statement_import_id
LEFT JOIN reconciliations r ON r.statement_transaction_id = t.id AND r.undone_at IS NULL
WHERE t.id = sqlc.arg('id');

-- name: ListLineCandidateEntries :many
-- Os lançamentos livres da empresa (candidatos da linha: mesmo valor e sentido em D±3, mais os
-- que o matching anotou), com o documento de origem.
SELECT e.id, e.entry_date, e.amount, e.direction, e.counterparty, e.counterparty_doc, e.document_ref, e.operation_type, e.status,
       e.document_id, e.flags
FROM client_entries e
WHERE e.company_id = sqlc.arg('company_id')
  AND NOT EXISTS (
      SELECT 1 FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
      WHERE i.client_entry_id = e.id AND i.released_at IS NULL AND r.undone_at IS NULL)
ORDER BY e.entry_date NULLS LAST, e.id;

-- name: ListEntryCandidateLines :many
-- As linhas livres da empresa com o valor e o sentido do comprovante em D±3 (os candidatos do
-- comprovante sem extrato).
SELECT t.id, t.txn_date, t.amount, t.direction, t.description, si.bank, si.branch, si.account AS bank_account
FROM statement_transactions t
JOIN statement_imports si ON si.id = t.statement_import_id
WHERE t.company_id = sqlc.arg('company_id') AND t.amount = sqlc.arg('amount') AND t.direction = sqlc.arg('direction')
  AND t.txn_date BETWEEN sqlc.arg('date_from') AND sqlc.arg('date_to')
  AND NOT EXISTS (SELECT 1 FROM reconciliations r WHERE r.statement_transaction_id = t.id AND r.undone_at IS NULL)
ORDER BY abs(t.txn_date - sqlc.arg('entry_date')::date), t.txn_date, t.id;

-- name: GetReconciliationForUpdate :one
SELECT r.*, t.txn_date FROM reconciliations r JOIN statement_transactions t ON t.id = r.statement_transaction_id
WHERE r.id = $1 FOR UPDATE OF r;

-- name: UndoReconciliation :exec
-- Desfazer não apaga (D-020): marca quem, quando e por quê.
UPDATE reconciliations SET undone_at = sqlc.arg('now'), undone_by = sqlc.arg('undone_by'), undo_reason = sqlc.arg('reason')
WHERE id = sqlc.arg('id') AND undone_at IS NULL;

-- name: ReleaseReconciliationItems :exec
UPDATE reconciliation_items SET released_at = sqlc.arg('now') WHERE reconciliation_id = sqlc.arg('reconciliation_id') AND released_at IS NULL;

-- name: ResetLineClassification :exec
-- A linha desconciliada perde a classificação (ela dependia dos comprovantes): conciliada de novo,
-- passa outra vez por regra → histórico → agente.
UPDATE statement_transactions
SET account_code = NULL, classification_origin = NULL, classification_pending = false, classification_rule_id = NULL,
    classification_history_id = NULL, classification_suggestion_id = NULL, classified_by = NULL, classified_at = NULL
WHERE id = sqlc.arg('id');

-- name: AppendStatementTransactionMark :exec
UPDATE statement_transactions SET flags = flags || jsonb_build_array(sqlc.arg('mark')::jsonb) WHERE id = sqlc.arg('id');

-- name: ListRecentReconciliations :many
-- As últimas conciliações ativas (a origem listava as 8 últimas manuais para desfazer).
SELECT r.id, r.company_id, c.internal_code AS company_code, r.method, r.origin, r.pass, r.created_by, r.created_at,
       t.id AS statement_transaction_id, t.txn_date, t.amount, t.direction, t.description
FROM reconciliations r
JOIN statement_transactions t ON t.id = r.statement_transaction_id
JOIN companies c ON c.id = r.company_id
WHERE r.undone_at IS NULL
  AND (sqlc.narg('company_id')::uuid IS NULL OR r.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('origin')::text IS NULL OR r.origin = sqlc.narg('origin')::text)
ORDER BY r.created_at DESC, r.id DESC
LIMIT sqlc.arg('row_limit');

-- name: GetClientEntryForUpdate :one
SELECT e.*, EXISTS (
    SELECT 1 FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
    WHERE i.client_entry_id = e.id AND i.released_at IS NULL AND r.undone_at IS NULL) AS reconciled
FROM client_entries e WHERE e.id = $1 FOR UPDATE OF e;

-- name: UpdateClientEntryFields :one
-- A correção humana dos campos do comprovante (a fila campo a campo).
UPDATE client_entries
SET entry_date = sqlc.narg('entry_date'), amount = sqlc.narg('amount'), direction = sqlc.narg('direction'),
    counterparty = sqlc.narg('counterparty'), counterparty_doc = sqlc.narg('counterparty_doc'), document_ref = sqlc.narg('document_ref'),
    status = sqlc.arg('status'), field_confidence = sqlc.narg('field_confidence'), field_origin = sqlc.narg('field_origin'),
    updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: IdentifyReconciliationDocument :exec
-- "Que documento é esse?": o tipo, a empresa e os dados do extrato informados pelo contador (os
-- que ele não informa ficam como a triagem leu); o documento volta para a extração.
UPDATE reconciliation_documents
SET kind = sqlc.arg('kind'), company_id = sqlc.narg('company_id'), bank = COALESCE(sqlc.narg('bank'), bank),
    branch = COALESCE(sqlc.narg('branch'), branch), account = COALESCE(sqlc.narg('account'), account),
    period_start = COALESCE(sqlc.narg('period_start'), period_start), period_end = COALESCE(sqlc.narg('period_end'), period_end),
    status = sqlc.arg('status'), diagnosis = NULL, job_id = sqlc.narg('job_id'), updated_at = sqlc.arg('now')
WHERE id = sqlc.arg('id');

-- name: ListActiveReconciliationsOfEntry :many
-- A conciliação ativa do lançamento (no máximo uma, pelo índice da 0020).
SELECT r.id, r.statement_transaction_id
FROM reconciliation_items i JOIN reconciliations r ON r.id = i.reconciliation_id
WHERE i.client_entry_id = $1 AND i.released_at IS NULL AND r.undone_at IS NULL;
