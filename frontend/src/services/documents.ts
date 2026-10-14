import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";
import type { paths } from "@/types/api";

export type DocumentIndex = Schemas["DocumentIndex"];
export type DocumentIndexItem = Schemas["DocumentIndexItem"];
export type DocumentModule = Schemas["DocumentModule"];
export type DocumentKind = Schemas["DocumentKind"];
export type DocumentSituation = Schemas["DocumentSituation"];
export type DocumentIndexQuery = NonNullable<paths["/documents"]["get"]["parameters"]["query"]>;

/** GET /documents — a área Documentos consolidada (TASK-026): o índice dos documentos dos módulos. */
export function useDocumentIndex(query: DocumentIndexQuery) {
  return useQuery({
    queryKey: ["documents", "index", query] as const,
    queryFn: ({ signal }) => api.get("/documents", { query, signal }),
    placeholderData: keepPreviousData,
  });
}
