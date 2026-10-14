import type { BadgeProps } from "@/components/ui/badge";
import { DOC_STATUS } from "@/features/reconciliation/labels";
import type { DocumentKind, DocumentModule, DocumentSituation } from "@/services/documents";

/** As origens do índice (TASK-026, D-040), com o armazenamento de cada uma. */
export const MODULES: [DocumentModule, string][] = [
  ["factor_r", "Fator R"],
  ["reconciliation", "Conciliação"],
  ["credentials", "Procurações/Certificados"],
];

export const STORAGE_LABELS: Record<string, string> = { local: "armazenamento local", minio: "MinIO" };

export const KIND_LABELS: Record<DocumentKind, string> = {
  pgdas: "Extrato do PGDAS-D",
  extrato: "Extrato bancário",
  comprovante: "Comprovante",
  desconhecido: "Tipo a definir",
  procuracao: "Procuração RFB",
  certificado: "Certificado digital",
};

/** Os tipos de cada origem, para o filtro. */
export const KINDS_BY_MODULE: Record<DocumentModule, DocumentKind[]> = {
  factor_r: ["pgdas"],
  reconciliation: ["extrato", "comprovante", "desconhecido"],
  credentials: ["procuracao", "certificado"],
};

/** A situação comum é só leitura, para filtrar entre módulos (decisão do usuário). */
export const SITUATIONS: [DocumentSituation, string, NonNullable<BadgeProps["variant"]>][] = [
  ["em_processamento", "Em processamento", "default"],
  ["em_revisao", "Em revisão", "warning"],
  ["concluido", "Concluído", "success"],
  ["descartado", "Descartado", "muted"],
];

/** Os status de cada módulo, como cada um os mostra (o Fator R no singular da lista do inbox). */
export const STATUS_LABELS: Record<DocumentModule, Record<string, string>> = {
  factor_r: { received: "Recebido", parsed: "Extraído", needs_review: "Em revisão", linked: "Vinculado", rejected: "Rejeitado" },
  reconciliation: DOC_STATUS,
  credentials: { arquivado: "Arquivado" },
};

export function statusText(module: DocumentModule, status: string): string {
  return STATUS_LABELS[module][status] ?? status;
}

/** O tamanho da página do índice. */
export const PAGE_SIZE = 50;
