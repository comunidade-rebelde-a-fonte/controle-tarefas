import { isApiError } from "@/lib/api";
import type { ClientEntry, ReconciliationDocumentDetail } from "@/services/reconciliation";
import { FIELD_LABELS } from "../labels";
import type { Highlight } from "../OriginalViewer";

/** A origem de um campo do comprovante: a página, o trecho e o bbox lidos, ou a correção humana. */
export interface FieldOrigin {
  pagina?: number;
  trecho?: string;
  bbox?: number[] | null;
  origem?: string;
}

/** As marcações dos campos do comprovante (bbox por página) para o visualizador. */
export function entryHighlights(entry: ClientEntry | null | undefined): Highlight[] {
  const origin = (entry?.field_origin ?? {}) as Record<string, FieldOrigin | undefined>;
  const out: Highlight[] = [];
  for (const [field, o] of Object.entries(origin)) {
    if (o?.bbox && o.bbox.length === 4) out.push({ page: o.pagina ?? 1, bbox: o.bbox, label: FIELD_LABELS[field] ?? field });
  }
  return out;
}

/** Pode reprocessar: revisão, período fechado ou job encerrado com falha (06 §4). */
export function canRetry(d: ReconciliationDocumentDetail): boolean {
  return (
    d.status === "revisao_triagem" ||
    d.status === "pendencia_periodo_fechado" ||
    (d.status === "revisao_extracao" && !d.entry) ||
    ((d.status === "pendente" || d.status === "triado") && d.job?.status === "dead")
  );
}

/** Os erros por campo do 400 VALIDATION_ERROR (details.fields). */
export function fieldErrorsOf(err: unknown): Record<string, string> {
  if (!isApiError(err)) return {};
  const fields = err.details.fields;
  return typeof fields === "object" && fields !== null ? (fields as Record<string, string>) : {};
}
