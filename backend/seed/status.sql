-- Conjuntos de status [PRD §11.1-§11.5]. Idempotente (ON CONFLICT DO UPDATE).
-- Colunas: código, label, ordem, iniciado?, conta no progresso?, final?, pendência (bloqueia)?, fora do denominador?

INSERT INTO status_sets (code, name) VALUES
    ('FISCAL_CLOSING', 'Fiscal — Fechamento'),
    ('FISCAL_ITEM',    'Fiscal — Controles individuais'),
    ('DECLARATION',    'Declarações / DCTFWeb'),
    ('ACCOUNTING',     'Contábil'),
    ('PAYROLL',        'Folha')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name;

INSERT INTO status_definitions
    (status_set_id, code, label, sort_order, counts_as_started, counts_as_operational_progress, is_final, is_blocking, excluded_from_denominator)
SELECT s.id, v.code, v.label, v.sort_order, v.started, v.progress, v.final, v.blocking, v.excluded
FROM (VALUES
    -- §11.1 Fiscal — Fechamento
    ('FISCAL_CLOSING', 'NOT_STARTED',       'A iniciar',            1, false, false, false, false, false),
    ('FISCAL_CLOSING', 'DOWNLOADED',        'Baixado',              2, true,  true,  false, false, false),
    ('FISCAL_CLOSING', 'POSTED',            'Lançado',              3, true,  true,  false, false, false),
    ('FISCAL_CLOSING', 'CHECKED',           'Conferido',            4, true,  true,  false, false, false),
    ('FISCAL_CLOSING', 'MISSING_DOCUMENTS', 'Faltando documentos',  5, true,  false, false, true,  false),
    ('FISCAL_CLOSING', 'ERROR',             'Erro',                 6, true,  false, false, true,  false),
    ('FISCAL_CLOSING', 'CLOSED',            'Encerrado',            7, true,  true,  true,  false, false),
    -- §11.2 Fiscal — Controles individuais
    ('FISCAL_ITEM', 'NOT_STARTED',       'A iniciar',            1, false, false, false, false, false),
    ('FISCAL_ITEM', 'DOWNLOADED',        'Baixadas',             2, true,  true,  false, false, false),
    ('FISCAL_ITEM', 'POSTED',            'Lançadas',             3, true,  true,  false, false, false),
    ('FISCAL_ITEM', 'CLOSED',            'Fechadas',             4, true,  true,  true,  false, false),
    ('FISCAL_ITEM', 'MISSING_DOCUMENTS', 'Documentos faltantes', 5, true,  false, false, true,  false),
    ('FISCAL_ITEM', 'NO_MOVEMENT',       'Sem movimento',        6, true,  true,  true,  false, false),
    ('FISCAL_ITEM', 'NOT_APPLICABLE',    'Não se aplica',        7, true,  false, true,  false, true),
    -- §11.3 Declarações / DCTFWeb
    ('DECLARATION', 'NOT_STARTED', 'A iniciar',     1, false, false, false, false, false),
    ('DECLARATION', 'TRANSMITTED', 'Transmitida',   2, true,  true,  true,  false, false),
    ('DECLARATION', 'CLOSED',      'Fechado',       3, true,  true,  true,  false, false),
    ('DECLARATION', 'PENDING',     'Pendente',      4, true,  false, false, true,  false),
    ('DECLARATION', 'EXEMPT',      'Dispensado',    5, true,  true,  true,  false, false),
    ('DECLARATION', 'NO_MOVEMENT', 'Sem movimento', 6, true,  true,  true,  false, false),
    -- §11.4 Contábil
    ('ACCOUNTING', 'NOT_STARTED', 'A iniciar',     1, false, false, false, false, false),
    ('ACCOUNTING', 'PENDING',     'Pendente',      2, true,  false, false, true,  false),
    ('ACCOUNTING', 'NO_MOVEMENT', 'Sem movimento', 3, true,  true,  true,  false, false),
    ('ACCOUNTING', 'OK',          'OK',            4, true,  true,  true,  false, false),
    -- §11.5 Folha
    ('PAYROLL', 'NOT_STARTED',    'A iniciar',          1, false, false, false, false, false),
    ('PAYROLL', 'IN_PROGRESS',    'Em andamento',       2, true,  true,  false, false, false),
    ('PAYROLL', 'SENT_TO_CLIENT', 'Enviada ao Cliente', 3, true,  true,  false, false, false),
    ('PAYROLL', 'CLOSED',         'Fechada',            4, true,  true,  true,  false, false),
    ('PAYROLL', 'ARCHIVED',       'Salvo na pasta',     5, true,  true,  true,  false, false),
    ('PAYROLL', 'PENDING',        'Pendências',         6, true,  false, false, true,  false),
    ('PAYROLL', 'NO_MOVEMENT',    'Sem movimento',      7, true,  true,  true,  false, false)
) AS v(set_code, code, label, sort_order, started, progress, final, blocking, excluded)
JOIN status_sets s ON s.code = v.set_code
ON CONFLICT (status_set_id, code) DO UPDATE SET
    label                          = EXCLUDED.label,
    sort_order                     = EXCLUDED.sort_order,
    counts_as_started              = EXCLUDED.counts_as_started,
    counts_as_operational_progress = EXCLUDED.counts_as_operational_progress,
    is_final                       = EXCLUDED.is_final,
    is_blocking                    = EXCLUDED.is_blocking,
    excluded_from_denominator      = EXCLUDED.excluded_from_denominator;
