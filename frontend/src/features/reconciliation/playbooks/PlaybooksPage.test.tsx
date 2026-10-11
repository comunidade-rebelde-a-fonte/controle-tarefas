import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BankPlaybook } from "@/services/reconciliation";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { PlaybooksPage } from "./PlaybooksPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

function playbook(over: Partial<BankPlaybook>): BankPlaybook {
  return {
    id: "00000000-0000-4000-8000-0000000000b1", bank: "a_fonte", version: 2, valid: true, successes: 5, failures: 0, success_percent: "100.0",
    table_start_page: 1, ignored_pages: [2, 3], date_format: "DD/MM/YYYY", decimal_separator: ",", sign_representation: "coluna_tipo",
    balance_location: "rodape", winning_strategy: "texto", validated_at: "2026-08-10T12:00:00Z", created_at: "2026-08-10T12:00:00Z",
    updated_at: "2026-08-20T12:00:00Z", ...over,
  };
}

describe("PlaybooksPage", () => {
  it("as versões por banco com o estado, a taxa e a receita", async () => {
    mockFetchRoutes(({ method, url }) =>
      method === "GET" && url.pathname === "/api/v1/reconciliation/playbooks"
        ? jsonResponse({
            items: [
              playbook({}),
              playbook({ id: "00000000-0000-4000-8000-0000000000b2", version: 1, valid: false, successes: 3, failures: 1, success_percent: "75.0" }),
              playbook({ id: "00000000-0000-4000-8000-0000000000b3", bank: "beta", version: 1, successes: 0, success_percent: null, ignored_pages: [] }),
            ],
          })
        : undefined,
    );
    renderWithProviders(<PlaybooksPage />, { route: "/reconciliation/playbooks" });
    const table = await screen.findByRole("table", { name: "Playbooks por banco" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows.map((r) => r.getAttribute("data-playbook"))).toEqual(["a_fonte-v2", "a_fonte-v1", "beta-v1"]);
    expect(rows[0]).toHaveTextContent("válido");
    expect(rows[0]).toHaveTextContent("100,0%");
    expect(rows[1]).toHaveTextContent("inválido");
    expect(rows[1]).toHaveTextContent("75,0%");
    expect(rows[2]).toHaveTextContent("—");
    const [first] = rows;
    if (!first) throw new Error("sem linhas");
    await userEvent.click(within(first).getByText("receita da v2"));
    expect(within(first).getByText("Páginas ignoradas").nextElementSibling).toHaveTextContent("2, 3");
    expect(within(first).getByText("Formato da data").nextElementSibling).toHaveTextContent("DD/MM/YYYY");
  });

  it("sem playbook, o aviso da origem", async () => {
    mockFetchRoutes(({ method, url }) =>
      method === "GET" && url.pathname === "/api/v1/reconciliation/playbooks" ? jsonResponse({ items: [] }) : undefined,
    );
    renderWithProviders(<PlaybooksPage />, { route: "/reconciliation/playbooks" });
    expect(await screen.findByText("Nenhum playbook ainda")).toBeInTheDocument();
    expect(screen.getByText("O primeiro extrato aprovado de cada banco cria um.")).toBeInTheDocument();
  });
});
