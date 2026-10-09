import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";
import { aiRunKeys } from "./aiRuns";
import { factorRKeys } from "./factorR";

export type FactorRSimulation = Schemas["FactorRSimulation"];
export type FactorRSimulationInput = Schemas["FactorRSimulationInput"];
export type FactorRVerdict = Schemas["FactorRVerdict"];
export type FactorRAgentResponse = Schemas["FactorRAgentResponse"];

export const simulationKeys = {
  list: (companyId: string) => [...factorRKeys.company(companyId), "simulations"] as const,
};

/** GET /companies/{id}/factor-r/simulations — últimas simulações, com o texto da corrida. */
export function useFactorRSimulations(companyId: string, limit = 5) {
  return useQuery({
    queryKey: [...simulationKeys.list(companyId), limit],
    queryFn: ({ signal }) => api.get("/companies/{id}/factor-r/simulations", { path: { id: companyId }, query: { limit }, signal }),
  });
}

/** POST /companies/{id}/factor-r/simulations — simula e grava no trace (é o que autoriza "corrigir"). */
export function useCreateFactorRSimulation(companyId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: FactorRSimulationInput) => api.post("/companies/{id}/factor-r/simulations", { path: { id: companyId }, body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: simulationKeys.list(companyId) });
      void qc.invalidateQueries({ queryKey: aiRunKeys.all });
    },
  });
}

/** POST /factor-r/agents/consult — o consultor pode simular: recarrega histórico e corridas. */
export function useConsultAgent(companyId?: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (mensagem: string) => api.post("/factor-r/agents/consult", { body: { mensagem, company_id: companyId ?? null } }),
    onSettled: () => {
      if (companyId) void qc.invalidateQueries({ queryKey: simulationKeys.list(companyId) });
      void qc.invalidateQueries({ queryKey: aiRunKeys.all });
    },
  });
}

/** POST /factor-r/agents/prioritize — fila da carteira do PA; só lê, nada muda. */
export function usePrioritizeAgent(pa: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (mensagem: string) => api.post("/factor-r/agents/prioritize", { body: { mensagem, pa } }),
    onSettled: () => void qc.invalidateQueries({ queryKey: aiRunKeys.all }),
  });
}
