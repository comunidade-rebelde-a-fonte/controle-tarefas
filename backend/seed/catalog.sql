-- Catálogo inicial de obrigações [PRD §13-§18, §50; plan D1, D2, D3, D5, D7].
-- Idempotente (ON CONFLICT DO UPDATE): rodar `make seed` reaplica estes valores de referência.
--
-- Valores PROVISÓRIOS aguardando o escritório (T-000), registrados em docs/pendencias.md:
--   Q1 regras de vencimento · Q5 status "Sem folha" · Q6 matriz declarações × regime.

------------------------------------------------------------------------------------------
-- Regras de vencimento
-- A VALIDAR (Q1): regras PROVISÓRIAS e genéricas, apenas para o motor funcionar. NÃO são
-- prazos legais; os prazos reais de cada obrigação serão informados pelo escritório.
------------------------------------------------------------------------------------------
INSERT INTO due_date_rules (name, month_offset, base_day, day_type, non_business_day_adjustment, notes) VALUES
    ('Provisória — dia 10 do mês seguinte',             1, 10, 'CALENDAR', 'PREVIOUS', 'A VALIDAR (Q1)'),
    ('Provisória — dia 15 do mês seguinte',             1, 15, 'CALENDAR', 'PREVIOUS', 'A VALIDAR (Q1)'),
    ('Provisória — dia 20 do mês seguinte',             1, 20, 'CALENDAR', 'PREVIOUS', 'A VALIDAR (Q1)'),
    ('Provisória — dia 25 do mês seguinte',             1, 25, 'CALENDAR', 'PREVIOUS', 'A VALIDAR (Q1)'),
    ('Provisória — 5º dia útil do mês seguinte',        1,  5, 'BUSINESS', 'NONE',     'A VALIDAR (Q1)'),
    ('Provisória — último dia útil do mês seguinte',    1, 31, 'CALENDAR', 'PREVIOUS', 'A VALIDAR (Q1)')
ON CONFLICT (name) DO UPDATE SET
    month_offset                = EXCLUDED.month_offset,
    base_day                    = EXCLUDED.base_day,
    day_type                    = EXCLUDED.day_type,
    non_business_day_adjustment = EXCLUDED.non_business_day_adjustment,
    notes                       = EXCLUDED.notes,
    updated_at                  = now();

------------------------------------------------------------------------------------------
-- Tipos de obrigação
-- colunas: código, nome, módulo principal, conjunto de status, ordem, regra de vencimento,
--          permite transmissão, permite protocolo, estágio de fechamento (D1)
------------------------------------------------------------------------------------------
INSERT INTO obligation_types
    (code, name, primary_module, status_set_id, sort_order, due_date_rule_id,
     allows_transmission_date, allows_protocol, is_closing_stage, counts_in_progress, initial_status_code)
