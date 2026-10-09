import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";

export type AIRun = Schemas["AIRun"];
export type AIRunList = Schemas["AIRunList"];
export type AIRunDetail = Schemas["AIRunDetail"];
export type AIRunSpan = Schemas["AIRunSpan"];
export type AIRunStatus = Schemas["AIRunStatus"];
export type AISyncStatus = Schemas["AISyncStatus"];
export type AISummary = Schemas["AISummary"];
export type AIGoldEval = Schemas["AIGoldEval"];
export type AIHumanEval = Schemas["AIHumanEval"];
export type AIHumanEvalInput = Schemas["AIHumanEvalInput"];
export type AIHumanEvalRating = Schemas["AIHumanEvalRating"];

/** Filtros da lista de corridas (GET /ai/runs). */
export interface AIRunFilter {
  agent?: string | undefined;
  status?: AIRunStatus | undefined;
  company_id?: string | undefined;
  without_human_eval?: boolean | undefined;
  limit?: number | undefined;
  offset?: number | undefined;
}

export const aiRunKeys = {
  all: ["ai"] as const,
  summary: ["ai", "summary"] as const,
  list: (f: AIRunFilter) => ["ai", "runs", "list", f] as const,
  detail: (traceId: string) => ["ai", "runs", "detail", traceId] as const,
};

/** GET /ai/summary — painel dos agentes (FR-18), da trilha local. */
export function useAISummary() {
  return useQuery({ queryKey: aiRunKeys.summary, queryFn: ({ signal }) => api.get("/ai/summary", { signal }) });
}

/** GET /ai/runs — corridas, mais recentes primeiro. */
export function useAIRuns(filter: AIRunFilter) {
  return useQuery({
    queryKey: aiRunKeys.list(filter),
    queryFn: ({ signal }) => api.get("/ai/runs", { query: filter, signal }),
    placeholderData: keepPreviousData,
  });
}

/** GET /ai/runs/{trace_id} — entrada, saída, decisão, spans e notas. */
export function useAIRun(traceId: string) {
  return useQuery({
    queryKey: aiRunKeys.detail(traceId),
    queryFn: ({ signal }) => api.get("/ai/runs/{trace_id}", { path: { trace_id: traceId }, signal }),
  });
}

/** POST /ai/runs/{trace_id}/evaluation — nota humana (erro exige comentário). */
export function useEvaluateAIRun(traceId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AIHumanEvalInput) => api.post("/ai/runs/{trace_id}/evaluation", { path: { trace_id: traceId }, body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: aiRunKeys.all }),
  });
}
