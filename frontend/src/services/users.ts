import { useQuery } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";

export type User = Schemas["User"];

export const userKeys = { all: ["users"] as const, list: (active?: boolean) => ["users", "list", active ?? "all"] as const };

/** Usuários/responsáveis (sem login no MVP). */
export function useUsers(active?: boolean) {
  return useQuery({
    queryKey: userKeys.list(active),
    queryFn: ({ signal }) => api.get("/users", { query: active === undefined ? {} : { active }, signal }),
    staleTime: 60_000,
  });
}
