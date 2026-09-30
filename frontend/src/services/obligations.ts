import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiError, api, isApiError, type Schemas } from "@/lib/api";

export type ObligationInstance = Schemas["ObligationInstance"];
export type StatusChangeResult = Schemas["StatusChangeResult"];
export type StatusDefinition = Schemas["StatusDefinition"];
export type HistoryEntry = Schemas["HistoryEntry"];
export type InstanceDetails = Schemas["InstanceDetails"];
export type BulkStatusChange = Schemas["BulkStatusChange"];
export type BulkStatusResult = Schemas["BulkStatusResult"];

/** Chaves das consultas de obrigações. As grades (T-M3-01) usam o prefixo `obligations`. */
export const obligationKeys = {
  all: ["obligations"] as const,
  instance: (id: string) => ["obligations", "instance", id] as const,
  history: (id: string) => ["obligations", "history", id] as const,
};

/** Códigos de erro da alteração de status [AGENTS §8.4]. */
export const StatusErrorCode = {
  VersionConflict: "VERSION_CONFLICT",
  DependencyWarning: "DEPENDENCY_WARNING",
  DependencyBlocked: "DEPENDENCY_BLOCKED",
  PeriodClosed: "PERIOD_CLOSED",
} as const;

/** Pré-requisito pendente (HARD, 422) ou alerta SOFT (409): tratados nos modais de dependência. */
export function isDependencyError(err: unknown): err is ApiError {
  return isApiError(err) && (err.code === StatusErrorCode.DependencyWarning || err.code === StatusErrorCode.DependencyBlocked);
}

export function isVersionConflict(err: unknown): err is ApiError {
  return isApiError(err) && err.code === StatusErrorCode.VersionConflict;
}

export type ChangeStatusInput = Schemas["StatusChange"] & { id: string };

/** PATCH /obligation-instances/{id}/status [PRD §26.2]. */
export function useChangeStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: ChangeStatusInput) => api.patch("/obligation-instances/{id}/status", { path: { id }, body }),
    onSuccess: (res) => {
      qc.setQueryData(obligationKeys.instance(res.instance.id), res.instance);
      void qc.invalidateQueries({ queryKey: obligationKeys.history(res.instance.id) });
    },
  });
}

/** Pré-requisito pendente devolvido em `details.pending` (DEPENDENCY_WARNING/BLOCKED). */
export interface PendingPrerequisite {
  code: string;
  name: string;
}

/** Lê `details.pending` do erro de dependência (formato do contrato, validado em tempo de execução). */
export function pendingPrerequisites(err: ApiError): PendingPrerequisite[] {
  const raw = err.details.pending;
  if (!Array.isArray(raw)) return [];
  return raw.flatMap((p: unknown) => {
    if (typeof p !== "object" || p === null) return [];
    const { code, name } = p as Record<string, unknown>;
    return typeof code === "string" && typeof name === "string" ? [{ code, name }] : [];
  });
}

/** GET /obligation-instances/{id}/history (mais recente primeiro) [PRD §23]. */
export function useInstanceHistory(id: string | null, enabled = true) {
  return useQuery({
    queryKey: obligationKeys.history(id ?? ""),
    queryFn: ({ signal }) => api.get("/obligation-instances/{id}/history", { path: { id: id ?? "" }, signal }),
    enabled: enabled && !!id,
  });
}

export type UpdateDetailsInput = InstanceDetails & { id: string };

/** PATCH /obligation-instances/{id}/details: vencimento (com motivo), transmissão, protocolo [PRD §21.3]. */
export function useUpdateDetails() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: UpdateDetailsInput) => api.patch("/obligation-instances/{id}/details", { path: { id }, body }),
    onSuccess: (res) => {
      qc.setQueryData(obligationKeys.instance(res.instance.id), res.instance);
      // Grades usam o prefixo `obligations`: a linha e os indicadores são recarregados.
      void qc.invalidateQueries({ queryKey: obligationKeys.all });
    },
  });
}

/** POST /obligation-instances/bulk-status (com dry_run para a prévia) [PRD §26.3, CA-016]. */
export function useBulkStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BulkStatusChange) => api.post("/obligation-instances/bulk-status", { body }),
    onSuccess: (res) => {
      if (!res.dry_run) void qc.invalidateQueries({ queryKey: obligationKeys.all });
    },
  });
}
