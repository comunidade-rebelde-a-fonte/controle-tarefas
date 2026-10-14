import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { formatDateTime } from "@/lib/format";
import { useAIRuns } from "@/services/aiRuns";
import { agentLabel, triggerLabel } from "./labels";
import { HumanEvalBadge, RunStatusBadge } from "./RunBadges";

/** Corridas dos agentes ligadas à empresa [08 §6: traces e avaliações relacionados]. */
export function CompanyRunsCard({ companyId }: { companyId: string }) {
  const runs = useAIRuns({ company_id: companyId, limit: 8 });
  const items = runs.data?.items ?? [];
  return (
    <Card>
      <CardHeader
        title="Corridas dos agentes"
        description="Extratos, simulações e respostas do consultor desta empresa."
        actions={
          <Link to={`/factor-r/runs?company=${companyId}`} className="text-xs underline underline-offset-4">
            Ver todas
          </Link>
        }
      />
      <CardBody className="grid gap-2 text-sm">
        <ErrorAlert error={runs.error} fallback="Não foi possível carregar as corridas." />
        {runs.data && items.length === 0 ? <p className="text-muted-foreground">Nenhuma corrida ainda.</p> : null}
        <ul className="grid gap-1.5" aria-label="Corridas da empresa">
          {items.map((r) => (
            <li key={r.trace_id} className="flex flex-wrap items-center gap-2">
              <Link to={`/factor-r/runs/${r.trace_id}`} className="font-medium hover:underline">
                {formatDateTime(r.created_at)}
              </Link>
              <span>
                {agentLabel(r.agent)} <span className="text-muted-foreground">· {triggerLabel(r.trigger)}</span>
              </span>
              <RunStatusBadge status={r.status} />
              <HumanEvalBadge evaluated={r.has_human_eval} />
            </li>
          ))}
        </ul>
      </CardBody>
    </Card>
  );
}
