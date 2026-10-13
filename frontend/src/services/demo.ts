import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { companyKeys } from "./companies";
import { userKeys } from "./users";

/** Carrega as 21 empresas fictícias de demonstração e o histórico (somente fora de produção). */
export function useLoadDemoData() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.post("/demo/seed"),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: companyKeys.all });
      void qc.invalidateQueries({ queryKey: userKeys.all });
    },
  });
}
