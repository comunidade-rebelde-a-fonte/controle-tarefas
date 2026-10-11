import { Lock } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { CompetenceInput } from "@/components/CompetenceInput";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { isCompetenceKey } from "@/lib/competence";
import { formatMoney } from "@/lib/decimal";
import { formatDate } from "@/lib/format";
import type { Company } from "@/services/companies";
import { useLatestCompetence } from "@/services/competencies";
import { useCompanyReconciliationSummary } from "@/services/reconciliation";
import { AutoRate, RunReconciliation } from "../common";
import { BUCKETS } from "../labels";
import { BankAccounts } from "./BankAccounts";

/** A leitura do extrato: o leitor determinístico ou a tentativa em que o agente fechou o saldo. */
function readText(attempts: number): string {
  return attempts === 0 ? "leitor, sem agente" : `agente, ${String(attempts)}ª tentativa`;
}

/**
 * A aba Conciliação da ficha da empresa [REQ-036, AC-034]: a competência com a trava do Contábil,
 * o percentual automático, a fila, a classificação, os extratos e os atalhos para as telas da
 * Conciliação já filtradas pela empresa; e as contas bancárias (CC-11).
 */
export function ReconciliationTab({ company }: { company: Company }) {
  const latest = useLatestCompetence();
  const [chosen, setChosen] = useState("");
  const competence = isCompetenceKey(chosen) ? chosen : (latest.data?.current ?? "");
  const summary = useCompanyReconciliationSummary(company.id, competence || null);
  const s = summary.data;
  const query = (extra: Record<string, string> = {}) => new URLSearchParams({ company_id: company.id, ...extra }).toString();
  const shortcuts: [string, string][] = [
    ["Fila de revisão", `/reconciliation?${query({ competence })}`],
    ["Documentos", `/reconciliation/documents?${query()}`],
    ["Lançamentos", `/reconciliation/entries?${query({ competence })}`],
    ["Razão", `/reconciliation/ledger?${query({ from: competence, to: competence })}`],
    ["Plano de contas", `/reconciliation/chart?${query()}`],
    ["Regras", `/reconciliation/rules?${query()}`],
    ["Métricas", `/reconciliation/metrics?${query({ competence })}`],
  ];
  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-end gap-3">
        <label className="grid gap-1 text-xs font-medium text-muted-foreground">
          Competência
          <CompetenceInput value={competence} onChange={setChosen} />
        </label>
        {s ? (
          s.accounting_closed ? (
            <Badge variant="danger">
              <Lock aria-hidden /> Contábil fechado
            </Badge>
          ) : (
            <Badge variant="success">Contábil aberto</Badge>
          )
        ) : null}
      </div>
      <ErrorAlert error={summary.error ?? latest.error} fallback="Não foi possível carregar a Conciliação da empresa." />
      {summary.isPending && competence ? <Skeleton className="h-64" /> : null}
      {s ? (
        <>
          {s.accounting_closed ? (
            <p role="status" className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
              <Lock className="mt-0.5 size-4 shrink-0" aria-hidden />O Contábil da competência está fechado: a Conciliação fica travada. Para
              mudar algo, reabra o Contábil (com motivo).
            </p>
          ) : (
            <RunReconciliation companyId={company.id} competence={competence} showQueueLink />
          )}
          <div className="grid gap-4 lg:grid-cols-3">
            <Card aria-label="Conciliação da competência">
              <CardHeader title="Conciliação" />
              <CardBody className="grid gap-2">
                <AutoRate matching={s.matching} />
                <p className="text-xs text-muted-foreground">
                  {s.matching.reconciled} conciliadas ({s.matching.manual} à mão) e {s.matching.free} livres.
                </p>
              </CardBody>
            </Card>
            <Card aria-label="Fila de revisão da competência">
              <CardHeader title="Fila de revisão" description={(s.buckets.total ?? 0) === 1 ? "1 item" : `${String(s.buckets.total ?? 0)} itens`} />
              <CardBody className="p-0">
                <ul className="divide-y text-sm">
                  {BUCKETS.map(([bucket, label]) => (
                    <li key={bucket} className="flex items-center gap-2 px-4 py-1.5">
                      <Link to={`/reconciliation?${query({ competence, bucket })}`} className="inline-flex min-h-6 items-center hover:underline">
                        {label}
                      </Link>
                      <span className="ml-auto tabular-nums">{s.buckets[bucket] ?? 0}</span>
                    </li>
                  ))}
                </ul>
              </CardBody>
            </Card>
            <Card aria-label="Classificação e lançamentos">
              <CardHeader title="Classificação e lançamentos" />
              <CardBody className="grid gap-2 text-sm">
                <p>
                  <span className="font-semibold tabular-nums">{s.classification.classified}</span>{" "}
                  {s.classification.classified === 1 ? "linha conciliada classificada" : "linhas conciliadas classificadas"} e{" "}
                  <span className="font-semibold tabular-nums">{s.classification.pending}</span> à espera da conta.
                </p>
                <p>
                  <span className="font-semibold tabular-nums">{s.entries.total}</span>{" "}
                  {s.entries.total === 1 ? "lançamento do cliente" : "lançamentos do cliente"}
                  {s.entries.partial > 0 ? `, ${String(s.entries.partial)} ${s.entries.partial === 1 ? "parcial" : "parciais"} (a revisar)` : ""}.
                </p>
              </CardBody>
            </Card>
          </div>
          <Card aria-label="Extratos da competência">
            <CardHeader title="Extratos da competência" description="Importados com o saldo fechando exatamente." />
            <CardBody className="overflow-x-auto p-0">
              {s.statements.length === 0 ? (
                <p className="p-4 text-sm text-muted-foreground">Nenhum extrato importado cobre a competência.</p>
              ) : (
                <Table aria-label="Extratos importados da competência">
                  <thead>
                    <tr>
                      <Th scope="col">Conta</Th>
                      <Th scope="col">Período</Th>
                      <Th scope="col" className="text-right">
                        Saldo inicial
                      </Th>
                      <Th scope="col" className="text-right">
                        Saldo final
                      </Th>
                      <Th scope="col" className="text-right">
                        Linhas
                      </Th>
                      <Th scope="col">Leitura</Th>
                      <Th scope="col">
                        <span className="sr-only">Documento</span>
                      </Th>
                    </tr>
                  </thead>
                  <tbody>
                    {s.statements.map((st) => (
                      <tr key={st.import_id}>
                        <Td className="font-mono text-xs whitespace-nowrap">{st.bank_account}</Td>
                        <Td className="whitespace-nowrap tabular-nums">
                          {formatDate(st.period_start)} a {formatDate(st.period_end)}
                        </Td>
                        <Td className="text-right whitespace-nowrap tabular-nums">{formatMoney(st.opening_balance)}</Td>
                        <Td className="text-right whitespace-nowrap tabular-nums">{formatMoney(st.closing_balance)}</Td>
                        <Td className="text-right tabular-nums">{st.transactions}</Td>
                        <Td className="text-xs whitespace-nowrap">{readText(st.attempts)}</Td>
                        <Td>
                          <Link
                            to={`/reconciliation/documents/${st.document_id}`}
                            className="inline-flex min-h-6 items-center text-xs underline underline-offset-4"
                            aria-label={`Abrir o extrato ${st.file_name}`}
                          >
                            ver
                          </Link>
                        </Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              )}
            </CardBody>
          </Card>
        </>
      ) : null}
      <nav aria-label="Conciliação da empresa" className="flex flex-wrap gap-2">
        {shortcuts.map(([label, to]) => (
          <Link key={label} to={to} className="inline-flex h-8 items-center rounded-md border bg-card px-3 text-xs font-medium hover:bg-accent">
            {label}
          </Link>
        ))}
      </nav>
      <BankAccounts companyId={company.id} />
    </div>
  );
}