SELECT v.code, v.name, v.module, s.id, v.sort_order, r.id, v.transmission, v.protocol, v.closing, NOT v.closing, 'NOT_STARTED'
FROM (VALUES
    -- Fiscal [§13, §14, §50.1-§50.2]. FISCAL_FECHAMENTO = coluna "Fechamento" (D1).
    ('FISCAL_FECHAMENTO',        'Fechamento',       'FISCAL', 'FISCAL_CLOSING', 10,  'Provisória — dia 25 do mês seguinte',          false, false, true),
    ('FISCAL_ENTRADAS',          'Entradas',         'FISCAL', 'FISCAL_ITEM',    20,  'Provisória — dia 10 do mês seguinte',          false, false, false),
    ('FISCAL_SAIDAS',            'Saídas',           'FISCAL', 'FISCAL_ITEM',    30,  'Provisória — dia 10 do mês seguinte',          false, false, false),
    ('FISCAL_PRESTADOS',         'Prestados',        'FISCAL', 'FISCAL_ITEM',    40,  'Provisória — dia 10 do mês seguinte',          false, false, false),
    ('FISCAL_TOMADOS',           'Tomados',          'FISCAL', 'FISCAL_ITEM',    50,  'Provisória — dia 10 do mês seguinte',          false, false, false),
    ('FISCAL_PIS',               'PIS',              'FISCAL', 'FISCAL_ITEM',    60,  'Provisória — dia 20 do mês seguinte',          false, false, false),
    ('FISCAL_COFINS',            'COFINS',           'FISCAL', 'FISCAL_ITEM',    70,  'Provisória — dia 20 do mês seguinte',          false, false, false),
    ('FISCAL_IRPJ',              'IRPJ',             'FISCAL', 'FISCAL_ITEM',    80,  'Provisória — último dia útil do mês seguinte', false, false, false),
    ('FISCAL_CSLL',              'CSLL',             'FISCAL', 'FISCAL_ITEM',    90,  'Provisória — último dia útil do mês seguinte', false, false, false),
    ('FISCAL_SIMPLES_NACIONAL',  'Simples Nacional', 'FISCAL', 'FISCAL_ITEM',    95,  'Provisória — dia 20 do mês seguinte',          false, false, false),
    ('FISCAL_ISSQN',             'ISSQN',            'FISCAL', 'FISCAL_ITEM',    100, 'Provisória — dia 15 do mês seguinte',          false, false, false),
    ('FISCAL_ICMS',              'ICMS',             'FISCAL', 'FISCAL_ITEM',    110, 'Provisória — dia 15 do mês seguinte',          false, false, false),
    ('FISCAL_IPI',               'IPI',              'FISCAL', 'FISCAL_ITEM',    120, 'Provisória — dia 25 do mês seguinte',          false, false, false),
    ('FISCAL_RETIDOS',           'Retidos',          'FISCAL', 'FISCAL_ITEM',    130, 'Provisória — dia 20 do mês seguinte',          false, false, false),
    -- Declarações [§15, §50.3]; REINF Retidos/Lucros pertencem a Declarações e aparecem só na DCTFWeb (D5).
    ('REINF',                    'REINF',             'DECLARATIONS', 'DECLARATION', 210, 'Provisória — dia 15 do mês seguinte',          true, true, false),
    ('EFD_CONTRIBUICOES',        'EFD Contribuições', 'DECLARATIONS', 'DECLARATION', 220, 'Provisória — 5º dia útil do mês seguinte',     true, true, false),
    ('SPED_FISCAL',              'SPED Fiscal',       'DECLARATIONS', 'DECLARATION', 230, 'Provisória — dia 20 do mês seguinte',          true, true, false),
    ('SINTEGRA',                 'SINTEGRA',          'DECLARATIONS', 'DECLARATION', 240, 'Provisória — dia 15 do mês seguinte',          true, true, false),
    ('MIT',                      'MIT',               'DECLARATIONS', 'DECLARATION', 250, 'Provisória — dia 15 do mês seguinte',          true, true, false),
    ('DCTFWEB',                  'DCTFWeb',           'DECLARATIONS', 'DECLARATION', 260, 'Provisória — dia 15 do mês seguinte',          true, true, false),
    ('PGDAS',                    'PGDAS',             'DECLARATIONS', 'DECLARATION', 270, 'Provisória — dia 20 do mês seguinte',          true, true, false),
    ('DESTDA',                   'DeSTDA',            'DECLARATIONS', 'DECLARATION', 280, 'Provisória — dia 20 do mês seguinte',          true, true, false),
    ('REINF_RETIDOS',            'REINF Retidos',     'DECLARATIONS', 'DECLARATION', 290, 'Provisória — dia 15 do mês seguinte',          true, true, false),
    ('REINF_LUCROS',             'REINF Lucros',      'DECLARATIONS', 'DECLARATION', 295, 'Provisória — dia 15 do mês seguinte',          true, true, false),
    -- Folha [§17, §50.5]
    ('PAYROLL_PROCESSING',       'Folha',             'PAYROLL', 'PAYROLL', 310, 'Provisória — 5º dia útil do mês seguinte', false, false, false),
    ('ESOCIAL',                  'eSocial',           'PAYROLL', 'PAYROLL', 320, 'Provisória — dia 15 do mês seguinte',      false, false, false),
    ('FGTS',                     'FGTS',              'PAYROLL', 'PAYROLL', 330, 'Provisória — dia 20 do mês seguinte',      false, false, false),
    ('INSS_IRRF',                'INSS/IRRF',         'PAYROLL', 'PAYROLL', 340, 'Provisória — dia 20 do mês seguinte',      false, false, false),
    -- Contábil [§18, §50.6], em sequência
    ('ACCOUNTING_MOVEMENT_RECEIVED',    'Movimento recebido',           'ACCOUNTING', 'ACCOUNTING', 410, 'Provisória — último dia útil do mês seguinte', false, false, false),
    ('ACCOUNTING_PAYROLL_POSTED',       'Contabilização da folha',      'ACCOUNTING', 'ACCOUNTING', 420, 'Provisória — último dia útil do mês seguinte', false, false, false),
    ('ACCOUNTING_TAXES_POSTED',         'Contabilização dos impostos',  'ACCOUNTING', 'ACCOUNTING', 430, 'Provisória — último dia útil do mês seguinte', false, false, false),
    ('ACCOUNTING_PROCESSED',            'Contábil digitado/processado', 'ACCOUNTING', 'ACCOUNTING', 440, 'Provisória — último dia útil do mês seguinte', false, false, false),
    ('ACCOUNTING_RECONCILIATION',       'Conciliação',                  'ACCOUNTING', 'ACCOUNTING', 450, 'Provisória — último dia útil do mês seguinte', false, false, false),
    ('ACCOUNTING_TRIAL_BALANCE_CLOSED', 'Fechamento do balancete',      'ACCOUNTING', 'ACCOUNTING', 460, 'Provisória — último dia útil do mês seguinte', false, false, false)
) AS v(code, name, module, set_code, sort_order, rule_name, transmission, protocol, closing)
JOIN status_sets s ON s.code = v.set_code
JOIN due_date_rules r ON r.name = v.rule_name
ON CONFLICT (code) DO UPDATE SET
    name                     = EXCLUDED.name,
    primary_module           = EXCLUDED.primary_module,
    status_set_id            = EXCLUDED.status_set_id,
    sort_order               = EXCLUDED.sort_order,
    due_date_rule_id         = EXCLUDED.due_date_rule_id,
    allows_transmission_date = EXCLUDED.allows_transmission_date,
    allows_protocol          = EXCLUDED.allows_protocol,
    is_closing_stage         = EXCLUDED.is_closing_stage,
    counts_in_progress       = EXCLUDED.counts_in_progress,
    initial_status_code      = EXCLUDED.initial_status_code,
    updated_at               = now();

