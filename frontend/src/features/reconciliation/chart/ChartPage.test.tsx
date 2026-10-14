import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChartAccount } from "@/services/reconciliation";
import { jsonResponse, requestBody, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { ChartPage } from "./ChartPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const COMPANY = "00000000-0000-4000-8000-0000000000c1";
const OTHER = "00000000-0000-4000-8000-0000000000c2";
const A1 = "00000000-0000-4000-8000-0000000000b1";
const A2 = "00000000-0000-4000-8000-0000000000b2";

function account(over: Partial<ChartAccount>): ChartAccount {
  return { id: A1, company_id: COMPANY, code: "1.1.01", name: "Caixa", active: true, created_at: "2026-08-01T12:00:00Z", updated_at: "2026-08-01T12:00:00Z", ...over };
}

const accounts = [account({}), account({ id: A2, code: "3.1.01", name: "Tarifas bancárias", active: false })];

function mock(extra?: (req: { method: string; url: URL; body: unknown }) => Response | undefined) {
  return mockFetchRoutes(
    ({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/companies") {
        return jsonResponse({
          items: [
            { id: COMPANY, internal_code: "900", legal_name: "Empresa 900" },
            { id: OTHER, internal_code: "100", legal_name: "Plano padrão" },
          ],
          total: 2,
        });
      }
      if (method === "GET" && url.pathname === `/api/v1/companies/${COMPANY}/chart-accounts`) return jsonResponse({ items: accounts });
      return undefined;
    },
    ...(extra ? [extra] : []),
  );
}

describe("ChartPage", () => {
  it("lista o plano com a situação de cada conta e inclui uma conta nova", async () => {
    let created: unknown;
    mock(({ method, url, body }) => {
      if (method === "POST" && url.pathname === `/api/v1/companies/${COMPANY}/chart-accounts`) {
        created = body;
        return jsonResponse(account({ id: "00000000-0000-4000-8000-0000000000b3", code: "4.1.01", name: "Fornecedores" }), 201);
      }
      return undefined;
    });
    renderWithProviders(<ChartPage />, { route: `/reconciliation/chart?company_id=${COMPANY}` });
    const table = await screen.findByRole("table", { name: "Contas do plano" });
    const rows = within(table).getAllByRole("row");
    expect(rows[1]).toHaveTextContent("1.1.01CaixaAtiva");
    expect(rows[2]).toHaveTextContent("3.1.01Tarifas bancáriasDesativada");
    expect(within(table).getByRole("button", { name: "Reativar a conta 3.1.01" })).toBeInTheDocument();
    expect(screen.getByText("2 contas, 1 ativas.")).toBeInTheDocument();
    const form = screen.getByRole("form", { name: "Nova conta" });
    await userEvent.type(within(form).getByLabelText(/^Código/), "4.1.01");
    await userEvent.type(within(form).getByLabelText(/^Nome/), "Fornecedores");
    await userEvent.click(within(form).getByRole("button", { name: "Incluir" }));
    expect(await screen.findByText("Conta 4.1.01 incluída")).toBeInTheDocument();
    expect(created).toEqual({ code: "4.1.01", name: "Fornecedores" });
  });

  it("desativar pede confirmação e manda active=false; reativar vai direto", async () => {
    const patches: unknown[] = [];
    mock(({ method, url, body }) => {
      if (method === "PATCH" && url.pathname.startsWith(`/api/v1/companies/${COMPANY}/chart-accounts/`)) {
        patches.push({ id: url.pathname.split("/").pop(), body });
        return jsonResponse(account({}));
      }
      return undefined;
    });
    renderWithProviders(<ChartPage />, { route: `/reconciliation/chart?company_id=${COMPANY}` });
    await userEvent.click(await screen.findByRole("button", { name: "Desativar a conta 1.1.01" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Desativar a conta 1.1.01?" });
    expect(patches).toHaveLength(0);
    await userEvent.click(within(confirm).getByRole("button", { name: "Desativar" }));
    await waitFor(() => expect(patches).toEqual([{ id: A1, body: { active: false } }]));
    expect(await screen.findByText("Conta desativada")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Reativar a conta 3.1.01" }));
    await waitFor(() => expect(patches).toContainEqual({ id: A2, body: { active: true } }));
  });

  it("CSV do plano recusado mostra o erro de cada linha", async () => {
    mock(({ method, url }) => {
      if (method === "POST" && url.pathname === `/api/v1/companies/${COMPANY}/chart-accounts/import`) {
        return jsonResponse(
          { code: "INVALID_ACCOUNTS_CSV", message: "O CSV tem linhas inválidas; nada foi importado.", details: { lines: [{ line: 3, message: "Código repetido no arquivo." }] } },
          400,
        );
      }
      return undefined;
    });
    renderWithProviders(<ChartPage />, { route: `/reconciliation/chart?company_id=${COMPANY}` });
    await userEvent.upload(await screen.findByLabelText("Arquivo do plano (codigo;nome)"), new File(["codigo;nome\n"], "plano.csv", { type: "text/csv" }));
    await userEvent.click(screen.getByRole("button", { name: "Importar CSV" }));
    const alert = await screen.findByRole("list", { name: "Linhas com erro" });
    expect(alert).toHaveTextContent("Linha 3: Código repetido no arquivo.");
  });

  it("copia o plano de outra empresa e informa o que já existia", async () => {
    const fetchFn = mock(({ method, url }) => {
      if (method === "POST" && url.pathname === `/api/v1/companies/${COMPANY}/chart-accounts/copy`) return jsonResponse({ inserted: 12, skipped: 2 }, 201);
      return undefined;
    });
    renderWithProviders(<ChartPage />, { route: `/reconciliation/chart?company_id=${COMPANY}` });
    const source = await screen.findByLabelText("Empresa de origem");
    await waitFor(() => expect(within(source).getByRole("option", { name: "100 · Plano padrão" })).toBeInTheDocument());
    await userEvent.selectOptions(source, OTHER);
    await userEvent.click(screen.getByRole("button", { name: "Copiar o plano" }));
    expect(await screen.findByText("12 contas copiadas; 2 já existiam.")).toBeInTheDocument();
    const index = fetchFn.mock.calls.findIndex(([u, i]) => urlOf(u).endsWith("/chart-accounts/copy") && i?.method === "POST");
    expect(requestBody(fetchFn, index)).toEqual({ source_company_id: OTHER });
  });
});
