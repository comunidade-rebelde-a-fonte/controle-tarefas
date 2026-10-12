import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CompaniesPage } from "./CompaniesPage";

const alfa = {
  id: "c1",
  internal_code: "101",
  legal_name: "Empresa Fictícia Alfa Comércio Ltda",
  trade_name: "Alfa Comércio",
  cnpj: "00000001000101",
  city: "Curitiba",
  state: "PR",
  state_registration: null,
  municipal_registration: null,
  service_start_date: "2025-01-01",
  service_end_date: null,
  active: true,
  payroll_type: "PAYROLL",
  factor_r: null,
  responsibles: { fiscal: { id: "u1", name: "Ana Souza" }, accounting: null, payroll: null, declarations: null },
  notes: null,
  current_tax_regime: "LUCRO_PRESUMIDO",
  current_regime_group: "NORMAL",
  active_modules: ["FISCAL", "PAYROLL"],
  version: 1,
};

function mockCompanies() {
  const fetchFn = vi.fn<typeof fetch>().mockImplementation(() =>
    Promise.resolve(new Response(JSON.stringify({ items: [alfa], total: 1 }), { status: 200 })),
  );
  vi.stubGlobal("fetch", fetchFn);
  return fetchFn;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("CompaniesPage", () => {
  it("TASK-025: os indicadores compactos da competência atual (Fator R, Conciliação e alertas críticos)", async () => {
    mockFetchRoutes(({ method, url }) => {
      if (method !== "GET") return undefined;
      if (url.pathname === "/api/v1/companies") return jsonResponse({ items: [alfa, { ...alfa, id: "c2", internal_code: "102", legal_name: "Beta Ltda" }], total: 2 });
      if (url.pathname === "/api/v1/competencies/latest") return jsonResponse({ competence: "2026-09", current: "2026-09" });
      if (url.pathname === "/api/v1/company-indicators") {
        expect(url.searchParams.get("competence")).toBe("2026-09");
        return jsonResponse({
          competence: "2026-09",
          items: [
            { company_id: "c1", factor_r: { semaforo: "vermelho", acao: "reuniao_correcao" }, reconciliation: { state: "pronta", blocks_accounting: true }, overdue: 2, credentials_expired: 1 },
          ],
        });
      }
      return undefined;
    });
    renderWithProviders(<CompaniesPage />, { route: "/companies", path: "/companies" });
    const row = (await screen.findByText("Empresa Fictícia Alfa Comércio Ltda")).closest("tr");
    if (!row) throw new Error("linha não encontrada");
    const indicators = await within(row).findByRole("list", { name: "Indicadores" });
    expect(within(indicators).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
      "Fator R vermelho",
      "Conciliação pronta · trava o Contábil",
      "2 obrigações atrasadas",
      "Credencial vencida",
    ]);
    const other = screen.getByText("Beta Ltda").closest("tr");
    expect(other && within(other).queryByRole("list", { name: "Indicadores" })).toBeNull();
  });

  it("lista as colunas principais e copia o CNPJ sem máscara", async () => {
    mockCompanies();
    const user = userEvent.setup();
    renderWithProviders(<CompaniesPage />, { route: "/companies", path: "/companies" });

    const row = (await screen.findByText("Empresa Fictícia Alfa Comércio Ltda")).closest("tr");
    if (!row) throw new Error("linha não encontrada");
    const cells = within(row);
    expect(cells.getByText("00.000.001/0001-01")).toBeInTheDocument();
    expect(cells.getByText("Lucro Presumido")).toBeInTheDocument();
    expect(cells.getByText("Curitiba/PR")).toBeInTheDocument();
    expect(cells.getByLabelText("Fiscal")).toBeInTheDocument();
    expect(cells.getByText("Ana Souza")).toBeInTheDocument();
    expect(cells.getByText("Ativa")).toBeInTheDocument();

    await user.click(cells.getByRole("button", { name: "Copiar CNPJ" }));
    expect(await navigator.clipboard.readText()).toBe("00000001000101");
  });

  it.each(["00.000.001/0001-01", "00000001000101"])("envia a busca por CNPJ %s para a API", async (term) => {
    const fetchFn = mockCompanies();
    const user = userEvent.setup();
    renderWithProviders(<CompaniesPage />, { route: "/companies", path: "/companies" });
    await screen.findByText("Empresa Fictícia Alfa Comércio Ltda");

    await user.type(screen.getByRole("searchbox", { name: "Buscar empresa" }), term);

    await waitFor(() => {
      const urls = requestedUrls(fetchFn);
      expect(urls.some((u) => new URL(u, "http://x").searchParams.get("q") === term)).toBe(true);
    });
    expect(screen.getByText("Empresa Fictícia Alfa Comércio Ltda")).toBeInTheDocument();
  });

  it("aplica o filtro padrão de empresas ativas e permite ver todas", async () => {
    const fetchFn = mockCompanies();
    const user = userEvent.setup();
    renderWithProviders(<CompaniesPage />, { route: "/companies", path: "/companies" });
    await screen.findByText("Empresa Fictícia Alfa Comércio Ltda");
    expect(requestedUrls(fetchFn)[0]).toContain("active=true");

    await user.selectOptions(screen.getByRole("combobox", { name: "Situação" }), "all");
    await waitFor(() => {
      expect(requestedUrls(fetchFn).some((u) => !u.includes("active="))).toBe(true);
    });
  });
});
