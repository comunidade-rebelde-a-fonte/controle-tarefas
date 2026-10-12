import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, apiUpload, buildUrl, type Schemas } from "@/lib/api";
import type { CompanyFilter } from "@/lib/companyFilter";
import { JOB_POLL_MS, jobActive } from "./jobs";
import { obligationKeys } from "./obligations";

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
  /** Um ou mais status, separados por vírgula (os grupos do Dashboard). */
  status?: string | undefined;
  companyId?: string | undefined;
  competence?: string | undefined;
  /** Os filtros de empresa do Dashboard, pelo snapshot de filterCompetence (D-038). */
  filter?: CompanyFilter | undefined;
  filterCompetence?: string | undefined;
}

/** GET /reconciliation/documents — acompanha a lista enquanto há documento em processamento. */
export function useReconciliationDocuments(f: DocumentFilters) {
  return useQuery({
    queryKey: reconciliationKeys.documents(f),
    queryFn: ({ signal }) =>
      api.get("/reconciliation/documents", {
        query: {
          status: f.status,
          company_id: f.companyId,
          competence: f.competence,
          ...f.filter,
          filter_competence: f.filterCompetence,
          limit: 200,
        },
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
/**
 * Invalida as consultas da Conciliação sem esperar o recarregamento: o retorno de cada ação (o
 * aviso, fechar o diálogo) roda logo após a resposta, com o componente que a disparou ainda na
 * tela. Esperando, a linha recarregada podia tirar da tela o botão da ação (o "Desfazer" some
 * quando a conciliação é desfeita), e o aviso se perdia de vez em quando.
 */
function useInvalidateAll() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: reconciliationKeys.all });
  };
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
  /** Os filtros de empresa do Dashboard, pelo snapshot da competência (D-038). */
  filter?: CompanyFilter | undefined;
}

/** GET /reconciliation/review-queue — os itens e as contagens por balde. */
export function useReviewQueue(f: QueueFilters) {
  return useQuery({
    queryKey: reconciliationKeys.queue(f),
    queryFn: ({ signal }) =>
      api.get("/reconciliation/review-queue", {
        query: { company_id: f.companyId, competence: f.competence, bucket: f.bucket, ...f.filter, limit: 300 },
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

// --- TASK-021b: lançamentos, rodada, razão, plano de contas e regras ---

export type EntryCreate = Schemas["ReconciliationClientEntryCreate"];
export type EntriesImport = Schemas["ReconciliationEntriesImport"];
export type RunSummary = Schemas["ReconciliationRunSummary"];
export type Ledger = Schemas["Ledger"];
export type LedgerLine = Schemas["LedgerLine"];
export type ReconciliationDetail = Schemas["ReconciliationDetail"];
export type ChartAccountsImport = Schemas["ChartAccountsImport"];
export type ChartAccountsCopy = Schemas["ChartAccountsCopy"];
export type ClassificationRule = Schemas["ClassificationRule"];
export type ClassificationRuleCreate = Schemas["ClassificationRuleCreate"];
export type ClassificationRunSummary = Schemas["ClassificationRunSummary"];
export type ClassificationRuleType = Schemas["ClassificationRuleType"];
export type ReconciliationEntryStatus = Schemas["ReconciliationEntryStatus"];

export const reconciliationExtraKeys = {
  entries: (filters: object) => ["reconciliation", "entries", filters] as const,
  ledger: (filters: object) => ["reconciliation", "ledger", filters] as const,
  detail: (id: string) => ["reconciliation", "detail", id] as const,
  chartAll: (companyId: string) => ["reconciliation", "chart-all", companyId] as const,
  rules: (companyId: string) => ["reconciliation", "rules", companyId] as const,
};

export interface EntryFilters {
  companyId?: string | undefined;
  status?: ReconciliationEntryStatus | undefined;
  dateFrom?: string | undefined;
  dateTo?: string | undefined;
  offset?: number;
}

/** GET /reconciliation/entries — os lançamentos do cliente (lidos e manuais). */
export function useEntries(f: EntryFilters) {
  return useQuery({
    queryKey: reconciliationExtraKeys.entries(f),
    queryFn: ({ signal }) =>
      api.get("/reconciliation/entries", {
        query: { company_id: f.companyId, status: f.status, date_from: f.dateFrom, date_to: f.dateTo, limit: 100, offset: f.offset ?? 0 },
        signal,
      }),
    placeholderData: keepPreviousData,
  });
}

/** POST /reconciliation/entries — lançamento manual (CC-12). */
export function useCreateEntry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: EntryCreate) => api.post("/reconciliation/entries", { body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: reconciliationKeys.all }),
  });
}

/** POST /reconciliation/entries/import — CSV data;valor;sentido;contraparte, tudo ou nada. */
export function useImportEntries() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ file, companyId }: { file: File; companyId: string }) => {
      const form = new FormData();
      form.append("file", file);
      form.append("company_id", companyId);
      return apiUpload<EntriesImport>("post", "/reconciliation/entries/import", form);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: reconciliationKeys.all }),
  });
}

