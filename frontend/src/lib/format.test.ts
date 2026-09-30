import { describe, expect, it } from "vitest";
import { formatCompetence, formatDate, formatPercent } from "./format";

describe("format", () => {
  it("formata data civil sem deslocar fuso", () => {
    expect(formatDate("2026-01-01")).toBe("01/01/2026");
    expect(formatDate(null)).toBe("");
  });
  it("formata competência AAAA-MM como MM/AAAA", () => {
    expect(formatCompetence("2026-09")).toBe("09/2026");
    expect(formatCompetence("2025-12")).toBe("12/2025");
  });
  it("formata percentual", () => {
    expect(formatPercent(0.5)).toBe("50%");
    expect(formatPercent(null)).toBe("—");
  });
});
