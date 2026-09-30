import { ModuleScreen, regimeColumn } from "@/components/ModuleGrid";
import { useGridFilters } from "@/lib/useGridFilters";
import { cn } from "@/lib/utils";
import type { FiscalGroup } from "@/services/grids";

/** Texto do §13.5 para empresas com todos os controles concluídos. */
export const FISCAL_READY_TEXT = "Todos os controles concluídos — sugerido encerrar o Fiscal desta competência.";

const GROUPS: { value: FiscalGroup; label: string }[] = [
  { value: "normal", label: "Normais" },
  { value: "simples", label: "Simples Nacional" },
];

const EXTRA_COLUMNS = [regimeColumn];

/**
 * Tela Fiscal [PRD §13, §14, §28, UC-02]: alternador Normais | Simples Nacional na URL e a grade
 * com as colunas do catálogo e o fechamento por linha.
 */
export function FiscalPage() {
  const { filters, setFilter } = useGridFilters();
  const group: FiscalGroup = filters.group || "normal";
  return (
    <ModuleScreen
      module="fiscal"
      title="Fiscal"
      description="Controles fiscais por competência — Normais e Simples Nacional."
      moduleLabel="Fiscal"
      group={group}
      extraColumns={EXTRA_COLUMNS}
      readyToCloseText={FISCAL_READY_TEXT}
      toolbar={
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
      }
    />
  );
}