/** POST /companies/{id}/reconciliation/run — a rodada do matching da competência (e a classificação). */
export function useRunReconciliation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ companyId, competence }: { companyId: string; competence: string }) =>
      api.post("/companies/{id}/reconciliation/run", { path: { id: companyId }, query: { competence } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: reconciliationKeys.all }),
  });
}

export interface LedgerFilters {
  companyId: string;
  from: string;
  to: string;
  account?: string | undefined;
}

/** GET /companies/{id}/ledger — o razão como consulta (M5.6). */
export function useLedger(f: LedgerFilters, enabled: boolean) {
  return useQuery({
    queryKey: reconciliationExtraKeys.ledger(f),
    queryFn: ({ signal }) =>
      api.get("/companies/{id}/ledger", { path: { id: f.companyId }, query: { from: f.from, to: f.to, account: f.account || undefined }, signal }),
    enabled,
    placeholderData: keepPreviousData,
  });
}

/** Link da exportação CSV do razão filtrado (AC-022). */
export function ledgerExportUrl(f: LedgerFilters): string {
  return buildUrl("/companies/{id}/ledger/export", { id: f.companyId }, { format: "csv", from: f.from, to: f.to, account: f.account || undefined });
}

/** GET /reconciliation/reconciliations/{id} — o drill-down da linha do razão (AC-021). */
export function useReconciliationDetail(id: string | null) {
  return useQuery({
    queryKey: reconciliationExtraKeys.detail(id ?? ""),
    queryFn: ({ signal }) => api.get("/reconciliation/reconciliations/{id}", { path: { id: id ?? "" }, signal }),
    enabled: !!id,
  });
}

/** O plano de contas inteiro da empresa (ativas e desativadas). */
export function useChart(companyId: string | undefined) {
  return useQuery({
    queryKey: reconciliationExtraKeys.chartAll(companyId ?? ""),
    queryFn: ({ signal }) => api.get("/companies/{id}/chart-accounts", { path: { id: companyId ?? "" }, signal }),
    enabled: !!companyId,
  });
}

/** Incluir, alterar, importar e copiar o plano de contas. */
export function useChartActions(companyId: string) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: reconciliationKeys.all });
  return {
    create: useMutation({
      mutationFn: (body: { code: string; name: string }) => api.post("/companies/{id}/chart-accounts", { path: { id: companyId }, body }),
      onSuccess: done,
    }),
    update: useMutation({
      mutationFn: ({ accountId, ...body }: { accountId: string; name?: string; active?: boolean }) =>
        api.patch("/companies/{id}/chart-accounts/{accountId}", { path: { id: companyId, accountId }, body }),
      onSuccess: done,
    }),
    importCsv: useMutation({
      mutationFn: (file: File) => {
        const form = new FormData();
        form.append("file", file);
        return apiUpload<ChartAccountsImport>("post", "/companies/{id}/chart-accounts/import", form, { id: companyId });
      },
      onSuccess: done,
    }),
    copy: useMutation({
      mutationFn: (sourceId: string) =>
        api.post("/companies/{id}/chart-accounts/copy", { path: { id: companyId }, body: { source_company_id: sourceId } }),
      onSuccess: done,
    }),
  };
}

