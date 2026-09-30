import { describe, expect, it } from "vitest";
import { EMPTY_GRID_FILTERS } from "@/lib/useGridFilters";
import { gridExportUrl, gridFilterQuery } from "./grids";

describe("gridExportUrl", () => {
  it("CA-017 leva competência, agrupamento, filtros e busca da tela para a exportação", () => {
    const filters = { ...EMPTY_GRID_FILTERS, taxRegime: "LUCRO_REAL", overdue: true, q: " Alfa " };
    const url = gridExportUrl("fiscal", { competence: "2026-10", group: "normal", ...gridFilterQuery(filters) }, filters.q);
    const [path, qs] = url.split("?");
    expect(path).toBe("/api/v1/exports/fiscal.xlsx");
    expect(Object.fromEntries(new URLSearchParams(qs))).toEqual({
      competence: "2026-10",
      group: "normal",
      tax_regime: "LUCRO_REAL",
      overdue: "true",
      q: "Alfa",
    });
  });

  it("omite busca e filtros vazios", () => {
    expect(gridExportUrl("payroll", { competence: "2026-10" }, "  ")).toBe("/api/v1/exports/payroll.xlsx?competence=2026-10");
  });
});
