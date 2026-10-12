import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { DocumentsPage } from "./DocumentsPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const R = "00000000-0000-4000-8000-00000000a001";

function doc(id: string, status: string) {
  return {
    id, company_id: null, company_name: null, file_name: `${id}.pdf`, content_type: "application/pdf", size_bytes: 10, sha256: "a".repeat(64),
    kind: null, status, bank: null, branch: null, account: null, competence: null, period_start: null, period_end: null,
    triage_confidence: null, diagnosis: null, job: null, created_at: "2026-09-10T12:00:00Z", updated_at: "2026-09-10T12:00:00Z",
  };
}

describe("DocumentsPage", () => {
  it("TEST-024: os grupos de status do Dashboard e os filtros de empresa vão à API, com o total", async () => {
    const fetchFn = mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/reconciliation/documents") {
        const status = url.searchParams.get("status");
        const items = status === "pendente,triado" ? [doc("d1", "pendente"), doc("d2", "triado")] : [doc("d1", "pendente"), doc("d2", "triado"), doc("d3", "importado")];
        return jsonResponse({ items, total: items.length, count_by_status: { pendente: 1, triado: 1, importado: 1, revisao_triagem: 0 } });
      }
      if (method === "GET" && url.pathname === "/api/v1/companies") return jsonResponse({ items: [], total: 0 });
      if (method === "GET" && url.pathname === "/api/v1/users") return jsonResponse({ items: [] });
      return undefined;
    });
    renderWithProviders(<DocumentsPage />, {
      route: `/reconciliation/documents?status=pendente,triado&responsible_id=${R}&filter_competence=2026-09`,
    });
    expect(await screen.findByText("2 documentos")).toBeInTheDocument();
    const statuses = screen.getByRole("group", { name: "Filtrar por status" });
    expect(within(statuses).getByRole("button", { name: "Processando (2)" })).toHaveAttribute("aria-pressed", "true");
    expect(within(statuses).getByRole("button", { name: "Todos (3)" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("group", { name: "Filtros do Dashboard" })).toHaveTextContent("empresas de 09/2026");
    const listUrl = fetchFn.mock.calls.map(([u]) => urlOf(u)).find((u) => u.includes("/reconciliation/documents?"));
    expect(listUrl).toContain("status=pendente%2Ctriado");
    expect(listUrl).toContain(`responsible_id=${R}`);
    expect(listUrl).toContain("filter_competence=2026-09");

    await userEvent.click(within(statuses).getByRole("button", { name: "Todos (3)" }));
    expect(await screen.findByText("3 documentos")).toBeInTheDocument();
  });
});
