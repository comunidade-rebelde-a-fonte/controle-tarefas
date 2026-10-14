import { CircleAlert, Lock, TriangleAlert } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { LightBadge } from "@/features/factor-r/common";
import { KIND_LABELS, statusText } from "@/features/documents/labels";
import { blockingText, COMPLETION_STATES } from "@/features/reconciliation/labels";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate, isPositive } from "@/lib/decimal";
import { formatDate, formatPercent } from "@/lib/format";
import { type CompanyOverview, useCompanyOverview } from "@/services/companies";
import { drilldownHref } from "@/services/dashboard";
import { useDocumentIndex } from "@/services/documents";
import { fichaHref } from "./fichaHref";

const linkClass = "inline-flex min-h-6 items-center text-sm underline underline-offset-4";

const LIGHT_TEXT: Record<string, string> = { vermelho: "vermelho", amarelo: "amarelo", verde: "verde" };

function Bar({ value, label }: { value: number; label: string }) {
  const pct = formatPercent(value);
  return (
    <div className="grid gap-1" role="group" aria-label={`${label}: ${pct}`}>
      <div className="flex justify-between text-xs">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium tabular-nums">{pct}</span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-muted" aria-hidden>
        <div className="h-full rounded-full bg-primary" style={{ width: `${Math.round(value * 100)}%` }} />
      </div>
    </div>
  );
}

function Modules({ modules }: { modules: CompanyOverview["modules"] }) {
  return (
    <Card aria-label="Progresso por módulo">
      <CardHeader title="Progresso por módulo" />
      <CardBody className="grid gap-4">
        {modules.length === 0 ? <p className="text-sm text-muted-foreground">A empresa não tem módulo nesta competência.</p> : null}
        {modules.map((m) => (
          <div key={m.module} className="grid gap-2" role="group" aria-label={`Módulo ${m.label}`}>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="flex items-center gap-2 text-sm font-medium">
                {m.label}
                {m.period_state === "CLOSED" ? (
                  <Badge variant="danger">
                    <Lock aria-hidden /> Fechado
                  </Badge>
                ) : (
                  <Badge variant="muted">Aberto</Badge>
                )}
              </span>
              {m.drilldown ? (
                <Link to={drilldownHref(m.drilldown)} className={linkClass}>
                  Abrir a grade
                </Link>
              ) : null}
            </div>
            <Bar value={m.operational_progress} label="Progresso operacional" />
            <Bar value={m.final_completion} label="Conclusão final" />
            <p className="text-xs text-muted-foreground">
              {m.done} de {m.applicable} concluídas
              {m.pending > 0 ? ` · ${String(m.pending)} pendentes` : ""}
              {m.overdue > 0 ? ` · ${String(m.overdue)} atrasadas` : ""}
            </p>
          </div>
        ))}
      </CardBody>
    </Card>
  );
}

function NextDue({ items }: { items: CompanyOverview["next_due"] }) {
  return (
    <Card aria-label="Próximos vencimentos">
      <CardHeader title="Próximos vencimentos" description="As obrigações em aberto da competência, a mais urgente primeiro." />
      <CardBody>
        {items.length === 0 ? (
          <p className="text-sm text-muted-foreground">Nenhuma obrigação em aberto nesta competência.</p>
        ) : (
          <ol aria-label="Próximos vencimentos da empresa" className="grid gap-2 text-sm">
            {items.map((it) => (
              <li key={it.instance_id} className="flex flex-wrap items-center gap-x-2 gap-y-1">
                {it.drilldown ? (
                  <Link to={drilldownHref(it.drilldown)} className="inline-flex min-h-6 items-center font-medium hover:underline">
                    {it.module_label} — {it.obligation}
                  </Link>
                ) : (
                  <span className="font-medium">
                    {it.module_label} — {it.obligation}
                  </span>
                )}
                <span className="text-muted-foreground">
                  {it.status_label} · vence em {formatDate(it.due_date) || "—"}
                </span>
                {it.overdue ? (
                  <Badge variant="danger">
                    <CircleAlert aria-hidden /> Atrasada
                  </Badge>
                ) : null}
              </li>
            ))}
          </ol>
        )}
      </CardBody>
    </Card>
  );
}

