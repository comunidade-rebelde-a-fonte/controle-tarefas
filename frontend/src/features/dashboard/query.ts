import { companyFilterFrom } from "@/lib/companyFilter";
import type { DashboardQuery } from "@/services/dashboard";

/** Filtros do dashboard na URL [PRD §24.4-§24.5] → query das APIs de gestão. */
export function dashboardQuery(competence: string, params: URLSearchParams): DashboardQuery {
  return { competence, ...companyFilterFrom(params) };
}
