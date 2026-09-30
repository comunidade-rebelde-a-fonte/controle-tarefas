import { useSearchParams } from "react-router";

/** Parâmetros simples da URL (filtros de listas) [AGENTS §8.5]; vazio remove o parâmetro. */
export function useUrlParams() {
  const [params, setParams] = useSearchParams();
  const get = (name: string) => params.get(name) ?? "";
  const set = (patch: Record<string, string>, { resetPage = true }: { resetPage?: boolean } = {}) => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        if (resetPage && !("page" in patch)) next.delete("page");
        return next;
      },
      { replace: true },
    );
  };
  return { params, get, set };
}
