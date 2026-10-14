import { useSearchParams } from "react-router";
import { isApiError } from "@/lib/api";

/** Os filtros da tela na URL: lê e troca parâmetros sem empilhar histórico. */
export function useSearchUpdate() {
  const [params, setParams] = useSearchParams();
  const update = (patch: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        return next;
      },
      { replace: true },
    );
  return [params, update] as const;
}

export interface CsvLineError {
  line: number;
  message: string;
}

function isLineError(v: unknown): v is CsvLineError {
  if (typeof v !== "object" || v === null) return false;
  const l = v as Record<string, unknown>;
  return typeof l.line === "number" && typeof l.message === "string";
}

/** As linhas recusadas do CSV (400 INVALID_ENTRIES_CSV ou INVALID_ACCOUNTS_CSV, details.lines). */
export function csvLinesOf(err: unknown): CsvLineError[] {
  if (!isApiError(err)) return [];
  const lines = err.details.lines;
  return Array.isArray(lines) ? lines.filter(isLineError) : [];
}
