import { CircleAlert, CircleCheck, LoaderCircle } from "lucide-react";
import { useHealth } from "@/services/health";

/** Indicador do estado da API. Usa texto + ícone (não só cor) [PRD §45.4]. */
export function ApiStatus() {
  const { data, isPending, isError } = useHealth();
  if (isPending) {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-sidebar-muted" role="status">
        <LoaderCircle className="size-3.5 animate-spin" aria-hidden />
        Verificando API…
      </span>
    );
  }
  const ok = !isError && data.status === "ok";
  return (
    <span
      className={ok ? "inline-flex items-center gap-1.5 text-xs text-success" : "inline-flex items-center gap-1.5 text-xs text-warning"}
      role="status"
      aria-label={ok ? "API disponível" : "API indisponível"}
    >
      {ok ? <CircleCheck className="size-3.5" aria-hidden /> : <CircleAlert className="size-3.5" aria-hidden />}
      {ok ? "API disponível" : "API indisponível"}
    </span>
  );
}
