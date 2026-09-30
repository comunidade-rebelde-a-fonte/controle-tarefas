import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes, type Route, usersRoute } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { ReportsPage } from "./ReportsPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const latestRoute: Route = ({ method, url }) =>
  method === "GET" && url.pathname === "/api/v1/competencies/latest" ? jsonResponse({ competence: "2026-10", current: "2026-10" }) : undefined;

function linkOf(title: string): URL {
  const card = screen.getByRole("region", { name: title });
  const href = within(card).getByRole("link", { name: `Baixar PDF — ${title}` }).getAttribute("href");
  return new URL(href ?? "", "http://localhost");
}

describe("ReportsPage", () => {
  it("lista os 6 relatórios com a competência e os filtros atuais da URL", async () => {
    mockFetchRoutes(latestRoute, usersRoute);
    renderWithProviders(<ReportsPage />, { route: "/reports?competence=2026-09&regime_group=SIMPLES&responsible_id=00000000-0000-4000-8000-000000000001" });

    expect(await screen.findByRole("link", { name: "Baixar PDF — Situação geral da competência" })).toBeInTheDocument();
    expect(screen.getAllByRole("link", { name: /^Baixar PDF — / })).toHaveLength(6);

    const general = linkOf("Situação geral da competência");
    expect(general.pathname).toBe("/api/v1/reports/general.pdf");
    expect(Object.fromEntries(general.searchParams)).toEqual({
      competence: "2026-09",
      regime_group: "SIMPLES",
      responsible_id: "00000000-0000-4000-8000-000000000001",
    });
    expect(linkOf("Backlog").pathname).toBe("/api/v1/reports/backlog.pdf");
    expect(linkOf("Obrigações atrasadas").searchParams.get("competence")).toBe("2026-09");

    // Credenciais não usam os filtros do topo.
    expect(Object.fromEntries(linkOf("Procurações e certificados a vencer").searchParams)).toEqual({ competence: "2026-09" });

    // Controle completo: Fiscal por padrão, com o agrupamento derivado do filtro do topo.
    expect(Object.fromEntries(linkOf("Controle completo de um módulo").searchParams)).toEqual({
      competence: "2026-09",
      module: "fiscal",
      group: "simples",
      responsible_id: "00000000-0000-4000-8000-000000000001",
    });
  });

  it("os parâmetros de cada relatório entram na URL e no link", async () => {
    mockFetchRoutes(latestRoute, usersRoute);
    const user = userEvent.setup();
    const { router } = renderWithProviders(<ReportsPage />, { route: "/reports" });

    // Sem competência na URL, usa a mais recente.
    await screen.findByRole("link", { name: "Baixar PDF — Pendências por módulo" });
    expect(linkOf("Pendências por módulo").searchParams.get("competence")).toBe("2026-10");

    const pending = screen.getByRole("region", { name: "Pendências por módulo" });
    await user.selectOptions(within(pending).getByLabelText("Módulo"), "ACCOUNTING");
    await user.selectOptions(within(pending).getByLabelText("Classe"), "pending");
    expect(Object.fromEntries(linkOf("Pendências por módulo").searchParams)).toEqual({ competence: "2026-10", module: "ACCOUNTING", state: "pending" });
    expect(router.state.location.search).toContain("pending.module=ACCOUNTING");

    const module = screen.getByRole("region", { name: "Controle completo de um módulo" });
    await user.selectOptions(within(module).getByLabelText("Módulo"), "payroll");
    // O agrupamento só existe no Fiscal.
    expect(within(module).queryByLabelText("Agrupamento do Fiscal")).not.toBeInTheDocument();
    expect(Object.fromEntries(linkOf("Controle completo de um módulo").searchParams)).toEqual({ competence: "2026-10", module: "payroll" });

    const creds = screen.getByRole("region", { name: "Procurações e certificados a vencer" });
    await user.selectOptions(within(creds).getByLabelText("Situação"), "expired");
    await user.selectOptions(within(creds).getByLabelText("Documento"), "DIGITAL_CERTIFICATE");
    expect(Object.fromEntries(linkOf("Procurações e certificados a vencer").searchParams)).toEqual({
      competence: "2026-10",
      filter: "expired",
      type: "DIGITAL_CERTIFICATE",
    });
  });
});
