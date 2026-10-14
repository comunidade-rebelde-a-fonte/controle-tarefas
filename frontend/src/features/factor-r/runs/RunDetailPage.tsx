import { ArrowLeft, ExternalLink } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useParams } from "react-router";
import { CopyButton } from "@/components/CopyButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button-variants";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { displayCompetence } from "@/lib/competence";
import { formatRate } from "@/lib/decimal";
import { formatDateTime } from "@/lib/format";
import { type AIRunDetail, type AIRunSpan, useAIRun } from "@/services/aiRuns";
import { useCompany } from "@/services/companies";
import { FactorRDisclaimer } from "../common";
import { HumanEvalForm } from "./HumanEvalForm";
import { agentLabel, formatUsd, SPAN_TYPE_LABELS, SYNC_LABELS, triggerLabel } from "./labels";
import { RatingBadge, RunStatusBadge } from "./RunBadges";

/** JSON da corrida. O bloco rola e por isso recebe foco: dá para lê-lo só pelo teclado (WCAG 2.1.1). */
function Json({ value }: { value: unknown }) {
  return (
    <pre tabIndex={0} className="max-h-96 overflow-auto rounded-md bg-muted/60 p-2 text-xs leading-relaxed focus-visible:outline-2 focus-visible:outline-ring">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

function Meta({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{label}</dt>
      <dd className="mt-0.5 text-sm">{children}</dd>
    </div>
  );
}

function CompanyName({ id, pa }: { id: string; pa: string | null }) {
  const company = useCompany(id);
  return (
    <Link to={`/companies/${id}?tab=factor-r${pa ? `&competence=${pa}` : ""}`} className="underline underline-offset-4">
      {company.data?.legal_name ?? "empresa"}
    </Link>
  );
}

/** Passos da corrida, com início relativo ao primeiro passo e o detalhe de cada um. */
function Spans({ spans }: { spans: AIRunSpan[] }) {
  if (spans.length === 0) return <p className="text-sm text-muted-foreground">Corrida sem passos registrados.</p>;
  const origin = Math.min(...spans.map((s) => new Date(s.start).getTime()));
  return (
    <div className="overflow-x-auto">
      <Table aria-label="Passos da corrida" data-testid="timeline-spans">
        <thead>
          <tr>
            <Th scope="col">Passo</Th>
            <Th scope="col">Tipo</Th>
            <Th scope="col" className="text-right">
              Início (+ms)
            </Th>
            <Th scope="col" className="text-right">
              Duração
            </Th>
            <Th scope="col">Nível</Th>
            <Th scope="col">Modelo</Th>
            <Th scope="col">Detalhe</Th>
          </tr>
        </thead>
        <tbody>
          {spans.map((s) => {
            const start = new Date(s.start).getTime();
            const duration = new Date(s.end).getTime() - start;
            const tokens = s.tokens_in !== undefined || s.tokens_out !== undefined ? `${String(s.tokens_in ?? 0)} → ${String(s.tokens_out ?? 0)} tokens` : null;
            return (
              <tr key={s.id} data-span={s.name}>
                <Td className={s.parent_id ? "pl-7" : "font-medium"}>{s.name}</Td>
                <Td>{SPAN_TYPE_LABELS[s.type] ?? s.type}</Td>
                <Td className="text-right tabular-nums">{start - origin}</Td>
                <Td className="text-right whitespace-nowrap tabular-nums">{duration} ms</Td>
                <Td>{s.level ? <Badge variant={s.level === "ERROR" ? "danger" : "warning"}>{s.level === "ERROR" ? "erro" : "aviso"}</Badge> : "—"}</Td>
                <Td className="text-xs">
                  {s.model ?? "—"}
                  {tokens ? <span className="block text-muted-foreground">{tokens}</span> : null}
                  {s.cost_usd ? <span className="block text-muted-foreground">{formatUsd(s.cost_usd, 6)}</span> : null}
                </Td>
                <Td className="min-w-48">
                  <details>
                    <summary className="cursor-pointer text-xs text-primary">ver</summary>
                    <div className="mt-1 grid gap-1">
                      {s.status_message ? <p className="text-xs">Mensagem: {s.status_message}</p> : null}
                      {s.metadata && Object.keys(s.metadata).length > 0 ? <Json value={s.metadata} /> : null}
                      {s.input !== undefined ? (
                        <>
                          <p className="text-xs font-medium">Entrada</p>
                          <Json value={s.input} />
                        </>
                      ) : null}
                      {s.output !== undefined ? (
                        <>
                          <p className="text-xs font-medium">Saída</p>
                          <Json value={s.output} />
                        </>
                      ) : null}
                    </div>
                  </details>
                </Td>
              </tr>
            );
          })}
        </tbody>
      </Table>
    </div>
  );
}

function GoldEvals({ run }: { run: AIRunDetail }) {
  if (run.gold_evals.length === 0) return <p className="text-sm text-muted-foreground">Sem avaliação ouro para esta corrida.</p>;
  return (
    <div className="overflow-x-auto">
      <Table aria-label="Nota ouro" data-testid="evals-ouro">
        <thead>
          <tr>
            <Th scope="col">Campo</Th>
            <Th scope="col">Esperado</Th>
            <Th scope="col">Obtido</Th>
            <Th scope="col">Resultado</Th>
          </tr>
        </thead>
        <tbody>
          {run.gold_evals.map((g) => (
            <tr key={g.field}>
              <Td className="font-medium">{g.field}</Td>
              <Td className="tabular-nums">{g.expected ?? "—"}</Td>
              <Td className="tabular-nums">{g.obtained ?? "—"}</Td>
              <Td>
                <Badge variant={g.status === "ok" ? "success" : g.status === "erro" ? "danger" : "muted"}>
                  {g.status === "ok" ? "ok" : g.status === "erro" ? "erro" : "ouro indisponível"}
                </Badge>
              </Td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}

function Detail({ run }: { run: AIRunDetail }) {
  return (
    <>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <RunStatusBadge status={run.status} />
        <span className="font-mono text-xs" data-testid="trace-id">
          {run.trace_id}
        </span>
        <CopyButton value={run.trace_id} label="Copiar o trace_id" />
      </div>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-3 rounded-lg border bg-card p-4 md:grid-cols-4">
        <Meta label="Gatilho">{triggerLabel(run.trigger)}</Meta>
        <Meta label="Quando">{formatDateTime(run.created_at)}</Meta>
        <Meta label="Empresa">{run.company_id ? <CompanyName id={run.company_id} pa={run.competence} /> : "—"}</Meta>
        <Meta label="PA / competência">{run.competence ? displayCompetence(run.competence) : "—"}</Meta>
        <Meta label="Confiança">{formatRate(run.confidence)}</Meta>
        <Meta label="Latência">{run.latency_ms === null ? "—" : `${run.latency_ms.toLocaleString("pt-BR")} ms`}</Meta>
        <Meta label="Modelo">
          {run.model ? (
            <>
              {run.provider ? `${run.provider}/${run.model}` : run.model}
              <span className="block text-xs text-muted-foreground">
                alias {run.alias ?? "—"}
                {run.model_config_version !== null ? ` · configuração v${String(run.model_config_version)}` : ""}
              </span>
            </>
          ) : (
            "sem LLM"
          )}
        </Meta>
        <Meta label="Tokens e custo">
          {run.tokens_in === null && run.tokens_out === null
            ? "—"
            : `${String(run.tokens_in ?? 0)} → ${String(run.tokens_out ?? 0)} tokens · ${formatUsd(run.cost_usd, 6)}`}
        </Meta>
        <Meta label="Prompts">
          {run.prompt_version
            ? run.prompt_version.split(";").map((ref) => (
                <span key={ref} className="block font-mono text-xs">
                  {ref}
                </span>
              ))
            : "—"}
        </Meta>
        <Meta label="Código do erro">{run.error_code ?? "—"}</Meta>
        <Meta label="Langfuse">
          {SYNC_LABELS[run.langfuse_sync_status]}
          {run.langfuse_attempts > 0 ? ` · ${String(run.langfuse_attempts)} envio(s)` : ""}
          {run.langfuse_last_error ? <span className="block text-xs text-muted-foreground">{run.langfuse_last_error}</span> : null}
        </Meta>
      </dl>

      <Card>
        <CardHeader title="Passos" description="plan → parse/tool → decide → render, e as chamadas ao LLM. Vêm da trilha local, mesmo com o Langfuse fora." />
        <Spans spans={run.spans} />
      </Card>

      <div className="grid grid-cols-1 items-start gap-4 xl:grid-cols-3">
        <Card>
          <CardHeader title="Entrada" description="Sanitizada: sem o texto do documento." />
          <CardBody>
            <Json value={run.input} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Saída" />
          <CardBody>
            <Json value={run.output} />
          </CardBody>
        </Card>
        <Card>
          <CardHeader title="Decisão" description={run.decisions.map((d) => d.kind).join(", ") || undefined} />
          <CardBody>
            <Json value={run.decision} />
          </CardBody>
        </Card>
      </div>

      <Card>
        <CardHeader title="Nota ouro" description="Comparação automática do que o agente leu com a série lançada (tolerância da política)." />
        <CardBody>
          <GoldEvals run={run} />
        </CardBody>
      </Card>

      <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-2">
        <HumanEvalForm traceId={run.trace_id} />
        <Card data-testid="evals-humanas">
          <CardHeader title="Notas registradas" />
          <CardBody className="grid gap-2">
            {run.human_evals.length === 0 ? <p className="text-sm text-muted-foreground">Nenhuma nota ainda.</p> : null}
            {run.human_evals.map((h) => (
              <div key={h.id} className="rounded-md border p-2 text-sm">
                <div className="flex items-center gap-2">
                  <RatingBadge rating={h.rating} />
                  <span className="text-xs text-muted-foreground">{formatDateTime(h.created_at)}</span>
                </div>
                {h.comment ? <p className="mt-1 whitespace-pre-line">{h.comment}</p> : null}
              </div>
            ))}
          </CardBody>
        </Card>
      </div>
    </>
  );
}

/**
 * Detalhe da corrida [FR-16 a FR-18, AC-029, AC-031]: metadados, passos, entrada, saída,
 * decisão, nota ouro e nota humana. Tudo da trilha local; o Langfuse é só um link.
 */
export function RunDetailPage() {
  const { traceId = "" } = useParams();
  const run = useAIRun(traceId);
  const r = run.data;
  return (
    <>
      <PageHeader
        title={r ? `Trace do ${agentLabel(r.agent)}` : "Trace"}
        description="Auditoria local da corrida do agente."
        actions={
          r?.langfuse_url ? (
            <a href={r.langfuse_url} target="_blank" rel="noreferrer" className={buttonVariants({ variant: "outline", size: "sm" })} data-testid="abrir-langfuse">
              <ExternalLink aria-hidden /> Abrir no Langfuse
            </a>
          ) : null
        }
      />
      <div className="grid grid-cols-1 gap-4 p-4">
        <Link to="/factor-r/runs" className="inline-flex w-fit items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-3.5" aria-hidden /> Observabilidade
        </Link>
        <ErrorAlert error={run.error} fallback="Corrida não encontrada." />
        {run.isPending ? <Skeleton className="h-96" /> : null}
        {r ? <Detail run={r} /> : null}
        <FactorRDisclaimer />
      </div>
    </>
  );
}
