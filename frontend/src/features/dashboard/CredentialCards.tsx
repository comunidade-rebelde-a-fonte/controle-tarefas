import { CircleAlert, Clock, ShieldCheck } from "lucide-react";
import { Link } from "react-router";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { type Dashboard, drilldownHref, type Metric } from "@/services/dashboard";

function Row({ icon, label, metric, tone }: { icon: React.ReactNode; label: string; metric: Metric; tone?: string }) {
  const content = (
    <span className={`flex items-center justify-between gap-2 rounded px-1 py-0.5 text-sm ${tone ?? ""}`}>
      <span className="flex items-center gap-1.5">
        {icon}
        {label}
      </span>
      <span className="font-semibold tabular-nums">{metric.value}</span>
    </span>
  );
  return metric.drilldown ? (
    <Link to={drilldownHref(metric.drilldown)} aria-label={`${label}: ${metric.value}`} className="hover:bg-accent">
      {content}
    </Link>
  ) : (
    content
  );
}

/** Card de vencimentos de procurações e certificados [PRD §24.3, §19.3]. */
export function CredentialCards({ cards }: { cards: Dashboard["credentials"] }) {
  return (
    <Card aria-label="Vencimentos de procurações e certificados">
      <CardHeader title="Procurações e certificados" description="Situação da validade pelas faixas configuradas." />
      <CardBody className="grid grid-cols-2 gap-4">
        {cards.map((c) => (
          <div key={c.type} className="grid gap-1">
            <span className="text-sm font-medium">{c.label}</span>
            <Row icon={<CircleAlert aria-hidden className="size-3.5" />} label="Vencidos" metric={c.expired} tone="text-destructive" />
            <Row icon={<Clock aria-hidden className="size-3.5" />} label={`Vencem em até ${c.due_soon_days} dias`} metric={c.due_soon} tone="text-warning" />
            <Row icon={<ShieldCheck aria-hidden className="size-3.5" />} label="Vigentes" metric={c.current} />
            <Row icon={<span aria-hidden className="inline-block size-3.5" />} label="Sem data" metric={c.no_date} tone="text-muted-foreground" />
          </div>
        ))}
      </CardBody>
    </Card>
  );
}