-- Status aplicado na geração para empresas "Sem folha" (D7).
-- A VALIDAR (Q5): provisório — Folha e eSocial = Sem movimento; FGTS e INSS/IRRF = Não se aplica.
UPDATE obligation_types t SET status_when_no_payroll = v.status
FROM (VALUES
    ('PAYROLL_PROCESSING', 'NO_MOVEMENT'),
    ('ESOCIAL',            'NO_MOVEMENT'),
    ('FGTS',               'NOT_APPLICABLE'),
    ('INSS_IRRF',          'NOT_APPLICABLE')
) AS v(code, status)
WHERE t.code = v.code;

------------------------------------------------------------------------------------------
-- Contextos de exibição (colunas das telas) [§38.9]. A mesma instância aparece em várias telas.
------------------------------------------------------------------------------------------
INSERT INTO obligation_display_contexts (obligation_type_id, module_code, sort_order, active)
SELECT t.id, v.module, v.sort_order, true
FROM (VALUES
    ('FISCAL_FECHAMENTO', 'FISCAL', 10), ('FISCAL_ENTRADAS', 'FISCAL', 20), ('FISCAL_SAIDAS', 'FISCAL', 30),
    ('FISCAL_PRESTADOS', 'FISCAL', 40), ('FISCAL_TOMADOS', 'FISCAL', 50), ('FISCAL_PIS', 'FISCAL', 60),
    ('FISCAL_COFINS', 'FISCAL', 70), ('FISCAL_IRPJ', 'FISCAL', 80), ('FISCAL_CSLL', 'FISCAL', 90),
    ('FISCAL_SIMPLES_NACIONAL', 'FISCAL', 95), ('FISCAL_ISSQN', 'FISCAL', 100), ('FISCAL_ICMS', 'FISCAL', 110),
    ('FISCAL_IPI', 'FISCAL', 120), ('FISCAL_RETIDOS', 'FISCAL', 130),
    ('REINF', 'DECLARATIONS', 10), ('EFD_CONTRIBUICOES', 'DECLARATIONS', 20), ('SPED_FISCAL', 'DECLARATIONS', 30),
    ('SINTEGRA', 'DECLARATIONS', 40), ('MIT', 'DECLARATIONS', 50), ('DCTFWEB', 'DECLARATIONS', 60),
    ('PGDAS', 'DECLARATIONS', 70), ('DESTDA', 'DECLARATIONS', 80),
    -- Visão DCTFWeb [§16.2, §31]: MIT, REINF Retidos, REINF Lucros, eSocial e DCTFWeb.
    ('MIT', 'DCTFWEB', 10), ('REINF_RETIDOS', 'DCTFWEB', 20), ('REINF_LUCROS', 'DCTFWEB', 30),
    ('ESOCIAL', 'DCTFWEB', 40), ('DCTFWEB', 'DCTFWEB', 50),
    ('PAYROLL_PROCESSING', 'PAYROLL', 10), ('ESOCIAL', 'PAYROLL', 20), ('FGTS', 'PAYROLL', 30), ('INSS_IRRF', 'PAYROLL', 40),
    ('ACCOUNTING_MOVEMENT_RECEIVED', 'ACCOUNTING', 10), ('ACCOUNTING_PAYROLL_POSTED', 'ACCOUNTING', 20),
    ('ACCOUNTING_TAXES_POSTED', 'ACCOUNTING', 30), ('ACCOUNTING_PROCESSED', 'ACCOUNTING', 40),
    ('ACCOUNTING_RECONCILIATION', 'ACCOUNTING', 50), ('ACCOUNTING_TRIAL_BALANCE_CLOSED', 'ACCOUNTING', 60)
) AS v(code, module, sort_order)
JOIN obligation_types t ON t.code = v.code
ON CONFLICT (obligation_type_id, module_code) DO UPDATE SET
    sort_order = EXCLUDED.sort_order,
    active     = EXCLUDED.active;

