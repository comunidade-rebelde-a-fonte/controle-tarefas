import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DocumentIndex, DocumentIndexItem } from "@/services/documents";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { DocumentsAreaPage } from "./DocumentsAreaPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const A = "00000000-0000-4000-8000-00000000000a";

function item(id: string, patch: Partial<DocumentIndexItem>): DocumentIndexItem {
  return {
    id, module: "factor_r", storage_backend: "local", kind: "pgdas", status: "linked", situation: "concluido",
    company: { id: A, internal_code: "101", legal_name: "Alfa Comércio Ltda" }, competence: "2026-09", period_start: null, period_end: null,
    bank: null, file_name: `${id}.txt`, expires_at: null, received_at: "2026-09-20T12:00:00Z", updated_at: "2026-09-20T12:00:00Z",
    detail: { path: `/factor-r/inbox/${id}`, query: {} },
    ...patch,
  };
}

const index: DocumentIndex = {
  items: [
    item("00000000-0000-4000-8000-0000000000f1", {}),
    item("00000000-0000-4000-8000-0000000000c1", {
      module: "reconciliation", storage_backend: "minio", kind: "extrato", status: "revisao_extracao", situation: "em_revisao",
      competence: null, period_start: "2026-09-01", period_end: "2026-09-30", bank: "beta", file_name: "extrato-beta.pdf",
      detail: { path: "/reconciliation/documents/00000000-0000-4000-8000-0000000000c1", query: {} },
    }),
    item("00000000-0000-4000-8000-0000000000a1", {
      module: "credentials", kind: "procuracao", status: "arquivado", competence: null, file_name: "procuracao.pdf", expires_at: "2027-01-31",
      detail: { path: `/companies/${A}`, query: { tab: "credentials" } },
    }),
  ],
  total: 3,
  counts: [
    { module: "credentials", situation: "concluido", status: "arquivado", total: 1 },
    { module: "factor_r", situation: "concluido", status: "linked", total: 1 },
    { module: "reconciliation", situation: "em_revisao", status: "revisao_extracao", total: 1 },
  ],
};

function mock() {
  return mockFetchRoutes(({ method, url }) => {
    if (method !== "GET") return undefined;
    if (url.pathname === "/api/v1/documents") return jsonResponse(index);
    if (url.pathname === "/api/v1/companies") return jsonResponse({ items: [], total: 0 });
    return undefined;
  });
}

describe("DocumentsAreaPage", () => {
  it("TEST-026: os documentos das três origens, cada um com o seu status e o caminho para o módulo", async () => {
    mock();
    renderWithProviders(<DocumentsAreaPage />, { route: "/documents" });
    const table = await screen.findByRole("table", { name: "Documentos" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.map((r) => r.getAttribute("data-modulo"))).toEqual(["factor_r", "reconciliation", "credentials"]);
    const [pgdas, statement, poa] = rows as [HTMLElement, HTMLElement, HTMLElement];
    expect(pgdas).toHaveTextContent("Fator R");
    expect(pgdas).toHaveTextContent("armazenamento local");
    expect(pgdas).toHaveTextContent("Vinculado");
    expect(within(pgdas).getByRole("link", { name: "00000000-0000-4000-8000-0000000000f1.txt" })).toHaveAttribute(
      "href",
      "/factor-r/inbox/00000000-0000-4000-8000-0000000000f1",
    );
    expect(statement).toHaveTextContent("MinIO");
    expect(statement).toHaveTextContent("Revisão da extração");
    expect(statement).toHaveTextContent("01/09/2026 a 30/09/2026");
    expect(within(statement).getByText("Em revisão")).toBeInTheDocument();
    expect(poa).toHaveTextContent("Procuração RFB");
    expect(poa).toHaveTextContent("válido até 31/01/2027");
    expect(within(poa).getByRole("link", { name: "procuracao.pdf" })).toHaveAttribute("href", `/companies/${A}?tab=credentials`);
    expect(screen.getByText("3 documentos")).toBeInTheDocument();
    const origins = screen.getByRole("group", { name: "Filtrar por origem" });
    expect(within(origins).getByRole("button", { name: "Conciliação (1)" })).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Filtrar pelo status do módulo" })).toBeNull();
  });

  it("os filtros vão à API; o status do módulo aparece com o módulo escolhido", async () => {
    const fetchFn = mock();
    const { router } = renderWithProviders(<DocumentsAreaPage />, {
      route: `/documents?company_id=${A}&competence=2026-09&situation=em_revisao&bank=beta`,
    });
    await screen.findByRole("table", { name: "Documentos" });
    const url = requestedUrls(fetchFn).find((u) => u.includes("/documents?")) ?? "";
    for (const part of [`company_id=${A}`, "competence=2026-09", "situation=em_revisao", "bank=beta", "limit=50"]) {
      expect(url).toContain(part);
    }
    await userEvent.click(within(screen.getByRole("group", { name: "Filtrar por origem" })).getByRole("button", { name: "Conciliação (1)" }));
    expect(new URLSearchParams(router.state.location.search).get("module")).toBe("reconciliation");
    const statuses = await screen.findByRole("group", { name: "Filtrar pelo status do módulo" });
    await userEvent.click(within(statuses).getByRole("button", { name: "Revisão da extração (1)" }));
    expect(new URLSearchParams(router.state.location.search).get("status")).toBe("revisao_extracao");
  });
});
