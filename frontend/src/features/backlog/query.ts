import type { BacklogQuery } from "@/services/dashboard";

const SORTS = ["days_overdue", "due_date", "competence", "company"] as const;
const STATES = ["open", "applicable", "done", "in_progress", "not_started", "pending", "overdue"] as const;
const MODULES = ["FISCAL", "DECLARATIONS", "PAYROLL", "ACCOUNTING"] as const;
const GROUPS = ["NORMAL", "SIMPLES"] as const;
const REGIMES = ["SIMPLES_NACIONAL", "LUCRO_PRESUMIDO", "LUCRO_REAL", "LUCRO_ARBITRADO", "IMUNE_ISENTA"] as const;

function pick<T extends string>(v: string | null, allowed: readonly T[]): T | undefined {
  return v !== null && (allowed as readonly string[]).includes(v) ? (v as T) : undefined;
}

function nonNegative(v: string | null): number | undefined {
  if (v === null || v === "") return undefined;
  const n = Number(v);
  return Number.isInteger(n) && n >= 0 ? n : undefined;
}

/** Filtros da URL → query de GET /backlog (valores inválidos são ignorados). */
export function backlogQueryFromParams(params: URLSearchParams, base: Pick<BacklogQuery, "before" | "competence">): BacklogQuery {
  const q: BacklogQuery = { ...base };
  const state = pick(params.get("state"), STATES);
  if (state) q.state = state;
  const module = pick(params.get("module"), MODULES);
  if (module) q.module = module;
  const status = params.get("status");
  if (status) q.status = status;
  const responsible = params.get("responsible_id");
  if (responsible) q.responsible_id = responsible;
  const group = pick(params.get("regime_group"), GROUPS);
  if (group) q.regime_group = group;
  const regime = pick(params.get("tax_regime"), REGIMES);
  if (regime) q.tax_regime = regime;
  const search = params.get("q");
  if (search?.trim()) q.q = search.trim();
  const min = nonNegative(params.get("overdue_min"));
  if (min !== undefined) q.overdue_min = min;
  const max = nonNegative(params.get("overdue_max"));
  if (max !== undefined) q.overdue_max = max;
  const sort = pick(params.get("sort"), SORTS);
  if (sort) q.sort = sort;
  const page = nonNegative(params.get("page"));
  if (page) q.page = page;
  return q;
}
