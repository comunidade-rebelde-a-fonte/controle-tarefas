import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";

const key = ["settings", "credential-thresholds"] as const;

export function useCredentialThresholds() {
  return useQuery({ queryKey: key, queryFn: ({ signal }) => api.get("/settings/credential-thresholds", { signal }) });
}

export function useSetCredentialThresholds() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (days: number[]) => api.put("/settings/credential-thresholds", { body: { days } }),
    onSuccess: (data) => qc.setQueryData(key, data),
  });
}
