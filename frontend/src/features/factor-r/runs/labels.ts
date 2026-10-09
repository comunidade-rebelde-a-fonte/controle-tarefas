import { formatDecimal } from "@/lib/decimal";
import type { AIHumanEvalRating, AIRunStatus, AISyncStatus } from "@/services/aiRuns";

/** Agentes com nome legível; os demais aparecem pelo nome técnico. */
const AGENTS: Record<string, string> = {
  parser_pgdas: "Parser PGDAS-D",
  consultor: "Consultor",
  priorizador: "Priorizador",
};

export function agentLabel(agent: string): string {
  return AGENTS[agent] ?? agent;
}

const TRIGGERS: Record<string, string> = {
  upload: "envio do extrato",
  vinculo_manual: "vínculo manual",
  rejeicao: "rejeição",
  cadastro_pelo_extrato: "cadastro pelo extrato",
  ficha: "simulador da ficha",
  chat: "chat",
  rotina: "rotina semanal",
};

export function triggerLabel(trigger: string): string {
  return TRIGGERS[trigger] ?? trigger;
}

export const RUN_STATUS_LABELS: Record<AIRunStatus, string> = {
  ok: "ok",
  needs_review: "em revisão",
  error: "erro",
};

export const SYNC_LABELS: Record<AISyncStatus, string> = {
  pending: "pendente",
  ok: "enviado",
  failed: "falhou",
};

export const RATING_LABELS: Record<AIHumanEvalRating, string> = {
  acerto: "Acerto",
  parcial: "Parcial",
  erro: "Erro",
};

export const RATINGS = Object.keys(RATING_LABELS) as AIHumanEvalRating[];

/** Tipo do span (cadeia, ferramenta ou chamada ao LLM). */
export const SPAN_TYPE_LABELS: Record<string, string> = {
  chain: "etapa",
  tool: "ferramenta",
  generation: "LLM",
};

/**
 * Custo informado pelo gateway, em dólar (origem: US$ 0.0000, 4 casas), sem float. Uma chamada
 * isolada custa frações de centavo: o detalhe usa 6 casas.
 */
export function formatUsd(value: string | null | undefined, places = 4): string {
  const text = formatDecimal(value, places);
  return text === "—" ? text : `US$ ${text}`;
}
