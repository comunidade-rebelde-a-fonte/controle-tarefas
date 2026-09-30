import type { FieldValues, Path, UseFormSetError } from "react-hook-form";
import { isApiError } from "./api";

/**
 * Leva os erros de campo da API (400 VALIDATION_ERROR, details.fields) para o formulário.
 * Devolve true se algum campo conhecido recebeu erro.
 */
export function applyApiFieldErrors<T extends FieldValues>(err: unknown, setError: UseFormSetError<T>, known: readonly string[]): boolean {
  if (!isApiError(err) || err.code !== "VALIDATION_ERROR") return false;
  const fields = err.details.fields;
  if (!fields || typeof fields !== "object") return false;
  let applied = false;
  for (const [field, message] of Object.entries(fields as Record<string, unknown>)) {
    if (known.includes(field) && typeof message === "string") {
      setError(field as Path<T>, { type: "server", message });
      applied = true;
    }
  }
  return applied;
}
