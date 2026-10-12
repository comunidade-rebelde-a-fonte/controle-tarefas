import type { paths } from "@/types/api";

/**
 * Os filtros de empresa do Dashboard na URL (PRD §24.4-§24.5; TASK-024, D-038): agrupamento,
 * regime e responsável. O Dashboard os leva no drill-down, e a carteira do Fator R, a fila, os
 * documentos e a Conciliação por empresa os aplicam (com o responsável do módulo de cada uma).
 */
export type CompanyFilter = Pick<NonNullable<paths["/dashboard"]["get"]["parameters"]["query"]>, "regime_group" | "tax_regime" | "responsible_id">;

export const COMPANY_FILTER_KEYS = ["regime_group", "tax_regime", "responsible_id"] as const;

const GROUPS = ["NORMAL", "SIMPLES"] as const;
const REGIMES = ["SIMPLES_NACIONAL", "LUCRO_PRESUMIDO", "LUCRO_REAL", "LUCRO_ARBITRADO", "IMUNE_ISENTA"] as const;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function pick<T extends string>(v: string | null, allowed: readonly T[]): T | undefined {
  return v !== null && (allowed as readonly string[]).includes(v) ? (v as T) : undefined;
}

/** Os filtros válidos da URL (os inválidos são ignorados). */
export function companyFilterFrom(params: URLSearchParams): CompanyFilter {
  const f: CompanyFilter = {};
  const group = pick(params.get("regime_group"), GROUPS);
  if (group) f.regime_group = group;
  const regime = pick(params.get("tax_regime"), REGIMES);
  if (regime) f.tax_regime = regime;
  const responsible = params.get("responsible_id");
  if (responsible && UUID.test(responsible)) f.responsible_id = responsible;
  return f;
}

export function hasCompanyFilter(f: CompanyFilter): boolean {
  return f.regime_group !== undefined || f.tax_regime !== undefined || f.responsible_id !== undefined;
}
