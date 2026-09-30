import { CompetenceNav } from "@/components/CompetenceNav";
import { ErrorAlert } from "@/components/ErrorAlert";
import { GeneratePdf } from "@/components/GeneratePdf";
import { PageHeader } from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { formatPercent } from "@/lib/format";
import { useCompetence } from "@/lib/useCompetence";
import { useUrlParams } from "@/lib/useUrlParams";
import { useAttention, useDashboard, useDashboardHistory } from "@/services/dashboard";
import { dashboardReportUrl } from "@/services/reports";
import { AttentionPanel } from "./AttentionPanel";
import { CredentialCards } from "./CredentialCards";
import { DashboardFilters } from "./DashboardFilters";
import { HistoryComparison } from "./HistoryComparison";
import { ModuleCards } from "./ModuleCards";
import { dashboardQuery } from "./query";
import { ResponsibleTable } from "./ResponsibleTable";
import { StatTile } from "./StatTile";

/**
 * Dashboard [PRD §24, UC-01, UC-03]: abre na competência mais recente; KPIs clicáveis que levam
 * à lista filtrada, cards por módulo, vencimentos, responsáveis e comparação histórica.
 */
export function DashboardPage() {
  const { competence, error: competenceError } = useCompetence();
  const { params } = useUrlParams();
  const query = competence ? dashboardQuery(competence, params) : null;
  const dashboard = useDashboard(query);
  const history = useDashboardHistory(query);
  const attention = useAttention(query);
  const d = dashboard.data;
  const k = d?.kpis;

  return (
    <>
      <PageHeader
        title="Dashboard"
        description="Visão da competência: prazos, pendências e fechamento por módulo."
        actions={
          <>
            <CompetenceNav />
            <GeneratePdf href={query ? dashboardReportUrl(query) : null} label="Situação geral da competência" />
          </>
        }
      />
      <div className="grid gap-4 p-4">
        <DashboardFilters />
        <ErrorAlert error={competenceError ?? dashboard.error} fallback="Não foi possível carregar o dashboard." />
        {!k ? (
          <Skeleton className="h-28" />
        ) : (
          <section aria-label="Indicadores principais" className="flex flex-wrap gap-3">
            <StatTile label="Aplicáveis" value={String(k.applicable.value)} metric={k.applicable} />
            <StatTile label="Concluídas" value={String(k.done.value)} metric={k.done} tone="success" />
            <StatTile label="Em andamento" value={String(k.in_progress.value)} metric={k.in_progress} />
            <StatTile label="Não iniciadas" value={String(k.not_started.value)} metric={k.not_started} />
            <StatTile label="Pendentes" value={String(k.pending.value)} metric={k.pending} tone={k.pending.value > 0 ? "warning" : undefined} />
            <StatTile label="Atrasadas" value={String(k.overdue.value)} metric={k.overdue} tone={k.overdue.value > 0 ? "danger" : undefined} />
            <StatTile label="Progresso operacional" value={formatPercent(k.operational_progress)} />
            <StatTile label="Conclusão final" value={formatPercent(k.final_completion)} />
            <StatTile label="Prontas para fechar" value={String(k.ready_to_close.value)} metric={k.ready_to_close} hint="empresas com módulo pronto" />
            <StatTile
              label="Competências anteriores abertas"
              value={String(k.previous_open.value)}
              metric={k.previous_open}
              hint="empresas"
              tone={k.previous_open.value > 0 ? "warning" : undefined}
            />
          </section>
        )}
        {d ? <ModuleCards modules={d.modules} /> : null}
        <div className="grid grid-cols-[3fr_2fr] items-start gap-4">
          <div className="grid gap-4">
            {history.data ? <HistoryComparison history={history.data} /> : <Skeleton className="h-64" />}
            {d ? <CredentialCards cards={d.credentials} /> : <Skeleton className="h-48" />}
          </div>
          {attention.data ? <AttentionPanel attention={attention.data} /> : <Skeleton className="h-96" />}
        </div>
        {d ? <ResponsibleTable rows={d.responsibles} /> : null}
      </div>
    </>
  );
}
