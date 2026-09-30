import { useMutation, useQueryClient } from "@tanstack/react-query";
import { API_BASE, api, apiUpload, type Schemas } from "@/lib/api";
import { companyKeys } from "./companies";
import { credentialKeys } from "./credentials";

export type ImportPreview = Schemas["ImportPreview"];
export type ImportRow = Schemas["ImportRow"];
export type ImportRowStatus = Schemas["ImportRowStatus"];
export type ImportResult = Schemas["ImportResult"];

/** Download do modelo XLSX (GET /imports/companies/template). */
export const IMPORT_TEMPLATE_URL = `${API_BASE}/imports/companies/template`;

/** POST /imports/companies/preview: valida e classifica, sem gravar [PRD §35.3]. */
export function useImportPreview() {
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return apiUpload<ImportPreview>("post", "/imports/companies/preview", form);
    },
  });
}

/** POST /imports/companies/commit: grava as novas e as existentes confirmadas. */
export function useImportCommit() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["ImportCommit"]) => api.post("/imports/companies/commit", { body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: companyKeys.all });
      void qc.invalidateQueries({ queryKey: credentialKeys.all });
    },
  });
}
