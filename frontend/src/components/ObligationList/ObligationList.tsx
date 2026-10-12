import { ChevronLeft, ChevronRight, CircleAlert, ListChecks } from "lucide-react";
import { Link } from "react-router";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { formatCnpj } from "@/lib/cnpj";
import { displayCompetence } from "@/lib/competence";
import { formatDate } from "@/lib/format";
import { MODULE_LABELS, type ModuleCode } from "@/lib/labels";
import { type Backlog, drilldownHref } from "@/services/dashboard";

export interface ObligationListProps {
  "aria-label": string;
  data: Backlog | undefined;
  isLoading: boolean;
  error: unknown;
  onPage: (page: number) => void;
  emptyMessage?: string;
  /** Sem as colunas da empresa (a aba Obrigações da ficha, que já é de uma empresa). */
  showCompany?: boolean;
}

const moduleLabel = (m: string) => (m in MODULE_LABELS ? MODULE_LABELS[m as ModuleCode] : m);

/**
 * Lista de obrigações [PRD §25.2]: empresa, código, CNPJ, competência, módulo, obrigação,
 * status, vencimento, dias em atraso e responsável. Cada item abre a grade do módulo na
 * competência dele, filtrada na empresa. Usada no Backlog e no drill-down do dashboard.
 */
export function ObligationList({ "aria-label": ariaLabel, data, isLoading, error, onPage, emptyMessage, showCompany = true }: ObligationListProps) {
  if (error) return <ErrorAlert error={error} />;
  if (isLoading && !data) return <Skeleton className="h-64" />;
  if (!data || data.items.length === 0) {
    return <EmptyState icon={ListChecks} title="Nada encontrado" description={emptyMessage ?? "Nenhuma obrigação para os filtros selecionados."} />;
  }
  const first = (data.page - 1) * data.page_size + 1;
  const last = first + data.items.length - 1;
  const pages = Math.max(1, Math.ceil(data.total / data.page_size));
  return (
    <div className="grid gap-2">
      <div className="overflow-x-auto rounded-md border bg-card">
        <Table aria-label={ariaLabel}>
          <thead>
            <tr>
              {showCompany ? (
                <>
                  <Th>Empresa</Th>
                  <Th>Código</Th>
                  <Th>CNPJ</Th>
                </>
              ) : null}
              <Th>Competência</Th>
              <Th>Módulo</Th>
              <Th>Obrigação</Th>
              <Th>Status</Th>
              <Th>Vencimento</Th>
              <Th className="text-right">Dias em atraso</Th>
              <Th>Responsável</Th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((it) => (
              <tr key={`${it.kind}:${it.instance_id ?? `${it.company.id}:${it.competence}:${it.module}`}`}>
                {showCompany ? (
                  <>
                    <Td className="max-w-72 truncate font-medium">
                      <Link className="hover:underline" to={drilldownHref(it.drilldown)}>
                        {it.company.legal_name}
                      </Link>
                    </Td>
                    <Td className="tabular-nums">{it.company.internal_code}</Td>
                    <Td className="text-xs tabular-nums">{formatCnpj(it.company.cnpj)}</Td>
                  </>
                ) : null}
                <Td className="tabular-nums">{displayCompetence(it.competence)}</Td>
                <Td>{moduleLabel(it.module)}</Td>
                <Td>
                  {showCompany || !it.obligation_type ? (
                    (it.obligation_type?.name ?? <span className="text-muted-foreground">—</span>)
                  ) : (
                    <Link className="inline-flex min-h-6 items-center font-medium hover:underline" to={drilldownHref(it.drilldown)}>
                      {it.obligation_type.name}
                    </Link>
                  )}
                </Td>
                <Td>{it.kind === "MODULE" ? <Badge variant="warning">Módulo aberto</Badge> : it.status_label}</Td>
                <Td className="tabular-nums">{formatDate(it.due_date) || "—"}</Td>
                <Td className="text-right tabular-nums">
                  {it.days_overdue > 0 ? (
                    <span className="inline-flex items-center gap-1 font-medium text-destructive">
                      <CircleAlert aria-hidden className="size-3.5" /> {it.days_overdue}
                    </span>
                  ) : (
                    "—"
                  )}
                </Td>
                <Td className="text-xs">{it.responsible?.name ?? "—"}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
      </div>
      <div className="flex items-center justify-between text-sm text-muted-foreground">
        <span aria-live="polite">
          {first}–{last} de {data.total}
        </span>
        <div className="flex items-center gap-1">
          <Button variant="outline" size="sm" disabled={data.page <= 1} onClick={() => onPage(data.page - 1)} aria-label="Página anterior">
            <ChevronLeft aria-hidden />
          </Button>
          <span className="px-2 tabular-nums">
            {data.page} / {pages}
          </span>
          <Button variant="outline" size="sm" disabled={data.page >= pages} onClick={() => onPage(data.page + 1)} aria-label="Próxima página">
            <ChevronRight aria-hidden />
          </Button>
        </div>
      </div>
    </div>
  );
}
