import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ApiError, api, isApiError, type Schemas } from "@/lib/api";

export type ObligationInstance = Schemas["ObligationInstance"];
export type StatusChangeResult = Schemas["StatusChangeResult"];
export type StatusDefinition = Schemas["StatusDefinition"];

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
