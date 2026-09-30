import { Search } from "lucide-react";
import { useEffect, useEffectEvent, useMemo, useState } from "react";
import { Input, NativeSelect } from "@/components/ui/input";
import { MODULE_LABELS } from "@/lib/labels";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { useUrlParams } from "@/lib/useUrlParams";
import { useStatusSets } from "@/services/catalog";
import { useUsers } from "@/services/users";
import { SORT_LABELS } from "./labels";

const MODULES = ["FISCAL", "DECLARATIONS", "PAYROLL", "ACCOUNTING"] as const;

/** Filtros do Backlog e da lista filtrada [PRD §25.3], todos na URL. */
export function BacklogFilters({ showSort = true, showOverdue = true }: { showSort?: boolean; showOverdue?: boolean }) {
  const { get, set } = useUrlParams();
  const users = useUsers(true);
  const sets = useStatusSets();
  const [search, setSearch] = useState(get("q"));
  const debounced = useDebouncedValue(search, 250);
  const syncSearch = useEffectEvent((v: string) => {
    if (v !== get("q")) set({ q: v });
  });
  useEffect(() => {
    syncSearch(debounced);
  }, [debounced]);

  // Um rótulo por código de status (os conjuntos compartilham os códigos principais).
  const statuses = useMemo(() => {
    const m = new Map<string, string>();
    for (const set of sets.data?.items ?? []) for (const s of set.statuses) if (!m.has(s.code)) m.set(s.code, s.label);
    return [...m];
  }, [sets.data]);

  return (
    <div className="flex flex-wrap items-center gap-2" role="search" aria-label="Filtros da lista">
      <div className="relative w-80">
        <Search aria-hidden className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input type="search" aria-label="Buscar empresa" placeholder="Empresa, código ou CNPJ" className="pl-8" value={search} onChange={(e) => setSearch(e.target.value)} />
      </div>
      <NativeSelect aria-label="Módulo" className="w-40" value={get("module")} onChange={(e) => set({ module: e.target.value })}>
        <option value="">Todos os módulos</option>
        {MODULES.map((m) => (
          <option key={m} value={m}>
            {MODULE_LABELS[m]}
          </option>
        ))}
      </NativeSelect>
      <NativeSelect aria-label="Responsável" className="w-44" value={get("responsible_id")} onChange={(e) => set({ responsible_id: e.target.value })}>
        <option value="">Todos os responsáveis</option>
        {(users.data?.items ?? []).map((u) => (
          <option key={u.id} value={u.id}>
            {u.name}
          </option>
        ))}
      </NativeSelect>
      <NativeSelect aria-label="Status" className="w-44" value={get("status")} onChange={(e) => set({ status: e.target.value })}>
        <option value="">Qualquer status</option>
        {statuses.map(([code, label]) => (
          <option key={code} value={code}>
            {label}
          </option>
        ))}
      </NativeSelect>
      {showOverdue ? (
        <>
          <Input
            type="number"
            min={0}
            aria-label="Atraso mínimo (dias)"
            placeholder="Atraso de"
            className="w-28"
            value={get("overdue_min")}
            onChange={(e) => set({ overdue_min: e.target.value })}
          />
          <Input
            type="number"
            min={0}
            aria-label="Atraso máximo (dias)"
            placeholder="até (dias)"
            className="w-28"
            value={get("overdue_max")}
            onChange={(e) => set({ overdue_max: e.target.value })}
          />
        </>
      ) : null}
      {showSort ? (
        <NativeSelect aria-label="Ordenação" className="w-48" value={get("sort") || "days_overdue"} onChange={(e) => set({ sort: e.target.value === "days_overdue" ? "" : e.target.value })}>
          {Object.entries(SORT_LABELS).map(([v, l]) => (
            <option key={v} value={v}>
              {l}
            </option>
          ))}
        </NativeSelect>
      ) : null}
    </div>
  );
}
