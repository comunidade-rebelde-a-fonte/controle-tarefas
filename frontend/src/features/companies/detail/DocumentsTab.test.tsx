import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { DocumentsTab } from "./DocumentsTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

const A = "00000000-0000-4000-8000-00000000000a";

const index = {
  items: [
    {
      id: "00000000-0000-4000-8000-0000000000c1", module: "reconciliation", storage_backend: "minio", kind: "comprovante", status: "importado",
      situation: "concluido", company: { id: A, internal_code: "101", legal_name: "Alfa Comércio Ltda" }, competence: "2026-09",
      period_start: null, period_end: null, bank: null, file_name: "comprovante.pdf", expires_at: null,
      received_at: "2026-09-20T12:00:00Z", updated_at: "2026-09-20T12:00:00Z",
      detail: { path: "/reconciliation/documents/00000000-0000-4000-8000-0000000000c1", query: {} },
    },
  ],
  total: 1,
  counts: [{ module: "reconciliation", situation: "concluido", status: "importado", total: 1 }],
};

describe("DocumentsTab", () => {
  it("TEST-026: os documentos da empresa na competência da ficha, ou em todas", async () => {
    const fetchFn = mockFetchRoutes(({ method, url }) => (method === "GET" && url.pathname === "/api/v1/documents" ? jsonResponse(index) : undefined));
    const { router } = renderWithProviders(<DocumentsTab companyId={A} competence="2026-09" />, { route: `/companies/${A}?tab=documents&competence=2026-09` });
    const table = await screen.findByRole("table", { name: "Documentos da empresa" });
    expect(within(table).queryByText("Alfa Comércio Ltda")).toBeNull();
    expect(within(table).getByRole("link", { name: "comprovante.pdf" })).toHaveAttribute(
      "href",
      "/reconciliation/documents/00000000-0000-4000-8000-0000000000c1",
    );
    const first = requestedUrls(fetchFn).find((u) => u.includes("/documents?")) ?? "";
    expect(first).toContain(`company_id=${A}`);
    expect(first).toContain("competence=2026-09");

    await userEvent.click(screen.getByRole("button", { name: "Todas as competências" }));
    expect(new URLSearchParams(router.state.location.search).get("doc_all")).toBe("1");
    const last = requestedUrls(fetchFn).filter((u) => u.includes("/documents?")).at(-1) ?? "";
    expect(last).toContain(`company_id=${A}`);
    expect(last).not.toContain("competence=");
    expect(screen.getByRole("link", { name: "Procurações e certificados" })).toHaveAttribute(
      "href",
      `/companies/${A}?tab=credentials&competence=2026-09`,
    );
  });
});