------------------------------------------------------------------------------------------
-- Regras tipo × regime (D2/D3). Tipo sem regra ativa para o regime não é gerado.
------------------------------------------------------------------------------------------
WITH normais(regime) AS (
    VALUES ('LUCRO_PRESUMIDO'), ('LUCRO_REAL'), ('LUCRO_ARBITRADO'), ('IMUNE_ISENTA')
), todos(regime) AS (
    SELECT regime FROM normais UNION ALL VALUES ('SIMPLES_NACIONAL')
), regras(code, regime, counts) AS (
    -- Fiscal Normal: os 12 controles [§13.3] + Fechamento (fora dos percentuais, R3).
    SELECT c.code, n.regime, c.code <> 'FISCAL_FECHAMENTO'
    FROM (VALUES ('FISCAL_FECHAMENTO'), ('FISCAL_ENTRADAS'), ('FISCAL_SAIDAS'), ('FISCAL_PRESTADOS'), ('FISCAL_TOMADOS'),
                 ('FISCAL_PIS'), ('FISCAL_COFINS'), ('FISCAL_IRPJ'), ('FISCAL_CSLL'), ('FISCAL_ISSQN'),
                 ('FISCAL_ICMS'), ('FISCAL_IPI'), ('FISCAL_RETIDOS')) AS c(code)
    CROSS JOIN normais n
    UNION ALL
    -- Fiscal Simples: 8 controles [§14.2]; ICMS visível mas fora do progresso (RN-010, D2); Fechamento fora (R3).
    SELECT c.code, 'SIMPLES_NACIONAL', c.code NOT IN ('FISCAL_FECHAMENTO', 'FISCAL_ICMS')
    FROM (VALUES ('FISCAL_FECHAMENTO'), ('FISCAL_ENTRADAS'), ('FISCAL_SAIDAS'), ('FISCAL_PRESTADOS'), ('FISCAL_TOMADOS'),
                 ('FISCAL_SIMPLES_NACIONAL'), ('FISCAL_ISSQN'), ('FISCAL_ICMS'), ('FISCAL_RETIDOS')) AS c(code)
    UNION ALL
    -- Declarações — A VALIDAR (Q6): matriz provisória.
    --   todos os regimes: REINF, SINTEGRA, DCTFWeb, REINF Retidos, REINF Lucros
    --   só Normais: EFD Contribuições, SPED Fiscal, MIT
    --   só Simples: PGDAS, DeSTDA
    SELECT c.code, t.regime, true
    FROM (VALUES ('REINF'), ('SINTEGRA'), ('DCTFWEB'), ('REINF_RETIDOS'), ('REINF_LUCROS')) AS c(code) CROSS JOIN todos t
    UNION ALL
    SELECT c.code, n.regime, true
    FROM (VALUES ('EFD_CONTRIBUICOES'), ('SPED_FISCAL'), ('MIT')) AS c(code) CROSS JOIN normais n
    UNION ALL
    SELECT c.code, 'SIMPLES_NACIONAL', true
    FROM (VALUES ('PGDAS'), ('DESTDA')) AS c(code)
    UNION ALL
    -- Folha e Contábil valem para todos os regimes.
    SELECT c.code, t.regime, true
    FROM (VALUES ('PAYROLL_PROCESSING'), ('ESOCIAL'), ('FGTS'), ('INSS_IRRF'),
                 ('ACCOUNTING_MOVEMENT_RECEIVED'), ('ACCOUNTING_PAYROLL_POSTED'), ('ACCOUNTING_TAXES_POSTED'),
                 ('ACCOUNTING_PROCESSED'), ('ACCOUNTING_RECONCILIATION'), ('ACCOUNTING_TRIAL_BALANCE_CLOSED')) AS c(code)
    CROSS JOIN todos t
)
INSERT INTO obligation_type_regime_rules (obligation_type_id, tax_regime, default_applicable, counts_in_progress, active)
SELECT t.id, r.regime, true, r.counts, true
FROM regras r JOIN obligation_types t ON t.code = r.code
ON CONFLICT (obligation_type_id, tax_regime) DO UPDATE SET
    default_applicable = EXCLUDED.default_applicable,
    counts_in_progress = EXCLUDED.counts_in_progress,
    active             = EXCLUDED.active;

