import type { AIRun, AIRunDetail, AISummary } from "@/services/aiRuns";
import type { FactorRSimulation } from "@/services/factorRAgents";
import type { PgdasDocument } from "@/services/factorRInbox";

/** Dados de teste das telas do Fator R (TASK-014b), no formato do contrato OpenAPI. */

export const DOC_ID = "33333333-3333-4333-8333-333333333333";
export const COMPANY_ID = "44444444-4444-4444-8444-444444444444";
export const TRACE_ID = "0123456789abcdef0123456789abcdef";
export const JOB_ID = "55555555-5555-4555-8555-555555555555";

export function pgdasDocument(patch: Partial<PgdasDocument> = {}): PgdasDocument {
  return {
    id: DOC_ID,
    status: "needs_review",
    motivo: "cnpj_nao_encontrado",
    company_id: null,
    nome_original: "extrato.txt",
    mime: "text/plain",
    tamanho_bytes: 900,
    campos: { cnpj: "45723174000110", pa: "2026-09", rbt12: "600000.00", rpa: "50000.00", fs12: "180000.00", fator_r: "0.30", das: "5280.00", anexo: "III" },
    confianca_campos: { cnpj: "1", pa: "1", rbt12: "1", rpa: "1", fs12: "1", fator_r: "1", das: "1", anexo: "1" },
    motivos: [],
    identificacao: { nome_empresarial: "CLINICA EXEMPLO LTDA", sujeita_fator_r: true, inicio_atividade: "2019-03" },
    series_anteriores: { receitas: {}, folhas: {}, receitas_conferem: false, folhas_conferem: false },
    confianca: "1.0000",
    parser_version: "2026.09.3",
    trace_id: TRACE_ID,
    texto_agente: "Extrato do PA 2026-09 precisa de revisão: cnpj_nao_encontrado.\n\nEsta plataforma não é a apuração oficial. O PGDAS-D da Receita Federal prevalece.",
    job: { id: JOB_ID, status: "done", attempts: 1, max_attempts: 3, last_error_code: null },
    sugestao_cadastro: {
      cnpj: "45723174000110",
      cnpj_formatado: "45.723.174/0001-10",
      nome_empresarial: "CLINICA EXEMPLO LTDA",
      sujeita_fator_r: true,
      inicio_atividade: "2019-03",
    },
    version: 1,
    created_at: "2026-09-15T15:00:00Z",
    updated_at: "2026-09-15T15:00:01Z",
    ...patch,
  };
}

export function aiRun(patch: Partial<AIRun> = {}): AIRun {
  return {
    trace_id: TRACE_ID,
    agent: "priorizador",
    trigger: "chat",
    status: "ok",
    confidence: null,
    latency_ms: 12,
    company_id: null,
    competence: null,
    alias: "priorizador",
    provider: "openai",
    model: "gpt-6-luna",
    tokens_in: 240,
    tokens_out: 48,
    cost_usd: "0.000084",
    langfuse_sync_status: "pending",
    has_human_eval: false,
    created_at: "2026-09-15T15:00:00Z",
    ...patch,
  };
}

export function aiRunDetail(patch: Partial<AIRunDetail> = {}): AIRunDetail {
  return {
    ...aiRun(),
    model_config_version: 1,
    prompt_version: "fatorr/plan@2026.10.1#eddce0a92af0;fatorr/render@2026.10.1#6df09143757a",
    error_code: null,
    metadata: {},
    input: { pa: "2026-09", mensagem: "O que priorizar?" },
    output: { texto: "Fila da semana." },
    decision: { intencao: "priorizar", pa: "2026-09", fila: [] },
    spans: [
      { id: "s1", name: "plan", type: "chain", start: "2026-09-15T15:00:00.000Z", end: "2026-09-15T15:00:00.004Z" },
      {
        id: "g1",
        parent_id: "s1",
        name: "classificar_intencao",
        type: "generation",
        start: "2026-09-15T15:00:00.001Z",
        end: "2026-09-15T15:00:00.003Z",
        model: "openai/gpt-6-luna",
        tokens_in: 120,
        tokens_out: 24,
        cost_usd: "0.000042",
      },
      { id: "s2", name: "tool", type: "chain", start: "2026-09-15T15:00:00.004Z", end: "2026-09-15T15:00:00.008Z" },
      { id: "s3", name: "decide", type: "chain", start: "2026-09-15T15:00:00.008Z", end: "2026-09-15T15:00:00.009Z" },
      {
        id: "s4",
        name: "render",
        type: "chain",
        start: "2026-09-15T15:00:00.009Z",
        end: "2026-09-15T15:00:00.012Z",
        level: "WARNING",
        metadata: { render_rejeitado: "true", numeros_orfaos: "123" },
      },
    ],
    decisions: [{ id: "66666666-6666-4666-8666-666666666666", kind: "fila_priorizacao", data: {}, created_at: "2026-09-15T15:00:00Z" }],
    gold_evals: [],
    human_evals: [],
    langfuse_attempts: 0,
    langfuse_last_error: null,
    langfuse_synced_at: null,
    langfuse_url: null,
    ...patch,
  };
}

export function aiSummary(patch: Partial<AISummary> = {}): AISummary {
  return {
    runs_24h: 7,
    runs_total: 42,
    pending_human_eval: 5,
    gold_accuracy: "0.9500",
    human_accuracy: null,
    by_agent: [
      { agent: "parser_pgdas", runs: 30, avg_latency_ms: 14, avg_confidence: "0.9800", errors: 1, reviews: 4 },
      { agent: "priorizador", runs: 12, avg_latency_ms: 1200, avg_confidence: null, errors: 0, reviews: 0 },
    ],
    llm_cost_30d: "0.012345",
    llm_tokens_30d: 15234,
    ...patch,
  };
}

export function simulation(patch: Partial<FactorRSimulation> = {}): FactorRSimulation {
  return {
    simulation_id: "77777777-7777-4777-8777-777777777777",
    trace_id: TRACE_ID,
    pa: "2026-09",
    veredito: "corrigir",
    texto: "Vale corrigir: a economia de DAS supera o custo.\n\nEsta plataforma não é a apuração oficial. O PGDAS-D da Receita Federal prevalece.",
    parametros: { meta: "0.30", fracao_pro_labore: "1", inss: "0.11", irrf: "0.275", horizonte_meses: 12, piso_economia_anual: "6000.00" },
    resultado: {
      status: "ok",
      veredito: "corrigir",
      fator_atual: "0.133393",
      reforco_12m: "104916.00",
      reforco_mensal: "8743.00",
      pro_labore_extra_mensal: "8743.00",
      custo_inss: "11540.76",
      custo_irrf: "28851.90",
      economia_horizonte: "45523.20",
      liquido: "5130.54",
    },
    created_at: "2026-09-15T15:00:00Z",
    ...patch,
  };
}
