import { Download } from "lucide-react";
import { useMemo } from "react";
import { CompetenceNav } from "@/components/CompetenceNav";
import { type DataGridColumn } from "@/components/DataGrid";
import { ErrorAlert } from "@/components/ErrorAlert";
import { GridFilters, type Option } from "@/components/GridFilters";
import { ModuleGrid } from "@/components/ModuleGrid";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Tooltip } from "@/components/ui/tooltip";
import { TAX_REGIME_LABELS } from "@/lib/labels";
import { useCompetence } from "@/lib/useCompetence";
import { filterBySearch, useGridFilters } from "@/lib/useGridFilters";
import { cn } from "@/lib/utils";
import { type FiscalGroup, type GridColumn, type GridRow, gridFilterQuery, useFiscalGrid } from "@/services/grids";

/** Texto do §13.5 para empresas com todos os controles concluídos. */
export const FISCAL_READY_TEXT = "Todos os controles concluídos — sugerido encerrar o Fiscal desta competência.";

const GROUPS: { value: FiscalGroup; label: string }[] = [
  { value: "normal", label: "Normais" },
  { value: "simples", label: "Simples Nacional" },
];

const regimeColumn: DataGridColumn<GridRow> = {
  id: "regime",
  header: "Regime",
  width: 140,
  cell: (row) => <span className="truncate text-xs">{TAX_REGIME_LABELS[row.snapshot.tax_regime]}</span>,
};

/** Estável entre renders (as colunas da grade não podem ser recriadas). */
const EXTRA_COLUMNS = [regimeColumn];

function options(columns: GridColumn[]): { status: Option[]; columns: Option[]; closing: Option[] } {
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

/**
 * Tela Fiscal [PRD §13, §14, §28, UC-02]: competência, busca, filtros e alternador
 * Normais | Simples Nacional na URL; grade com as colunas do catálogo e fechamento por linha.
 */
export function FiscalPage() {
  const { competence, error: competenceError } = useCompetence();
  const { filters, setFilter } = useGridFilters();
  const group: FiscalGroup = filters.group || "normal";
  const query = competence ? { competence, group, ...gridFilterQuery(filters) } : null;
  const grid = useFiscalGrid(query);
  const rows = useMemo(() => filterBySearch(grid.data?.rows ?? [], filters.q, (r) => r.company), [grid.data, filters.q]);
  const opts = useMemo(() => options(grid.data?.columns ?? []), [grid.data?.columns]);
  const ready = rows.filter((r) => r.ready_to_close).length;
  const overdue = rows.filter((r) => r.overdue > 0).length;

  return (
    <>
      <PageHeader
        title="Fiscal"
        description="Controles fiscais por competência — Normais e Simples Nacional."
        actions={
          <>
            <CompetenceNav />
            <Tooltip content="Exportação disponível em breve.">
              <span>
                <Button variant="outline" disabled>
                  <Download aria-hidden /> Exportar
                </Button>
              </span>
            </Tooltip>
          </>
        }
      />
      <div className="grid gap-3 p-4">
        <div className="flex flex-wrap items-center gap-3">
          <div role="group" aria-label="Agrupamento" className="inline-flex rounded-md border bg-card p-0.5">
            {GROUPS.map((g) => (
              <button
                key={g.value}
                type="button"
                aria-pressed={group === g.value}
                onClick={() => setFilter("group", g.value)}
                className={cn(
                  "rounded px-3 py-1 text-sm font-medium",
                  group === g.value ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-accent",
                )}
              >
                {g.label}
              </button>
            ))}
          </div>
          <p className="text-sm text-muted-foreground" aria-live="polite">
            {rows.length} {rows.length === 1 ? "empresa" : "empresas"}
            {ready > 0 ? (
              <>
                {" · "}
                <button type="button" className="font-medium text-emerald-700 underline-offset-2 hover:underline" onClick={() => setFilter("readyToClose", true)}>
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
          moduleLabel="Fiscal"
          grid={grid.data}
          rows={rows}
          isLoading={grid.isPending}
          error={grid.error}
          extraColumns={EXTRA_COLUMNS}
          readyToCloseText={FISCAL_READY_TEXT}
        />
      </div>
    </>
  );
}
