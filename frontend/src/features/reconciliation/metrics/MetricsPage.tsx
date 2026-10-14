import { ChartColumn } from "lucide-react";
import { Link } from "react-router";
import { CompetenceInput } from "@/components/CompetenceInput";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { isCompetenceKey } from "@/lib/competence";
import { formatDecimal } from "@/lib/decimal";
import { type ReconciliationAgentCost, type ReconciliationExtraction, useReconciliationMetrics } from "@/services/reconciliation";
import { AutoRate, CompanySelect, RateBar } from "../common";
import { BUCKETS, methodText, percentText, usdText } from "../labels";
import { useSearchUpdate } from "../model";

/** A ordem dos métodos: os passes do automático e depois as manuais. */
const METHOD_ORDER = ["passe_1", "passe_2", "passe_3", "passe_4", "resolvedor", "manual", "sem_comprovante"];

function byMethodOrder([a]: [string, number], [b]: [string, number]): number {
  const ia = METHOD_ORDER.indexOf(a);
  const ib = METHOD_ORDER.indexOf(b);
  return (ia === -1 ? METHOD_ORDER.length : ia) - (ib === -1 ? METHOD_ORDER.length : ib) || a.localeCompare(b);
}

function ExtractionRow({ label, e }: { label: string; e: ReconciliationExtraction }) {
  return (
    <tr>
      <Th scope="row" className="text-left">
        {label}
      </Th>
      <Td className="text-right tabular-nums">{e.documents}</Td>
      <Td className="text-right tabular-nums">{e.local_reader}</Td>
      <Td className="text-right tabular-nums">{e.agent}</Td>
      <Td className="text-right tabular-nums whitespace-nowrap">
        {e.first_attempt} ({percentText(e.first_attempt_percent)})
      </Td>
      <Td className="text-right tabular-nums">{e.average_attempts ? formatDecimal(e.average_attempts, 2) : "—"}</Td>
      <Td className="text-right tabular-nums">{e.not_approved}</Td>
      <Td className="text-right tabular-nums whitespace-nowrap">{usdText(e.cost_usd)}</Td>
      <Td className="text-right tabular-nums whitespace-nowrap">{usdText(e.cost_per_document_usd, 6)}</Td>
    </tr>
  );
}

function AgentRow({ label, a }: { label: string; a: ReconciliationAgentCost }) {
  return (
    <tr>
      <Th scope="row" className="text-left">
        {label}
      </Th>
      <Td className="text-right tabular-nums">{a.runs}</Td>
      <Td className="text-right tabular-nums whitespace-nowrap">{usdText(a.cost_usd)}</Td>
      <Td className="text-right tabular-nums whitespace-nowrap">{usdText(a.cost_per_run_usd, 6)}</Td>
    </tr>
  );
}

/** "Meta zero": nenhum é o esperado; qualquer um é falha de portão a investigar (PRD §15). */
function ZeroGoal({ n }: { n: number }) {
  return n === 0 ? <Badge variant="success">meta zero</Badge> : <Badge variant="warning">acima da meta zero</Badge>;
}

/**
 * Métricas da Conciliação [11-observability §7]: o /metricas da origem (o percentual automático
 * contra a meta e as conciliações por passe) e os demais números do §7, por empresa e competência.
 */
