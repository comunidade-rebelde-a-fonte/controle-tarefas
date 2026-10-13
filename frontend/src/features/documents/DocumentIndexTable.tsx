import { ChevronLeft, ChevronRight, Files } from "lucide-react";
import { Link } from "react-router";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, Td, Th } from "@/components/ui/table";
import { displayCompetence } from "@/lib/competence";
import { formatDate, formatDateTime } from "@/lib/format";
import { drilldownHref } from "@/services/dashboard";
import type { DocumentIndex, DocumentIndexItem } from "@/services/documents";
import { KIND_LABELS, MODULES, PAGE_SIZE, SITUATIONS, STORAGE_LABELS, statusText } from "./labels";

/** O período do documento: o PA, a competência ou o período lido; a validade nas credenciais. */
function periodText(d: DocumentIndexItem): string {
  if (d.module === "credentials") return d.expires_at ? `válido até ${formatDate(d.expires_at)}` : "sem validade informada";
  if (d.period_start) return `${formatDate(d.period_start)} a ${formatDate(d.period_end)}`;
  if (d.competence) return displayCompetence(d.competence);
  return "—";
}

/**
 * A lista do índice de documentos (TASK-026, D-040): cada documento com a origem e o
 * armazenamento, o tipo, a empresa, o período, o status do módulo e a situação comum. O nome do
 * arquivo abre o documento na tela do módulo.
 */
export function DocumentIndexTable({
  data,
  offset,
  onOffset,
  showCompany = true,
  "aria-label": ariaLabel,
}: {
  data: DocumentIndex;
  offset: number;
  onOffset: (offset: number) => void;
  showCompany?: boolean;
  "aria-label": string;
}) {
  if (data.items.length === 0) {
    return <EmptyState icon={Files} title="Nenhum documento" description="Com os filtros escolhidos, não há documento." />;
  }
  const first = offset + 1;
  const last = offset + data.items.length;
  return (
    <div className="grid gap-2">
      <div className="overflow-x-auto rounded-lg border bg-card">
        <Table aria-label={ariaLabel}>
          <thead>
            <tr>
              <Th scope="col">Recebido em</Th>
              <Th scope="col">Arquivo</Th>
              <Th scope="col">Origem</Th>
              <Th scope="col">Tipo</Th>
              {showCompany ? <Th scope="col">Empresa</Th> : null}
              <Th scope="col">Período</Th>
              <Th scope="col">Status</Th>
              <Th scope="col">Situação</Th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((d) => {
              const situation = SITUATIONS.find(([s]) => s === d.situation);
              return (
                <tr key={`${d.module}:${d.id}`} data-modulo={d.module}>
                  <Td className="text-xs whitespace-nowrap tabular-nums">{formatDateTime(d.received_at)}</Td>
                  <Td className="max-w-72">
                    <Link to={drilldownHref(d.detail)} className="block max-w-72 truncate leading-6 font-medium hover:underline" title={d.file_name}>
                      {d.file_name}
                    </Link>
                  </Td>
                  <Td className="text-xs whitespace-nowrap">
                    {MODULES.find(([m]) => m === d.module)?.[1] ?? d.module}
                    <div className="text-muted-foreground">{STORAGE_LABELS[d.storage_backend] ?? d.storage_backend}</div>
                  </Td>
                  <Td className="text-xs">{KIND_LABELS[d.kind]}</Td>
                  {showCompany ? (
                    <Td className="text-xs">
                      {d.company ? (
                        <Link to={`/companies/${d.company.id}`} className="inline-flex min-h-6 items-center hover:underline">
                          {d.company.legal_name}
                        </Link>
                      ) : (
                        <span className="text-muted-foreground">sem empresa</span>
                      )}
                    </Td>
                  ) : null}
                  <Td className="text-xs whitespace-nowrap tabular-nums">{periodText(d)}</Td>
                  <Td className="text-xs">{statusText(d.module, d.status)}</Td>
                  <Td>{situation ? <Badge variant={situation[2]}>{situation[1]}</Badge> : d.situation}</Td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      </div>
      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span aria-live="polite">
          {first}–{last} de {data.total}
        </span>
        <div className="flex items-center gap-1">
          <Button variant="outline" size="sm" disabled={offset === 0} onClick={() => onOffset(Math.max(0, offset - PAGE_SIZE))} aria-label="Página anterior">
            <ChevronLeft aria-hidden />
          </Button>
          <Button variant="outline" size="sm" disabled={last >= data.total} onClick={() => onOffset(offset + PAGE_SIZE)} aria-label="Próxima página">
            <ChevronRight aria-hidden />
          </Button>
        </div>
      </div>
    </div>
  );
}
