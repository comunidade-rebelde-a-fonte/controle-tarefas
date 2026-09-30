import { useSearchParams } from "react-router";
import { looksLikeCnpj, sanitizeCnpj } from "@/lib/cnpj";
import { TAX_REGIMES, type TaxRegime } from "@/lib/labels";

/**
 * Filtros das grades [PRD §26.4-§26.5], serializados na URL [AGENTS §8.5]. Os parâmetros da
 * URL têm os mesmos nomes da query das grades (T-M3-01). A busca livre (`q`) é aplicada no
 * cliente; os demais são filtros derivados, calculados pela API.
 */
export interface GridFilters {
  /** Busca livre: razão social, nome fantasia, código e CNPJ com ou sem máscara (cliente). */
  q: string;
  taxRegime: TaxRegime | "";
  /** Agrupamento Fiscal. */
  group: "normal" | "simples" | "";
  responsibleId: string;
  /** Código de status de obrigação: linhas com ao menos uma célula aplicável nesse status. */
  status: string;
  /** Restringe o filtro de status a uma coluna (tipo de obrigação). */
  obligationTypeId: string;
  /** Status da coluna Fechamento (Fiscal). */
  closingStatus: string;
  /** Período do módulo aberto ou fechado. */
  periodState: "OPEN" | "CLOSED" | "";
  note: "with" | "without" | "";
  overdue: boolean;
  pending: boolean;
  notStarted: boolean;
  readyToClose: boolean;
}

export const EMPTY_GRID_FILTERS: GridFilters = {
  q: "",
  taxRegime: "",
  group: "",
  responsibleId: "",
  status: "",
  obligationTypeId: "",
  closingStatus: "",
  periodState: "",
  note: "",
  overdue: false,
  pending: false,
  notStarted: false,
  readyToClose: false,
};

type TextKey = "q" | "taxRegime" | "group" | "responsibleId" | "status" | "obligationTypeId" | "closingStatus" | "periodState" | "note";
type FlagKey = "overdue" | "pending" | "notStarted" | "readyToClose";

/** Nome do parâmetro na URL e na API de cada filtro. */
export const GRID_FILTER_PARAMS: Record<keyof GridFilters, string> = {
  q: "q",
  taxRegime: "tax_regime",
  group: "group",
  responsibleId: "responsible_id",
  status: "status",
  obligationTypeId: "obligation_type_id",
  closingStatus: "closing_status",
  periodState: "period_state",
  note: "note",
  overdue: "overdue",
  pending: "pending",
  notStarted: "not_started",
  readyToClose: "ready_to_close",
};

const TEXT_KEYS: TextKey[] = ["q", "taxRegime", "group", "responsibleId", "status", "obligationTypeId", "closingStatus", "periodState", "note"];
const FLAG_KEYS: FlagKey[] = ["overdue", "pending", "notStarted", "readyToClose"];

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const STATUS_CODE = /^[A-Z][A-Z0-9_]*$/;

function oneOf<T extends string>(value: string | null, allowed: readonly T[]): T | "" {
  return value !== null && (allowed as readonly string[]).includes(value) ? (value as T) : "";
}

/** Lê os filtros da URL; valores inválidos são ignorados (não quebram a tela). */
export function parseGridFilters(params: URLSearchParams): GridFilters {
  const get = (k: keyof GridFilters) => params.get(GRID_FILTER_PARAMS[k]);
  const uuid = (k: keyof GridFilters) => {
    const v = get(k) ?? "";
    return UUID.test(v) ? v : "";
  };
  const code = (k: keyof GridFilters) => {
    const v = get(k) ?? "";
    return STATUS_CODE.test(v) ? v : "";
  };
  return {
    q: get("q") ?? "",
    taxRegime: oneOf(get("taxRegime"), TAX_REGIMES),
    group: oneOf(get("group"), ["normal", "simples"] as const),
    responsibleId: uuid("responsibleId"),
    status: code("status"),
    obligationTypeId: uuid("obligationTypeId"),
    closingStatus: code("closingStatus"),
    periodState: oneOf(get("periodState"), ["OPEN", "CLOSED"] as const),
    note: oneOf(get("note"), ["with", "without"] as const),
    overdue: get("overdue") === "true",
    pending: get("pending") === "true",
    notStarted: get("notStarted") === "true",
    readyToClose: get("readyToClose") === "true",
  };
}