export function MetricsPage() {
  const [params, update] = useSearchUpdate();
  const companyId = params.get("company_id") ?? "";
  const rawCompetence = params.get("competence");
  const competence = isCompetenceKey(rawCompetence) ? rawCompetence : "";
  const metrics = useReconciliationMetrics({ companyId: companyId || undefined, competence: competence || undefined });
  const m = metrics.data;
  const filter = (extra: Record<string, string>) => {
    const q = new URLSearchParams(extra);
    if (companyId) q.set("company_id", companyId);
    if (competence) q.set("competence", competence);
    return q.toString();
  };
  const read = m ? m.extraction.statements.documents + m.extraction.receipts.documents : 0;
  return (
    <>
      <PageHeader title="Métricas" description="Quanto o código fecha sozinho e quanto sobra para a fila, com a leitura, as correções, o custo e os playbooks." />
      <div className="grid gap-4 p-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Empresa
            <CompanySelect value={companyId} onChange={(id) => update({ company_id: id || null })} allLabel="Todas as empresas" />
          </label>
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Competência
            <CompetenceInput value={competence} onChange={(v) => update({ competence: v || null })} />
          </label>
        </div>
        <ErrorAlert error={metrics.error} fallback="Não foi possível carregar as métricas." />
        {metrics.isPending ? <Skeleton className="h-96" /> : null}
        {m && m.matching.lines === 0 && read === 0 ? (
          <EmptyState icon={ChartColumn} title="Sem dados no filtro" description="Nenhum extrato nem comprovante lido para a empresa e a competência escolhidas." />
        ) : null}
        {m ? (
          <div className="grid gap-4 lg:grid-cols-2">
            <Card aria-label="Conciliação automática">
              <CardHeader title="Conciliação automática" description="Linhas do extrato conciliadas pelo automático, contra a meta." />
              <CardBody className="grid gap-3">
                <AutoRate matching={m.matching} />
                <p className="text-xs text-muted-foreground">
                  {m.matching.reconciled} conciliadas ({m.matching.manual} à mão), {m.matching.free} livres;{" "}
                  {m.matching.undone_auto} {m.matching.undone_auto === 1 ? "automática desfeita" : "automáticas desfeitas"} pelo contador.
                </p>
              </CardBody>
            </Card>
            <Card aria-label="Por método">
              <CardHeader title="Por método" description="Conciliações ativas por passe do automático e pelas ações humanas." />
              <CardBody className="p-0">
                {Object.keys(m.matching.by_method).length === 0 ? (
                  <p className="p-4 text-sm text-muted-foreground">Nenhuma conciliação ainda.</p>
                ) : (
                  <Table aria-label="Conciliações por método">
                    <tbody>
                      {Object.entries(m.matching.by_method)
                        .sort(byMethodOrder)
                        .map(([method, n]) => (
                          <tr key={method}>
                            <Th scope="row" className="text-left font-normal">
                              {methodText(method)}
                            </Th>
                            <Td className="text-right tabular-nums">{n}</Td>
                          </tr>
                        ))}
                    </tbody>
                  </Table>
                )}
              </CardBody>
            </Card>
            <Card aria-label="Itens por balde">
              <CardHeader title="Itens por balde" description="O que sobrou para a fila de revisão." />
              <CardBody className="p-0">
                <Table aria-label="Itens da fila por balde">
                  <tbody>
                    {BUCKETS.map(([bucket, label]) => (
                      <tr key={bucket}>
                        <Th scope="row" className="text-left font-normal">
                          <Link to={`/reconciliation?${filter({ bucket })}`} className="inline-flex min-h-6 items-center hover:underline">
                            {label}
                          </Link>
                        </Th>
                        <Td className="text-right tabular-nums">{m.buckets[bucket] ?? 0}</Td>
                      </tr>
                    ))}
                    <tr className="font-medium">
                      <Th scope="row" className="text-left">
                        Total
                      </Th>
                      <Td className="text-right tabular-nums">{m.buckets.total ?? 0}</Td>
                    </tr>
                  </tbody>
                </Table>
              </CardBody>
            </Card>
            <Card aria-label="Depois do fechamento">
              <CardHeader title="Depois do fechamento" description="Erro achado depois de fechar o Contábil: a meta é zero." />
              <CardBody className="grid gap-3 text-sm">
                <p className="flex flex-wrap items-center gap-2">
                  <span className="text-2xl font-semibold tabular-nums">{m.after_close.reopenings}</span>
                  {m.after_close.reopenings === 1 ? "reabertura do Contábil" : "reaberturas do Contábil"} (sempre com motivo)
                  <ZeroGoal n={m.after_close.reopenings} />
                </p>
                <p className="flex flex-wrap items-center gap-2">
                  <span className="text-2xl font-semibold tabular-nums">{m.after_close.late_documents}</span>
                  <Link
                    to={`/reconciliation/documents?${new URLSearchParams({ status: "pendencia_periodo_fechado", ...(companyId ? { company_id: companyId } : {}) }).toString()}`}
                    className="underline underline-offset-4"
                  >
                    {m.after_close.late_documents === 1 ? "documento chegou" : "documentos chegaram"} com a competência fechada
                  </Link>
                  <ZeroGoal n={m.after_close.late_documents} />
                </p>
              </CardBody>
            </Card>
            <Card aria-label="Leitura dos documentos" className="lg:col-span-2">
              <CardHeader
                title="Leitura dos documentos"
                description="Como o make eval-extrator: as tentativas são as do agente; o leitor determinístico fica à parte. Custo de modelo da triagem e da extração, em dólares, como o LiteLLM informa."
              />
              <CardBody className="overflow-x-auto p-0">
                <Table aria-label="Leitura por tipo de documento">
                  <thead>
                    <tr>
                      <Th scope="col">Tipo</Th>
                      <Th scope="col" className="text-right">
                        Lidos
                      </Th>
                      <Th scope="col" className="text-right">
                        Sem agente
                      </Th>
                      <Th scope="col" className="text-right">
                        Pelo agente
                      </Th>
                      <Th scope="col" className="text-right">
                        Aprovados na 1ª
                      </Th>
                      <Th scope="col" className="text-right">
                        Tentativas médias
                      </Th>
                      <Th scope="col" className="text-right">
                        Não aprovados
                      </Th>
                      <Th scope="col" className="text-right">
                        Custo
                      </Th>
                      <Th scope="col" className="text-right">
                        Por documento
                      </Th>
                    </tr>
                  </thead>
                  <tbody>
                    <ExtractionRow label="Extratos" e={m.extraction.statements} />
                    <ExtractionRow label="Comprovantes" e={m.extraction.receipts} />
                  </tbody>
                </Table>
              </CardBody>
            </Card>
            <Card aria-label="Correções humanas">
              <CardHeader title="Correções humanas" description="Campo do comprovante corrigido e “que documento é esse?”, depois da leitura." />
              <CardBody className="grid gap-1 text-sm">
                <p className="text-2xl font-semibold tabular-nums">{percentText(m.corrections.rate_percent)}</p>
                <p className="text-xs text-muted-foreground">
                  {m.corrections.documents} de {read} documentos lidos corrigidos à mão ({m.corrections.actions}{" "}
                  {m.corrections.actions === 1 ? "correção" : "correções"}).
                </p>
              </CardBody>
            </Card>
            <Card aria-label="Agentes que sugerem">
              <CardHeader title="Agentes que sugerem" description="Pedidos e custo por item do resolvedor e do classificador." />
              <CardBody className="p-0">
                <Table aria-label="Custo dos agentes">
                  <thead>
                    <tr>
                      <Th scope="col">Agente</Th>
                      <Th scope="col" className="text-right">
                        Pedidos
                      </Th>
                      <Th scope="col" className="text-right">
                        Custo
                      </Th>
                      <Th scope="col" className="text-right">
                        Por item
                      </Th>
                    </tr>
                  </thead>
                  <tbody>
                    <AgentRow label="Resolvedor" a={m.agents.resolver} />
                    <AgentRow label="Classificador" a={m.agents.classifier} />
                  </tbody>
                </Table>
              </CardBody>
            </Card>
            <Card aria-label="Playbooks por banco" className="lg:col-span-2">
              <CardHeader
                title="Playbooks por banco"
                description="Sucesso das receitas de extração de cada banco, somando as versões."
                actions={
                  <Link to="/reconciliation/playbooks" className="text-xs underline underline-offset-4">
                    Ver os playbooks
                  </Link>
                }
              />
              <CardBody className="p-0">
                {m.playbooks.length === 0 ? (
                  <p className="p-4 text-sm text-muted-foreground">Nenhum playbook ainda. O primeiro extrato aprovado de cada banco cria um.</p>
                ) : (
                  <Table aria-label="Sucesso dos playbooks por banco">
                    <thead>
                      <tr>
                        <Th scope="col">Banco</Th>
                        <Th scope="col">Versão vigente</Th>
                        <Th scope="col" className="text-right">
                          Sucessos
                        </Th>
                        <Th scope="col" className="text-right">
                          Falhas
                        </Th>
                        <Th scope="col">Taxa</Th>
                      </tr>
                    </thead>
                    <tbody>
                      {m.playbooks.map((p) => (
                        <tr key={p.bank}>
                          <Td>{p.bank}</Td>
                          <Td>{p.valid_version !== null ? `v${String(p.valid_version)}` : "nenhuma (redescobre)"}</Td>
                          <Td className="text-right tabular-nums">{p.successes}</Td>
                          <Td className="text-right tabular-nums">{p.failures}</Td>
                          <Td>
                            <RateBar value={p.success_percent} />
                          </Td>
                        </tr>
                      ))}
                    </tbody>
                  </Table>
                )}
              </CardBody>
            </Card>
          </div>
        ) : null}
      </div>
    </>
  );
}
