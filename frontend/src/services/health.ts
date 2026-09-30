import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

/** Estado da API (via proxy /api), atualizado a cada 30 s. */
export function useHealth() {
  return useQuery({
    queryKey: ["health"],
    queryFn: ({ signal }) => api.get("/health", { signal }),
    refetchInterval: 30_000,
    retry: false,
  });
}
