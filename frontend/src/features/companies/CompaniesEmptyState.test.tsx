import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CompaniesPage } from "./CompaniesPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const company = {
  id: "c1", internal_code: "101", legal_name: "Empresa Fictícia Alfa Comércio Ltda", trade_name: null, cnpj: "00000001000101",
  city: null, state: null, state_registration: null, municipal_registration: null, service_start_date: "2025-01-01",
  service_end_date: null, active: true, payroll_type: "PAYROLL", factor_r: null,
  responsibles: { fiscal: null, accounting: null, payroll: null, declarations: null }, notes: null,
  current_tax_regime: "LUCRO_PRESUMIDO", current_regime_group: "NORMAL", active_modules: [], version: 1,
};

describe("Estado vazio de empresas", () => {
  it("oferece cadastrar, importar planilha e carregar demonstração; o CTA carrega as empresas", async () => {
    let seeded = false;
    mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/companies") {
        return jsonResponse(seeded ? { items: [company], total: 1 } : { items: [], total: 0 });
      }
      if (method === "POST" && url.pathname === "/api/v1/demo/seed") {
        seeded = true;
        return jsonResponse({ companies: 20 });
      }
      return undefined;
    });
    const user = userEvent.setup();
    renderWithProviders(<CompaniesPage />, { route: "/companies", path: "/companies" });

    expect(await screen.findByRole("heading", { name: "Nenhuma empresa cadastrada" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cadastrar empresa" })).toBeEnabled();
    expect(screen.getByRole("link", { name: "Importar planilha" })).toHaveAttribute("href", "/imports");

    await user.click(screen.getByRole("button", { name: "Carregar dados de demonstração" }));

    await waitFor(() => {
      expect(screen.getByText("Empresa Fictícia Alfa Comércio Ltda")).toBeInTheDocument();
    });
    expect(screen.getByText("Dados de demonstração carregados")).toBeInTheDocument();
  });

  it("com filtros ativos mostra mensagem de filtro, não o estado vazio", async () => {
    mockFetchRoutes(({ method, url }) => (method === "GET" && url.pathname === "/api/v1/companies" ? jsonResponse({ items: [], total: 0 }) : undefined));
    renderWithProviders(<CompaniesPage />, { route: "/companies?tax_regime=LUCRO_REAL", path: "/companies" });
    expect(await screen.findByText("Nenhuma empresa encontrada para os filtros selecionados.")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Nenhuma empresa cadastrada" })).not.toBeInTheDocument();
  });
});
