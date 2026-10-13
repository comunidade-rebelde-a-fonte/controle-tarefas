import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useParams } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate } from "@/lib/decimal";
import { formatDate, formatDateTime } from "@/lib/format";
import { useReconciliationDocument } from "@/services/reconciliation";
import { DocStatusBadge, DocumentJob } from "../common";
import { DOC_KIND, reasonText } from "../labels";
import { OriginalViewer } from "../OriginalViewer";
import { entryHighlights } from "../review/model";
import { DocumentPanel } from "../review/panels";

/** Uma linha de metadado: rótulo e valor. */
function Meta({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/**
 * O documento da Conciliação (AC-014): o original ao lado, o estado e o job, os dados da triagem,
 * a importação do extrato ou os campos do comprovante, as tentativas e as ações da revisão.
 */
export function DocumentPage() {
  const { id = "" } = useParams();
  const doc = useReconciliationDocument(id);
  if (doc.isPending) return <Skeleton className="m-4 h-96" />;
  if (doc.isError) return <ErrorAlert error={doc.error} fallback="Não foi possível abrir o documento." />;
  const d = doc.data;
  const reason = (d.diagnosis as { motivo?: string } | null)?.motivo;
  return (
    <>
      <PageHeader title={d.file_name} description={d.kind ? DOC_KIND[d.kind] : "Tipo ainda não definido pela triagem"} />
      <div className="grid gap-4 p-4">
        <Link to="/reconciliation/documents" className="inline-flex w-fit items-center gap-1 text-sm underline underline-offset-4">
          <ArrowLeft className="size-4" aria-hidden /> Documentos
        </Link>
        <div className="flex flex-wrap items-center gap-2">
          <DocStatusBadge status={d.status} />
          {reason ? <span className="text-sm text-warning">{reasonText(reason)}</span> : null}
        </div>
        <DocumentJob document={d} />
        <div className="grid gap-4 lg:grid-cols-2">
          <OriginalViewer documentId={d.id} highlights={entryHighlights(d.entry)} />
          <div className="grid content-start gap-4">
            <Card>
              <CardHeader title="Dados do documento" />
              <CardBody>
                <dl className="grid grid-cols-2 gap-3">
                  <Meta label="Recebido em">{formatDateTime(d.created_at)}</Meta>
                  <Meta label="Competência do envio">{d.competence ? displayCompetence(d.competence) : "—"}</Meta>
                  <Meta label="Banco">{d.bank ?? "—"}</Meta>
                  <Meta label="Agência e conta">{d.branch || d.account ? `${d.branch ?? "—"} / ${d.account ?? "—"}` : "—"}</Meta>
                  <Meta label="Período">{d.period_start ? `${formatDate(d.period_start)} a ${formatDate(d.period_end)}` : "—"}</Meta>
                  <Meta label="Confiança da triagem">{formatRate(d.triage_confidence, 0)}</Meta>
                  <Meta label="Hash (SHA-256)">
                    <span className="font-mono text-xs break-all">{d.sha256}</span>
                  </Meta>
                </dl>
              </CardBody>
            </Card>
            {d.import ? (
              <Card>
                <CardHeader title="Extrato importado" description="O saldo fechou exatamente (tolerância R$ 0,00)." />
                <CardBody>
                  <dl className="grid grid-cols-2 gap-3">
                    <Meta label="Conta">{`${d.import.bank} ${d.import.branch}/${d.import.account}`}</Meta>
                    <Meta label="Período">{`${formatDate(d.import.period_start)} a ${formatDate(d.import.period_end)}`}</Meta>
                    <Meta label="Saldo inicial">{formatMoney(d.import.opening_balance)}</Meta>
                    <Meta label="Saldo final">{formatMoney(d.import.closing_balance)}</Meta>
                    <Meta label="Transações">{d.import.transactions}</Meta>
                    <Meta label="Tentativas">{d.import.attempts === 0 ? "Leitura determinística" : d.import.attempts}</Meta>
                  </dl>
                </CardBody>
              </Card>
            ) : null}
            <DocumentPanel id={d.id} showHeader={false} />
          </div>
        </div>
        {d.attempts.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Tentativas e intervenções">
              <thead>
                <tr>
                  <Th scope="col">Quando</Th>
                  <Th scope="col">Quem</Th>
                  <Th scope="col">Tentativa</Th>
                  <Th scope="col">Modelo</Th>
                  <Th scope="col">O que mudou</Th>
                </tr>
              </thead>
              <tbody>
                {d.attempts.map((a) => (
                  <tr key={a.id}>
                    <Td className="whitespace-nowrap">{formatDateTime(a.created_at)}</Td>
                    <Td>{a.alias === "humano" ? "Contador" : a.alias === "leitor_local" ? "Leitura determinística" : a.alias}</Td>
                    <Td className="tabular-nums">{a.attempt || "—"}</Td>
                    <Td className="text-xs">{a.model ?? "—"}</Td>
                    <Td className="max-w-96 text-xs break-words text-muted-foreground">{a.change ?? a.error ?? "—"}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        ) : null}
      </div>
    </>
  );
}
