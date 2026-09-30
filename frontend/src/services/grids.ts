import { keepPreviousData, type QueryClient, useQuery } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";
import type { GridFilters } from "@/lib/useGridFilters";
import type { paths } from "@/types/api";
import { obligationKeys, type StatusChangeResult } from "./obligations";

export type ModuleGrid = Schemas["ModuleGrid"];
export type GridRow = Schemas["GridRow"];
export type GridCell = Schemas["GridCell"];
export type GridColumn = Schemas["GridColumn"];

export type FiscalGroup = "normal" | "simples";
export type FiscalGridQuery = NonNullable<paths["/modules/fiscal/grid"]["get"]["parameters"]["query"]>;

/** Filtros derivados da URL para a query da grade (a busca livre fica no cliente). */
export function gridFilterQuery(f: GridFilters): Omit<FiscalGridQuery, "competence" | "group"> {
  const q: Omit<FiscalGridQuery, "competence" | "group"> = {};
  if (f.taxRegime) q.tax_regime = f.taxRegime;
  if (f.responsibleId) q.responsible_id = f.responsibleId;
  if (f.status) q.status = f.status;
  if (f.status && f.obligationTypeId) q.obligation_type_id = f.obligationTypeId;
  if (f.closingStatus) q.closing_status = f.closingStatus;
  if (f.periodState) q.period_state = f.periodState;
  if (f.note) q.note = f.note;
  if (f.overdue) q.overdue = true;
  if (f.pending) q.pending = true;
  if (f.notStarted) q.not_started = true;
  if (f.readyToClose) q.ready_to_close = true;
  return q;
}

/** Grades disponíveis (segmento da rota /modules/{module}/grid). */
export type GridModule = "fiscal" | "declarations" | "payroll" | "accounting" | "dctfweb";

/** Query das grades: competência, filtros derivados e, no Fiscal, o agrupamento. */
export type GridQuery = Omit<FiscalGridQuery, "group"> & { group?: FiscalGroup };

/** Chaves das grades: prefixo `obligations`, invalidado por alterações e conflitos de versão. */
export const gridKeys = {
  all: [...obligationKeys.all, "grid"] as const,
  grid: (module: GridModule, query: GridQuery | null) => [...obligationKeys.all, "grid", module, query] as const,
};

type Fetcher = (query: GridQuery, signal: AbortSignal) => Promise<ModuleGrid>;

const fetchers: Record<GridModule, Fetcher> = {
  fiscal: (query, signal) => api.get("/modules/fiscal/grid", { query: { ...query, group: query.group ?? "normal" }, signal }),
  declarations: (query, signal) => api.get("/modules/declarations/grid", { query, signal }),
  payroll: (query, signal) => api.get("/modules/payroll/grid", { query, signal }),
  accounting: (query, signal) => api.get("/modules/accounting/grid", { query, signal }),
  dctfweb: (query, signal) => api.get("/modules/dctfweb/grid", { query, signal }),
};

/** GET /modules/{module}/grid [PRD §41.6]; `query` null enquanto a competência carrega. */
export function useModuleGrid(module: GridModule, query: GridQuery | null) {
  return useQuery({
    queryKey: gridKeys.grid(module, query),
    queryFn: ({ signal }) => fetchers[module](query ?? { competence: "" }, signal),
    enabled: query !== null,
    placeholderData: keepPreviousData,
  });
}

/**
 * Aplica a resposta de uma alteração de status às grades em cache: a célula e o agregado da
 * linha vêm prontos da API (progresso, atraso, prazo e "pronto para fechar"), sem recálculo.
 */
export function applyStatusChange(qc: QueryClient, res: StatusChangeResult) {
  const code = res.instance.obligation_type.code;
  qc.setQueriesData<ModuleGrid | undefined>({ queryKey: gridKeys.all }, (grid) => {
    if (!grid) return grid;
    const index = grid.rows.findIndex((row) => row.cells[code]?.id === res.instance.id);
    const row = grid.rows[index];
    const cell = row?.cells[code];
    if (!row || !cell || cell.version > res.instance.version) return grid;
    const rows = [...grid.rows];
    const updated = { ...row, cells: { ...row.cells, [code]: { ...cell, ...res.instance } } };
    // O agregado devolvido é o do módulo da instância; só vale para a linha se for o mesmo módulo
    // do período da grade (na DCTFWeb, o eSocial é da Folha).
    rows[index] =
      res.row.module === grid.period_module
        ? {
            ...updated,
            progress: res.row.progress,
            overdue: res.row.overdue,
            nearest_deadline: res.row.nearest_deadline,
            // Status só muda com o módulo aberto (fechado → 423), então o indicador é o da API.
            ready_to_close: grid.closable && res.row.progress.ready_to_close,
          }
        : updated;
    return { ...grid, rows };
  });
  // Indicadores de outras células (blocked_by no Contábil, dependências da DCTFWeb) e filtros
  // derivados dependem da alteração: as grades são recarregadas em segundo plano.
  void qc.invalidateQueries({ queryKey: gridKeys.all });
}
