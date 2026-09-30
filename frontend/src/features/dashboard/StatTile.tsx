import type { ReactNode } from "react";
import { Link } from "react-router";
import { cn } from "@/lib/utils";
import { drilldownHref, type Metric } from "@/services/dashboard";

/**
 * Número do dashboard [PRD §24.2, §24.7]: rótulo, valor e, quando há drill-down, o tile inteiro
 * é um link para a lista que explica o número ("Atrasadas: 14" → as 14 obrigações).
 */
export function StatTile({
  label,
  value,
  metric,
  hint,
  tone,
}: {
  label: string;
  value: string;
  metric?: Metric | undefined;
  hint?: ReactNode;
  tone?: "danger" | "warning" | "success" | undefined;
}) {
  const body = (
    <>
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <span
        className={cn(
          "text-2xl font-semibold tabular-nums",
          tone === "danger" && "text-destructive",
          tone === "warning" && "text-amber-700",
          tone === "success" && "text-emerald-700",
        )}
      >
        {value}
      </span>
      {hint ? <span className="text-xs text-muted-foreground">{hint}</span> : null}
    </>
  );
  const className = "flex min-w-36 flex-col gap-0.5 rounded-lg border bg-card px-4 py-3";
  if (metric?.drilldown) {
    return (
      <Link to={drilldownHref(metric.drilldown)} aria-label={`${label}: ${value}`} className={cn(className, "transition-colors hover:border-primary hover:bg-accent/40")}>
        {body}
      </Link>
    );
  }
  return (
    <div className={className} role="group" aria-label={`${label}: ${value}`}>
      {body}
    </div>
  );
}