/** GET /companies/{id}/classification-rules — da mais nova para a mais antiga. */
export function useRules(companyId: string | undefined) {
  return useQuery({
    queryKey: reconciliationExtraKeys.rules(companyId ?? ""),
    queryFn: ({ signal }) => api.get("/companies/{id}/classification-rules", { path: { id: companyId ?? "" }, signal }),
    enabled: !!companyId,
  });
}

/** Confirmar e desativar regras, e aplicar as regras às linhas que esperam classificação. */
export function useRuleActions(companyId: string) {
  const qc = useQueryClient();
  const done = () => qc.invalidateQueries({ queryKey: reconciliationKeys.all });
  return {
    create: useMutation({
      mutationFn: (body: ClassificationRuleCreate) => api.post("/companies/{id}/classification-rules", { path: { id: companyId }, body }),
      onSuccess: done,
    }),
    deactivate: useMutation({
      mutationFn: (ruleId: string) => api.post("/companies/{id}/classification-rules/{ruleId}/deactivate", { path: { id: companyId, ruleId } }),
      onSuccess: done,
    }),
    classify: useMutation({
      mutationFn: (competence: string) => api.post("/companies/{id}/classification/run", { path: { id: companyId }, query: { competence } }),
      onSuccess: done,
    }),
  };
}

// --- TASK-021c: métricas, playbooks, resumo da ficha e contas bancárias ---

export type ReconciliationMetrics = Schemas["ReconciliationMetrics"];
export type ReconciliationMatching = Schemas["ReconciliationMatching"];
export type ReconciliationExtraction = Schemas["ReconciliationExtraction"];
export type ReconciliationAgentCost = Schemas["ReconciliationAgentCost"];
export type BankPlaybook = Schemas["BankPlaybook"];
export type CompanyReconciliationSummary = Schemas["CompanyReconciliationSummary"];
export type BankAccount = Schemas["BankAccount"];
export type BankAccountList = Schemas["BankAccountList"];
export type BankAccountCreate = Schemas["BankAccountCreate"];

export const reconciliationPanelKeys = {
  metrics: (filters: object) => ["reconciliation", "metrics", filters] as const,
  playbooks: ["reconciliation", "playbooks"] as const,
  summary: (companyId: string, competence: string) => ["reconciliation", "summary", companyId, competence] as const,
  bankAccounts: (companyId: string) => ["reconciliation", "bank-accounts", companyId] as const,
};

/** GET /reconciliation/metrics — o painel do 11-observability §7, por empresa e competência. */
export function useReconciliationMetrics(f: { companyId?: string | undefined; competence?: string | undefined }) {
  return useQuery({
    queryKey: reconciliationPanelKeys.metrics(f),
    queryFn: ({ signal }) => api.get("/reconciliation/metrics", { query: { company_id: f.companyId, competence: f.competence }, signal }),
    placeholderData: keepPreviousData,
  });
}

/** GET /reconciliation/playbooks — as versões dos playbooks por banco (somente leitura). */
export function usePlaybooks() {
  return useQuery({ queryKey: reconciliationPanelKeys.playbooks, queryFn: ({ signal }) => api.get("/reconciliation/playbooks", { signal }) });
}

/** GET /companies/{id}/reconciliation — a Conciliação da empresa na competência (a aba da ficha). */
export function useCompanyReconciliationSummary(companyId: string, competence: string | null) {
  return useQuery({
    queryKey: reconciliationPanelKeys.summary(companyId, competence ?? ""),
    queryFn: ({ signal }) =>
      api.get("/companies/{id}/reconciliation", { path: { id: companyId }, query: { competence: competence ?? "" }, signal }),
    enabled: !!competence,
    placeholderData: keepPreviousData,
  });
}

