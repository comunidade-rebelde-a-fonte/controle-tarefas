import { createContext, useContext } from "react";
import type { CredentialRow, CredentialType } from "@/services/credentials";

/** Ações de página disparadas pelas células (as colunas da grade são estáveis e não recebem props). */
export interface CredentialsActions {
  openRenewed: (row: CredentialRow, type: CredentialType) => void;
  openDocuments: (row: CredentialRow) => void;
}

/** Identificador do controle editável da célula (a grade o foca no Enter). */
export const editId = (companyId: string, columnId: string) => `${companyId}:${columnId}`;

export const CredentialsContext = createContext<CredentialsActions | null>(null);

export function useCredentialsActions(): CredentialsActions {
  const ctx = useContext(CredentialsContext);
  if (!ctx) throw new Error("useCredentialsActions fora do CredentialsContext");
  return ctx;
}
