import { buildUrl } from "@/lib/api";
import type { operations } from "@/types/api";
import type { BacklogQuery, DashboardQuery } from "./dashboard";

/** Os 6 relatórios iniciais [PRD §36.3] (GET /reports/{report}.pdf). */
export type ReportName = "general" | "pending" | "overdue" | "backlog" | "credentials" | "module";

export type ReportQuery = NonNullable<operations["downloadReport"]["parameters"]["query"]>;

/** URL do PDF: o navegador baixa direto (Content-Disposition: attachment). */
export function reportUrl(name: ReportName, query: ReportQuery): string {
  return buildUrl("/reports/{file}", { file: `${name}.pdf` }, { ...query });
}

/** Atalho do dashboard: situação geral com a competência e os filtros atuais. */
export function dashboardReportUrl(q: DashboardQuery): string {
  return reportUrl("general", q);
}

/** Atalho do backlog: o relatório recebe a competência de referência como `competence`. */
export function backlogReportUrl(q: BacklogQuery): string {
  return reportUrl("backlog", {
    competence: q.before ?? q.competence,
    state: q.state,
    module: q.module,
    status: q.status,
    responsible_id: q.responsible_id,
    regime_group: q.regime_group,
    tax_regime: q.tax_regime,
    q: q.q,
    overdue_min: q.overdue_min,
    overdue_max: q.overdue_max,
    sort: q.sort,
  });
}
