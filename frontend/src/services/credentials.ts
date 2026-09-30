import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiUpload, type Schemas } from "@/lib/api";

export type CredentialList = Schemas["CredentialList"];
export type CredentialRow = Schemas["CredentialRow"];
export type CredentialControl = Schemas["CredentialControl"];
export type CredentialType = Schemas["CredentialType"];
export type RenewalStatus = Schemas["RenewalStatus"];
export type CredentialSituation = Schemas["CredentialSituation"];

export const CREDENTIAL_TYPES: CredentialType[] = ["RFB_POWER_OF_ATTORNEY", "DIGITAL_CERTIFICATE"];

export const CREDENTIAL_TYPE_LABELS: Record<CredentialType, string> = {
  RFB_POWER_OF_ATTORNEY: "Procuração RFB",
  DIGITAL_CERTIFICATE: "Certificado digital",
};

export const RENEWAL_LABELS: Record<RenewalStatus, string> = {
  CURRENT: "Vigente",
  RENEWAL_REQUESTED: "Renovação solicitada",
  AWAITING_CLIENT: "Aguardando cliente",
  RENEWED: "Renovado",
};

export const credentialKeys = {
  all: ["credentials"] as const,
  list: (filter: string) => ["credentials", "list", filter] as const,
  company: (id: string) => ["credentials", "company", id] as const,
};

/** Item da linha pelo tipo. */
export function controlOf(row: CredentialRow, type: CredentialType): CredentialControl {
  return type === "RFB_POWER_OF_ATTORNEY" ? row.rfb_power_of_attorney : row.digital_certificate;
}

/** GET /credential-controls com o filtro rápido do §33 (vazio = todos). */
export function useCredentialList(filter: string) {
  return useQuery({
    queryKey: credentialKeys.list(filter),
    queryFn: ({ signal }) => api.get("/credential-controls", { query: filter ? { filter } : {}, signal }),
  });
}

/** GET /companies/{id}/credential-controls. */
export function useCompanyCredentials(companyId: string) {
  return useQuery({
    queryKey: credentialKeys.company(companyId),
    queryFn: ({ signal }) => api.get("/companies/{id}/credential-controls", { path: { id: companyId }, signal }),
  });
}

export interface UpdateCredentialInput {
  companyId: string;
  type: CredentialType;
  body: Schemas["CredentialUpdate"];
}

/** PUT /companies/{id}/credential-controls/{type} [UC-05]. */
export function useUpdateCredential() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ companyId, type, body }: UpdateCredentialInput) =>
      api.put("/companies/{id}/credential-controls/{type}", { path: { id: companyId, type }, body }),
    onSettled: () => qc.invalidateQueries({ queryKey: credentialKeys.all }),
  });
}

export interface UploadCredentialInput {
  companyId: string;
  type: CredentialType;
  file: File;
}

/** PUT …/{type}/file (multipart). Recusas (chave privada, formato) vêm com a mensagem do backend. */
export function useUploadCredentialFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ companyId, type, file }: UploadCredentialInput) => {
      const form = new FormData();
      form.append("file", file);
      return apiUpload<CredentialControl>("put", "/companies/{id}/credential-controls/{type}/file", form, { id: companyId, type });
    },
    onSettled: () => qc.invalidateQueries({ queryKey: credentialKeys.all }),
  });
}

/** DELETE …/{type}/file. */
export function useDeleteCredentialFile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ companyId, type }: { companyId: string; type: CredentialType }) =>
      api.delete("/companies/{id}/credential-controls/{type}/file", { path: { id: companyId, type } }),
    onSettled: () => qc.invalidateQueries({ queryKey: credentialKeys.all }),
  });
}
