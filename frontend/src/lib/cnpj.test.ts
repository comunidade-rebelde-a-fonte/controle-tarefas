import { describe, expect, it } from "vitest";
import { formatCnpj, looksLikeCnpj, maskCnpjInput, sanitizeCnpj } from "./cnpj";

describe("cnpj", () => {
  it.each([
    ["00.000.001/0001-01", "00000001000101"],
    [" 00.000.001/0001-01 ", "00000001000101"],
    ["00 000 001 0001 01", "00000001000101"],
    ["00000001000101", "00000001000101"],
  ])("sanitiza %s", (input, expected) => {
    expect(sanitizeCnpj(input)).toBe(expected);
  });

  it("formata 14 dígitos e mantém valores inválidos", () => {
    expect(formatCnpj("00000001000101")).toBe("00.000.001/0001-01");
    expect(formatCnpj("123")).toBe("123");
  });

  it("aplica máscara progressiva", () => {
    expect(maskCnpjInput("0000000")).toBe("00.000.00");
    expect(maskCnpjInput("00000001000101999")).toBe("00.000.001/0001-01");
  });

  it("reconhece termo de busca com cara de CNPJ", () => {
    expect(looksLikeCnpj("00.000.001")).toBe(true);
    expect(looksLikeCnpj("Alfa")).toBe(false);
  });
});
