import { useSearchParams } from "react-router";
import { isCompetenceKey } from "@/lib/competence";
import type { DocumentIndexQuery, DocumentKind, DocumentModule, DocumentSituation } from "@/services/documents";
import { KIND_LABELS, MODULES, PAGE_SIZE, SITUATIONS } from "./labels";

const isModule = (v: string | null): v is DocumentModule => MODULES.some(([m]) => m === v);
const isSituation = (v: string | null): v is DocumentSituation => SITUATIONS.some(([s]) => s === v);
export const ALL_KINDS = Object.keys(KIND_LABELS) as DocumentKind[];
const isKind = (v: string | null): v is DocumentKind => ALL_KINDS.some((k) => k === v);

/**
 * Os filtros da área na URL: origem, situação, status do módulo, empresa, competência, tipo e
 * banco. Na ficha, a empresa vem fixa e a competência é a da ficha (nula: todas).
 */
export function useDocumentFilters(fixed: { companyId?: string; competence?: string | null } = {}) {
  const [params, setParams] = useSearchParams();
  const raw = (k: string) => params.get(k);
  const module = isModule(raw("module")) ? (raw("module") as DocumentModule) : undefined;
  const situation = isSituation(raw("situation")) ? (raw("situation") as DocumentSituation) : undefined;
  const status = module && raw("status") ? (raw("status") ?? undefined) : undefined;
  const companyId = fixed.companyId ?? (raw("company_id") || undefined);
  const fromUrl = raw("competence");
  const competence = fixed.competence !== undefined ? (fixed.competence ?? undefined) : isCompetenceKey(fromUrl) ? fromUrl : undefined;
  const kind = isKind(raw("kind")) ? (raw("kind") as DocumentKind) : undefined;
  const bank = raw("bank") || undefined;
  const offset = Math.max(0, Number(raw("offset") ?? 0) || 0);
  const query: DocumentIndexQuery = { limit: PAGE_SIZE, offset };
  if (module) query.module = module;
  if (situation) query.situation = situation;
  if (status) query.status = status;
  if (companyId) query.company_id = companyId;
  if (competence) query.competence = competence;
  if (kind) query.kind = kind;
  if (bank) query.bank = bank;
  const update = (patch: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        if (!("offset" in patch)) next.delete("offset");
        return next;
      },
      { replace: true },
    );
  return { module, situation, status, companyId, competence, kind, bank, offset, query, update };
}
