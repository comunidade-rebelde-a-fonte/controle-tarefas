import { UserX } from "lucide-react";
import type { DataGridColumn } from "@/components/DataGrid";
import { regimeColumn } from "@/components/ModuleGrid";
import { Badge } from "@/components/ui/badge";
import { PAYROLL_TYPE_LABELS } from "@/lib/labels";
import type { GridRow } from "@/services/grids";

/** Tipo de Folha do snapshot; "Sem folha" com destaque textual e ícone [PRD §17.1, §30]. */
export const payrollTypeColumn: DataGridColumn<GridRow> = {
  id: "payroll_type",
  header: "Tipo de Folha",
  width: 130,
  cell: (row) =>
    row.snapshot.payroll_type === "NO_PAYROLL" ? (
      <Badge variant="warning">
        <UserX aria-hidden className="size-3" /> Sem folha
      </Badge>
    ) : (
      <span className="text-xs">{PAYROLL_TYPE_LABELS[row.snapshot.payroll_type]}</span>
    ),
};

/** Fator R: a API só o envia para o Simples [PRD §17.2]; nos demais regimes a célula fica vazia. */
export const factorRColumn: DataGridColumn<GridRow> = {
  id: "factor_r",
  header: "Fator R",
  width: 90,
  cell: (row) =>
    row.snapshot.factor_r === null ? (
      <span className="text-xs text-muted-foreground" aria-label="Fator R: não se aplica (somente Simples Nacional)">
        —
      </span>
    ) : (
      <span className="text-xs">{row.snapshot.factor_r ? "Sim" : "Não"}</span>
    ),
};

export const PAYROLL_EXTRA_COLUMNS = [regimeColumn, payrollTypeColumn, factorRColumn];
