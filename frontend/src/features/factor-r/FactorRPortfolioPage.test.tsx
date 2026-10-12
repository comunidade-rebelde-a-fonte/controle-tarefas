import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FactorRPortfolio, FactorRPortfolioLine } from "@/services/factorR";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { FactorRPortfolioPage } from "./FactorRPortfolioPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const latest = { competence: "2026-09", current: "2026-09" };

function line(code: string, patch: Partial<FactorRPortfolioLine>): FactorRPortfolioLine {
  return {
    company: { id: `id-${code}`, internal_code: code, legal_name: `Empresa ${code}`, trade_name: null, cnpj: "00100008000108" },
    pacote_comercial: null,
    honorario_mensal: "0.00",
    acao: "manter",
    acao_texto: "manter",
    status: "ok",
    motivo: null,
    semaforo: "verde",
    fator_r: "0.3323",
    anexo: "III",
    rbt12: "1089900.00",
    fs12: "362160.00",
    reforco_mensal_28: "0",
    reforco_mensal_meta: "0",
    meta_operacional: "0.300000",
    economia_12m: "84762.00000000",
    meses_faltantes: [],
    ...patch,
  };
}

const portfolio: FactorRPortfolio = {
  pa: "2026-09",
  meta_operacional: "0.300000",
  kpis: {
    monitoradas: 3,
    no_v: 1,
    no_limite: 1,
    seguras: 0,
    dados_insuficientes: 1,
    economia_em_jogo: "104580.00000000",
    honorarios_pacotes: "1650.00",
  },
  linhas: [
    line("208", {
      acao: "reuniao_correcao",
      acao_texto: "reunião de correção",
      semaforo: "vermelho",
      fator_r: "0.2",
      anexo: "V",
      pacote_comercial: "correcao",
      reforco_mensal_28: "4000.0000",
      reforco_mensal_meta: "5000.00000000",
      economia_12m: "43740.00000000",
      meses_faltantes: ["2026-05", "2026-08"],
    }),
    line("206", {
      acao: "vigiar_simular",
      acao_texto: "vigiar / simular",
      semaforo: "amarelo",
      fator_r: "0.28",
      reforco_mensal_28: "0",
      reforco_mensal_meta: "1000.00000000",
    }),
    line("210", {
      acao: "completar_lancamentos",
      acao_texto: "completar lançamentos",
      status: "dados_insuficientes",
      motivo: "rbt12_zero",
      semaforo: null,
      fator_r: null,
      anexo: null,
      rbt12: "0",
      fs12: "0",
      reforco_mensal_28: null,
      reforco_mensal_meta: null,
      economia_12m: null,
    }),
  ],
};

function mock(response: () => Response) {
  return mockFetchRoutes(({ method, url }) => {
    if (method === "GET" && url.pathname === "/api/v1/competencies/latest") return jsonResponse(latest);
    if (method === "GET" && url.pathname === "/api/v1/factor-r/portfolio") return response();
    return undefined;
  });
}

