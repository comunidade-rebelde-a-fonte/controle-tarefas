import { Download } from "lucide-react";
import { type ReactNode, useMemo } from "react";
import { CompetenceNav } from "@/components/CompetenceNav";
import type { DataGridColumn } from "@/components/DataGrid";
import { ErrorAlert } from "@/components/ErrorAlert";
import { GridFilters, type Option } from "@/components/GridFilters";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { useCompetence } from "@/lib/useCompetence";
import { filterBySearch, useGridFilters } from "@/lib/useGridFilters";
import { type FiscalGroup, type GridColumn, type GridModule, type GridRow, gridExportUrl, gridFilterQuery, useModuleGrid } from "@/services/grids";
import { ModuleGrid } from "./ModuleGrid";

function filterOptions(columns: GridColumn[]): { status: Option[]; columns: Option[]; closing: Option[] } {
  const status = new Map<string, string>();
  const closing: Option[] = [];
  const controls: Option[] = [];
  for (const c of columns) {
    if (c.is_closing_stage) {
      for (const s of c.statuses) closing.push({ value: s.code, label: s.label });
      continue;
    }
    controls.push({ value: c.obligation_type.id, label: c.obligation_type.name });
    for (const s of c.statuses) if (!status.has(s.code)) status.set(s.code, s.label);
  }
  return { status: [...status].map(([value, label]) => ({ value, label })), columns: controls, closing };
}

export interface ModuleScreenProps {
  module: GridModule;
  title: string;
  description: string;
  /** Nome do módulo nas mensagens (ex.: "Declarações"). */
  moduleLabel: string;
  /** Agrupamento (somente Fiscal). */
  group?: FiscalGroup;
  /** Controles extras à esquerda do resumo (ex.: alternador Normais | Simples). */
  toolbar?: ReactNode;
  /** Estáveis entre renders (constantes de módulo): as colunas da grade não podem ser recriadas. */
  extraColumns?: DataGridColumn<GridRow>[];
  trailingColumns?: DataGridColumn<GridRow>[];
  showProgress?: boolean;
  columnHeader?: (column: GridColumn, index: number) => string;
  readyToCloseText?: string;
}

/**
 * Tela operacional de módulo [PRD §26, §28-§32]: competência, busca e filtros na URL, resumo e
 * a grade reutilizável. Cada módulo só configura colunas extras e textos.
 */
export function ModuleScreen({
  module,
  title,
  description,
  moduleLabel,
  group,
  toolbar,
  extraColumns,
  trailingColumns,
  showProgress,
  columnHeader,
  readyToCloseText,
}: ModuleScreenProps) {
  const { competence, error: competenceError } = useCompetence();
  const { filters, setFilter } = useGridFilters();
  const query = competence ? { competence, ...gridFilterQuery(filters), ...(group ? { group } : {}) } : null;
  const grid = useModuleGrid(module, query);
  const rows = useMemo(() => filterBySearch(grid.data?.rows ?? [], filters.q, (r) => r.company), [grid.data, filters.q]);
  const opts = useMemo(() => filterOptions(grid.data?.columns ?? []), [grid.data?.columns]);
  const ready = rows.filter((r) => r.ready_to_close).length;
  const overdue = rows.filter((r) => r.overdue > 0).length;

  return (
    <>
      <PageHeader
        title={title}
        description={description}
        actions={
          <>
            <CompetenceNav />
            {query ? (
              <Button variant="outline" asChild>
                <a href={gridExportUrl(module, query, filters.q)} download title="Exporta a grade com a competência, os filtros e a busca atuais">
                  <Download aria-hidden /> Exportar
                </a>
              </Button>
            ) : (
              <Button variant="outline" disabled>
                <Download aria-hidden /> Exportar
              </Button>
            )}
          </>
        }
      />
      <div className="grid gap-3 p-4">
        <div className="flex flex-wrap items-center gap-3">
          {toolbar}
          <p className="text-sm text-muted-foreground" aria-live="polite">
            {rows.length} {rows.length === 1 ? "empresa" : "empresas"}
            {ready > 0 ? (
              <>
                {" · "}
                <button
                  type="button"
                  className="font-medium text-success underline-offset-2 hover:underline"
                  onClick={() => setFilter("readyToClose", true)}
                >
                  {ready} {ready === 1 ? "pronta" : "prontas"} para fechar
                </button>
              </>
            ) : null}
            {overdue > 0 ? ` · ${overdue} com atraso` : null}
          </p>
        </div>
        <GridFilters statusOptions={opts.status} columnOptions={opts.columns} closingStatusOptions={opts.closing} resultCount={rows.length} />
        <ErrorAlert error={competenceError} fallback="Não foi possível carregar a competência." />
        <ModuleGrid
          moduleLabel={moduleLabel}
          grid={grid.data}
          rows={rows}
          isLoading={grid.isPending}
          error={grid.error}
          {...(extraColumns ? { extraColumns } : {})}
          {...(trailingColumns ? { trailingColumns } : {})}
          {...(showProgress !== undefined ? { showProgress } : {})}
          {...(columnHeader ? { columnHeader } : {})}
          {...(readyToCloseText ? { readyToCloseText } : {})}
        />
      </div>
    </>
  );
}
