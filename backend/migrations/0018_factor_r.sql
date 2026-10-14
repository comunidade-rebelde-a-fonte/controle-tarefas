-- +goose Up
-- Extensão Fator R [docs/ai-spec/05 §4, TASK-006a]: configuração por empresa, movimentos
-- mensais, política única do escritório e tabelas do Simples por vigência. Tudo referencia
-- companies(id), o cadastro canônico [REQ-002]. "Sujeita ao Fator R" é o companies.factor_r
-- que já existe, e não uma coluna paralela (D-017). Nomes de coluna como no 05 §4.
-- Sem exclusão física: FKs ON DELETE RESTRICT.

-- Uma linha por empresa, só quando ela tem configuração própria; sem linha valem os padrões.
-- Os padrões e limites repetem a origem (Fator R, companies), que é o contrato de comportamento.
CREATE TABLE company_factor_r_settings (
    company_id        uuid          PRIMARY KEY REFERENCES companies (id) ON DELETE RESTRICT,
    cnae              text          NULL,
    atividade         text          NULL,
    quantidade_socios integer       NOT NULL DEFAULT 1 CHECK (quantidade_socios BETWEEN 0 AND 1000),
    contato           text          NULL,
    -- NULL = sem pacote comercial (o "none" do 05 §4).
    pacote_comercial  text          NULL CHECK (pacote_comercial IN ('monitoramento', 'correcao', 'retainer')),
    honorario_mensal  numeric(14,2) NOT NULL DEFAULT 0 CHECK (honorario_mensal >= 0),
    -- Mês de início da atividade (dia 1). Diferente de companies.service_start_date, que é o
    -- início do atendimento pelo escritório.
    inicio_atividade  date          NULL CHECK (inicio_atividade IS NULL OR extract(day FROM inicio_atividade) = 1),
    notas             text          NULL,
    version           integer       NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by        uuid          NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    updated_at        timestamptz   NOT NULL DEFAULT now()
);

-- Série mensal de receita e folha [REQ-009]. A competência é o mês da receita ou da folha.
CREATE TABLE factor_r_movements (
    id                          uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id                  uuid          NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    competence                  date          NOT NULL CHECK (extract(day FROM competence) = 1),
    receita_bruta               numeric(14,2) NOT NULL DEFAULT 0 CHECK (receita_bruta >= 0),
    pro_labore                  numeric(14,2) NOT NULL DEFAULT 0 CHECK (pro_labore >= 0),
    salarios_13_ferias          numeric(14,2) NOT NULL DEFAULT 0 CHECK (salarios_13_ferias >= 0),
    cpp_recolhida               numeric(14,2) NOT NULL DEFAULT 0 CHECK (cpp_recolhida >= 0),
    fgts_recolhido              numeric(14,2) NOT NULL DEFAULT 0 CHECK (fgts_recolhido >= 0),
    -- Folha total que o PGDAS-D declarou, sem a divisão: nunca se inventa a divisão (origem,
    -- CLAUDE §3.8). Quando existe, é a folha do mês para a FS12.
    folha_total_declarada_pgdas numeric(14,2) NULL CHECK (folha_total_declarada_pgdas IS NULL OR folha_total_declarada_pgdas >= 0),
    folha_mes                   numeric(16,2) GENERATED ALWAYS AS (
        COALESCE(folha_total_declarada_pgdas, pro_labore + salarios_13_ferias + cpp_recolhida + fgts_recolhido)
    ) STORED,
    origem                      text          NOT NULL CHECK (origem IN ('manual', 'pgdas', 'folha', 'agente')),
    observacao                  text          NULL,
    -- Documento PGDAS-D que gerou o movimento. A FK chega com factor_r_pgdas_documents (TASK-006b).
    pgdas_document_id           uuid          NULL,
    version                     integer       NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by                  uuid          NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at                  timestamptz   NOT NULL DEFAULT now(),
    updated_at                  timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT factor_r_movements_company_competence_uq UNIQUE (company_id, competence),
    -- A folha declarada só vem do PGDAS-D e não convive com a divisão (sem dupla contagem).
    CONSTRAINT factor_r_movements_folha_pgdas_ck CHECK (
        folha_total_declarada_pgdas IS NULL
        OR (origem = 'pgdas' AND pro_labore = 0 AND salarios_13_ferias = 0 AND cpp_recolhida = 0 AND fgts_recolhido = 0)
    )
);

-- Política única do escritório [05 §4; origem: firms]. Uma linha só (id = true). Não há tela
-- de edição: na origem, a UI da política é Fase 2.
CREATE TABLE factor_r_policy (
    id                      boolean       PRIMARY KEY DEFAULT true CHECK (id),
    -- Meta operacional do semáforo e do simulador; o corte legal é sempre 28% [REQ-011].
    meta_operacional        numeric(7,6)  NOT NULL CHECK (meta_operacional >= 0.28 AND meta_operacional < 1),
    -- A CPP recolhida no DAS integra a FS12? Vale para todas as empresas, nunca por empresa.
    cpp_das_integra_fs12    boolean       NOT NULL,
    piso_economia_anual     numeric(14,2) NOT NULL CHECK (piso_economia_anual >= 0),
    limiar_confianca_parser numeric(7,6)  NOT NULL CHECK (limiar_confianca_parser > 0 AND limiar_confianca_parser <= 1),
    tolerancia_ouro_pct     numeric(7,6)  NOT NULL CHECK (tolerancia_ouro_pct >= 0 AND tolerancia_ouro_pct < 1),
    version                 integer       NOT NULL DEFAULT 1 CHECK (version >= 1),
    updated_by              uuid          NULL REFERENCES users (id) ON DELETE RESTRICT,
    updated_at              timestamptz   NOT NULL DEFAULT now()
);

-- Padrões da origem (fator_r/seed.py): meta 30%, limiar do parser 0,40, piso R$ 6.000,00,
-- CPP integra a FS12, tolerância da nota ouro 1%.
INSERT INTO factor_r_policy (meta_operacional, cpp_das_integra_fs12, piso_economia_anual, limiar_confianca_parser, tolerancia_ouro_pct)
VALUES (0.300000, true, 6000.00, 0.400000, 0.010000);

-- Tabelas dos Anexos III e V do Simples Nacional por vigência: referência legal, carregada
-- pelo seed (make seed). O motor escolhe a tabela pelo PA; nenhum valor fixo no código.
CREATE TABLE factor_r_simples_tables (
    id               uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    anexo            text          NOT NULL CHECK (anexo IN ('III', 'V')),
    faixa            smallint      NOT NULL CHECK (faixa BETWEEN 1 AND 6),
    rbt12_ate        numeric(14,2) NOT NULL,
    aliquota_nominal numeric(7,6)  NOT NULL CHECK (aliquota_nominal > 0 AND aliquota_nominal < 1),
    parcela_deduzir  numeric(14,2) NOT NULL,
    vigencia_inicio  date          NOT NULL,
    vigencia_fim     date          NULL,
    CONSTRAINT factor_r_simples_tables_faixa_vigencia_uq UNIQUE (anexo, faixa, vigencia_inicio),
    CONSTRAINT factor_r_simples_tables_vigencia_ck CHECK (vigencia_fim IS NULL OR vigencia_fim > vigencia_inicio),
    CONSTRAINT factor_r_simples_tables_valores_ck CHECK (rbt12_ate > 0 AND parcela_deduzir >= 0)
);

-- +goose Down
DROP TABLE factor_r_simples_tables;
DROP TABLE factor_r_policy;
DROP TABLE factor_r_movements;
DROP TABLE company_factor_r_settings;
