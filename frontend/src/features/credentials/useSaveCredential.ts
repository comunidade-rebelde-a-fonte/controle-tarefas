import { useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import {
  controlOf,
  credentialKeys,
  type CredentialControl,
  type CredentialRow,
  type CredentialType,
  type RenewalStatus,
  useUpdateCredential,
} from "@/services/credentials";

export interface CredentialPatch {
  expires_at?: string | null;
  renewal_status?: RenewalStatus;
  notes?: string | null;
}

/**
 * Salva vencimento, etapa ou observação de um item mantendo os demais campos. Conflito de
 * versão recarrega a grade e avisa; os demais erros viram aviso com a mensagem da API.
 */
export function useSaveCredential() {
  const update = useUpdateCredential();
  const toast = useToast();
  const qc = useQueryClient();

  const save = (row: CredentialRow, type: CredentialType, patch: CredentialPatch, onDone?: (c: CredentialControl) => void, onFail?: (msg: string) => void) => {
    const ctl = controlOf(row, type);
    update.mutate(
      {
        companyId: row.company.id,
        type,
        body: {
          version: ctl.version,
          expires_at: patch.expires_at !== undefined ? patch.expires_at : ctl.expires_at,
          renewal_status: patch.renewal_status ?? ctl.renewal_status,
          notes: patch.notes !== undefined ? patch.notes : ctl.notes,
        },
      },
      {
        onSuccess: (c) => onDone?.(c),
        onError: (err) => {
          if (isApiError(err) && err.code === "VERSION_CONFLICT") {
            void qc.invalidateQueries({ queryKey: credentialKeys.all });
            toast({ title: "Registro alterado em outro lugar", description: "Os dados foram recarregados. Confira e tente de novo.", variant: "error" });
            return;
          }
          const message = isApiError(err) ? err.message : "Falha de comunicação com o servidor.";
          if (onFail) onFail(message);
          else toast({ title: "Não foi possível salvar", description: message, variant: "error" });
        },
      },
    );
  };

  return { save, isPending: update.isPending };
}