------------------------------------------------------------------------------------------
-- Dependências [§16.4, §18.2, §38.15]
--   Contábil: cadeia HARD sequencial (cada etapa depende da anterior; a checagem é transitiva).
--   DCTFWeb: SOFT sobre MIT, REINF Retidos, REINF Lucros e eSocial.
------------------------------------------------------------------------------------------
INSERT INTO obligation_dependencies (obligation_type_id, prerequisite_obligation_type_id, dependency_mode, active)
SELECT t.id, p.id, v.mode, true
FROM (VALUES
    ('ACCOUNTING_PAYROLL_POSTED',       'ACCOUNTING_MOVEMENT_RECEIVED', 'HARD'),
    ('ACCOUNTING_TAXES_POSTED',         'ACCOUNTING_PAYROLL_POSTED',    'HARD'),
    ('ACCOUNTING_PROCESSED',            'ACCOUNTING_TAXES_POSTED',      'HARD'),
    ('ACCOUNTING_RECONCILIATION',       'ACCOUNTING_PROCESSED',         'HARD'),
    ('ACCOUNTING_TRIAL_BALANCE_CLOSED', 'ACCOUNTING_RECONCILIATION',    'HARD'),
    ('DCTFWEB', 'MIT',           'SOFT'),
    ('DCTFWEB', 'REINF_RETIDOS', 'SOFT'),
    ('DCTFWEB', 'REINF_LUCROS',  'SOFT'),
    ('DCTFWEB', 'ESOCIAL',       'SOFT')
) AS v(code, prereq, mode)
JOIN obligation_types t ON t.code = v.code
JOIN obligation_types p ON p.code = v.prereq
ON CONFLICT (obligation_type_id, prerequisite_obligation_type_id) DO UPDATE SET
    dependency_mode = EXCLUDED.dependency_mode,
    active          = EXCLUDED.active;
