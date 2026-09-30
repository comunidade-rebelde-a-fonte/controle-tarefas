import { useQuery } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";

export type CompetenceInfo = Schemas["CompetenceInfo"];
export type LatestCompetence = Schemas["LatestCompetence"];

export const competenceKeys = {
  list: ["competencies", "list"] as const,
  latest: ["competencies", "latest"] as const,
};

/** Competência mais recente gerada e a corrente (a API garante a corrente antes). */
export function useLatestCompetence() {
  return useQuery({
    queryKey: competenceKeys.latest,
    queryFn: ({ signal }) => api.get("/competencies/latest", { signal }),
    staleTime: 60_000,
  });
}

/** Competências geradas, da mais recente para a mais antiga. */
export function useCompetencies() {
  return useQuery({ queryKey: competenceKeys.list, queryFn: ({ signal }) => api.get("/competencies", { signal }), staleTime: 60_000 });
}
