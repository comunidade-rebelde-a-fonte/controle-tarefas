import { History, Lock, LockOpen, RotateCcw } from "lucide-react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { Tooltip } from "@/components/ui/tooltip";
import { displayCompetence } from "@/lib/competence";
import { formatDateTime, formatPercent } from "@/lib/format";
import { MODULE_LABELS, TAX_REGIME_LABELS } from "@/lib/labels";
import { type CompanyHistory, useCompanyHistory } from "@/services/companies";

type ModuleEntry = CompanyHistory["competences"][number]["modules"][number];

const MODULE_ORDER = ["FISCAL", "DECLARATIONS", "PAYROLL", "ACCOUNTING"] as const;
const MODULE_PATHS: Record<(typeof MODULE_ORDER)[number], string> = {
  FISCAL: "/fiscal",
  DECLARATIONS: "/declarations",
  PAYROLL: "/payroll",
  ACCOUNTING: "/accounting",
};
const EVENT_LABELS = { OPENED: "Aberto", CLOSED: "Fechado", REOPENED: "Reaberto" } as const;

function EventsTooltip({ entry }: { entry: ModuleEntry }) {
  return (
    <ol className="grid gap-1 text-xs">
      {entry.events.map((e, i) => (
        <li key={i}>
          <span className="font-medium">{EVENT_LABELS[e.event_type]}</span> · {formatDateTime(e.created_at)} · {e.user_name}
          {e.reason ? <span className="block text-muted-foreground">Motivo: {e.reason}</span> : null}
        </li>
      ))}
    </ol>
  );
}

function ModuleCell({ entry, competence, code }: { entry: ModuleEntry | undefined; competence: string; code: string }) {
  if (!entry) return <span className="text-xs text-muted-foreground">—</span>;
  const reopened = entry.events.some((e) => e.event_type === "REOPENED");
  const closed = entry.state === "CLOSED";
  const path = MODULE_PATHS[entry.module];
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-xs">
      <Badge variant={closed ? "muted" : "outline"}>
        {closed ? <Lock aria-hidden className="size-3" /> : <LockOpen aria-hidden className="size-3" />}
        {closed ? "Fechado" : "Aberto"}
      </Badge>
      {reopened ? (
        <Badge variant="warning">
          <RotateCcw aria-hidden className="size-3" /> Reaberto
        </Badge>
      ) : null}
      <span className="tabular-nums" aria-label={`Conclusão: ${formatPercent(entry.progress.final_completion)}`}>
        {formatPercent(entry.progress.final_completion)}
      </span>
      <Tooltip content={<EventsTooltip entry={entry} />}>
        <span role="img" aria-label={`Eventos: ${entry.events.map((e) => EVENT_LABELS[e.event_type]).join(", ")}`} className="text-muted-foreground">
          <History aria-hidden className="size-3.5" />
        </span>
      </Tooltip>
      <Link className="text-primary underline-offset-2 hover:underline" to={`${path}?competence=${competence}&q=${encodeURIComponent(code)}`}>
        Abrir grade
      </Link>
    </div>
  );
}

/** Aba 8 da ficha [PRD §27.3]: competências × módulos com estado, conclusão, eventos e link à grade. */
export function HistoryTab({ companyId, internalCode }: { companyId: string; internalCode: string }) {
  const { data, isPending, error } = useCompanyHistory(companyId);
  if (error) return <ErrorAlert error={error} />;
  if (isPending) return <Skeleton className="h-48" />;
  const present = MODULE_ORDER.filter((m) => data.competences.some((c) => c.modules.some((x) => x.module === m)));
  return (
    <Card>
      <CardHeader title="Histórico de competências" description="Estado de cada módulo por competência, com fechamentos e reaberturas." />
      <CardBody className="p-0">
        {data.competences.length === 0 ? (
          <p className="p-4 text-sm text-muted-foreground">Nenhuma competência gerada para esta empresa.</p>
        ) : (
          <Table aria-label="Histórico de competências">
            <thead>
              <tr>
                <Th>Competência</Th>
                <Th>Regime</Th>
                {present.map((m) => (
                  <Th key={m}>{MODULE_LABELS[m]}</Th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.competences.map((c) => (
                <tr key={c.competence}>
                  <Td className="font-medium tabular-nums">{displayCompetence(c.competence)}</Td>
                  <Td className="text-xs">{TAX_REGIME_LABELS[c.tax_regime]}</Td>
                  {present.map((m) => (
                    <Td key={m}>
                      <ModuleCell entry={c.modules.find((x) => x.module === m)} competence={c.competence} code={internalCode} />
                    </Td>
                  ))}
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </CardBody>
    </Card>
  );
}
