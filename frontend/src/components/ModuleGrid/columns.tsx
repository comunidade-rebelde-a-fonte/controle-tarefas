import type { DataGridColumn } from "@/components/DataGrid";
import { TAX_REGIME_LABELS } from "@/lib/labels";
import type { GridRow } from "@/services/grids";

/** Coluna "Regime" (do snapshot da competência), comum às grades. */
export const regimeColumn: DataGridColumn<GridRow> = {
  id: "regime",
  header: "Regime",
  width: 140,
  cell: (row) => <span className="truncate text-xs">{TAX_REGIME_LABELS[row.snapshot.tax_regime]}</span>,
};

