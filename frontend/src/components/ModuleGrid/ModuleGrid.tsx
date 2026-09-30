import { useQueryClient } from "@tanstack/react-query";
import { CircleAlert, Lock, MoreHorizontal } from "lucide-react";
import { createContext, useContext, useMemo, useState } from "react";
import { BulkActionBar } from "@/components/BulkActionBar";
import { CopyButton, CopyCnpjButton } from "@/components/CopyButton";
import { DataGrid, type DataGridColumn } from "@/components/DataGrid";
import {
  CellDetailsPopover,
  CloseModuleDialog,
  DependencyWarningDialog,
  DueDateOverrideDialog,
  HistoryDrawer,
  type PeriodTarget,
  ReopenDialog,
} from "@/components/dialogs";
import { ErrorAlert } from "@/components/ErrorAlert";
import { NoteCell } from "@/components/NoteCell";
import { type DependencyErrorEvent, StatusCell } from "@/components/StatusCell";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip } from "@/components/ui/tooltip";
import { formatCnpj } from "@/lib/cnpj";
import { formatDate, formatPercent } from "@/lib/format";
import {
  type GridColumn,
  type GridRow,
  type ModuleGrid as Grid,
  applyStatusChange,
} from "@/services/grids";
import type {
  ObligationInstance,
  StatusChangeResult,
} from "@/services/obligations";

export interface ModuleGridProps {
  /** Nome do módulo nas mensagens (ex.: "Fiscal"). */
  moduleLabel: string;
  grid: Grid | undefined;
  /** Linhas exibidas (após a busca livre no cliente). */
  rows: GridRow[];
  isLoading: boolean;
  error: unknown;
  /** Colunas extras depois do CNPJ (ex.: regime, tipo de folha). */
  extraColumns?: DataGridColumn<GridRow>[];
  /** Colunas extras depois das obrigações (ex.: dependências da DCTFWeb). */
  trailingColumns?: DataGridColumn<GridRow>[];
  /** Colunas de progresso e conclusão (a visão DCTFWeb não tem agregado próprio). */
  showProgress?: boolean;
  /** Cabeçalho da coluna de obrigação (ex.: etapa numerada no Contábil). Estável entre renders. */
  columnHeader?: (column: GridColumn, index: number) => string;
  /** Texto do "pronto para fechar" (ex.: PRD §13.5). */
  readyToCloseText?: string;
}

const OB = "ob:";

/** Padrão estável para as colunas extras (um array novo a cada render remontaria as células). */
const NO_COLUMNS: DataGridColumn<GridRow>[] = [];

/** Estado de interface compartilhado com as células (as colunas ficam estáveis e não remontam). */
interface GridUi {
  openCell: { row: string; col: string } | null;
  setOpenCell: (v: { row: string; col: string } | null) => void;
  editingNote: string | null;
  setEditingNote: (v: string | null) => void;
  onDependency: (e: DependencyErrorEvent) => void;
  onChanged: (res: StatusChangeResult) => void;
  openDueDate: (instance: ObligationInstance, label: string) => void;
  openHistory: (id: string, title: string) => void;
  togglePeriod: (row: GridRow) => void;
}

const GridUiContext = createContext<GridUi | null>(null);

function useGridUi(): GridUi {
  const ui = useContext(GridUiContext);
  if (!ui) throw new Error("ModuleGrid: célula fora do contexto da grade");
  return ui;
}

function ObligationCell({ row, col }: { row: GridRow; col: GridColumn }) {
  const ui = useGridUi();
  const cell = row.cells[col.obligation_type.code];
  if (!cell)
    return (
      <span
        className="text-xs text-muted-foreground"
        aria-label="Não gerada nesta competência"
      >
        —
      </span>
    );
  const colId = OB + col.obligation_type.code;
  const label = `${col.obligation_type.name} — ${row.company.legal_name}`;
  const locked = cell.locked;
  const isOpen =
    ui.openCell?.row === row.company.id && ui.openCell.col === colId;
  return (
    <div className="flex min-w-0 items-center">
      <StatusCell
        instance={cell}
        statuses={col.statuses}
        label={label}
        locked={locked}
        tabIndex={-1}
        open={isOpen}
        onOpenChange={(open) =>
          ui.setOpenCell(open ? { row: row.company.id, col: colId } : null)
        }
        onChanged={ui.onChanged}
        onDependencyError={ui.onDependency}
        blockedMessage={cell.blocked_message}
      />
      <CellDetailsPopover
        instance={cell}
        allowsTransmissionDate={col.allows_transmission_date}
        allowsProtocol={col.allows_protocol}
        locked={locked}
        onEditDueDate={() => ui.openDueDate(cell, label)}
        onShowHistory={() => ui.openHistory(cell.id, label)}
      >
        <button
          type="button"
          tabIndex={-1}
          aria-label={`Detalhes de ${label}`}
          className="inline-flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-accent"
        >
          <MoreHorizontal aria-hidden className="size-3.5" />
        </button>
      </CellDetailsPopover>
    </div>
  );
}

