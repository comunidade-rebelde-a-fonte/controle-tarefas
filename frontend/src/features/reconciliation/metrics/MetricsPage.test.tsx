import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReconciliationMetrics } from "@/services/reconciliation";
import { jsonResponse, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { MetricsPage } from "./MetricsPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const COMPANY = "00000000-0000-4000-8000-0000000000c1";

const metrics: ReconciliationMetrics = {
  company_id: COMPANY,
  competence: "2026-08",
  matching: {
    lines: 10, reconciled: 6, auto: 4, manual: 2, free: 4, auto_percent: "40.0", target_percent: "80",
    by_method: { sem_comprovante: 1, passe_2: 1, manual: 1, passe_1: 2, passe_3: 1 }, undone_auto: 1,
  },
  buckets: { total: 7, revisao: 2, divergencia: 1, extrato_sem: 3, comprovante_sem: 0, classificacao: 1 },
  extraction: {
    statements: {
      documents: 4, local_reader: 1, agent: 3, first_attempt: 1, first_attempt_percent: "33.3", average_attempts: "2.00", not_approved: 1,
      cost_usd: "0.07100000", cost_per_document_usd: "0.01775000",
    },
    receipts: {
      documents: 4, local_reader: 1, agent: 3, first_attempt: 1, first_attempt_percent: "33.3", average_attempts: "1.33", not_approved: 2,
      cost_usd: "0.02000000", cost_per_document_usd: "0.00500000",
    },
  },
  corrections: { actions: 3, documents: 2, rate_percent: "25.0" },
  after_close: { reopenings: 2, late_documents: 0 },
  agents: {
    resolver: { runs: 2, cost_usd: "0.00600000", cost_per_run_usd: "0.00300000" },
    classifier: { runs: 0, cost_usd: "0.00000000", cost_per_run_usd: null },
  },
  playbooks: [
    { bank: "a_fonte", versions: 2, valid_version: 2, successes: 8, failures: 1, success_percent: "88.9" },
    { bank: "beta", versions: 1, valid_version: null, successes: 0, failures: 0, success_percent: null },
  ],
};

function mock() {
  return mockFetchRoutes(({ method, url }) => {
    if (method === "GET" && url.pathname === "/api/v1/reconciliation/metrics") return jsonResponse(metrics);
    if (method === "GET" && url.pathname === "/api/v1/companies") {
      return jsonResponse({ items: [{ id: COMPANY, internal_code: "900", legal_name: "Empresa 900" }], total: 1 });
    }
    return undefined;
  });
}

describe("MetricsPage", () => {
  it("11 §7: o percentual automático contra a meta, os métodos, os baldes e o depois do fechamento", async () => {
    const fetchFn = mock();
    renderWithProviders(<MetricsPage />, { route: `/reconciliation/metrics?company_id=${COMPANY}&competence=2026-08` });
    const auto = await screen.findByRole("region", { name: "Conciliação automática" });
    expect(within(auto).getByTestId("percentual-automatico")).toHaveTextContent("40,0%");
    expect(auto).toHaveTextContent("4 de 10 linhas do extrato conciliadas pelo automático · meta 80,0%");
    expect(auto).toHaveTextContent("1 automática desfeita pelo contador");
    expect(fetchFn.mock.calls.some(([u]) => urlOf(u).includes(`company_id=${COMPANY}`) && urlOf(u).includes("competence=2026-08"))).toBe(true);

    // Os passes do automático primeiro, depois as manuais.
    const methods = within(screen.getByRole("table", { name: "Conciliações por método" })).getAllByRole("row");
    expect(methods.map((r) => r.textContent)).toEqual([
      "Automática, passe 12",
      "Automática, passe 21",
      "Automática, passe 31",
      "Manual1",
      "Sem comprovante1",
    ]);
    const buckets = screen.getByRole("table", { name: "Itens da fila por balde" });
    expect(within(buckets).getByRole("link", { name: "Extrato sem comprovante" })).toHaveAttribute(
      "href",
      `/reconciliation?bucket=extrato_sem&company_id=${COMPANY}&competence=2026-08`,
    );
    expect(within(buckets).getByRole("row", { name: /Total/ })).toHaveTextContent("7");

    const after = screen.getByRole("region", { name: "Depois do fechamento" });
    expect(after).toHaveTextContent("2reaberturas do Contábil");
    expect(within(after).getByText("acima da meta zero")).toBeInTheDocument();
    expect(within(after).getByText("meta zero")).toBeInTheDocument();
  });

  it("a leitura como o eval-extrator, as correções, o custo em dólares e os playbooks", async () => {
    mock();
    renderWithProviders(<MetricsPage />, { route: "/reconciliation/metrics" });
    const reading = await screen.findByRole("table", { name: "Leitura por tipo de documento" });
    const statements = within(reading).getByRole("row", { name: /Extratos/ });
    expect(statements).toHaveTextContent("1 (33,3%)");
    expect(statements).toHaveTextContent("2,00");
    expect(statements).toHaveTextContent("US$ 0,0710");
    expect(statements).toHaveTextContent("US$ 0,017750");
    expect(screen.getByRole("region", { name: "Correções humanas" })).toHaveTextContent("2 de 8 documentos lidos corrigidos à mão (3 correções)");
    const agents = screen.getByRole("table", { name: "Custo dos agentes" });
    expect(within(agents).getByRole("row", { name: /Resolvedor/ })).toHaveTextContent("US$ 0,003000");
    expect(within(agents).getByRole("row", { name: /Classificador/ })).toHaveTextContent("—");
    const playbooks = screen.getByRole("table", { name: "Sucesso dos playbooks por banco" });
    expect(within(playbooks).getByRole("row", { name: /a_fonte/ })).toHaveTextContent("88,9%");
    expect(within(playbooks).getByRole("row", { name: /beta/ })).toHaveTextContent("nenhuma (redescobre)");
    expect(screen.getByRole("link", { name: "Ver os playbooks" })).toHaveAttribute("href", "/reconciliation/playbooks");
  });

  it("sem nada no filtro, o aviso de vazio", async () => {
    mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/reconciliation/metrics") {
        return jsonResponse({
          ...metrics,
          matching: { ...metrics.matching, lines: 0, auto: 0, manual: 0, reconciled: 0, free: 0, auto_percent: null, by_method: {} },
          extraction: {
            statements: { ...metrics.extraction.statements, documents: 0 },
            receipts: { ...metrics.extraction.receipts, documents: 0 },
          },
        });
      }
      if (method === "GET" && url.pathname === "/api/v1/companies") return jsonResponse({ items: [], total: 0 });
      return undefined;
    });
    renderWithProviders(<MetricsPage />, { route: "/reconciliation/metrics?competence=2025-01" });
    expect(await screen.findByText("Sem dados no filtro")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("percentual-automatico")).toHaveTextContent("—"));
    expect(screen.getByText("Nenhuma conciliação ainda.")).toBeInTheDocument();
  });
});
