import { Search, X } from "lucide-react";
import { type ReactNode, useEffect, useEffectEvent, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input, NativeSelect } from "@/components/ui/input";
import { TAX_REGIME_LABELS, TAX_REGIMES } from "@/lib/labels";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { type GridFilters as Filters, useGridFilters } from "@/lib/useGridFilters";
import { useUsers } from "@/services/users";

export interface Option {
  value: string;
  label: string;
}

export interface GridFiltersProps {
  /** Status possíveis nas colunas da grade. */
  statusOptions: Option[];
  /** Colunas (tipos de obrigação) para restringir o filtro de status. */
  columnOptions?: Option[];
  /** Status da coluna Fechamento (Fiscal); sem opções, o filtro não aparece. */
  closingStatusOptions?: Option[];
  /** Grades de um único regime escondem o filtro de regime. */
  showRegime?: boolean;
  /** Total de empresas exibidas (anunciado aos leitores de tela). */
  resultCount?: number;
}

const FLAGS: { key: "overdue" | "pending" | "notStarted" | "readyToClose"; label: string }[] = [
  { key: "overdue", label: "Somente atrasadas" },
  { key: "pending", label: "Somente pendentes" },
  { key: "notStarted", label: "Somente não iniciadas" },
  { key: "readyToClose", label: "Somente prontas para fechar" },
];

function FilterSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (v: string) => void; children: ReactNode }) {
  return (
    <NativeSelect aria-label={label} className="w-44" value={value} onChange={(e) => onChange(e.target.value)}>
      {children}
    </NativeSelect>
  );
}

/**
 * Barra de filtros das grades [PRD §26.4-§26.5], com o estado na URL. A busca livre é aplicada
 * no cliente (use `filterBySearch`); os demais filtros vão para a API (`apiQuery`).
 */
export function GridFilters({ statusOptions, columnOptions, closingStatusOptions, showRegime = true, resultCount }: GridFiltersProps) {
  const { filters, setFilter, clear, active } = useGridFilters();
  const users = useUsers(true);
  const [search, setSearch] = useState(filters.q);
  const debounced = useDebouncedValue(search, 250);

  // Leva o termo para a URL quando a digitação estabiliza.
  const syncSearch = useEffectEvent((value: string) => {
    if (value !== filters.q) setFilter("q", value);
  });
  useEffect(() => {
    syncSearch(debounced);
  }, [debounced]);

  const set = <K extends keyof Filters>(key: K) => (value: Filters[K]) => setFilter(key, value);

  return (
    <div className="flex flex-wrap items-center gap-2" role="search" aria-label="Filtros da grade">
      <div className="relative w-80">
        <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
        <Input
          type="search"
          aria-label="Buscar empresa"
          placeholder="Nome, código ou CNPJ (com ou sem máscara)"
          className="pl-8"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>
      {showRegime && (
        <FilterSelect label="Regime" value={filters.taxRegime} onChange={(v) => setFilter("taxRegime", v as Filters["taxRegime"])}>
          <option value="">Todos os regimes</option>
          {TAX_REGIMES.map((r) => (
            <option key={r} value={r}>
              {TAX_REGIME_LABELS[r]}
            </option>
          ))}
        </FilterSelect>
      )}
      <FilterSelect label="Responsável" value={filters.responsibleId} onChange={set("responsibleId")}>
        <option value="">Todos os responsáveis</option>
        {(users.data?.items ?? []).map((u) => (
          <option key={u.id} value={u.id}>
            {u.name}
          </option>
        ))}
      </FilterSelect>
      <FilterSelect label="Status da obrigação" value={filters.status} onChange={set("status")}>
        <option value="">Qualquer status</option>
        {statusOptions.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </FilterSelect>
      {columnOptions && filters.status && (
        <FilterSelect label="Coluna do status" value={filters.obligationTypeId} onChange={set("obligationTypeId")}>
          <option value="">Em qualquer coluna</option>
          {columnOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </FilterSelect>
      )}
      {closingStatusOptions && closingStatusOptions.length > 0 && (
        <FilterSelect label="Status de fechamento" value={filters.closingStatus} onChange={set("closingStatus")}>
          <option value="">Qualquer fechamento</option>
          {closingStatusOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </FilterSelect>
      )}
      <FilterSelect label="Módulo aberto ou fechado" value={filters.periodState} onChange={(v) => setFilter("periodState", v as Filters["periodState"])}>
        <option value="">Aberto ou fechado</option>
        <option value="OPEN">Aberto</option>
        <option value="CLOSED">Fechado</option>
      </FilterSelect>
      <FilterSelect label="Observação" value={filters.note} onChange={(v) => setFilter("note", v as Filters["note"])}>
        <option value="">Com ou sem observação</option>
        <option value="with">Com observação</option>
        <option value="without">Sem observação</option>
      </FilterSelect>
      <fieldset className="flex flex-wrap items-center gap-3">
        <legend className="sr-only">Filtros rápidos</legend>
        {FLAGS.map(({ key, label }) => (
          <label key={key} className="flex items-center gap-1.5 text-sm">
            <input type="checkbox" className="size-4 accent-primary" checked={filters[key]} onChange={(e) => setFilter(key, e.target.checked)} />
            {label}
          </label>
        ))}
      </fieldset>
      <Button
        variant="ghost"
        size="sm"
        disabled={!active}
        onClick={() => {
          setSearch("");
          clear();
        }}
      >
        <X aria-hidden="true" />
        Limpar filtros
      </Button>
      {resultCount !== undefined && (
        <p className="ml-auto text-sm text-muted-foreground" aria-live="polite">
          {resultCount} empresa{resultCount === 1 ? "" : "s"}
        </p>
      )}
    </div>
  );
}
