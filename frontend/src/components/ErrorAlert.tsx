import { CircleAlert } from "lucide-react";
import { isApiError } from "@/lib/api";

/** Mensagem de erro da API (texto + ícone). */
export function ErrorAlert({ error, fallback = "Não foi possível concluir a operação." }: { error: unknown; fallback?: string }) {
  if (!error) return null;
  const message = isApiError(error) ? error.message : fallback;
  return (
    <p className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" role="alert">
      <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
      {message}
    </p>
  );
}
