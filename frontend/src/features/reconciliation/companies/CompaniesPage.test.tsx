import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReconciliationOverview, ReconciliationOverviewCompany } from "@/services/reconciliation";
import { jsonResponse, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CompaniesPage } from "./CompaniesPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const R = "00000000-0000-4000-8000-00000000a001";

function company(id: string, name: string, patch: Partial<ReconciliationOverviewCompany>): ReconciliationOverviewCompany {
  return {
    company: { id, internal_code: id.slice(-3), legal_name: name },
    state: "em_andamento",
    statements: 1,
    blocking: { missing_statement: false, divergent: 0, without_receipt: 0, review: 0, classification: 0 },
    without_statement: 0,
    uses_reconciliation: true,
    accounting_state: "OPEN",
    obligation: { instance_id: "00000000-0000-4000-8000-0000000000b1", status_code: "NOT_STARTED", status_label: "A iniciar", is_final: false, applicable: true },
    blocks_accounting: true,
    ...patch,
  };
}

const items = [
  company("00000000-0000-4000-8000-000000000a01", "Alfa Comércio Ltda", {
    blocking: { missing_statement: false, divergent: 1, without_receipt: 2, review: 3, classification: 0 },
  }),
  company("00000000-0000-4000-8000-000000000b01", "Beta Serviços Ltda", { state: "pronta" }),
  company("00000000-0000-4000-8000-000000000c01", "Gama Indústria Ltda", {
    state: "concluida",
    blocks_accounting: false,
    accounting_state: "CLOSED",
    obligation: { instance_id: "00000000-0000-4000-8000-0000000000b2", status_code: "OK", status_label: "OK", is_final: true, applicable: true },
  }),
  company("00000000-0000-4000-8000-000000000d01", "Delta ME", {
    statements: 0,
    blocking: { missing_statement: true, divergent: 0, without_receipt: 0, review: 0, classification: 0 },
    uses_reconciliation: false,
    blocks_accounting: false,
    obligation: null,
  }),
];

const overview: ReconciliationOverview = {
  competence: "2026-09",
  counts: { total: 4, em_andamento: 2, pronta: 1, concluida: 1, blocks_accounting: 2 },
  items,
};

describe("CompaniesPage", () => {
  it("TEST-024: a Conciliação por empresa, com o estado, o que falta, o Contábil e a obrigação", async () => {
    const fetchFn = mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/competencies/latest") return jsonResponse({ competence: "2026-09", current: "2026-09" });
      if (method === "GET" && url.pathname === "/api/v1/reconciliation/companies") {
        const blocked = url.searchParams.get("blocked") === "true";
        const state = url.searchParams.get("state");
        return jsonResponse({ ...overview, items: items.filter((i) => (!blocked || i.blocks_accounting) && (!state || i.state === state)) });
      }
      if (method === "GET" && url.pathname === "/api/v1/users") return jsonResponse({ items: [] });
      return undefined;
    });
    renderWithProviders(<CompaniesPage />, { route: `/reconciliation/companies?competence=2026-09&blocked=true&responsible_id=${R}` });
    const table = await screen.findByRole("table", { name: "Conciliação por empresa" });
    expect(screen.getByText("2 empresas")).toBeInTheDocument();
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("Alfa Comércio Ltda");
    expect(rows[0]).toHaveTextContent("1 com divergência · 2 sem comprovante · 3 em revisão");
    expect(rows[0]).toHaveTextContent("Não fecha: a Conciliação não está concluída");
    expect(rows[1]).toHaveTextContent("Pronta para concluir");
    expect(within(rows[0] as HTMLElement).getByRole("link", { name: "Alfa Comércio Ltda" })).toHaveAttribute(
      "href",
      "/companies/00000000-0000-4000-8000-000000000a01?tab=reconciliation&competence=2026-09",
    );
    const filters = screen.getByRole("group", { name: "Filtrar por estado" });
    expect(within(filters).getByRole("button", { name: "Bloqueando o Contábil (2)" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("group", { name: "Filtros do Dashboard" })).toBeInTheDocument();
    const listUrl = fetchFn.mock.calls.map(([u]) => urlOf(u)).find((u) => u.includes("/reconciliation/companies?"));
    expect(listUrl).toContain("blocked=true");
    expect(listUrl).toContain(`responsible_id=${R}`);

    await userEvent.click(within(filters).getByRole("button", { name: "Todas (4)" }));
    expect(await screen.findByText("4 empresas")).toBeInTheDocument();
    const all = within(screen.getByRole("table", { name: "Conciliação por empresa" })).getAllByRole("row").slice(1);
    expect(all[2]).toHaveTextContent("Concluída");
    expect(all[2]).toHaveTextContent("Fechado");
    expect(all[3]).toHaveTextContent("falta importar o extrato");
    expect(all[3]).toHaveTextContent("Não gerada");
  });
});