function NoteColumnCell({ row }: { row: GridRow }) {
  const ui = useGridUi();
  return (
    <NoteCell
      periodId={row.period.id}
      version={row.period.version}
      note={row.period.general_note}
      label={row.company.legal_name}
      tabIndex={-1}
      editing={ui.editingNote === row.company.id}
      onEditingChange={(editing) =>
        ui.setEditingNote(editing ? row.company.id : null)
      }
    />
  );
}

function PeriodCell({
  row,
  moduleLabel,
  readyToCloseText,
}: {
  row: GridRow;
  moduleLabel: string;
  readyToCloseText: string | undefined;
}) {
  const ui = useGridUi();
  const closed = row.period.state === "CLOSED";
  return (
    <span className="flex items-center gap-1.5">
      {closed ? (
        <Badge variant="muted">
          <Lock aria-hidden className="size-3" /> Fechado
        </Badge>
      ) : row.ready_to_close ? (
        <Tooltip
          content={
            readyToCloseText ?? "Todos os itens concluídos — sugerido fechar."
          }
        >
          <Badge
            variant="success"
            aria-label={`Pronto para fechar. ${readyToCloseText ?? ""}`.trim()}
          >
            Pronto para fechar
          </Badge>
        </Tooltip>
      ) : null}
      <Button
        size="sm"
        variant={!closed && row.ready_to_close ? "default" : "outline"}
        tabIndex={-1}
        className="ml-auto h-7"
        aria-label={`${closed ? "Reabrir" : "Fechar"} ${moduleLabel} de ${row.company.legal_name}`}
        onClick={() => ui.togglePeriod(row)}
      >
        {closed ? "Reabrir" : "Fechar"}
      </Button>
    </span>
  );
}

/**
 * Grade operacional reutilizável [PRD §26, §28]: empresas nas linhas, obrigações do catálogo
 * nas colunas, com status inline, detalhes, observação, progresso e fechamento por linha.
 * Tudo que é regra (atraso, progresso, pronto para fechar, bloqueios) vem calculado da API.
 */
