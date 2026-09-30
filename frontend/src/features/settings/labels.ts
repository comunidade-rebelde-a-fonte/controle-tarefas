import type { Schemas } from "@/lib/api";

type Rule = Pick<Schemas["DueDateRule"], "month_offset" | "base_day" | "day_type" | "non_business_day_adjustment">;

export const DAY_TYPE_LABELS: Record<Rule["day_type"], string> = { CALENDAR: "Dia corrido", BUSINESS: "Dia útil" };

export const ADJUSTMENT_LABELS: Record<Rule["non_business_day_adjustment"], string> = {
  PREVIOUS: "Antecipar para o dia útil anterior",
  NEXT: "Adiar para o próximo dia útil",
  NONE: "Sem ajuste",
};

export const STATUS_SET_LABELS: Record<string, string> = {
  FISCAL_CLOSING: "Fiscal — Fechamento",
  FISCAL_ITEM: "Fiscal — Controles",
  DECLARATION: "Declarações",
  ACCOUNTING: "Contábil",
  PAYROLL: "Folha",
};

function monthText(offset: number): string {
  if (offset === 0) return "do mês da competência";
  if (offset === 1) return "do mês seguinte";
  return `de ${String(offset)} meses após a competência`;
}

/** Descrição textual da regra (somente exibição; o cálculo é feito pela API). */
export function describeRule(r: Rule): string {
  if (!Number.isFinite(r.base_day) || !Number.isFinite(r.month_offset)) return "—";
  const day = r.day_type === "BUSINESS" ? `${String(r.base_day)}º dia útil` : `Dia ${String(r.base_day)}`;
  const adjust =
    r.day_type === "BUSINESS"
      ? ""
      : r.non_business_day_adjustment === "PREVIOUS"
        ? "; se não for dia útil, antecipa"
        : r.non_business_day_adjustment === "NEXT"
          ? "; se não for dia útil, adia"
          : "; sem ajuste para dia não útil";
  return `${day} ${monthText(r.month_offset)}${adjust}.`;
}
