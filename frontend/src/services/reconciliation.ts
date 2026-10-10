import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiUpload, buildUrl, type Schemas } from "@/lib/api";
import { JOB_POLL_MS, jobActive } from "./jobs";

export type ReconciliationDocument = Schemas["ReconciliationDocument"];
export type ReconciliationDocumentDetail = Schemas["ReconciliationDocumentDetail"];
export type ReconciliationDocumentStatus = Schemas["ReconciliationDocumentStatus"];
export type ReconciliationUpload = Schemas["ReconciliationUpload"];
export type ClientEntry = Schemas["ReconciliationClientEntry"];
export type ReviewBucket = Schemas["ReviewBucket"];
export type ReviewQueue = Schemas["ReviewQueue"];
export type ReviewQueueItem = Schemas["ReviewQueueItem"];
export type ReviewLineDetail = Schemas["ReviewLineDetail"];
export type ReviewEntryDetail = Schemas["ReviewEntryDetail"];
export type ReviewCandidate = Schemas["ReviewCandidate"];
export type ManualMatchResult = Schemas["ManualMatchResult"];
export type RecentReconciliation = Schemas["RecentReconciliation"];
export type DocumentIdentify = Schemas["DocumentIdentify"];
export type EntryFieldsPatch = Schemas["EntryFieldsPatch"];
export type ResolverProposal = Schemas["ResolverProposal"];
export type ClassificationSuggestion = Schemas["ClassificationSuggestion"];
export type ChartAccount = Schemas["ChartAccount"];
export type RuleProposal = Schemas["ClassificationRuleProposal"];

/** Chaves sob ["reconciliation"]: toda ação da fila recarrega a fila, os documentos e a linha. */
export const reconciliationKeys = {
  all: ["reconciliation"] as const,
  documents: (filters: object) => ["reconciliation", "documents", filters] as const,
  document: (id: string) => ["reconciliation", "document", id] as const,
  pages: (id: string) => ["reconciliation", "pages", id] as const,
  queue: (filters: object) => ["reconciliation", "queue", filters] as const,
  line: (id: string) => ["reconciliation", "line", id] as const,
  entry: (id: string) => ["reconciliation", "entry", id] as const,
  recent: (companyId: string) => ["reconciliation", "recent", companyId] as const,
  proposals: (lineId: string) => ["reconciliation", "proposals", lineId] as const,
  suggestions: (lineId: string) => ["reconciliation", "suggestions", lineId] as const,
  chart: (companyId: string) => ["reconciliation", "chart", companyId] as const,
};

/** O documento ainda vai ser processado pelo worker (pendente ou triado com o job ativo)? */
export function documentProcessing(d: Pick<ReconciliationDocument, "status" | "job">): boolean {
  return (d.status === "pendente" || d.status === "triado") && jobActive(d.job?.status);
}

export interface DocumentFilters {
  status?: ReconciliationDocumentStatus | undefined;
  companyId?: string | undefined;
  competence?: string | undefined;
}

/** GET /reconciliation/documents — acompanha a lista enquanto há documento em processamento. */
export function useReconciliationDocuments(f: DocumentFilters) {
  return useQuery({
    queryKey: reconciliationKeys.documents(f),
    queryFn: ({ signal }) =>
      api.get("/reconciliation/documents", {
        query: { status: f.status, company_id: f.companyId, competence: f.competence, limit: 200 },
        signal,
      }),
    placeholderData: keepPreviousData,
    refetchInterval: (q) => (q.state.data?.items.some(documentProcessing) ? JOB_POLL_MS : false),
  });
}

