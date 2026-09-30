import type { DashboardQuery } from "@/services/dashboard";

const GROUPS = ["NORMAL", "SIMPLES"] as const;
const REGIMES = ["SIMPLES_NACIONAL", "LUCRO_PRESUMIDO", "LUCRO_REAL", "LUCRO_ARBITRADO", "IMUNE_ISENTA"] as const;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function pick<T extends string>(v: string | null, allowed: readonly T[]): T | undefined {
  return v !== null && (allowed as readonly string[]).includes(v) ? (v as T) : undefined;
}

/** Filtros do dashboard na URL [PRD §24.4-§24.5] → query das APIs de gestão. */
export function dashboardQuery(competence: string, params: URLSearchParams): DashboardQuery {
  const q: DashboardQuery = { competence };
  const group = pick(params.get("regime_group"), GROUPS);
  if (group) q.regime_group = group;
  const regime = pick(params.get("tax_regime"), REGIMES);
  if (regime) q.tax_regime = regime;
  const responsible = params.get("responsible_id");
  if (responsible && UUID.test(responsible)) q.responsible_id = responsible;
  return q;
}
