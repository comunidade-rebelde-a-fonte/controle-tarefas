import { useSearchParams } from "react-router";
import { isCompetenceKey } from "@/lib/competence";
import { useLatestCompetence } from "@/services/competencies";

export const PA_PARAM = "pa";

/**
 * Período de apuração (PA) das telas do Fator R, na URL (`?pa=AAAA-MM`). Sem PA válido na
 * URL, vale a competência corrente do escritório, como na origem.
 */
export function usePA() {
  const [params, setParams] = useSearchParams();
  const latest = useLatestCompetence();
  const fromUrl = params.get(PA_PARAM);
  const current = latest.data?.current ?? null;
  const pa = isCompetenceKey(fromUrl) ? fromUrl : current;
  const setPA = (key: string) => {
    if (!isCompetenceKey(key)) return;
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(PA_PARAM, key);
        return next;
      },
      { replace: true },
    );
  };
  return { pa, current, setPA, error: latest.error };
}