/** GET /reconciliation/documents/{id} — acompanha o documento enquanto o job não termina. */
export function useReconciliationDocument(id: string | undefined) {
  return useQuery({
    queryKey: reconciliationKeys.document(id ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/documents/{id}", { path: { id: id ?? "" }, signal }),
    enabled: !!id,
    refetchInterval: (q) => (q.state.data && documentProcessing(q.state.data) ? JOB_POLL_MS : false),
  });
}

/** Link do original (a versão gravada no envio). */
export function documentFileUrl(id: string): string {
  return buildUrl("/reconciliation/documents/{id}/file", { id });
}

/** Imagem da página do PDF no DPI do extrator (o bbox dos campos cai no lugar). */
export function documentPageUrl(id: string, page: number): string {
  return buildUrl("/reconciliation/documents/{id}/pages/{page}", { id, page });
}

/** GET /reconciliation/documents/{id}/pages — quantas páginas e em que DPI. */
export function useDocumentPages(id: string | null | undefined) {
  return useQuery({
    queryKey: reconciliationKeys.pages(id ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/documents/{id}/pages", { path: { id: id ?? "" }, signal }),
    enabled: !!id,
    staleTime: Infinity,
  });
}

export interface UploadInput {
  file: File;
  companyId?: string | undefined;
  competence?: string | undefined;
}

/** POST /reconciliation/documents (multipart): 202 com o job da triagem. */
export function useUploadDocument() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ file, companyId, competence }: UploadInput) => {
      const form = new FormData();
      form.append("file", file);
      if (companyId) form.append("company_id", companyId);
      if (competence) form.append("competence", competence);
      return apiUpload<ReconciliationUpload>("post", "/reconciliation/documents", form);
    },
    onSettled: () => qc.invalidateQueries({ queryKey: reconciliationKeys.all }),
  });
}

/** Toda ação da Conciliação muda a fila, os documentos e a linha: recarrega tudo sob a chave. */
function useInvalidateAll() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: reconciliationKeys.all });
}

/** POST /reconciliation/documents/{id}/retry — o documento volta para a fila. */
export function useRetryDocument(id: string) {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: () => api.post("/reconciliation/documents/{id}/retry", { path: { id } }),
    onSuccess: done,
  });
}

/** POST /reconciliation/documents/{id}/identify — "que documento é esse?". */
export function useIdentifyDocument(id: string) {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: (body: DocumentIdentify) => api.post("/reconciliation/documents/{id}/identify", { path: { id }, body }),
    onSuccess: done,
  });
}

export interface QueueFilters {
  companyId?: string | undefined;
  competence?: string | undefined;
  bucket?: ReviewBucket | undefined;
}

/** GET /reconciliation/review-queue — os itens e as contagens por balde. */
export function useReviewQueue(f: QueueFilters) {
  return useQuery({
    queryKey: reconciliationKeys.queue(f),
    queryFn: ({ signal }) =>
      api.get("/reconciliation/review-queue", {
        query: { company_id: f.companyId, competence: f.competence, bucket: f.bucket, limit: 300 },
        signal,
      }),
    placeholderData: keepPreviousData,
  });
}

/** GET /reconciliation/review/{id} — a linha, a conciliação ou os candidatos. */
export function useReviewLine(id: string | null | undefined) {
  return useQuery({
    queryKey: reconciliationKeys.line(id ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/review/{id}", { path: { id: id ?? "" }, signal }),
    enabled: !!id,
  });
}

/** GET /reconciliation/entries/{id} — o comprovante, a conciliação ou as linhas candidatas. */
export function useReviewEntry(id: string | null | undefined) {
  return useQuery({
    queryKey: reconciliationKeys.entry(id ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/entries/{id}", { path: { id: id ?? "" }, signal }),
    enabled: !!id,
  });
}

/** POST /reconciliation/review/{id}/manual-match — conciliar à mão (N:1). */
export function useManualMatch() {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: ({ lineId, entryIds }: { lineId: string; entryIds: string[] }) =>
      api.post("/reconciliation/review/{id}/manual-match", { path: { id: lineId }, body: { entry_ids: entryIds } }),
    onSuccess: done,
  });
}

/** POST /reconciliation/review/{id}/confirm-no-receipt — "não há comprovante". */
export function useConfirmNoReceipt() {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: (lineId: string) => api.post("/reconciliation/review/{id}/confirm-no-receipt", { path: { id: lineId } }),
    onSuccess: done,
  });
}

/** GET /reconciliation/reconciliations — as últimas conciliações (para desfazer). */
export function useRecentReconciliations(companyId: string | undefined) {
  return useQuery({
    queryKey: reconciliationKeys.recent(companyId ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/reconciliations", { query: { company_id: companyId, limit: 8 }, signal }),
  });
}

/** POST /reconciliation/reconciliations/{id}/undo — desfazer com motivo. */
export function useUndoReconciliation() {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      api.post("/reconciliation/reconciliations/{id}/undo", { path: { id }, body: { reason } }),
    onSuccess: done,
  });
}

