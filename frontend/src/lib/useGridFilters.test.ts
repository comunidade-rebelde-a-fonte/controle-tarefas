import { describe, expect, it } from "vitest";
import {
  EMPTY_GRID_FILTERS,
  filterBySearch,
  type GridFilters,
  hasActiveGridFilters,
  matchesSearch,
  parseGridFilters,
  toGridQuery,
  writeGridFilters,
} from "./useGridFilters";

const USER = "7f3c2a10-1b2c-4d5e-8f90-1234567890ab";
const TYPE = "0b1c2d3e-4f50-4617-a8b9-c0d1e2f3a4b5";

const full: GridFilters = {
  q: "Alfa Comércio",
  taxRegime: "SIMPLES_NACIONAL",
  group: "simples",
  responsibleId: USER,
  status: "MISSING_DOCUMENTS",
  obligationTypeId: TYPE,
  closingStatus: "CHECKED",
  periodState: "OPEN",
  note: "without",
  overdue: true,
  pending: true,
  notStarted: false,
  readyToClose: true,
};

describe("serialização dos filtros da grade", () => {
  it("ida e volta pela URL preserva todos os filtros", () => {
    const params = writeGridFilters(new URLSearchParams(), full);
    expect(parseGridFilters(new URLSearchParams(params.toString()))).toEqual(full);
  });

  it("usa os nomes de parâmetro da API e omite vazios e falsos", () => {
    const params = writeGridFilters(new URLSearchParams(), full);
    expect(Object.fromEntries(params)).toEqual({
      q: "Alfa Comércio",
      tax_regime: "SIMPLES_NACIONAL",
      group: "simples",
      responsible_id: USER,
      status: "MISSING_DOCUMENTS",
      obligation_type_id: TYPE,
      closing_status: "CHECKED",
      period_state: "OPEN",
      note: "without",
      overdue: "true",
      pending: "true",
      ready_to_close: "true",
    });
  });

  it("preserva parâmetros que não são filtros (competência) e remove os limpos", () => {
    const start = new URLSearchParams("competence=2026-09&tax_regime=LUCRO_REAL&overdue=true&q=abc");
    const next = writeGridFilters(start, { ...EMPTY_GRID_FILTERS, pending: true });
    expect(next.toString()).toBe("competence=2026-09&pending=true");
  });

  it("ignora valores inválidos vindos da URL", () => {
    const parsed = parseGridFilters(
      new URLSearchParams("tax_regime=MEI&group=todos&responsible_id=123&status=drop table&period_state=open&note=x&overdue=1&pending=false"),
    );
    expect(parsed).toEqual(EMPTY_GRID_FILTERS);
  });

  it("URL sem filtros = filtros vazios", () => {
    expect(parseGridFilters(new URLSearchParams("competence=2026-09"))).toEqual(EMPTY_GRID_FILTERS);
  });
});

describe("toGridQuery", () => {
  it("manda para a API só os filtros derivados (sem a busca livre)", () => {
    expect(toGridQuery(full)).toEqual({
      tax_regime: "SIMPLES_NACIONAL",
      group: "simples",
      responsible_id: USER,
      status: "MISSING_DOCUMENTS",
      obligation_type_id: TYPE,
      closing_status: "CHECKED",
      period_state: "OPEN",
      note: "without",
      overdue: true,
      pending: true,
      ready_to_close: true,
    });
    expect(toGridQuery(EMPTY_GRID_FILTERS)).toEqual({});
  });
});

describe("hasActiveGridFilters", () => {
  it("o agrupamento é visão, não filtro", () => {
    expect(hasActiveGridFilters({ ...EMPTY_GRID_FILTERS, group: "normal" })).toBe(false);
    expect(hasActiveGridFilters({ ...EMPTY_GRID_FILTERS, q: "x" })).toBe(true);
    expect(hasActiveGridFilters({ ...EMPTY_GRID_FILTERS, notStarted: true })).toBe(true);
  });
});

describe("busca livre no cliente", () => {
  const alfa = { legal_name: "Açúcar Alfa Comércio Ltda", trade_name: "Doce Alfa", internal_code: "101", cnpj: "00000001000101" };
  const beta = { legal_name: "Beta Serviços Ltda", trade_name: null, internal_code: "205", cnpj: "00000002000293" };

  it.each([
    ["razão social sem acento e maiúsculas", "ACUCAR ALFA"],
    ["nome fantasia", "doce"],
    ["código", "101"],
    ["CNPJ formatado", "00.000.001/0001-01"],
    ["CNPJ sem formatação", "00000001000101"],
    ["parte do CNPJ", "0001-01"],
  ])("encontra por %s", (_, q) => {
    expect(matchesSearch(alfa, q)).toBe(true);
    expect(matchesSearch(beta, q)).toBe(false);
  });

  it("termo vazio encontra tudo e a lista é filtrada", () => {
    expect(matchesSearch(beta, "  ")).toBe(true);
    const rows = [
      { id: 1, company: alfa },
      { id: 2, company: beta },
    ];
    expect(filterBySearch(rows, "serviços", (r) => r.company).map((r) => r.id)).toEqual([2]);
    expect(filterBySearch(rows, "", (r) => r.company).map((r) => r.id)).toEqual([1, 2]);
  });
});