function FactorR({ f, href }: { f: CompanyOverview["factor_r"]; href: string }) {
  return (
    <Card aria-label="Fator R da empresa">
      <CardHeader
        title={f ? `Fator R — PA ${displayCompetence(f.pa)}` : "Fator R"}
        actions={
          <Link to={href} className={linkClass}>
            Abrir o Fator R
          </Link>
        }
      />
      <CardBody className="grid gap-3 text-sm">
        {!f ? (
          <p className="text-muted-foreground">A empresa não está na carteira do Fator R neste PA.</p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <LightBadge light={f.semaforo} />
              {f.fator_r ? <span className="font-semibold tabular-nums">Fator R {formatRate(f.fator_r)}</span> : null}
              {f.anexo ? <span className="text-muted-foreground">Anexo {f.anexo}</span> : null}
            </div>
            <p>
              Ação sugerida: <span className="font-medium">{f.acao_texto}</span>
              {isPositive(f.economia_12m) ? ` · economia de ${formatMoney(f.economia_12m)} em 12 meses` : ""}
            </p>
            {f.trend.length > 1 ? (
              <div className="grid gap-1">
                <h3 className="text-xs font-medium text-muted-foreground">Tendência</h3>
                <ol aria-label="Tendência do Fator R" className="flex flex-wrap gap-2 text-xs">
                  {f.trend.map((p) => (
                    <li key={p.pa} className="rounded border px-2 py-1 tabular-nums">
                      {displayCompetence(p.pa)}: {p.fator_r ? formatRate(p.fator_r) : "sem dados"}
                      {p.semaforo ? ` (${LIGHT_TEXT[p.semaforo] ?? p.semaforo})` : ""}
                    </li>
                  ))}
                </ol>
              </div>
            ) : null}
          </>
        )}
      </CardBody>
    </Card>
  );
}

function Reconciliation({ r, href }: { r: CompanyOverview["reconciliation"]; href: string }) {
  const state = r ? COMPLETION_STATES.find(([s]) => s === r.state) : undefined;
  return (
    <Card aria-label="Conciliação da empresa">
      <CardHeader
        title="Conciliação"
        actions={
          <Link to={href} className={linkClass}>
            Abrir a Conciliação
          </Link>
        }
      />
      <CardBody className="grid gap-2 text-sm">
        {!r ? (
          <p className="text-muted-foreground">A empresa não usa a Conciliação nesta competência.</p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              {state ? <Badge variant={state[2]}>{state[1]}</Badge> : null}
              <span className="text-muted-foreground">
                Contábil {r.accounting_state === "CLOSED" ? "fechado" : r.accounting_state === "OPEN" ? "aberto" : "sem período"}
                {r.obligation ? ` · obrigação Conciliação: ${r.obligation.applicable ? r.obligation.status_label : "não aplicável"}` : ""}
              </span>
            </div>
            {r.state !== "concluida" ? <p>O que falta: {blockingText(r.blocking)}</p> : null}
            {r.blocks_accounting ? (
              <p className="flex items-center gap-1 text-warning">
                <Lock className="size-3.5" aria-hidden /> O Contábil não fecha enquanto a conciliação não estiver concluída.
              </p>
            ) : null}
          </>
        )}
      </CardBody>
    </Card>
  );
}