export function ModuleGrid({
  moduleLabel,
  grid,
  rows,
  isLoading,
  error,
  extraColumns = NO_COLUMNS,
  trailingColumns = NO_COLUMNS,
  showProgress = true,
  columnHeader,
  readyToCloseText,
}: ModuleGridProps) {
  const qc = useQueryClient();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [openCell, setOpenCell] = useState<{ row: string; col: string } | null>(
    null,
  );
  const [editingNote, setEditingNote] = useState<string | null>(null);
  const [dependency, setDependency] = useState<DependencyErrorEvent | null>(
    null,
  );
  const [closing, setClosing] = useState<PeriodTarget | null>(null);
  const [reopening, setReopening] = useState<PeriodTarget | null>(null);
  const [history, setHistory] = useState<{ id: string; title: string } | null>(
    null,
  );
  const [dueDate, setDueDate] = useState<{
    instance: ObligationInstance;
    label: string;
  } | null>(null);
  const competence = grid?.competence ?? "";
  const gridColumns = grid?.columns;
  // Sem fechamento próprio (visão DCTFWeb, plan R1), a grade não mostra fechar/reabrir.
  const closable = grid?.closable ?? false;

  const target = (row: GridRow): PeriodTarget => ({
    periodId: row.period.id,
    version: row.period.version,
    companyName: row.company.legal_name,
    moduleLabel,
    competence,
  });
  const togglePeriod = (row: GridRow) =>
    row.period.state === "CLOSED"
      ? setReopening(target(row))
      : setClosing(target(row));

  // Colunas estáveis: o flexRender trata a função da célula como componente, então recriá-las
  // remontaria as células (menus abertos fechariam). O estado de UI chega pelo contexto.
  const columns = useMemo<DataGridColumn<GridRow>[]>(() => {
    const base: DataGridColumn<GridRow>[] = [
      {
        id: "company",
        header: "Empresa",
        width: 260,
        cell: (row) => (
          <span
            className="block truncate"
            title={
              row.company.trade_name
                ? `${row.company.legal_name} (${row.company.trade_name})`
                : row.company.legal_name
            }
          >
            {row.company.legal_name}
          </span>
        ),
      },
      {
        id: "code",
        header: "Código",
        width: 96,
        cell: (row) => (
          <span className="flex items-center gap-0.5 tabular-nums">
            {row.company.internal_code}
            <CopyButton
              value={row.company.internal_code}
              label="Copiar código"
              tabIndex={-1}
            />
          </span>
        ),
      },
      {
        id: "cnpj",
        header: "CNPJ",
        width: 180,
        cell: (row) => (
          <span className="flex items-center gap-0.5 text-xs tabular-nums">
            {formatCnpj(row.company.cnpj)}
            <CopyCnpjButton cnpj={row.company.cnpj} tabIndex={-1} />
          </span>
        ),
      },
      ...extraColumns,
      ...(gridColumns ?? []).map((col, index): DataGridColumn<GridRow> => ({
        id: OB + col.obligation_type.code,
        header: columnHeader
          ? columnHeader(col, index)
          : col.obligation_type.name,
        width: 168,
        cell: (row) => <ObligationCell row={row} col={col} />,
      })),
      ...trailingColumns,
      ...(showProgress
        ? ([
            {
              id: "progress",
              header: "Progresso operacional",
              width: 110,
              cell: (row) => (
                <Percent
                  value={row.progress.operational_progress}
                  label="Progresso operacional"
                />
              ),
            },
            {
              id: "completion",
              header: "Conclusão final",
              width: 110,
              cell: (row) => (
                <Percent
                  value={row.progress.final_completion}
                  label="Conclusão final"
                />
              ),
            },
          ] satisfies DataGridColumn<GridRow>[])
        : []),
      {
        id: "deadline",
        header: "Prazo crítico",
        width: 150,
        cell: (row) => (
          <span className="flex items-center gap-1.5 text-xs tabular-nums">
            {formatDate(row.nearest_deadline) || "—"}
            {row.overdue > 0 ? (
              <Badge
                variant="danger"
                aria-label={`${row.overdue} ${row.overdue === 1 ? "obrigação atrasada" : "obrigações atrasadas"}`}
              >
                <CircleAlert aria-hidden className="size-3" /> {row.overdue}
              </Badge>
            ) : null}
          </span>
        ),
      },
      {
        id: "note",
        header: "Observações",
        width: 220,
        cell: (row) => <NoteColumnCell row={row} />,
      },
    ];
    if (!closable) return base;
    return [
      ...base,
      {
        id: "period",
        header: `Fechamento do ${moduleLabel}`,
        width: 230,
        cell: (row) => (
          <PeriodCell
            row={row}
            moduleLabel={moduleLabel}
            readyToCloseText={readyToCloseText}
          />
        ),
      },
    ];
  }, [
    gridColumns,
    extraColumns,
    trailingColumns,
    showProgress,
    columnHeader,
    closable,
    moduleLabel,
    readyToCloseText,
  ]);

  const ui: GridUi = {
    openCell,
    setOpenCell,
    editingNote,
    setEditingNote,
    onDependency: setDependency,
    onChanged: (res) => applyStatusChange(qc, res),
    openDueDate: (instance, label) => setDueDate({ instance, label }),
    openHistory: (id, title) => setHistory({ id, title }),
    togglePeriod,
  };

  const visibleIds = new Set(rows.map((r) => r.company.id));
  const selectedVisible = [...selected].filter((id) => visibleIds.has(id));

  if (error)
    return (
      <ErrorAlert error={error} fallback="Não foi possível carregar a grade." />
    );
  if (isLoading && !grid) return <Skeleton className="h-[60vh]" />;

  return (
    <GridUiContext.Provider value={ui}>
      <div className="grid gap-3">
        {grid ? (
          <BulkActionBar
            competence={grid.competence}
            columns={grid.columns}
            selectedCompanyIds={selectedVisible}
            onClearSelection={() => setSelected(new Set())}
          />
        ) : null}
        <DataGrid<GridRow>
          aria-label={`Grade ${moduleLabel}`}
          rows={rows}
          columns={columns}
          getRowId={(row) => row.company.id}
          selection={{
            selected,
            onChange: setSelected,
            rowLabel: (row) => `Selecionar ${row.company.legal_name}`,
          }}
          onCellActivate={(row, columnId) => {
            if (columnId.startsWith(OB))
              setOpenCell({ row: row.company.id, col: columnId });
            else if (columnId === "note") setEditingNote(row.company.id);
            else if (columnId === "period") togglePeriod(row);
          }}
          height="calc(100vh - 290px)"
        />
        <DependencyWarningDialog
          event={dependency}
          onClose={() => setDependency(null)}
          onResolved={(res) => applyStatusChange(qc, res)}
        />
        <CloseModuleDialog target={closing} onClose={() => setClosing(null)} />
        <ReopenDialog target={reopening} onClose={() => setReopening(null)} />
        <HistoryDrawer
          instanceId={history?.id ?? null}
          title={history?.title ?? ""}
          open={!!history}
          onOpenChange={(open) => !open && setHistory(null)}
        />
        <DueDateOverrideDialog
          instance={dueDate?.instance ?? null}
          {...(dueDate ? { label: dueDate.label } : {})}
          onClose={() => setDueDate(null)}
        />
      </div>
    </GridUiContext.Provider>
  );
}

function Percent({ value, label }: { value: number; label: string }) {
  const pct = formatPercent(value);
  return (
    <span className="flex items-center gap-1.5" aria-label={`${label}: ${pct}`}>
      <span
        className="h-1.5 w-10 overflow-hidden rounded-full bg-muted"
        aria-hidden
      >
        <span
          className="block h-full bg-primary"
          style={{ width: `${Math.round(value * 100)}%` }}
        />
      </span>
      <span className="text-xs tabular-nums">{pct}</span>
    </span>
  );
}
