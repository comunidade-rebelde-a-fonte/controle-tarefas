import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { ObligationsTab } from "./ObligationsTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ID = "00000000-0000-4000-8000-00000000000a";

const item = {
  kind: "OBLIGATION",
  instance_id: "00000000-0000-4000-8000-0000000000d1",
  competence: "2026-09",
  company: { id: ID, internal_code: "101", legal_name: "Alfa Comércio Ltda", trade_name: null, cnpj: "00000001000101" },
  module: "FISCAL",
  obligation_type: { code: "FISCAL_ICMS", name: "ICMS" },
  status_code: "NOT_STARTED",
  status_label: "A iniciar",
  due_date: "2026-09-10",
  days_overdue: 5,
  responsible: null,
  drilldown: { path: "/fiscal", query: { competence: "2026-09", q: "101", group: "normal" } },
};

describe("ObligationsTab", () => {
  it("TEST-025: as obrigações só da empresa na competência da ficha, com a classe e o status da URL", async () => {
    const fetchFn = mockFetchRoutes(({ method, url }) =>
      method === "GET" && url.pathname === "/api/v1/backlog" ? jsonResponse({ items: [item], total: 1, page: 1, page_size: 50 }) : undefined,
    );
    const { router } = renderWithProviders(<ObligationsTab companyId={ID} competence="2026-09" />, {
      route: `/companies/${ID}?tab=obligations&state=overdue&status=ERROR`,
    });
    const table = await screen.findByRole("table", { name: "Obrigações da empresa na competência" });
    expect(within(table).queryByText("Alfa Comércio Ltda")).toBeNull();
    expect(within(table).getByRole("link", { name: "ICMS" })).toHaveAttribute("href", "/fiscal?competence=2026-09&q=101&group=normal");
    const url = requestedUrls(fetchFn).find((u) => u.includes("/backlog?")) ?? "";
    for (const part of ["competence=2026-09", `company_id=${ID}`, "state=overdue", "status=ERROR", "sort=due_date"]) {
      expect(url).toContain(part);
    }
    expect(screen.getByText("Status: Erro")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Limpar o filtro de status" }));
    expect(new URLSearchParams(router.state.location.search).get("status")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Todas" }));
    expect(new URLSearchParams(router.state.location.search).get("state")).toBe("applicable");
    expect(screen.getByRole("button", { name: "Todas" })).toHaveAttribute("aria-pressed", "true");
  });
});
