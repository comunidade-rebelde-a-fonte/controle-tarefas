import { isApiError } from "@/lib/api";

/** Empresa em conflito num cadastro recusado (CNPJ ou código já cadastrado). */
export interface Conflict {
  message: string;
  company: { id: string; internal_code: string; legal_name: string } | null;
}

/** Recusa por CNPJ ou código já cadastrado (409), com a empresa em conflito. */
export function conflictFrom(err: unknown): Conflict | null {
  if (!isApiError(err) || (err.code !== "CNPJ_CONFLICT" && err.code !== "CODE_CONFLICT")) return null;
  const c = err.details.conflicting_company as Conflict["company"] | undefined;
  return { message: err.message, company: c ?? null };
}
