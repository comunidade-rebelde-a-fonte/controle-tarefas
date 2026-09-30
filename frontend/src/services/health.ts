import { useQuery } from "@tanstack/react-query";

export interface Health {
  status: string;
  database: string;
}

async function fetchHealth(): Promise<Health> {
  const res = await fetch("/api/v1/health", { headers: { Accept: "application/json" } });
  if (!res.ok) throw new Error(`health ${res.status}`);
  return (await res.json()) as Health;
}

/** Estado da API (via proxy /api), atualizado a cada 30 s. */
export function useHealth() {
  return useQuery({ queryKey: ["health"], queryFn: fetchHealth, refetchInterval: 30_000, retry: false });
}
