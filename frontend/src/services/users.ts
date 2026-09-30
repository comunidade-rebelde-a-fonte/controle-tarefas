import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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

export function useCreateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["UserCreate"]) => api.post("/users", { body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: userKeys.all }),
  });
}

export function useUpdateUser() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: Schemas["UserUpdate"] & { id: string }) => api.patch("/users/{id}", { path: { id }, body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: userKeys.all }),
  });
}
