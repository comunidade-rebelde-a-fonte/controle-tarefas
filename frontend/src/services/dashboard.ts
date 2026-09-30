import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";
import type { paths } from "@/types/api";

export type Dashboard = Schemas["Dashboard"];
export type Attention = Schemas["Attention"];
export type DashboardHistory = Schemas["DashboardHistory"];
export type Backlog = Schemas["Backlog"];
export type BacklogItem = Schemas["BacklogItem"];
export type Drilldown = Schemas["Drilldown"];
export type Metric = Schemas["Metric"];

export type DashboardQuery = NonNullable<paths["/dashboard"]["get"]["parameters"]["query"]>;
export type BacklogQuery = NonNullable<paths["/backlog"]["get"]["parameters"]["query"]>;

/** Todas as consultas de gestão ficam sob `obligations` para acompanhar as alterações de status. */
export const dashboardKeys = {
  dashboard: (q: DashboardQuery) => ["obligations", "dashboard", q] as const,
  attention: (q: DashboardQuery) => ["obligations", "attention", q] as const,
  history: (q: DashboardQuery) => ["obligations", "history-comparison", q] as const,
  backlog: (q: BacklogQuery) => ["obligations", "backlog", q] as const,
};

/** GET /dashboard [PRD §24]. `query` null enquanto a competência carrega. */
export function useDashboard(query: DashboardQuery | null) {
  return useQuery({
    queryKey: dashboardKeys.dashboard(query ?? { competence: "" }),
    queryFn: ({ signal }) => api.get("/dashboard", { query: query ?? { competence: "" }, signal }),
    enabled: query !== null,
    placeholderData: keepPreviousData,
  });
}

/** GET /dashboard/attention [PRD §24.6]. */
export function useAttention(query: DashboardQuery | null) {
  return useQuery({
    queryKey: dashboardKeys.attention(query ?? { competence: "" }),
    queryFn: ({ signal }) => api.get("/dashboard/attention", { query: query ?? { competence: "" }, signal }),
    enabled: query !== null,
    placeholderData: keepPreviousData,
  });
}

/** GET /dashboard/history [PRD §24.8]. */
export function useDashboardHistory(query: DashboardQuery | null) {
  return useQuery({
    queryKey: dashboardKeys.history(query ?? { competence: "" }),
    queryFn: ({ signal }) => api.get("/dashboard/history", { query: query ?? { competence: "" }, signal }),
    enabled: query !== null,
    placeholderData: keepPreviousData,
  });
}

/** GET /backlog: backlog (before) ou lista filtrada da competência (competence + state). */
export function useBacklog(query: BacklogQuery | null) {
  return useQuery({
    queryKey: dashboardKeys.backlog(query ?? {}),
    queryFn: ({ signal }) => api.get("/backlog", { query: query ?? {}, signal }),
    enabled: query !== null,
    placeholderData: keepPreviousData,
  });
}

/** URL da interface de um drill-down (rota + filtros na query). */
export function drilldownHref(d: Drilldown): string {
  const qs = new URLSearchParams(d.query).toString();
  return qs ? `${d.path}?${qs}` : d.path;
}
