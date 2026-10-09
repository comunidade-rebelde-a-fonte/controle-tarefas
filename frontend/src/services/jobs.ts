import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";

export type Job = Schemas["Job"];
export type JobStatus = Schemas["JobStatus"];

export const jobKeys = { detail: (id: string) => ["jobs", id] as const };

/** Intervalo de acompanhamento de um job em andamento (ms). */
export const JOB_POLL_MS = 1500;

/** O job ainda vai rodar (na fila, em execução ou com nova tentativa agendada)? */
export function jobActive(status: JobStatus | null | undefined): boolean {
  return status === "pending" || status === "running" || status === "retry";
}

/**
 * GET /jobs/{id} — estado do job e `worker_online`: job pendente sem worker ativo é fila parada
 * (runbook "Worker parado", 11 §9). Acompanha enquanto o job está ativo.
 */
export function useJob(id: string | null | undefined, enabled = true) {
  return useQuery({
    queryKey: jobKeys.detail(id ?? ""),
    queryFn: ({ signal }) => api.get("/jobs/{id}", { path: { id: id ?? "" }, signal }),
    enabled: !!id && enabled,
    refetchInterval: (q) => (jobActive(q.state.data?.status) ? JOB_POLL_MS : false),
  });
}

/** POST /jobs/{id}/retry — devolve à fila um job encerrado com falha (dead). */
export function useRetryJob(onDone?: () => void) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post("/jobs/{id}/retry", { path: { id } }),
    onSuccess: (job) => {
      qc.setQueryData(jobKeys.detail(job.id), job);
      onDone?.();
    },
  });
}