describe("FactorRPortfolioPage", () => {
  it("AC-007: mostra KPIs, ações e semáforo com texto além da cor, na ordem da API", async () => {
    const fetchFn = mock(() => jsonResponse(portfolio));
    renderWithProviders(<FactorRPortfolioPage />, { route: "/factor-r" });

    const table = await screen.findByRole("table", { name: "Carteira do Fator R — PA 09/2026" });
    const rows = within(table).getAllByRole("row").slice(1);
    const [red, yellow, insufficient] = rows;
    if (!red || !yellow || !insufficient) throw new Error("linhas da carteira");
    expect(rows.map((r) => r.getAttribute("data-semaforo"))).toEqual(["vermelho", "amarelo", "insuficiente"]);
    expect(within(red).getByText("Vermelho · abaixo de 28% (Anexo V)")).toBeInTheDocument();
    expect(within(yellow).getByText("Amarelo · no limite")).toBeInTheDocument();
    expect(within(insufficient).getByText("Dados insuficientes")).toBeInTheDocument();
    expect(within(red).getByText("reunião de correção")).toBeInTheDocument();
    expect(within(red).getByText("Faltam: 05/2026, 08/2026")).toBeInTheDocument();
    expect(within(red).getByText("20,00%")).toBeInTheDocument();
    expect(within(red).getByText("R$ 43.740,00")).toBeInTheDocument();
    expect(within(red).getByText("Correção")).toBeInTheDocument();
    expect(within(red).getByRole("link", { name: "Empresa 208" })).toHaveAttribute("href", "/companies/id-208?tab=factor-r&competence=2026-09");

    expect(screen.getByTestId("kpi-monitoradas")).toHaveTextContent("3");
    expect(screen.getByTestId("kpi-economia")).toHaveTextContent("R$ 104.580,00");
    expect(screen.getByTestId("kpi-honorarios")).toHaveTextContent("R$ 1.650,00");
    expect(screen.getByText("No limite (28% até 30%)")).toBeInTheDocument();
    expect(screen.getByRole("note")).toHaveTextContent("O PGDAS-D da Receita Federal prevalece.");
    expect(requestedUrls(fetchFn).some((u) => u.includes("/factor-r/portfolio?pa=2026-09"))).toBe(true);
  });

  it("FR-07: rotula o aumento de folha até 28% e até a meta", async () => {
    mock(() => jsonResponse(portfolio));
    renderWithProviders(<FactorRPortfolioPage />, { route: "/factor-r" });
    const [red, yellow] = within(await screen.findByRole("table")).getAllByRole("row").slice(1);
    if (!red || !yellow) throw new Error("linhas da carteira");
    expect(within(red).getByText("+ R$ 4.000,00")).toBeInTheDocument();
    expect(within(red).getByText("para chegar a 28% (Anexo III)")).toBeInTheDocument();
    expect(within(red).getByText("+ R$ 5.000,00")).toBeInTheDocument();
    expect(within(yellow).getByText("já atinge 28% (Anexo III)")).toBeInTheDocument();
    expect(within(yellow).getByText("para a meta de 30,00%")).toBeInTheDocument();
  });

  it("usa o PA da URL e mostra o estado vazio", async () => {
    const fetchFn = mock(() => jsonResponse({ ...portfolio, pa: "2025-11", linhas: [], kpis: { ...portfolio.kpis, monitoradas: 0 } }));
    renderWithProviders(<FactorRPortfolioPage />, { route: "/factor-r?pa=2025-11" });
    expect(await screen.findByText("Nenhuma empresa na carteira deste PA")).toBeInTheDocument();
    expect(requestedUrls(fetchFn).some((u) => u.includes("pa=2025-11"))).toBe(true);
  });

  it("TEST-024: o drill-down do Dashboard filtra pelo semáforo e leva os filtros de empresa à API", async () => {
    const R = "00000000-0000-4000-8000-00000000a001";
    const fetchFn = mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/competencies/latest") return jsonResponse(latest);
      if (method === "GET" && url.pathname === "/api/v1/factor-r/portfolio") return jsonResponse(portfolio);
      if (method === "GET" && url.pathname === "/api/v1/users") {
        return jsonResponse({ items: [{ id: R, name: "Bruno Lima", email: null, role: "OPERATOR", active: true, created_at: "", updated_at: "" }] });
      }
      return undefined;
    });
    renderWithProviders(<FactorRPortfolioPage />, { route: `/factor-r?pa=2026-09&semaforo=vermelho,amarelo&responsible_id=${R}` });
    const table = await screen.findByRole("table", { name: "Carteira do Fator R — PA 09/2026" });
    expect(within(table).getAllByRole("row").slice(1).map((r) => r.getAttribute("data-semaforo"))).toEqual(["vermelho", "amarelo"]);
    expect(screen.getByText("2 empresas")).toBeInTheDocument();
    const lights = screen.getByRole("group", { name: "Filtrar por semáforo" });
    expect(within(lights).getByRole("button", { name: "Vermelho (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(within(lights).getByRole("button", { name: "Amarelo (1)" })).toHaveAttribute("aria-pressed", "true");
    expect(within(lights).getByRole("button", { name: "Todas (3)" })).toHaveAttribute("aria-pressed", "false");
    expect(await screen.findByText("Responsável do Fiscal: Bruno Lima")).toBeInTheDocument();
    expect(requestedUrls(fetchFn).some((u) => u.includes("/factor-r/portfolio?") && u.includes(`responsible_id=${R}`))).toBe(true);

    await userEvent.click(within(lights).getByRole("button", { name: "Dados insuficientes (1)" }));
    expect(within(table).getAllByRole("row").slice(1).map((r) => r.getAttribute("data-semaforo"))).toEqual(["insuficiente"]);
    await userEvent.click(within(lights).getByRole("button", { name: "Todas (3)" }));
    expect(within(table).getAllByRole("row")).toHaveLength(4);
    await userEvent.click(screen.getByRole("button", { name: "Limpar filtros do Dashboard" }));
    expect(screen.queryByRole("group", { name: "Filtros do Dashboard" })).toBeNull();
  });

  it("explica a falta de tabela do Simples vigente", async () => {
    mock(() =>
      jsonResponse({ code: "SIMPLES_TABLE_NOT_IN_FORCE", message: "Não há tabela do Simples vigente para o PA 12/2017." }, 422),
    );
    renderWithProviders(<FactorRPortfolioPage />, { route: "/factor-r?pa=2017-12" });
    expect(await screen.findByRole("alert")).toHaveTextContent("Não há tabela do Simples vigente para o PA 12/2017.");
  });
});
