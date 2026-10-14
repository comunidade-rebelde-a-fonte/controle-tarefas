import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiUpload, buildUrl, type Schemas } from "@/lib/api";
import { aiRunKeys } from "./aiRuns";
import { companyKeys } from "./companies";
import { factorRKeys } from "./factorR";
import { JOB_POLL_MS, jobActive } from "./jobs";

export type PgdasDocument = Schemas["PgdasDocument"];
export type PgdasDocumentList = Schemas["PgdasDocumentList"];
export type PgdasStatus = Schemas["PgdasStatus"];
export type PgdasUpload = Schemas["PgdasUpload"];
export type PgdasSuggestion = Schemas["PgdasSuggestion"];
export type PgdasCreateCompanyInput = Schemas["PgdasCreateCompanyInput"];

/** Chaves sob ["factor-r"]: o que muda o Fator R (vínculo, cadastro) recarrega também o inbox. */
export const inboxKeys = {
  all: ["factor-r", "pgdas"] as const,
  list: (status: string, companyId: string) => ["factor-r", "pgdas", "list", status, companyId] as const,
  detail: (id: string) => ["factor-r", "pgdas", "detail", id] as const,
};

/** O worker ainda vai processar o documento (recebido, com o job na fila ou em execução)? */
export function isProcessing(d: Pick<PgdasDocument, "status" | "job">): boolean {
  return d.status === "received" && jobActive(d.job?.status);
}

/** GET /factor-r/pgdas — acompanha a lista enquanto algum documento está em processamento. */
export function usePgdasDocuments(filter: { status?: PgdasStatus | undefined; companyId?: string | undefined; limit?: number }) {
  return useQuery({
    queryKey: inboxKeys.list(filter.status ?? "", filter.companyId ?? ""),
    queryFn: ({ signal }) =>
      api.get("/factor-r/pgdas", { query: { status: filter.status, company_id: filter.companyId, limit: filter.limit ?? 200 }, signal }),
    placeholderData: keepPreviousData,
    refetchInterval: (q) => (q.state.data?.items.some(isProcessing) ? JOB_POLL_MS : false),
  });
}

/** GET /factor-r/pgdas/{id} — acompanha o documento enquanto o job não termina. */
export function usePgdasDocument(id: string) {
  return useQuery({
    queryKey: inboxKeys.detail(id),
    queryFn: ({ signal }) => api.get("/factor-r/pgdas/{id}", { path: { id }, signal }),
    refetchInterval: (q) => (q.state.data && isProcessing(q.state.data) ? JOB_POLL_MS : false),
  });
}

/** Link do original (anexo; o servidor nunca o exibe inline). */
export function pgdasFileUrl(id: string): string {
  return buildUrl("/factor-r/pgdas/{id}/file", { id });
}

/** POST /factor-r/pgdas (multipart, campo `file`): 202 com o job, ou 200 se o arquivo já existia. */
export function useUploadPgdas() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData();
      form.append("file", file);
      return apiUpload<PgdasUpload>("post", "/factor-r/pgdas", form);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: inboxKeys.all }),
  });
}

/** Vínculo, rejeição e cadastro mudam o documento, a série do Fator R e as corridas. */
function useAfterDecision() {
  const qc = useQueryClient();
  return (doc: PgdasDocument) => {
    qc.setQueryData(inboxKeys.detail(doc.id), doc);
    void qc.invalidateQueries({ queryKey: factorRKeys.all });
    void qc.invalidateQueries({ queryKey: aiRunKeys.all });
  };
}

/** POST /factor-r/pgdas/{id}/link — vínculo manual de um documento em revisão. */
export function useLinkPgdas(id: string) {
  const after = useAfterDecision();
  return useMutation({
    mutationFn: (companyId: string) => api.post("/factor-r/pgdas/{id}/link", { path: { id }, body: { company_id: companyId } }),
    onSuccess: after,
  });
}

/** POST /factor-r/pgdas/{id}/review — rejeição com motivo. */
export function useRejectPgdas(id: string) {
  const after = useAfterDecision();
  return useMutation({
    mutationFn: (motivo: string) => api.post("/factor-r/pgdas/{id}/review", { path: { id }, body: { decisao: "rejeitar", motivo } }),
    onSuccess: after,
  });
}

/** POST /factor-r/pgdas/{id}/create-company — cadastra a empresa do extrato e vincula, numa transação. */
export function useCreateCompanyFromPgdas(id: string) {
  const qc = useQueryClient();
  const after = useAfterDecision();
  return useMutation({
    mutationFn: (body: PgdasCreateCompanyInput) => api.post("/factor-r/pgdas/{id}/create-company", { path: { id }, body }),
    onSuccess: (result) => {
      after(result.documento);
      void qc.invalidateQueries({ queryKey: companyKeys.all });
    },
  });
}