/**
 * Grava os filtros na URL, preservando os demais parâmetros (ex.: `competence`).
 * Vazios e booleanos falsos saem da URL.
 */
export function writeGridFilters(params: URLSearchParams, filters: GridFilters): URLSearchParams {
  const next = new URLSearchParams(params);
  for (const k of TEXT_KEYS) {
    const v = filters[k].trim();
    if (v) next.set(GRID_FILTER_PARAMS[k], k === "q" ? filters.q : v);
    else next.delete(GRID_FILTER_PARAMS[k]);
  }
  for (const k of FLAG_KEYS) {
    if (filters[k]) next.set(GRID_FILTER_PARAMS[k], "true");
    else next.delete(GRID_FILTER_PARAMS[k]);
  }
  return next;
}

/** Filtros derivados para a query da grade (sem a busca livre, que é do cliente). */
export function toGridQuery(filters: GridFilters): Record<string, string | boolean> {
  const query: Record<string, string | boolean> = {};
  for (const k of TEXT_KEYS) {
    if (k !== "q" && filters[k]) query[GRID_FILTER_PARAMS[k]] = filters[k];
  }
  for (const k of FLAG_KEYS) {
    if (filters[k]) query[GRID_FILTER_PARAMS[k]] = true;
  }
  return query;
}

/** Há filtro aplicado além do agrupamento (que é alternador de visão, não filtro)? */
export function hasActiveGridFilters(filters: GridFilters): boolean {
  return (Object.keys(filters) as (keyof GridFilters)[]).some((k) => k !== "group" && filters[k] !== EMPTY_GRID_FILTERS[k]);
}

export interface SearchableCompany {
  legal_name: string;
  trade_name?: string | null;
  internal_code: string;
  cnpj: string;
}

const normalize = (s: string) =>
  s
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .toLocaleLowerCase("pt-BR")
    .trim();

/** Busca livre [PRD §26.5]: nome, razão social, nome fantasia, código e CNPJ com ou sem máscara. */
export function matchesSearch(company: SearchableCompany, q: string): boolean {
  const term = normalize(q);
  if (!term) return true;
  if (normalize(company.internal_code).includes(term)) return true;
  if (looksLikeCnpj(q) && sanitizeCnpj(company.cnpj).includes(sanitizeCnpj(q))) return true;
  return normalize(company.legal_name).includes(term) || normalize(company.trade_name ?? "").includes(term);
}

/** Aplica a busca livre às linhas da grade. */
export function filterBySearch<T>(rows: readonly T[], q: string, company: (row: T) => SearchableCompany): T[] {
  if (!q.trim()) return [...rows];
  return rows.filter((r) => matchesSearch(company(r), q));
}

/** Filtros da grade sincronizados com a URL. */
export function useGridFilters() {
  const [params, setParams] = useSearchParams();
  const filters = parseGridFilters(params);

  const update = (patch: Partial<GridFilters>) => {
    setParams((prev) => writeGridFilters(prev, { ...parseGridFilters(prev), ...patch }), { replace: true });
  };

  /** "Limpar filtros": mantém a competência e o agrupamento (alternador de visão). */
  const clear = () => {
    setParams((prev) => writeGridFilters(prev, { ...EMPTY_GRID_FILTERS, group: parseGridFilters(prev).group }), { replace: true });
  };

  return {
    filters,
    setFilter: <K extends keyof GridFilters>(key: K, value: GridFilters[K]) => update({ [key]: value }),
    update,
    clear,
    active: hasActiveGridFilters(filters),
    /** Query com os filtros derivados, para a API da grade. */
    apiQuery: toGridQuery(filters),
  };
}
