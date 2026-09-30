import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";

export type ObligationType = Schemas["ObligationType"];
export type DueDateRule = Schemas["DueDateRule"];
export type StatusSet = Schemas["StatusSet"];

export const catalogKeys = {
  types: ["catalog", "types"] as const,
  statusSets: ["catalog", "status-sets"] as const,
  dueDateRules: ["catalog", "due-date-rules"] as const,
};

export function useObligationTypes() {
  return useQuery({ queryKey: catalogKeys.types, queryFn: ({ signal }) => api.get("/obligation-types", { signal }), staleTime: 60_000 });
}

export function useStatusSets() {
  return useQuery({ queryKey: catalogKeys.statusSets, queryFn: ({ signal }) => api.get("/status-sets", { signal }), staleTime: 5 * 60_000 });
}

export function useCreateObligationType() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["ObligationTypeCreate"]) => api.post("/obligation-types", { body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: catalogKeys.types }),
  });
}

export function useUpdateObligationType() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: Schemas["ObligationTypeUpdate"] & { id: string }) => api.patch("/obligation-types/{id}", { path: { id }, body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: catalogKeys.types }),
  });
}

export function useDueDateRules() {
  return useQuery({ queryKey: catalogKeys.dueDateRules, queryFn: ({ signal }) => api.get("/due-date-rules", { signal }) });
}

export function useSaveDueDateRule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: Schemas["DueDateRuleInput"] & { id?: string }) =>
      id ? api.patch("/due-date-rules/{id}", { path: { id }, body }) : api.post("/due-date-rules", { body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: catalogKeys.dueDateRules }),
  });
}