/** GET /companies/{id}/bank-accounts — as contas cadastradas e as lidas nos extratos sem cadastro. */
export function useBankAccounts(companyId: string) {
  return useQuery({
    queryKey: reconciliationPanelKeys.bankAccounts(companyId),
    queryFn: ({ signal }) => api.get("/companies/{id}/bank-accounts", { path: { id: companyId }, signal }),
  });
}

/** POST /companies/{id}/bank-accounts — cadastra a conta (CC-11). */
export function useCreateBankAccount(companyId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: BankAccountCreate) => api.post("/companies/{id}/bank-accounts", { path: { id: companyId }, body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: reconciliationKeys.all }),
  });
}

// --- TASK-022: conclusão da conciliação e a obrigação Conciliação do Contábil ---

export type ReconciliationCompletion = Schemas["ReconciliationCompletion"];
export type ReconciliationCompletionRecord = Schemas["ReconciliationCompletionRecord"];

/** Concluir ou reabrir muda a obrigação Conciliação: recarrega a Conciliação e as grades do Controle. */
function useCompletionMutation<V>(fn: (v: V) => Promise<ReconciliationCompletion>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: reconciliationKeys.all });
      void qc.invalidateQueries({ queryKey: obligationKeys.all });
    },
  });
}

/** POST /companies/{id}/reconciliation/complete — conclui a competência pronta (a obrigação vai para OK). */
export function useCompleteReconciliation(companyId: string) {
  return useCompletionMutation((competence: string) =>
    api.post("/companies/{id}/reconciliation/complete", { path: { id: companyId }, query: { competence } }),
  );
}

/** POST /companies/{id}/reconciliation/reopen — reabre com motivo (a obrigação volta para Pendente). */
export function useReopenReconciliation(companyId: string) {
  return useCompletionMutation(({ competence, reason }: { competence: string; reason: string }) =>
    api.post("/companies/{id}/reconciliation/reopen", { path: { id: companyId }, query: { competence }, body: { reason } }),
  );
}

// --- TASK-023: o fechamento do Contábil visto pela Conciliação ---

export type ReconciliationClosure = Schemas["ReconciliationClosure"];

/** GET /companies/{id}/reconciliation/closure — o período do Contábil e os eventos (06 §5). */
export function useReconciliationClosure(companyId: string, competence: string | null) {
  return useQuery({
    queryKey: ["reconciliation", "closure", companyId, competence ?? ""] as const,
    queryFn: ({ signal }) =>
      api.get("/companies/{id}/reconciliation/closure", { path: { id: companyId }, query: { competence: competence ?? "" }, signal }),
    enabled: !!competence,
  });
}

// --- TASK-024: a Conciliação por empresa na competência ---

export type ReconciliationOverview = Schemas["ReconciliationOverview"];
export type ReconciliationOverviewCompany = Schemas["ReconciliationOverviewCompany"];
export type ReconciliationOverviewState = ReconciliationOverviewCompany["state"];

export interface OverviewFilters {
  state?: ReconciliationOverviewState | undefined;
  blocked?: boolean | undefined;
  filter?: CompanyFilter | undefined;
}

/** GET /reconciliation/companies — a lista das prontas para concluir e do Contábil bloqueado. */
export function useReconciliationOverview(competence: string | null, f: OverviewFilters = {}) {
  return useQuery({
    queryKey: ["reconciliation", "overview", competence ?? "", f] as const,
    queryFn: ({ signal }) =>
      api.get("/reconciliation/companies", {
        query: { competence: competence ?? "", state: f.state, blocked: f.blocked ? true : undefined, ...f.filter },
        signal,
      }),
    enabled: !!competence,
    placeholderData: keepPreviousData,
  });
}
