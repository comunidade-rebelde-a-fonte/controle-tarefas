import { NativeSelect } from "@/components/ui/input";
import { TAX_REGIME_LABELS, TAX_REGIMES } from "@/lib/labels";
import { useUrlParams } from "@/lib/useUrlParams";
import { cn } from "@/lib/utils";
import { useUsers } from "@/services/users";

const GROUPS = [
  { value: "", label: "Todas" },
  { value: "NORMAL", label: "Normais" },
  { value: "SIMPLES", label: "Simples Nacional" },
];

/** Filtros do dashboard em uma linha, na URL: agrupamento, regime e responsável [§24.4-§24.5]. */
export function DashboardFilters() {
  const { get, set } = useUrlParams();
  const users = useUsers(true);
  const group = get("regime_group");
  return (
    <div className="flex flex-wrap items-center gap-2" role="search" aria-label="Filtros do dashboard">
      <div role="group" aria-label="Agrupamento" className="inline-flex rounded-md border bg-card p-0.5">
        {GROUPS.map((g) => (
          <button
            key={g.value}
            type="button"
            aria-pressed={group === g.value}
            onClick={() => set({ regime_group: g.value, tax_regime: "" })}
            className={cn(
              "rounded px-3 py-1 text-sm font-medium",
              group === g.value ? "bg-primary text-primary-foreground" : "text-muted-foreground hover:bg-accent",
            )}
          >
            {g.label}
          </button>
        ))}
      </div>
      <NativeSelect aria-label="Regime" className="w-48" value={get("tax_regime")} onChange={(e) => set({ tax_regime: e.target.value, regime_group: "" })}>
        <option value="">Todos os regimes</option>
        {TAX_REGIMES.map((r) => (
          <option key={r} value={r}>
            {TAX_REGIME_LABELS[r]}
          </option>
        ))}
      </NativeSelect>
      <NativeSelect aria-label="Responsável" className="w-48" value={get("responsible_id")} onChange={(e) => set({ responsible_id: e.target.value })}>
        <option value="">Todos os responsáveis</option>
        {(users.data?.items ?? []).map((u) => (
          <option key={u.id} value={u.id}>
            {u.name}
          </option>
        ))}
      </NativeSelect>
    </div>
  );
}
