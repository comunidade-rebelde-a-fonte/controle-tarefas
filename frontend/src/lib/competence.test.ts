import { describe, expect, it } from "vitest";
import { addMonths, compareCompetence, displayCompetence, isCompetenceKey, nextCompetence, parseCompetence, prevCompetence } from "./competence";

describe("competence", () => {
  it.each([
    ["2026-12", "2027-01", "2026-11"],
    ["2027-01", "2027-02", "2026-12"],
    ["2026-06", "2026-07", "2026-05"],
  ])("navega a partir de %s (virada de ano)", (key, next, prev) => {
    expect(nextCompetence(key)).toBe(next);
    expect(prevCompetence(key)).toBe(prev);
  });

  it("desloca vários meses", () => {
    expect(addMonths("2026-11", 14)).toBe("2028-01");
    expect(addMonths("2026-01", -13)).toBe("2024-12");
  });

  it("exibe MM/AAAA e valida o formato", () => {
    expect(displayCompetence("2026-09")).toBe("09/2026");
    expect(parseCompetence("2026-13")).toBeNull();
    expect(isCompetenceKey("2026-9")).toBe(false);
    expect(isCompetenceKey("2026-09")).toBe(true);
    expect(compareCompetence("2026-09", "2026-10")).toBe(-1);
  });
});