/** PATCH /reconciliation/entries/{id} — correção campo a campo do comprovante. */
export function useCorrectEntry(id: string) {
  const done = useInvalidateAll();
  return useMutation({
    mutationFn: (body: EntryFieldsPatch) => api.patch("/reconciliation/entries/{id}", { path: { id }, body }),
    onSuccess: done,
  });
}

/** Propostas do resolvedor da linha (TASK-019): acompanha enquanto o pedido está na fila. */
export function useProposals(lineId: string | null | undefined, polling: boolean) {
  return useQuery({
    queryKey: reconciliationKeys.proposals(lineId ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/review/{id}/proposals", { path: { id: lineId ?? "" }, signal }),
    enabled: !!lineId,
    refetchInterval: polling ? JOB_POLL_MS : false,
  });
}

/** Pedir, aceitar e rejeitar a hipótese do resolvedor. */
export function useProposalActions(lineId: string) {
  const done = useInvalidateAll();
  return {
    request: useMutation({
      mutationFn: () => api.post("/reconciliation/review/{id}/proposal", { path: { id: lineId } }),
      onSuccess: done,
    }),
    accept: useMutation({
      mutationFn: (proposalId: string) =>
        api.post("/reconciliation/review/{id}/accept-proposal", { path: { id: lineId }, body: { proposal_id: proposalId } }),
      onSuccess: done,
    }),
    reject: useMutation({
      mutationFn: (proposalId: string) =>
        api.post("/reconciliation/review/{id}/reject-proposal", { path: { id: lineId }, body: { proposal_id: proposalId } }),
      onSuccess: done,
    }),
  };
}

/** Plano de contas ativo da empresa (a classificação na fila). */
export function useChartAccounts(companyId: string | null | undefined) {
  return useQuery({
    queryKey: reconciliationKeys.chart(companyId ?? ""),
    queryFn: ({ signal }) =>
      api.get("/companies/{id}/chart-accounts", { path: { id: companyId ?? "" }, query: { active: "true" }, signal }),
    enabled: !!companyId,
  });
}

/** Sugestões do classificador da linha (TASK-020): acompanha enquanto o pedido está na fila. */
export function useSuggestions(lineId: string | null | undefined, polling: boolean) {
  return useQuery({
    queryKey: reconciliationKeys.suggestions(lineId ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/review/{id}/classification-suggestions", { path: { id: lineId ?? "" }, signal }),
    enabled: !!lineId,
    refetchInterval: polling ? JOB_POLL_MS : false,
  });
}

/** Corrigir a conta, pedir/aceitar/rejeitar a sugestão e confirmar a regra proposta. */
export function useClassificationActions(lineId: string, companyId: string) {
  const done = useInvalidateAll();
  return {
    correct: useMutation({
      mutationFn: (accountCode: string) =>
        api.put("/reconciliation/review/{id}/classification", { path: { id: lineId }, body: { account_code: accountCode } }),
      onSuccess: done,
    }),
    request: useMutation({
      mutationFn: () => api.post("/reconciliation/review/{id}/classification-suggestion", { path: { id: lineId } }),
      onSuccess: done,
    }),
    accept: useMutation({
      mutationFn: (suggestionId: string) =>
        api.post("/reconciliation/review/{id}/accept-classification-suggestion", { path: { id: lineId }, body: { suggestion_id: suggestionId } }),
      onSuccess: done,
    }),
    reject: useMutation({
      mutationFn: (suggestionId: string) =>
        api.post("/reconciliation/review/{id}/reject-classification-suggestion", { path: { id: lineId }, body: { suggestion_id: suggestionId } }),
      onSuccess: done,
    }),
    confirmRule: useMutation({
      mutationFn: (rule: RuleProposal) =>
        api.post("/companies/{id}/classification-rules", {
          path: { id: companyId },
          body: { rule_type: rule.rule_type, pattern: rule.pattern, account_code: rule.account_code, source_statement_transaction_id: rule.source_statement_transaction_id },
        }),
      onSuccess: done,
    }),
  };
}
