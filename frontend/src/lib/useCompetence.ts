import { useSearchParams } from "react-router";
import { isCompetenceKey } from "@/lib/competence";
import { useLatestCompetence } from "@/services/competencies";

export const COMPETENCE_PARAM = "competence";

/**
 * Competência selecionada nas telas mensais, sincronizada com `?competence=AAAA-MM`
 * [AGENTS §8.5]. Sem parâmetro válido na URL, vale a mais recente da API [PRD §24.1].
 */
export function useCompetence() {
  const [params, setParams] = useSearchParams();
  const latestQuery = useLatestCompetence();
  const fromUrl = params.get(COMPETENCE_PARAM);
  const latest = latestQuery.data?.competence ?? null;
  const competence = isCompetenceKey(fromUrl) ? fromUrl : latest;

  /** Troca a competência preservando os demais parâmetros (filtros) da URL. */
  const setCompetence = (key: string) => {
    if (!isCompetenceKey(key)) return;
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set(COMPETENCE_PARAM, key);
      return next;
    });
  };

  return {
    /** `AAAA-MM`, ou null enquanto a mais recente carrega. */
    competence,
    latest,
    /** Competência corrente no fuso do escritório. */
    current: latestQuery.data?.current ?? null,
    setCompetence,
    isLoading: competence === null && latestQuery.isPending,
    error: latestQuery.error,
  };
}
