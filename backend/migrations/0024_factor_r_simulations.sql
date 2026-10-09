-- +goose Up
-- Simulações de correção do Fator R [05 §4, TASK-006b/TASK-013; origem: simulations, migration
-- 20260917_0008 e services/simulacao.py]. Toda simulação nasce numa corrida do agente consultor
-- (ficha ou chat) e fica no trace: é ela que autoriza uma recomendação "corrigir" (CLAUDE §3.11
-- da origem). Entrada e saída completas, com o valor exato do motor, ficam em JSON; as colunas
-- tipadas repetem o que o 05 §4 pede para auditoria e consulta. Sem exclusão física.
CREATE TABLE factor_r_simulations (
    id                  uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id          uuid          NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    pa                  date          NOT NULL CHECK (extract(day FROM pa) = 1),
    -- Parâmetros (ParametrosSimulacao): meta operacional, fração do reforço em pró-labore,
    -- alíquotas de INSS e IRRF, horizonte e piso de economia da política.
    meta                numeric(7,6)  NOT NULL CHECK (meta >= 0.28 AND meta < 1),
    fracao_pro_labore   numeric(7,6)  NOT NULL CHECK (fracao_pro_labore BETWEEN 0 AND 1),
    inss                numeric(7,6)  NOT NULL CHECK (inss BETWEEN 0 AND 1),
    irrf                numeric(7,6)  NOT NULL CHECK (irrf BETWEEN 0 AND 1),
    horizonte_meses     integer       NOT NULL CHECK (horizonte_meses BETWEEN 1 AND 60),
    piso_economia_anual numeric(14,2) NOT NULL CHECK (piso_economia_anual >= 0),
    -- Resultado (ResultadoSimulacao): valores exatos, sem arredondar (a API arredonda ao exibir).
    status              text          NOT NULL CHECK (status IN ('ok', 'dados_insuficientes')),
    veredito            text          NULL CHECK (veredito IN ('ja_na_meta', 'corrigir', 'nao_forcar')),
    reforco_12m         numeric       NOT NULL,
    reforco_mensal      numeric       NOT NULL,
    custo_total         numeric       NOT NULL,
    economia_12m        numeric       NULL,
    liquido             numeric       NULL,
    parametros_json     jsonb         NOT NULL CHECK (jsonb_typeof(parametros_json) = 'object'),
    resultado_json      jsonb         NOT NULL CHECK (jsonb_typeof(resultado_json) = 'object'),
    -- Corrida do consultor que fez a simulação [AC-029].
    trace_id            text          NOT NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT,
    created_by          uuid          NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at          timestamptz   NOT NULL DEFAULT now(),
    -- Veredito só com dados suficientes.
    CONSTRAINT factor_r_simulations_veredito_ck CHECK ((status = 'ok') = (veredito IS NOT NULL))
);

CREATE INDEX factor_r_simulations_company_idx ON factor_r_simulations (company_id, created_at DESC);
CREATE INDEX factor_r_simulations_trace_idx ON factor_r_simulations (trace_id);

-- +goose Down
DROP TABLE factor_r_simulations;
