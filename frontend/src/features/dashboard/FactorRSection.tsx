import { Link } from "react-router";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { LightBadge } from "@/features/factor-r/common";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate, isPositive } from "@/lib/decimal";
import { type Dashboard, drilldownHref } from "@/services/dashboard";
import { StatTile } from "./StatTile";

/**
 * A seção Fator R do Dashboard (PRD §8, 08 §3; TASK-024, D-038): a carteira no PA da competência,
 * pelo mesmo motor da tela da carteira, com os filtros do Dashboard (o responsável é o do Fiscal).
 * Cada número abre a carteira já filtrada pelo semáforo; as prioridades abrem a aba Fator R da
 * empresa (REQ-017).
 */
export function FactorRSection({ section: s }: { section: Dashboard["factor_r"] }) {
  const meta = s.meta_operacional ? `, meta operacional de ${formatRate(s.meta_operacional, 0)}` : "";
  return (
    <Card aria-label="Fator R">
      <CardHeader
        title="Fator R"
        description={`Carteira no PA ${displayCompetence(s.pa)}${meta}.`}
        actions={
          s.monitored.drilldown ? (
            <Link to={drilldownHref(s.monitored.drilldown)} className="inline-flex min-h-6 items-center text-sm underline underline-offset-4">
              Abrir a carteira
            </Link>
          ) : null
        }
      />
      <CardBody className="grid gap-4">
        {s.unavailable ? (
          <p role="status" className="text-sm text-muted-foreground">
            {s.unavailable}
          </p>
        ) : (
          <>
            <div className="flex flex-wrap gap-3">
              <StatTile label="Monitoradas" value={String(s.monitored.value)} metric={s.monitored} />
              <StatTile label="No Anexo V (vermelho)" value={String(s.red.value)} metric={s.red} tone={s.red.value > 0 ? "danger" : undefined} />
              <StatTile label="No limite (amarelo)" value={String(s.yellow.value)} metric={s.yellow} tone={s.yellow.value > 0 ? "warning" : undefined} />
              <StatTile label="Seguras (verde)" value={String(s.green.value)} metric={s.green} tone="success" />
              <StatTile label="Dados insuficientes" value={String(s.insufficient.value)} metric={s.insufficient} />
              <StatTile label="Economia em jogo (12 meses)" value={formatMoney(s.savings_at_play.value)} metric={s.savings_at_play} />
              <StatTile
                label="Exigem ação"
                value={String(s.action_required.value)}
                metric={s.action_required}
                tone={s.action_required.value > 0 ? "warning" : undefined}
              />
            </div>
            {s.priorities.length > 0 ? (
              <div className="grid gap-1.5">
                <h3 className="text-sm font-medium">Prioridades</h3>
                <ol aria-label="Prioridades do Fator R" className="grid gap-1 text-sm">
                  {s.priorities.map((p) => (
                    <li key={p.company.id} className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      <Link to={drilldownHref(p.drilldown)} className="inline-flex min-h-6 items-center font-medium hover:underline">
                        {p.company.legal_name}
                      </Link>
                      <LightBadge light={p.semaforo} />
                      <span className="text-muted-foreground">
                        {p.acao_texto}
                        {p.fator_r ? ` · Fator R ${formatRate(p.fator_r)}` : ""}
                        {isPositive(p.economia_12m) ? ` · economia de ${formatMoney(p.economia_12m)} em 12 meses` : ""}
                      </span>
                    </li>
                  ))}
                </ol>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">Nenhuma empresa da carteira exige ação.</p>
            )}
          </>
        )}
      </CardBody>
    </Card>
  );
}
