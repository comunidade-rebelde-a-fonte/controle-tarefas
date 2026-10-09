import { Activity, ChevronLeft, ChevronRight, X } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/ui/field";
import { NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { displayCompetence } from "@/lib/competence";
import { formatRate } from "@/lib/decimal";
import { formatDateTime } from "@/lib/format";
import { type AIRunStatus, type AISummary, useAIRuns, useAISummary } from "@/services/aiRuns";
import { useCompanies } from "@/services/companies";
import { FactorRDisclaimer, FilterToggle } from "../common";
import { agentLabel, formatUsd, RUN_STATUS_LABELS, triggerLabel } from "./labels";
import { HumanEvalBadge, RunStatusBadge, SyncBadge } from "./RunBadges";

const PAGE = 50;
const STATUSES = Object.keys(RUN_STATUS_LABELS) as AIRunStatus[];
/** Agentes do Fator R, listados no filtro mesmo antes da primeira corrida. */
const KNOWN_AGENTS = ["parser_pgdas", "consultor", "priorizador"];

function Kpi({ label, value, testId }: { label: string; value: ReactNode; testId: string }) {
  return (
    <div className="rounded-lg border bg-card px-4 py-3">
      <p className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{label}</p>
      <p className="mt-1 text-xl font-semibold tabular-nums" data-testid={testId}>
        {value}
      </p>
    </div>
  );
}

const ms = (v: number | null | undefined) => (v === null || v === undefined ? "—" : `${v.toLocaleString("pt-BR")} ms`);

/** Indicadores do painel [FR-18]: corridas, pendências de nota, acerto ouro e humano, custo. */
function Summary({ s }: { s: AISummary }) {
  return (
    <>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-7">
        <Kpi label="Corridas 24 h" value={s.runs_24h} testId="obs-24h" />
        <Kpi label="Corridas (total)" value={s.runs_total} testId="obs-total" />
        <Kpi label="Sem nota humana" value={s.pending_human_eval} testId="obs-pendencias" />
        <Kpi label="Acerto ouro" value={formatRate(s.gold_accuracy)} testId="obs-acerto-ouro" />
        <Kpi label="Acerto humano" value={formatRate(s.human_accuracy)} testId="obs-acerto-humano" />
        <Kpi label="Custo do LLM (30 dias)" value={formatUsd(s.llm_cost_30d)} testId="obs-custo" />
        <Kpi label="Tokens do LLM (30 dias)" value={s.llm_tokens_30d === null ? "—" : s.llm_tokens_30d.toLocaleString("pt-BR")} testId="obs-tokens" />
      </div>
      {s.by_agent.length > 0 ? (
        <div className="overflow-x-auto rounded-lg border bg-card">
          <Table aria-label="Corridas por agente">
            <thead>
              <tr>
                <Th scope="col">Agente</Th>
                <Th scope="col" className="text-right">
                  Corridas
                </Th>
                <Th scope="col" className="text-right">
                  Latência média
                </Th>
                <Th scope="col" className="text-right">
                  Confiança média
                </Th>
                <Th scope="col" className="text-right">
                  Erros
                </Th>
                <Th scope="col" className="text-right">
                  Em revisão
                </Th>
              </tr>
            </thead>
            <tbody>
              {s.by_agent.map((a) => (
                <tr key={a.agent}>
                  <Th scope="row" className="static bg-card font-medium text-foreground">
                    {agentLabel(a.agent)}
                  </Th>
                  <Td className="text-right tabular-nums">{a.runs}</Td>
                  <Td className="text-right tabular-nums">{ms(a.avg_latency_ms)}</Td>
                  <Td className="text-right tabular-nums">{formatRate(a.avg_confidence)}</Td>
                  <Td className="text-right tabular-nums">{a.errors}</Td>
                  <Td className="text-right tabular-nums">{a.reviews}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </div>
      ) : null}
    </>
  );
}

/**
 * Observabilidade dos agentes [FR-18, AC-029, AC-031]: painel da trilha local e as corridas,
 * filtráveis, com o caminho para o detalhe (spans, decisão, notas) e a nota humana.
 */
export function ObservabilityPage() {
  const [params, setParams] = useSearchParams();
  const agent = params.get("agent") ?? "";
  const statusParam = params.get("status");
  const status = STATUSES.find((s) => s === statusParam);
  const withoutEval = params.get("sem_nota") === "1";
  const companyId = params.get("company") ?? "";
  const page = Math.max(0, Number.parseInt(params.get("page") ?? "0", 10) || 0);

  const summary = useAISummary();
  const runs = useAIRuns({
    agent: agent || undefined,
    status,
    company_id: companyId || undefined,
    without_human_eval: withoutEval || undefined,
    limit: PAGE,
    offset: page * PAGE,
  });
  const companies = useCompanies({});
  const names = new Map((companies.data?.items ?? []).map((c) => [c.id, c.legal_name]));
  const agents = Array.from(new Set([...KNOWN_AGENTS, ...(summary.data?.by_agent.map((a) => a.agent) ?? [])]));

  const set = (key: string, value: string | null) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(key, value);
        else next.delete(key);
        if (key !== "page") next.delete("page");
        return next;
      },
      { replace: true },
    );

  const data = runs.data;
  const from = page * PAGE + 1;
  const to = page * PAGE + (data?.items.length ?? 0);

  return (
    <>
      <PageHeader
        title="Observabilidade dos agentes"
        description="Corridas dos agentes na trilha local: decisão, passos, notas ouro e humanas. O Langfuse complementa, sem ser necessário."
      />
      <div className="grid grid-cols-1 gap-4 p-4">
        <ErrorAlert error={summary.error} fallback="Não foi possível carregar o painel." />
        {summary.isPending ? <Skeleton className="h-24" /> : null}
        {summary.data ? <Summary s={summary.data} /> : null}

        <section aria-label="Corridas" className="grid grid-cols-1 gap-3">
          <h2 className="text-sm font-semibold">Corridas</h2>
          <div className="flex flex-wrap items-end gap-3">
            <Field label="Agente" className="w-48">
              <NativeSelect value={agent} onChange={(e) => set("agent", e.target.value)}>
                <option value="">Todos</option>
                {agents.map((a) => (
                  <option key={a} value={a}>
                    {agentLabel(a)}
                  </option>
                ))}
              </NativeSelect>
            </Field>
            <Field label="Resultado" className="w-40">
              <NativeSelect value={status ?? ""} onChange={(e) => set("status", e.target.value)}>
                <option value="">Todos</option>
                {STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {RUN_STATUS_LABELS[s]}
                  </option>
                ))}
              </NativeSelect>
            </Field>
            <div className="flex h-9 items-center">
              <FilterToggle active={withoutEval} onClick={() => set("sem_nota", withoutEval ? null : "1")}>
                Só sem nota humana
              </FilterToggle>
            </div>
            {companyId ? (
              <Button variant="outline" size="sm" onClick={() => set("company", null)} aria-label="Remover o filtro de empresa">
                Empresa: {names.get(companyId) ?? "selecionada"} <X aria-hidden />
              </Button>
            ) : null}
          </div>
          <ErrorAlert error={runs.error} fallback="Não foi possível carregar as corridas." />
          {runs.isPending ? <Skeleton className="h-64" /> : null}
          {data && data.items.length === 0 ? (
            <EmptyState icon={Activity} title="Nenhuma corrida com esses filtros" description="As corridas aparecem quando um agente lê um extrato, simula ou responde no chat." />
          ) : null}
          {data && data.items.length > 0 ? (
            <div className="overflow-x-auto rounded-lg border bg-card">
              <Table aria-label="Corridas dos agentes">
                <thead>
                  <tr>
                    <Th scope="col">Quando</Th>
                    <Th scope="col">Agente</Th>
                    <Th scope="col">Gatilho</Th>
                    <Th scope="col">Resultado</Th>
                    <Th scope="col">Empresa</Th>
                    <Th scope="col">PA</Th>
                    <Th scope="col" className="text-right">
                      Confiança
                    </Th>
                    <Th scope="col" className="text-right">
                      Latência
                    </Th>
                    <Th scope="col">Nota humana</Th>
                    <Th scope="col">Langfuse</Th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((r) => (
                    <tr key={r.trace_id} data-trace={r.trace_id}>
                      <Td className="whitespace-nowrap">
                        <Link to={`/factor-r/runs/${r.trace_id}`} className="inline-flex min-h-6 items-center font-medium hover:underline">
                          {formatDateTime(r.created_at)}
                        </Link>
                      </Td>
                      <Td>{agentLabel(r.agent)}</Td>
                      <Td className="text-muted-foreground">{triggerLabel(r.trigger)}</Td>
                      <Td>
                        <RunStatusBadge status={r.status} />
                      </Td>
                      <Td className="max-w-56 truncate">{r.company_id ? (names.get(r.company_id) ?? "—") : "—"}</Td>
                      <Td className="tabular-nums">{r.competence ? displayCompetence(r.competence) : "—"}</Td>
                      <Td className="text-right tabular-nums">{formatRate(r.confidence)}</Td>
                      <Td className="text-right whitespace-nowrap tabular-nums">{ms(r.latency_ms)}</Td>
                      <Td>
                        <HumanEvalBadge evaluated={r.has_human_eval} />
                      </Td>
                      <Td>
                        <SyncBadge status={r.langfuse_sync_status} />
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </Table>
              <nav aria-label="Páginas de corridas" className="flex items-center justify-between gap-2 border-t px-3 py-2 text-xs text-muted-foreground">
                <span>
                  {from}–{to} de {data.total}
                </span>
                <span className="flex gap-2">
                  <Button variant="outline" size="sm" disabled={page === 0} onClick={() => set("page", page > 1 ? String(page - 1) : null)}>
                    <ChevronLeft aria-hidden /> Anteriores
                  </Button>
                  <Button variant="outline" size="sm" disabled={to >= data.total} onClick={() => set("page", String(page + 1))}>
                    Seguintes <ChevronRight aria-hidden />
                  </Button>
                </span>
              </nav>
            </div>
          ) : null}
        </section>
        <FactorRDisclaimer />
      </div>
    </>
  );
}
