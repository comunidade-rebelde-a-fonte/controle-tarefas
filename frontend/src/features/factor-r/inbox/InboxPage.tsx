import { Inbox } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { formatCnpj } from "@/lib/cnpj";
import { displayCompetence } from "@/lib/competence";
import { formatRate } from "@/lib/decimal";
import { formatDateTime } from "@/lib/format";
import { type PgdasStatus, usePgdasDocuments } from "@/services/factorRInbox";
import { FactorRDisclaimer, FilterToggle } from "../common";
import { DocumentStatusBadge } from "./DocumentStatusBadge";
import { reasonText, STATUS_FILTERS } from "./labels";
import { UploadPanel } from "./UploadPanel";

const STATUS_PARAM = "status";

function isStatus(v: string | null): v is PgdasStatus {
  return STATUS_FILTERS.some(([s]) => s === v);
}

/**
 * Inbox PGDAS-D [REQ-012, FR-08 a FR-11]: envio dos extratos, documentos por status e o caminho
 * para a revisão. A leitura é do parser determinístico no worker (ADR-006).
 */
export function InboxPage() {
  const [params, setParams] = useSearchParams();
  const raw = params.get(STATUS_PARAM);
  const status = isStatus(raw) ? raw : undefined;
  const docs = usePgdasDocuments({ status });
  const data = docs.data;
  const counts = data?.contagem_por_status ?? {};
  const all = Object.values(counts).reduce((a, b) => a + b, 0);

  const setStatus = (s: PgdasStatus | undefined) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (s) next.set(STATUS_PARAM, s);
        else next.delete(STATUS_PARAM);
        return next;
      },
      { replace: true },
    );

  return (
    <>
      <PageHeader title="Inbox PGDAS-D" description="Extratos do PGDAS-D: leitura automática, vínculo pelo CNPJ e revisão do que não fecha." />
      <div className="grid grid-cols-1 gap-4 p-4">
        <UploadPanel />
        <div role="group" aria-label="Filtrar por status" className="flex flex-wrap gap-2">
          <FilterToggle active={!status} onClick={() => setStatus(undefined)}>
            Todos ({all})
          </FilterToggle>
          {STATUS_FILTERS.map(([s, label]) => (
            <FilterToggle key={s} active={status === s} onClick={() => setStatus(s)}>
              {label} ({counts[s] ?? 0})
            </FilterToggle>
          ))}
        </div>
        <ErrorAlert error={docs.error} fallback="Não foi possível carregar o inbox." />
        {docs.isPending ? <Skeleton className="h-64" /> : null}
        {data && data.items.length === 0 ? (
          <EmptyState
            icon={Inbox}
            title={status ? "Nenhum extrato nesta situação" : "Nenhum extrato enviado ainda"}
            description="Envie o extrato do PGDAS-D acima. O sistema lê o PA, o CNPJ, a receita e a folha declaradas e vincula à empresa do cadastro."
          />
        ) : null}
        {data && data.items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Extratos do inbox">
              <thead>
                <tr>
                  <Th scope="col">Recebido em</Th>
                  <Th scope="col">Arquivo</Th>
                  <Th scope="col">PA</Th>
                  <Th scope="col">CNPJ</Th>
                  <Th scope="col">Status</Th>
                  <Th scope="col">Motivo</Th>
                  <Th scope="col" className="text-right">
                    Confiança
                  </Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((d) => (
                  <tr key={d.id} data-documento={d.id}>
                    <Td className="whitespace-nowrap">
                      <Link to={`/factor-r/inbox/${d.id}`} className="inline-flex min-h-6 items-center font-medium hover:underline">
                        {formatDateTime(d.created_at)}
                      </Link>
                    </Td>
                    <Td className="max-w-64 truncate">{d.nome_original ?? "—"}</Td>
                    <Td className="tabular-nums">{d.campos.pa ? displayCompetence(d.campos.pa) : "—"}</Td>
                    <Td className="whitespace-nowrap tabular-nums">{d.campos.cnpj ? formatCnpj(d.campos.cnpj) : "—"}</Td>
                    <Td>
                      <DocumentStatusBadge document={d} />
                    </Td>
                    <Td className="text-xs text-muted-foreground">{reasonText(d.motivo)}</Td>
                    <Td className="text-right tabular-nums">{formatRate(d.confianca)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            {data.total > data.items.length ? (
              <p className="border-t px-3 py-2 text-xs text-muted-foreground">
                Mostrando os {data.items.length} mais recentes de {data.total}.
              </p>
            ) : null}
          </div>
        ) : null}
        <FactorRDisclaimer />
      </div>
    </>
  );
}
