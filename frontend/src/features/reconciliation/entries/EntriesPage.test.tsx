import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ClientEntry, RunSummary } from "@/services/reconciliation";
import { jsonResponse, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { EntriesPage } from "./EntriesPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const COMPANY = "00000000-0000-4000-8000-0000000000c1";
const DOC = "00000000-0000-4000-8000-0000000000d1";
const E1 = "00000000-0000-4000-8000-0000000000e1";
const E2 = "00000000-0000-4000-8000-0000000000e2";

function entry(over: Partial<ClientEntry>): ClientEntry {
  return {
    id: E1, company_id: COMPANY, document_id: null, date: "2026-08-05", amount: "1500.00", direction: "D", counterparty: "Fornecedor Alfa",
    counterparty_doc: null, document_ref: "NF 123", operation_type: null, status: "manual", field_confidence: {}, field_origin: {},
    duplicate_flagged: false, flags: [], account_code: null, classification_origin: null, classification_pending: false, trace_id: null,
    created_by: null, created_at: "2026-08-06T12:00:00Z", updated_at: "2026-08-06T12:00:00Z", ...over,
  };
}

const items = [
  entry({}),
  entry({ id: E2, document_id: DOC, date: "2026-08-07", amount: "320.00", direction: "C", counterparty: "Cliente Beta", document_ref: null, status: "parcial" }),
];

const summary: RunSummary = {
  company_id: COMPANY, competence: "2026-08", new_matches: 3, by_pass: { passe_1: 3 }, reconciled: 5, divergent: 1, without_receipt: 2,
  without_statement: 0, awaiting_review: 0, resolver_requested: 0, classification: { by_rule: 2, by_history: 1, pending: 2 },
  classifier_requested: 0,
};

const companiesRoute = ({ method, url }: { method: string; url: URL }) =>
  method === "GET" && url.pathname === "/api/v1/companies"
    ? jsonResponse({ items: [{ id: COMPANY, internal_code: "900", legal_name: "Empresa 900" }], total: 1 })
    : undefined;

describe("EntriesPage", () => {
  it("lista os lançamentos com a origem e o status; o filtro de status vai para a API", async () => {
    const fetchFn = mockFetchRoutes(companiesRoute, ({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/reconciliation/entries") {
        const status = url.searchParams.get("status");
        const list = status ? items.filter((i) => i.status === status) : items;
        return jsonResponse({ items: list, total: list.length });
      }
      return undefined;
    });
    renderWithProviders(<EntriesPage />, { route: "/reconciliation/entries" });
    const table = await screen.findByRole("table", { name: "Lançamentos do cliente" });
    expect(within(table).getByText("Fornecedor Alfa")).toBeInTheDocument();
    expect(within(table).getByText("Formulário ou CSV")).toBeInTheDocument();
    expect(within(table).getByText("Manual")).toBeInTheDocument();
    expect(within(table).getByRole("link", { name: "Comprovante" })).toHaveAttribute("href", `/reconciliation/documents/${DOC}`);
    expect(within(table).getByRole("link", { name: "revisar" })).toHaveAttribute("href", `/reconciliation?item=${E2}&kind=comprovante`);
    expect(screen.getByText(/Escolha a empresa para incluir lançamentos/)).toBeInTheDocument();
    await userEvent.click(within(screen.getByRole("group", { name: "Filtrar por status" })).getByRole("button", { name: "Parcial (a revisar)" }));
    await waitFor(() => expect(within(screen.getByRole("table", { name: "Lançamentos do cliente" })).queryByText("Fornecedor Alfa")).toBeNull());
    expect(fetchFn.mock.calls.some(([u]) => urlOf(u).includes("status=parcial"))).toBe(true);
  });

  it("inclui o lançamento manual na empresa escolhida e mostra o erro de cada campo", async () => {
    const bodies: unknown[] = [];
    mockFetchRoutes(companiesRoute, ({ method, url, body }) => {
      if (method === "GET" && url.pathname === "/api/v1/reconciliation/entries") return jsonResponse({ items: [], total: 0 });
      if (method === "POST" && url.pathname === "/api/v1/reconciliation/entries") {
        bodies.push(body);
        if (bodies.length === 1) {
          return jsonResponse({ code: "VALIDATION_ERROR", message: "Dados inválidos.", details: { fields: { amount: "Valor inválido: use até 2 casas." } } }, 400);
        }
        return jsonResponse(entry({ id: "00000000-0000-4000-8000-0000000000e9" }), 201);
      }
      return undefined;
    });
    renderWithProviders(<EntriesPage />, { route: `/reconciliation/entries?company_id=${COMPANY}` });
    const form = await screen.findByRole("form", { name: "Novo lançamento" });
    await userEvent.type(within(form).getByLabelText(/^Data/), "2026-08-05");
    await userEvent.type(within(form).getByLabelText(/^Valor/), "12,345");
    await userEvent.type(within(form).getByLabelText(/^Contraparte/), "Fornecedor Alfa");
    await userEvent.click(within(form).getByRole("button", { name: "Incluir" }));
    expect(await within(form).findByText("Valor inválido: use até 2 casas.")).toBeInTheDocument();
    expect(within(form).getByLabelText(/^Valor/)).toHaveAttribute("aria-invalid", "true");
    const amount = within(form).getByLabelText(/^Valor/);
    await userEvent.clear(amount);
    await userEvent.type(amount, "1234,56");
    await userEvent.click(within(form).getByRole("button", { name: "Incluir" }));
    expect(await screen.findByText("Lançamento incluído")).toBeInTheDocument();
    expect(bodies[1]).toEqual({
      company_id: COMPANY, date: "2026-08-05", amount: "1234,56", direction: "D", counterparty: "Fornecedor Alfa", counterparty_doc: null,
      document_ref: null,
    });
    // A data e o sentido ficam para o próximo; o resto limpa.
    expect(within(form).getByLabelText(/^Data/)).toHaveValue("2026-08-05");
    expect(within(form).getByLabelText(/^Valor/)).toHaveValue("");
  });

  it("CSV recusado: nada gravado e o erro de cada linha", async () => {
    const fetchFn = mockFetchRoutes(companiesRoute, ({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/reconciliation/entries") return jsonResponse({ items: [], total: 0 });
      if (method === "POST" && url.pathname === "/api/v1/reconciliation/entries/import") {
        return jsonResponse(
          {
            code: "INVALID_ENTRIES_CSV",
            message: "O CSV tem linhas inválidas; nada foi importado.",
            details: { lines: [{ line: 2, message: "Data inválida (use AAAA-MM-DD)." }, { line: 4, message: "Sentido deve ser C ou D." }] },
          },
          400,
        );
      }
      return undefined;
    });
    renderWithProviders(<EntriesPage />, { route: `/reconciliation/entries?company_id=${COMPANY}` });
    await userEvent.upload(
      await screen.findByLabelText("Ou importe o CSV do cliente"),
      new File(["data;valor;sentido;contraparte\n"], "cliente.csv", { type: "text/csv" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Importar CSV" }));
    const lines = await screen.findByRole("list", { name: "Linhas com erro" });
    expect(within(lines).getByText("Linha 2: Data inválida (use AAAA-MM-DD).")).toBeInTheDocument();
    expect(within(lines).getByText("Linha 4: Sentido deve ser C ou D.")).toBeInTheDocument();
    const sent = fetchFn.mock.calls.find(([u]) => urlOf(u).endsWith("/reconciliation/entries/import"))?.[1]?.body;
    expect(sent).toBeInstanceOf(FormData);
    expect((sent as FormData).get("company_id")).toBe(COMPANY);
  });

  it("roda a conciliação da competência e mostra o resultado com o caminho para a fila", async () => {
    const fetchFn = mockFetchRoutes(companiesRoute, ({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/reconciliation/entries") return jsonResponse({ items: [], total: 0 });
      if (method === "POST" && url.pathname === `/api/v1/companies/${COMPANY}/reconciliation/run`) return jsonResponse(summary);
      return undefined;
    });
    renderWithProviders(<EntriesPage />, { route: `/reconciliation/entries?company_id=${COMPANY}&competence=2026-08` });
    await userEvent.click(await screen.findByRole("button", { name: "Rodar a conciliação da competência" }));
    const status = await screen.findByText(/Rodada concluída/);
    expect(status).toHaveTextContent("3 conciliações novas · 1 na fila por divergência ou caso de borda · 2 sem comprovante");
    expect(status).toHaveTextContent("Classificação: 2 por regra, 1 por histórico e 2 à espera do agente ou do contador.");
    expect(within(status).getByRole("link", { name: "Abrir a fila de revisão" })).toHaveAttribute(
      "href",
      `/reconciliation?company_id=${COMPANY}&competence=2026-08`,
    );
    expect(fetchFn.mock.calls.some(([u, i]) => urlOf(u).includes("/reconciliation/run?competence=2026-08") && i?.method === "POST")).toBe(true);
  });
});
