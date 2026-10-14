import { describe, expect, it } from "vitest";
import { formatDecimal, formatMoney, formatRate, isPositive, moneyToInput, parseMoneyInput, parsePercentInput } from "./decimal";

describe("formatação decimal sem float (half-even, como a origem)", () => {
  it("formata reais com milhar e centavos", () => {
    expect(formatMoney("43740.00000000")).toBe("R$ 43.740,00");
    expect(formatMoney("600000.00")).toBe("R$ 600.000,00");
    expect(formatMoney("0")).toBe("R$ 0,00");
    expect(formatMoney("1234567890123.5")).toBe("R$ 1.234.567.890.123,50");
    expect(formatMoney("-12000.0000")).toBe("-R$ 12.000,00");
    expect(formatMoney(null)).toBe("—");
  });

  it("arredonda half-even, também em valores que o float erraria", () => {
    expect(formatMoney("2.675")).toBe("R$ 2,68");
    expect(formatMoney("2.665")).toBe("R$ 2,66");
    expect(formatMoney("2.6650000001")).toBe("R$ 2,67");
    expect(formatMoney("0.005")).toBe("R$ 0,00");
    expect(formatMoney("999.995")).toBe("R$ 1.000,00");
    expect(formatMoney("-0.001")).toBe("R$ 0,00");
    // 1.005 em float é 1,00499999…: o arredondamento por string acerta.
    expect(formatMoney("1.005")).toBe("R$ 1,00");
    expect(formatMoney("1.015")).toBe("R$ 1,02");
  });

  it("formata frações como percentual", () => {
    expect(formatRate("0.2799")).toBe("27,99%");
    expect(formatRate("0.2")).toBe("20,00%");
    expect(formatRate("0.300000")).toBe("30,00%");
    expect(formatRate("0.279949999")).toBe("27,99%");
    expect(formatRate("0.27995")).toBe("28,00%");
    expect(formatRate("0.105600")).toBe("10,56%");
    expect(formatRate("1")).toBe("100,00%");
    expect(formatRate(null)).toBe("—");
  });

  it("converte entre a API e o campo de edição", () => {
    expect(moneyToInput("1234.50")).toBe("1.234,50");
    expect(moneyToInput("0.00")).toBe("0,00");
    expect(parseMoneyInput("1.234,56")).toBe("1234.56");
    expect(parseMoneyInput("1234,5")).toBe("1234.50");
    expect(parseMoneyInput("1234.56")).toBe("1234.56");
    expect(parseMoneyInput("R$ 10")).toBe("10.00");
    expect(parseMoneyInput("")).toBe("0.00");
    expect(parseMoneyInput("007")).toBe("7.00");
    for (const bad of ["-1", "1,234", "1.2.3", "abc", "1e3", "12,345"]) expect(parseMoneyInput(bad)).toBeNull();
  });

  it("converte percentual digitado em fração, sem float (simulador)", () => {
    expect(parsePercentInput("27,5")).toBe("0.275");
    expect(parsePercentInput("11")).toBe("0.11");
    expect(parsePercentInput("100")).toBe("1");
    expect(parsePercentInput("0")).toBe("0");
    expect(parsePercentInput("0,5%")).toBe("0.005");
    expect(parsePercentInput(" 30 % ")).toBe("0.3");
    expect(parsePercentInput("28.1234")).toBe("0.281234");
    expect(parsePercentInput("099")).toBe("0.99");
    expect(parsePercentInput("100,0")).toBe("1");
    for (const bad of ["", "-1", "100,01", "101", "abc", "1e2", "27,12345", "27,5,1"]) expect(parsePercentInput(bad), bad).toBeNull();
  });

  it("formata decimal com casas fixas (custo do LLM)", () => {
    expect(formatDecimal("0.0000125", 4)).toBe("0,0000");
    expect(formatDecimal("0.00015", 4)).toBe("0,0002");
    expect(formatDecimal("1234.5", 4)).toBe("1.234,5000");
    expect(formatDecimal(null, 4)).toBe("—");
    expect(formatDecimal("1.25E-7", 4)).toBe("1.25E-7");
  });

  it("isPositive sem float", () => {
    expect(isPositive("0.0001")).toBe(true);
    expect(isPositive("0.0000")).toBe(false);
    expect(isPositive("-5")).toBe(false);
    expect(isPositive(null)).toBe(false);
  });
});
