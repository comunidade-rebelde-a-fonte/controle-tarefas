import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, isApiError, type Schemas } from "@/lib/api";

export type AIModelAlias = Schemas["AIModelAlias"];
export type AIModelList = Schemas["AIModelList"];
export type AIModelUpdate = Schemas["AIModelUpdate"];
export type AIModelValidateRequest = Schemas["AIModelValidateRequest"];
export type AIModelValidation = Schemas["AIModelValidation"];
export type AIModelCheck = Schemas["AIModelCheck"];
export type AIProvider = Schemas["AIProvider"];

export const AI_PROVIDERS: AIProvider[] = ["openai", "anthropic"];

export const aiModelKeys = { list: ["settings", "ai-models"] as const };

/** GET /settings/ai/models — aliases com a configuração não secreta. */
export function useAIModels() {
  return useQuery({ queryKey: aiModelKeys.list, queryFn: ({ signal }) => api.get("/settings/ai/models", { signal }) });
}

/** PUT /settings/ai/models/{alias} — vale para as próximas execuções do agente. */
export function useUpdateAIModel() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ alias, body }: { alias: string; body: AIModelUpdate }) =>
      api.put("/settings/ai/models/{alias}", { path: { alias }, body }),
    onSuccess: (item) =>
      qc.setQueryData<AIModelList>(aiModelKeys.list, (old) =>
        old ? { ...old, items: old.items.map((i) => (i.alias === item.alias ? item : i)) } : old,
      ),
    onError: (err) => {
      // Outra aba ou pessoa gravou antes: recarrega para mostrar a versão atual.
      if (isApiError(err) && err.code === "VERSION_CONFLICT") void qc.invalidateQueries({ queryKey: aiModelKeys.list });
    },
  });
}

/** POST /settings/ai/models/{alias}/validate — confere no gateway, sem gravar. */
export function useValidateAIModel() {
  return useMutation({
    mutationFn: ({ alias, body }: { alias: string; body: AIModelValidateRequest }) =>
      api.post("/settings/ai/models/{alias}/validate", { path: { alias }, body }),
  });
}
