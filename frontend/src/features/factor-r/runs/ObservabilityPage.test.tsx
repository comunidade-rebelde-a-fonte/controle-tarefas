import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { aiRun, aiSummary, COMPANY_ID, TRACE_ID } from "@/test/factorR";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { ObservabilityPage } from "./ObservabilityPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

function mock() {
  return mockFetchRoutes(({ method, url }) => {
    if (method === "GET" && url.pathname === "/api/v1/ai/summary") return jsonResponse(aiSummary());
    if (method === "GET" && url.pathname === "/api/v1/ai/runs") {
      return jsonResponse({
        items: [
          aiRun(),
          aiRun({
            trace_id: "f".repeat(32),
            agent: "parser_pgdas",
            trigger: "upload",
            status: "needs_review",
            confidence: "0.3500",
            company_id: COMPANY_ID,
            competence: "2026-09",
            alias: null,
            provider: null,
            model: null,
            has_human_eval: true,
            langfuse_sync_status: "failed",
          }),
        ],
        total: 2,
      });
    }
    if (method === "GET" && url.pathname === "/api/v1/companies") return jsonResponse({ items: [{ id: COMPANY_ID, legal_name: "Clínica Exemplo Ltda" }] });
    return undefined;
  });
}

describe("ObservabilityPage", () => {
  it("FR-18: painel da trilha local e corridas com resultado, nota e Langfuse", async () => {
    mock();
    renderWithProviders(<ObservabilityPage />, { route: "/factor-r/runs" });
    expect(await screen.findByTestId("obs-total")).toHaveTextContent("42");
    expect(screen.getByTestId("obs-pendencias")).toHaveTextContent("5");
    expect(screen.getByTestId("obs-acerto-ouro")).toHaveTextContent("95,00%");
    expect(screen.getByTestId("obs-acerto-humano")).toHaveTextContent("—");
    expect(screen.getByTestId("obs-custo")).toHaveTextContent("US$ 0,0123");
    expect(screen.getByTestId("obs-tokens")).toHaveTextContent("15.234");
    const byAgent = screen.getByRole("table", { name: "Corridas por agente" });
    expect(within(byAgent).getByRole("row", { name: /Parser PGDAS-D/ })).toHaveTextContent("3014 ms98,00%14");

    const runs = await screen.findByRole("table", { name: "Corridas dos agentes" });
    const rows = within(runs).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("Priorizador");
    expect(rows[0]).toHaveTextContent("sem nota");
    expect(rows[0]).toHaveTextContent("pendente");
    expect(within(rows[0] as HTMLElement).getByRole("link")).toHaveAttribute("href", `/factor-r/runs/${TRACE_ID}`);
    expect(rows[1]).toHaveTextContent("Parser PGDAS-D");
    expect(rows[1]).toHaveTextContent("envio do extrato");
    expect(rows[1]).toHaveTextContent("em revisão");
    expect(rows[1]).toHaveTextContent("Clínica Exemplo Ltda");
    expect(rows[1]).toHaveTextContent("09/2026");
    expect(rows[1]).toHaveTextContent("35,00%");
    expect(rows[1]).toHaveTextContent("avaliada");
    expect(rows[1]).toHaveTextContent("falhou");
    expect(screen.getByText("1–2 de 2")).toBeInTheDocument();
  });

  it("filtros vão para a URL e para a API; o filtro de empresa vem da ficha", async () => {
    const fetchFn = mock();
    const user = userEvent.setup();
    renderWithProviders(<ObservabilityPage />, { route: `/factor-r/runs?company=${COMPANY_ID}` });
    const chip = await screen.findByRole("button", { name: "Remover o filtro de empresa" });
    await waitFor(() => expect(chip).toHaveTextContent("Empresa: Clínica Exemplo Ltda"));
    await user.selectOptions(screen.getByLabelText("Agente"), "parser_pgdas");
    await user.selectOptions(screen.getByLabelText("Resultado"), "needs_review");
    await user.click(screen.getByRole("button", { name: "Só sem nota humana" }));
    expect(screen.getByRole("button", { name: "Só sem nota humana" })).toHaveAttribute("aria-pressed", "true");
    await waitFor(() =>
      expect(
        requestedUrls(fetchFn).some(
          (u) => u.includes("agent=parser_pgdas") && u.includes("status=needs_review") && u.includes(`company_id=${COMPANY_ID}`) && u.includes("without_human_eval=true"),
        ),
      ).toBe(true),
    );
  });
});
