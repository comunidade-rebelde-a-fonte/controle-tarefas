import { CircleCheck, TriangleAlert } from "lucide-react";
import type { DataGridColumn } from "@/components/DataGrid";
import { Badge } from "@/components/ui/badge";
import type { GridRow } from "@/services/grids";

/** Coluna "Dependências" [PRD §31]: apta à conclusão ou pendente, com os componentes. */
export const dependenciesColumn: DataGridColumn<GridRow> = {
  id: "dependencies",
  header: "Dependências",
  width: 260,
  cell: (row) => {
    const d = row.dependencies;
    if (!d) return <span className="text-xs text-muted-foreground">—</span>;
    if (d.ready) {
      return (
        <Badge variant="success">
          <CircleCheck aria-hidden className="size-3" /> Apta
        </Badge>
      );
    }
    const names = d.pending.map((p) => p.name).join(", ");
    return (
      <Badge variant="warning" className="max-w-full" title={`Pendente: ${names}`}>
        <TriangleAlert aria-hidden className="size-3 shrink-0" />
        <span className="truncate">Pendente: {names}</span>
      </Badge>
    );
  },
};
