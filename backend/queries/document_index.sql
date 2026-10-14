-- A área Documentos consolidada (TASK-026, D-040): um índice de navegação sobre os documentos de
-- cada módulo, cada um no seu armazenamento e com o seu estado (REQ-037, REQ-043, REQ-044). Não há
-- tabela nova nem cópia de arquivo: a consulta junta as tabelas de cada módulo. A situação é só
-- leitura (decisão do usuário): Fator R — recebido e extraído em processamento, em revisão,
-- vinculado concluído, rejeitado descartado; Conciliação — pendente e triado em processamento, as
-- revisões e a pendência de período fechado em revisão, extraído e importado concluídos,
-- duplicado descartado; Controle — o arquivo da procuração ou do certificado, arquivado.

-- name: ListDocumentIndex :many
WITH docs AS (
    SELECT 'factor_r'::text AS module, 'local'::text AS storage_backend, d.id, 'pgdas'::text AS kind, d.status,
           CASE d.status WHEN 'linked' THEN 'concluido' WHEN 'needs_review' THEN 'em_revisao' WHEN 'rejected' THEN 'descartado'
                ELSE 'em_processamento' END AS situation,
           d.company_id, d.pa AS competence, NULL::date AS period_start, NULL::date AS period_end, NULL::text AS bank,
           COALESCE(d.original_name, 'extrato-pgdas')::text AS file_name, NULL::date AS expires_at, d.created_at AS received_at, d.updated_at
    FROM factor_r_pgdas_documents d
    UNION ALL
    SELECT 'reconciliation', 'minio', d.id, COALESCE(d.kind, 'desconhecido'), d.status,
           CASE WHEN d.status IN ('pendente', 'triado') THEN 'em_processamento'
                WHEN d.status IN ('revisao_triagem', 'revisao_extracao', 'pendencia_periodo_fechado') THEN 'em_revisao'
                WHEN d.status = 'duplicado' THEN 'descartado' ELSE 'concluido' END,
           d.company_id, d.competence, d.period_start, d.period_end, d.bank, d.file_name, NULL::date, d.created_at, d.updated_at
    FROM reconciliation_documents d
    UNION ALL
    SELECT 'credentials', 'local', c.id, CASE c.type WHEN 'RFB_POWER_OF_ATTORNEY' THEN 'procuracao' ELSE 'certificado' END,
           'arquivado', 'concluido', c.company_id, NULL::date, NULL::date, NULL::date, NULL::text, c.file_name, c.expires_at,
           COALESCE(c.file_uploaded_at, c.updated_at), c.updated_at
    FROM credential_controls c
    WHERE c.file_name IS NOT NULL
), filtered AS (
    SELECT x.* FROM docs x
    WHERE (sqlc.narg('company_id')::uuid IS NULL OR x.company_id = sqlc.narg('company_id')::uuid)
      AND (sqlc.narg('kind')::text IS NULL OR x.kind = sqlc.narg('kind')::text)
      AND (sqlc.narg('bank')::text IS NULL OR lower(x.bank) = lower(sqlc.narg('bank')::text))
      AND (sqlc.narg('competence')::date IS NULL OR x.competence = sqlc.narg('competence')::date
           OR (x.period_start < (sqlc.narg('competence')::date + interval '1 month')
               AND COALESCE(x.period_end, x.period_start) >= sqlc.narg('competence')::date))
)
SELECT f.module, f.storage_backend, f.id, f.kind, f.status, f.situation, f.company_id,
       COALESCE(co.internal_code, '')::text AS company_code, COALESCE(co.legal_name, '')::text AS company_name,
       f.competence, f.period_start, f.period_end, f.bank, f.file_name, f.expires_at, f.received_at, f.updated_at,
       count(*) OVER ()::int AS total
FROM filtered f
LEFT JOIN companies co ON co.id = f.company_id
WHERE (sqlc.narg('module')::text IS NULL OR f.module = sqlc.narg('module')::text)
  AND (sqlc.narg('situation')::text IS NULL OR f.situation = sqlc.narg('situation')::text)
  AND (sqlc.narg('status')::text IS NULL OR f.status = sqlc.narg('status')::text)
ORDER BY f.received_at DESC, f.id
LIMIT sqlc.arg('row_limit') OFFSET sqlc.arg('row_offset');

-- name: CountDocumentIndex :many
-- As contagens por módulo, situação e status com os mesmos filtros de empresa, tipo, banco e
-- competência (os botões de filtro da área).
WITH docs AS (
    SELECT 'factor_r'::text AS module, d.status,
           CASE d.status WHEN 'linked' THEN 'concluido' WHEN 'needs_review' THEN 'em_revisao' WHEN 'rejected' THEN 'descartado'
                ELSE 'em_processamento' END AS situation,
           'pgdas'::text AS kind, d.company_id, d.pa AS competence, NULL::date AS period_start, NULL::date AS period_end, NULL::text AS bank
    FROM factor_r_pgdas_documents d
    UNION ALL
    SELECT 'reconciliation', d.status,
           CASE WHEN d.status IN ('pendente', 'triado') THEN 'em_processamento'
                WHEN d.status IN ('revisao_triagem', 'revisao_extracao', 'pendencia_periodo_fechado') THEN 'em_revisao'
                WHEN d.status = 'duplicado' THEN 'descartado' ELSE 'concluido' END,
           COALESCE(d.kind, 'desconhecido'), d.company_id, d.competence, d.period_start, d.period_end, d.bank
    FROM reconciliation_documents d
    UNION ALL
    SELECT 'credentials', 'arquivado', 'concluido', CASE c.type WHEN 'RFB_POWER_OF_ATTORNEY' THEN 'procuracao' ELSE 'certificado' END,
           c.company_id, NULL::date, NULL::date, NULL::date, NULL::text
    FROM credential_controls c
    WHERE c.file_name IS NOT NULL
)
SELECT x.module, x.situation, x.status, count(*)::int AS total
FROM docs x
WHERE (sqlc.narg('company_id')::uuid IS NULL OR x.company_id = sqlc.narg('company_id')::uuid)
  AND (sqlc.narg('kind')::text IS NULL OR x.kind = sqlc.narg('kind')::text)
  AND (sqlc.narg('bank')::text IS NULL OR lower(x.bank) = lower(sqlc.narg('bank')::text))
  AND (sqlc.narg('competence')::date IS NULL OR x.competence = sqlc.narg('competence')::date
       OR (x.period_start < (sqlc.narg('competence')::date + interval '1 month')
           AND COALESCE(x.period_end, x.period_start) >= sqlc.narg('competence')::date))
GROUP BY x.module, x.situation, x.status
ORDER BY x.module, x.situation, x.status;