function RecentDocuments({ companyId, href }: { companyId: string; href: string }) {
  const docs = useDocumentIndex({ company_id: companyId, limit: 5 });
  const items = docs.data?.items ?? [];
  return (
    <Card aria-label="Documentos recentes">
      <CardHeader
        title="Documentos recentes"
        description="Os últimos recebidos em todos os módulos."
        actions={
          <Link to={href} className={linkClass}>
            Ver todos
          </Link>
        }
      />
      <CardBody>
        <ErrorAlert error={docs.error} fallback="Não foi possível carregar os documentos." />
        {docs.data && items.length === 0 ? <p className="text-sm text-muted-foreground">Nenhum documento da empresa.</p> : null}
        {items.length > 0 ? (
          <ol aria-label="Documentos recentes da empresa" className="grid gap-1.5 text-sm">
            {items.map((d) => (
              <li key={`${d.module}:${d.id}`} className="flex flex-wrap items-center gap-x-2">
                <Link to={drilldownHref(d.detail)} className="inline-flex min-h-6 items-center font-medium hover:underline">
                  {d.file_name}
                </Link>
                <span className="text-xs text-muted-foreground">
                  {KIND_LABELS[d.kind]} · {statusText(d.module, d.status)} · {formatDate(d.received_at)}
                </span>
              </li>
            ))}
          </ol>
        ) : null}
      </CardBody>
    </Card>
  );
}

function Attention({ categories }: { categories: CompanyOverview["attention"] }) {
  return (
    <Card aria-label="Pendências da empresa">
      <CardHeader title="Pendências" description="O que exige atenção nesta competência." />
      <CardBody>
        {categories.length === 0 ? (
          <p className="text-sm text-muted-foreground">Nenhuma pendência nesta competência.</p>
        ) : (
          <ul className="grid gap-3">
            {categories.map((c) => (
              <li key={c.key} className="grid gap-1" aria-label={`${c.label}: ${String(c.count)}`}>
                <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
                  <TriangleAlert aria-hidden className="size-4 text-warning" />
                  {c.label}
                  <Badge variant="warning">{c.count}</Badge>
                  {c.drilldown ? (
                    <Link to={drilldownHref(c.drilldown)} className="inline-flex min-h-6 items-center text-xs font-normal underline underline-offset-4">
                      Ver todas
                    </Link>
                  ) : null}
                </div>
                <ul className="grid gap-0.5 pl-6 text-xs text-muted-foreground">
                  {c.sample.map((it, i) => (
                    <li key={`${c.key}:${String(i)}`}>
                      {it.title} · {it.detail}
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
        )}
      </CardBody>
    </Card>
  );
}

/**
 * A Visão geral da ficha (08 §5; TASK-025, D-039): o progresso por módulo, os próximos
 * vencimentos, o Fator R e a tendência, a Conciliação e as pendências da empresa na competência
 * da ficha, todos da mesma empresa, com os atalhos para as abas e as grades.
 */
export function OverviewTab({ companyId, competence }: { companyId: string; competence: string | null }) {
  const [params] = useSearchParams();
  const overview = useCompanyOverview(companyId, competence);
  const o = overview.data;
  if (overview.error) return <ErrorAlert error={overview.error} fallback="Não foi possível carregar a visão geral." />;
  if (!o) return <Skeleton className="h-96" />;
  return (
    <div className="grid gap-4">
      {!o.competence_generated ? (
        <p role="status" className="rounded-md border bg-accent/40 p-3 text-sm">
          A competência {displayCompetence(o.competence)} ainda não foi gerada: as obrigações e as pendências aparecem quando ela for
          gerada.
        </p>
      ) : null}
      <div className="grid items-start gap-4 xl:grid-cols-2">
        <Modules modules={o.modules} />
        <NextDue items={o.next_due} />
        <FactorR f={o.factor_r} href={fichaHref(companyId, params, "factor-r")} />
        <Reconciliation r={o.reconciliation} href={fichaHref(companyId, params, "reconciliation")} />
      </div>
      <div className="grid items-start gap-4 xl:grid-cols-2">
        <Attention categories={o.attention} />
        <RecentDocuments companyId={companyId} href={fichaHref(companyId, params, "documents", { doc_all: "1" })} />
      </div>
    </div>
  );
}
