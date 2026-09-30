import { type ColumnDef, flexRender, getCoreRowModel, useReactTable } from "@tanstack/react-table";
import { useVirtualizer } from "@tanstack/react-virtual";
import { type KeyboardEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { type CellPosition, nextCellPosition } from "./navigation";
import { selectAllState, toggleSelection } from "./selection";

/** Coluna definida por configuração; `cell` é o slot da célula customizada. */
export interface DataGridColumn<TRow> {
  id: string;
  /** Texto do cabeçalho, também usado como nome acessível da coluna. */
  header: string;
  /** Conteúdo do cabeçalho quando não é só texto (ex.: ícone + texto). */
  renderHeader?: () => ReactNode;
  /** Largura em px. */
  width?: number;
  cell: (row: TRow) => ReactNode;
}

export interface DataGridSelection<TRow> {
  selected: ReadonlySet<string>;
  onChange: (next: Set<string>) => void;
  /** Nome acessível do checkbox da linha (ex.: "Selecionar Empresa X"). */
  rowLabel?: (row: TRow) => string;
}

export interface DataGridProps<TRow> {
  "aria-label": string;
  rows: TRow[];
  /** A primeira coluna é o cabeçalho da linha e fica fixa na rolagem horizontal. */
  columns: DataGridColumn<TRow>[];
  getRowId: (row: TRow) => string;
  /** Seleção controlada; sem ela, a grade não tem coluna de checkbox. */
  selection?: DataGridSelection<TRow>;
  /** Enter ou Espaço numa célula de dados: abre o editor da célula. */
  onCellActivate?: (row: TRow, columnId: string) => void;
  emptyMessage?: ReactNode;
  rowHeight?: number;
  /** Altura da área rolável (a grade é virtualizada dentro dela). */
  height?: number | string;
  overscan?: number;
}

export const EMPTY_GRID_MESSAGE = "Nenhuma obrigação aplicável para os filtros selecionados.";

const SELECT_WIDTH = 40;
const FIRST_COL_WIDTH = 260;
const COL_WIDTH = 140;
const PAGE_ROWS = 10;

/**
 * Grade reutilizável das telas operacionais [PRD §26.1, §28.2, §45.1, §45.4]: empresas nas
 * linhas, obrigações nas colunas; cabeçalho e primeira coluna fixos; linhas virtualizadas;
 * seleção por checkbox com shift-click; navegação por teclado no padrão ARIA grid
 * (setas, Home/End, Ctrl+Home/End, PageUp/PageDown; Enter/Espaço ativa a célula).
 */
export function DataGrid<TRow>({
  "aria-label": ariaLabel,
  rows,
  columns,
  getRowId,
  selection,
  onCellActivate,
  emptyMessage = EMPTY_GRID_MESSAGE,
  rowHeight = 36,
  height = "70vh",
  overscan = 10,
}: DataGridProps<TRow>) {
  const columnDefs = useMemo<ColumnDef<TRow>[]>(
    () =>
      columns.map((c, i) => ({
        id: c.id,
        header: c.header,
        size: c.width ?? (i === 0 ? FIRST_COL_WIDTH : COL_WIDTH),
        cell: ({ row }) => c.cell(row.original),
      })),
    [columns],
  );
  // eslint-disable-next-line react-hooks/incompatible-library -- aviso do React Compiler, que o projeto não usa; TanStack Table é da stack [AGENTS §6.1].
  const table = useReactTable({ data: rows, columns: columnDefs, getRowId, getCoreRowModel: getCoreRowModel() });
  const modelRows = table.getRowModel().rows;
  const leafColumns = table.getVisibleLeafColumns();
  const rowIds = useMemo(() => modelRows.map((r) => r.id), [modelRows]);

  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: modelRows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan,
  });

  const selectOffset = selection ? 1 : 0;
  const colCount = selectOffset + leafColumns.length;
  const [active, setActive] = useState<CellPosition>({ row: 0, col: 0 });
  const current: CellPosition = {
    row: Math.min(active.row, Math.max(modelRows.length - 1, 0)),
    col: Math.min(active.col, colCount - 1),
  };
  const pendingFocus = useRef<CellPosition | null>(null);
  const anchor = useRef<number | null>(null);

  // Após navegar pelo teclado, foca a célula ativa assim que ela estiver renderizada.
  useEffect(() => {
    const target = pendingFocus.current;
    if (!target) return;
    const el = scrollRef.current?.querySelector<HTMLElement>(`[data-cell="${target.row}:${target.col}"]`);
    if (el) {
      el.focus();
      pendingFocus.current = null;
    }
  });

  const toggleRow = (index: number, shift: boolean) => {
    if (!selection) return;
    selection.onChange(toggleSelection(rowIds, selection.selected, index, anchor.current, shift));
    anchor.current = index;
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTableSectionElement>) => {
    const target = e.target as HTMLElement;
    const cellEl = target.closest<HTMLElement>("[data-cell]");
    // Teclas dentro de editores abertos na célula (ex.: dropdown) não são da grade.
    if (!cellEl || (target !== cellEl && target.dataset.gridControl !== "select")) return;
    const next = nextCellPosition(current, e.key, e.ctrlKey || e.metaKey, {
      rowCount: modelRows.length,
      colCount,
      pageSize: PAGE_ROWS,
    });
    if (next) {
      e.preventDefault();
      setActive(next);
      pendingFocus.current = next;
      virtualizer.scrollToIndex(next.row, { align: "auto" });
      return;
    }
    if (e.key !== "Enter" && e.key !== " ") return;
    e.preventDefault();
    if (selection && current.col === 0) {
      toggleRow(current.row, e.shiftKey);
      return;
    }
    const row = modelRows[current.row];
    const column = leafColumns[current.col - selectOffset];
    if (row && column) onCellActivate?.(row.original, column.id);
  };

  const widths = [...(selection ? [SELECT_WIDTH] : []), ...leafColumns.map((c) => c.getSize())];
  const totalWidth = widths.reduce((a, b) => a + b, 0);
  const firstColLeft = selection ? SELECT_WIDTH : 0;
  const allState = selection ? selectAllState(rowIds, selection.selected) : "none";

  const items = virtualizer.getVirtualItems();
  const paddingTop = items[0]?.start ?? 0;
  const paddingBottom = items.length > 0 ? virtualizer.getTotalSize() - (items[items.length - 1]?.end ?? 0) : 0;

  // Se a linha ativa saiu da janela virtual (rolagem com o mouse), a primeira linha
  // renderizada recebe o tabindex para a grade continuar alcançável pelo Tab.
  const tabRow = items.some((i) => i.index === current.row) ? current.row : (items[0]?.index ?? current.row);

  const cellProps = (row: number, col: number) => ({
    "data-cell": `${row}:${col}`,
    "aria-colindex": col + 1,
    tabIndex: tabRow === row && current.col === col ? 0 : -1,
    onFocus: () => {
      if (current.row !== row || current.col !== col) setActive({ row, col });
    },
  });
  const focusRing = "outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset";

  return (
    <div ref={scrollRef} className="relative overflow-auto rounded-md border bg-background" style={{ height }}>
      <table
        role="grid"
        aria-label={ariaLabel}
        aria-rowcount={modelRows.length + 1}
        aria-colcount={colCount}
        aria-multiselectable={selection ? true : undefined}
        className="border-separate border-spacing-0 text-sm"
        style={{ width: totalWidth, tableLayout: "fixed" }}
      >
        <colgroup>
          {widths.map((w, i) => (
            <col key={i} style={{ width: w }} />
          ))}
        </colgroup>
        <thead>
          <tr aria-rowindex={1}>
            {selection && (
              <th scope="col" aria-colindex={1} className="sticky top-0 left-0 z-30 h-9 border-b bg-muted px-2 text-center">
                <input
                  type="checkbox"
                  aria-label="Selecionar todas as linhas"
                  className="size-4 accent-primary align-middle"
                  checked={allState === "all"}
                  ref={(el) => {
                    if (el) el.indeterminate = allState === "some";
                  }}
                  disabled={rowIds.length === 0}
                  onChange={() => {
                    selection.onChange(allState === "all" ? new Set() : new Set(rowIds));
                    anchor.current = null;
                  }}
                />
              </th>
            )}
            {table.getFlatHeaders().map((header, i) => {
              const def = columns[i];
              return (
                <th
                  key={header.id}
                  scope="col"
                  aria-colindex={selectOffset + i + 1}
                  className={cn(
                    "sticky top-0 h-9 truncate border-b bg-muted px-2.5 text-left align-middle text-xs font-semibold tracking-wide text-muted-foreground",
                    i === 0 ? "z-30 border-r" : "z-20",
                  )}
                  style={i === 0 ? { left: firstColLeft } : undefined}
                  title={def?.header}
                >
                  {def?.renderHeader ? def.renderHeader() : flexRender(header.column.columnDef.header, header.getContext())}
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody onKeyDown={onKeyDown}>
          {modelRows.length === 0 && (
            <tr>
              <td colSpan={colCount} className="px-4 py-10 text-center text-muted-foreground">
                {emptyMessage}
              </td>
            </tr>
          )}
          {paddingTop > 0 && (
            <tr aria-hidden="true">
              <td colSpan={colCount} style={{ height: paddingTop, padding: 0 }} />
            </tr>
          )}
          {items.map((item) => {
            const row = modelRows[item.index];
            if (!row) return null;
            const r = item.index;
            const isSelected = selection?.selected.has(row.id) ?? false;
            const stickyBg = isSelected ? "bg-accent" : "bg-background group-hover:bg-muted";
            return (
              <tr
                key={row.id}
                aria-rowindex={r + 2}
                aria-selected={selection ? isSelected : undefined}
                className={cn("group", isSelected ? "bg-accent" : "hover:bg-muted")}
                style={{ height: rowHeight }}
              >
                {selection && (
                  <td
                    role="gridcell"
                    {...cellProps(r, 0)}
                    className={cn("sticky left-0 z-10 border-b px-2 text-center", stickyBg, focusRing)}
                  >
                    <input
                      type="checkbox"
                      tabIndex={-1}
                      data-grid-control="select"
                      aria-label={selection.rowLabel?.(row.original) ?? `Selecionar linha ${r + 1}`}
                      className="size-4 accent-primary align-middle"
                      checked={isSelected}
                      onChange={(e) => toggleRow(r, e.nativeEvent instanceof MouseEvent && e.nativeEvent.shiftKey)}
                    />
                  </td>
                )}
                {row.getVisibleCells().map((cell, i) => {
                  const col = selectOffset + i;
                  const content = flexRender(cell.column.columnDef.cell, cell.getContext());
                  return i === 0 ? (
                    <th
                      key={cell.id}
                      scope="row"
                      role="rowheader"
                      {...cellProps(r, col)}
                      className={cn("sticky z-10 truncate border-r border-b px-2.5 text-left font-medium", stickyBg, focusRing)}
                      style={{ left: firstColLeft }}
                    >
                      {content}
                    </th>
                  ) : (
                    <td key={cell.id} role="gridcell" {...cellProps(r, col)} className={cn("truncate border-b px-2.5", focusRing)}>
                      {content}
                    </td>
                  );
                })}
              </tr>
            );
          })}
          {paddingBottom > 0 && (
            <tr aria-hidden="true">
              <td colSpan={colCount} style={{ height: paddingBottom, padding: 0 }} />
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
