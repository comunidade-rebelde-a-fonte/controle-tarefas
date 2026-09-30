import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";
import { obligationKeys } from "./obligations";

export type ModulePeriod = Schemas["CompanyModulePeriod"];
export type ModulePeriodEvent = Schemas["CompanyModulePeriodEvent"];

export const periodKeys = {
  events: (id: string) => ["periods", "events", id] as const,
};

/** Código do 409 quando o estágio de fechamento (Fiscal: Fechamento) ainda não está encerrado [PRD §13.6]. */
export const CLOSING_STATUS_REQUIRED = "CLOSING_STATUS_REQUIRED";

function useInvalidateGrid() {
  const qc = useQueryClient();
  return (id: string) => {
    // A grade (prefixo `obligations`) mostra o estado do período e bloqueia as células.
    void qc.invalidateQueries({ queryKey: obligationKeys.all });
    void qc.invalidateQueries({ queryKey: periodKeys.events(id) });
  };
}

export type ClosePeriodInput = Schemas["CompanyModulePeriodClose"] & { id: string };

/** POST /company-module-periods/{id}/close [PRD §10.4, RN-006]. */
export function useClosePeriod() {
  const invalidate = useInvalidateGrid();
  return useMutation({
    mutationFn: ({ id, ...body }: ClosePeriodInput) => api.post("/company-module-periods/{id}/close", { path: { id }, body }),
    onSuccess: (p) => invalidate(p.id),
  });
}

export type ReopenPeriodInput = Schemas["CompanyModulePeriodReopen"] & { id: string };

/** POST /company-module-periods/{id}/reopen com motivo obrigatório [PRD §10.6, RN-007]. */
export function useReopenPeriod() {
  const invalidate = useInvalidateGrid();
  return useMutation({
    mutationFn: ({ id, ...body }: ReopenPeriodInput) => api.post("/company-module-periods/{id}/reopen", { path: { id }, body }),
    onSuccess: (p) => invalidate(p.id),
  });
}

/** GET /company-module-periods/{id}/events (abertura, fechamentos e reaberturas). */
export function usePeriodEvents(id: string | null, enabled = true) {
  return useQuery({
    queryKey: periodKeys.events(id ?? ""),
    queryFn: ({ signal }) => api.get("/company-module-periods/{id}/events", { path: { id: id ?? "" }, signal }),
    enabled: enabled && !!id,
  });
}
