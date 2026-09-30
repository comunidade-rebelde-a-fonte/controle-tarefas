import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { HistoryTab } from "./HistoryTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

const progress = (final: number) => ({
  denominator: 10, operational_done: 10, final_done: final * 10, blocking: 0, operational_progress: 1, final_completion: final, ready_to_close: final === 1,
});
const ev = (event_type: "OPENED" | "CLOSED" | "REOPENED", reason: string | null = null) => ({
  event_type, reason, user_name: "Administrador", created_at: "2026-09-05T15:00:00Z",
});

describe("HistoryTab", () => {
  it("mostra a matriz competência × módulo com a reabertura e o link para a grade", async () => {
    mockFetchRoutes(({ method, url }) =>
      method === "GET" && url.pathname === "/api/v1/companies/c1/history"
        ? jsonResponse({
            company_id: "c1",
            competences: [
              {
                competence: "2026-09",
                tax_regime: "LUCRO_PRESUMIDO",
                modules: [{ module: "FISCAL", period_id: "p2", state: "OPEN", closed_at: null, progress: progress(0.5), events: [ev("OPENED")] }],
              },
              {
                competence: "2026-08",
                tax_regime: "LUCRO_PRESUMIDO",
                modules: [
                  {
                    module: "FISCAL", period_id: "p1", state: "OPEN", closed_at: null, progress: progress(1),
                    events: [ev("OPENED"), ev("CLOSED"), ev("REOPENED", "Correção de obrigação")],
                  },
                  { module: "ACCOUNTING", period_id: "p3", state: "CLOSED", closed_at: "2026-09-06T12:00:00Z", progress: progress(1), events: [ev("OPENED"), ev("CLOSED")] },
                ],
              },
            ],
          })
        : undefined,
    );
    renderWithProviders(<HistoryTab companyId="c1" internalCode="101" />);
    const table = await screen.findByRole("table", { name: "Histórico de competências" });
    const rows = table.querySelectorAll("tbody tr");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("09/2026");
    expect(rows[1]).toHaveTextContent("08/2026");
    expect(rows[1]).toHaveTextContent("Reaberto");
    expect(screen.getByLabelText("Eventos: Aberto, Fechado, Reaberto")).toBeInTheDocument();
    expect(rows[1]).toHaveTextContent("Fechado");
    const links = screen.getAllByRole("link", { name: "Abrir grade" });
    expect(links[0]).toHaveAttribute("href", "/fiscal?competence=2026-09&q=101");
    expect(screen.getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["Competência", "Regime", "Fiscal", "Contábil"]);
  });
});
