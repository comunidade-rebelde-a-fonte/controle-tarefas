import { Link } from "react-router";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { formatPercent } from "@/lib/format";
import { type Dashboard, drilldownHref, type Metric } from "@/services/dashboard";

function Bar({ value, label }: { value: number; label: string }) {
  const pct = formatPercent(value);
  return (
    <div className="grid gap-1" aria-label={`${label}: ${pct}`} role="group">
      <div className="flex justify-between text-xs">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium tabular-nums">{pct}</span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-muted" aria-hidden>
        <div className="h-full rounded-full bg-[#2a78d6]" style={{ width: `${Math.round(value * 100)}%` }} />
      </div>
    </div>
  );
}

function CountLink({ label, metric }: { label: string; metric: Metric }) {
  const text = (
    <>
      <span className="text-muted-foreground">{label}</span> <span className="font-semibold tabular-nums">{metric.value}</span>
    </>
  );
  return metric.drilldown ? (
    <Link to={drilldownHref(metric.drilldown)} aria-label={`${label}: ${metric.value}`} className="rounded px-1 hover:bg-accent">
      {text}
    </Link>
  ) : (
    <span>{text}</span>
  );
}

/** Cards por módulo [PRD §24.3]: progresso, conclusão e números clicáveis. */
export function ModuleCards({ modules }: { modules: Dashboard["modules"] }) {
  return (
    <div className="grid grid-cols-4 gap-3">
      {modules.map((m) => (
        <Card key={m.module} aria-label={`Módulo ${m.label}`}>
          <CardHeader title={m.label} description={`${m.companies} ${m.companies === 1 ? "empresa" : "empresas"}`} />
          <CardBody className="grid gap-3">
            <Bar value={m.operational_progress} label="Progresso operacional" />
            <Bar value={m.final_completion} label="Conclusão final" />
            <div className="flex flex-wrap gap-x-3 gap-y-1 text-sm">
              <CountLink label="Atrasadas" metric={m.overdue} />
              <CountLink label="Pendentes" metric={m.pending} />
              <CountLink label="Prontas para fechar" metric={m.ready_to_close} />
              <CountLink label="Fechadas" metric={m.closed} />
            </div>
          </CardBody>
        </Card>
      ))}
    </div>
  );
}
