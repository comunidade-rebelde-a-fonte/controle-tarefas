import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CompanyFilterChips } from "./CompanyFilterChips";

afterEach(() => {
  vi.unstubAllGlobals();
});

const R = "00000000-0000-4000-8000-00000000a001";

describe("CompanyFilterChips", () => {
  it("mostra os filtros do Dashboard com o responsável do módulo e os limpa", async () => {
    mockFetchRoutes(({ method, url }) =>
      method === "GET" && url.pathname === "/api/v1/users"
        ? jsonResponse({ items: [{ id: R, name: "Bruno Lima", email: null, role: "OPERATOR", active: true, created_at: "", updated_at: "" }] })
        : undefined,
    );
    const { router } = renderWithProviders(<CompanyFilterChips module="Contábil" snapshotKey="filter_competence" />, {
      route: `/reconciliation/documents?status=pendente&regime_group=SIMPLES&responsible_id=${R}&filter_competence=2026-09`,
    });
    const group = screen.getByRole("group", { name: "Filtros do Dashboard" });
    expect(group).toHaveTextContent("Filtros do Dashboard (empresas de 09/2026):");
    expect(group).toHaveTextContent("Agrupamento: Simples Nacional");
    expect(await screen.findByText("Responsável do Contábil: Bruno Lima")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Limpar filtros do Dashboard" }));
    expect(screen.queryByRole("group", { name: "Filtros do Dashboard" })).toBeNull();
    expect(router.state.location.search).toBe("?status=pendente");
  });

  it("sem filtro, não aparece; filtro inválido é ignorado", () => {
    mockFetchRoutes(() => jsonResponse({ items: [] }));
    renderWithProviders(<CompanyFilterChips module="Fiscal" />, { route: "/factor-r?pa=2026-09&responsible_id=x&regime_group=OUTRO" });
    expect(screen.queryByRole("group", { name: "Filtros do Dashboard" })).toBeNull();
  });
});
