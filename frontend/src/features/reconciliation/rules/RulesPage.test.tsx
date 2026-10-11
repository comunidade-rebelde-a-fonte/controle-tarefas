import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ClassificationRule } from "@/services/reconciliation";
import { jsonResponse, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { RulesPage } from "./RulesPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const COMPANY = "00000000-0000-4000-8000-0000000000c1";
const R1 = "00000000-0000-4000-8000-0000000000f1";
const R2 = "00000000-0000-4000-8000-0000000000f2";
const LINE = "00000000-0000-4000-8000-0000000000a1";

function rule(over: Partial<ClassificationRule>): ClassificationRule {
  return {
    id: R1, company_id: COMPANY, rule_type: "contraparte", pattern: "Fornecedor Alfa", account_code: "4.1.01", active: true,
    source_statement_transaction_id: LINE, created_by: null, created_at: "2026-08-20T12:00:00Z", deactivated_by: null, deactivated_at: null, ...over,
  };
}

// Da mais nova para a mais antiga, como a API devolve (a ordem em que valem).
const rules = [
  rule({}),
  rule({
    id: R2, rule_type: "regex_descritivo", pattern: "TARIFA|PACOTE", account_code: "3.1.01", active: false, source_statement_transaction_id: null,
    created_at: "2026-08-01T12:00:00Z", deactivated_at: "2026-08-19T12:00:00Z",
  }),
];

const chart = {
  items: [
    { id: "a1", company_id: COMPANY, code: "3.1.01", name: "Tarifas bancárias", active: true, created_at: "", updated_at: "" },
    { id: "a2", company_id: COMPANY, code: "4.1.01", name: "Fornecedores", active: true, created_at: "", updated_at: "" },
    { id: "a3", company_id: COMPANY, code: "9.9.99", name: "Conta antiga", active: false, created_at: "", updated_at: "" },
  ],
};

function mock(extra?: (req: { method: string; url: URL; body: unknown }) => Response | undefined) {
  return mockFetchRoutes(
    ({ method, url }) => {
      if (method === "GET" && url.pathname === "/api/v1/companies") {
        return jsonResponse({ items: [{ id: COMPANY, internal_code: "900", legal_name: "Empresa 900" }], total: 1 });
      }
      if (method === "GET" && url.pathname === `/api/v1/companies/${COMPANY}/classification-rules`) return jsonResponse({ items: rules });
      if (method === "GET" && url.pathname === `/api/v1/companies/${COMPANY}/chart-accounts`) return jsonResponse(chart);
      return undefined;
    },
    ...(extra ? [extra] : []),
  );
}

describe("RulesPage", () => {
  it("lista da mais nova para a mais antiga, com a conta, a situação e a linha de origem; filtra as ativas", async () => {
    mock();
    renderWithProviders(<RulesPage />, { route: `/reconciliation/rules?company_id=${COMPANY}` });
    const table = await screen.findByRole("table", { name: "Regras da empresa" });
    const rows = within(table).getAllByRole("row");
    expect(rows[1]).toHaveTextContent("Contraparte");
    expect(rows[1]).toHaveTextContent("Fornecedor Alfa");
    expect(rows[1]).toHaveTextContent("4.1.01 · Fornecedores");
    expect(rows[1]).toHaveTextContent("Ativa");
    expect(within(rows[1] as HTMLElement).getByRole("link", { name: "da correção de uma linha" })).toHaveAttribute(
      "href",
      `/reconciliation?item=${LINE}&kind=linha`,
    );
    expect(rows[2]).toHaveTextContent("Descritivo (regex)");
    expect(rows[2]).toHaveTextContent("Desativada");
    await userEvent.click(screen.getByRole("button", { name: "Só as ativas (1)" }));
    await waitFor(() => expect(within(screen.getByRole("table", { name: "Regras da empresa" })).queryByText("TARIFA|PACOTE")).toBeNull());
  });

  it("confirma a regra com a conta do plano ativo e avisa a regra antiga substituída", async () => {
    const bodies: unknown[] = [];
    mock(({ method, url, body }) => {
      if (method === "POST" && url.pathname === `/api/v1/companies/${COMPANY}/classification-rules`) {
        bodies.push(body);
        if (bodies.length === 1) {
          return jsonResponse(
            { code: "VALIDATION_ERROR", message: "Dados inválidos.", details: { fields: { pattern: "Regex inválida: parêntese sem par." } } },
            400,
          );
        }
        return jsonResponse({ rule: rule({ id: "00000000-0000-4000-8000-0000000000f3" }), replaced_rule_ids: [R1] }, 201);
      }
      return undefined;
    });
    renderWithProviders(<RulesPage />, { route: `/reconciliation/rules?company_id=${COMPANY}` });
    const form = await screen.findByRole("form", { name: "Nova regra" });
    const account = within(form).getByLabelText(/^Conta/);
    await waitFor(() => expect(within(account).getByRole("option", { name: "4.1.01 · Fornecedores" })).toBeInTheDocument());
    // Só as contas ativas entram na escolha.
    expect(within(account).queryByRole("option", { name: "9.9.99 · Conta antiga" })).toBeNull();
    await userEvent.selectOptions(within(form).getByLabelText(/^Tipo/), "regex_descritivo");
    expect(within(form).getByText(/Expressão regular buscada no descritivo/)).toBeInTheDocument();
    await userEvent.type(within(form).getByLabelText(/^Padrão/), "PIX (ALFA");
    await userEvent.selectOptions(account, "4.1.01");
    await userEvent.click(within(form).getByRole("button", { name: "Confirmar regra" }));
    expect(await within(form).findByText("Regex inválida: parêntese sem par.")).toBeInTheDocument();
    const pattern = within(form).getByLabelText(/^Padrão/);
    await userEvent.clear(pattern);
    await userEvent.type(pattern, "PIX.*ALFA");
    await userEvent.click(within(form).getByRole("button", { name: "Confirmar regra" }));
    expect(await screen.findByText("Substitui 1 regra antiga com o mesmo padrão.")).toBeInTheDocument();
    expect(bodies[1]).toEqual({ rule_type: "regex_descritivo", pattern: "PIX.*ALFA", account_code: "4.1.01" });
  });

  it("desativar pede confirmação; aplicar às pendentes roda a classificação da competência", async () => {
    const fetchFn = mock(({ method, url }) => {
      if (method === "POST" && url.pathname === `/api/v1/companies/${COMPANY}/classification-rules/${R1}/deactivate`) {
        return jsonResponse(rule({ active: false, deactivated_at: "2026-08-21T12:00:00Z" }));
      }
      if (method === "POST" && url.pathname === `/api/v1/companies/${COMPANY}/classification/run`) {
        return jsonResponse({ by_rule: 4, by_history: 1, pending: 2 });
      }
      return undefined;
    });
    renderWithProviders(<RulesPage />, { route: `/reconciliation/rules?company_id=${COMPANY}` });
    await userEvent.click(await screen.findByRole("button", { name: "Desativar a regra Fornecedor Alfa" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Desativar a regra?" });
    await userEvent.click(within(confirm).getByRole("button", { name: "Desativar" }));
    expect(await screen.findByText("Regra desativada")).toBeInTheDocument();

    const apply = screen.getByRole("button", { name: "Aplicar" });
    expect(apply).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Competência"), { target: { value: "2026-08" } });
    await userEvent.click(apply);
    expect(await screen.findByText("Linhas: 4 por regra, 1 por histórico e 2 à espera do agente ou do contador.")).toBeInTheDocument();
    expect(fetchFn.mock.calls.some(([u, i]) => urlOf(u).includes("/classification/run?competence=2026-08") && i?.method === "POST")).toBe(true);
  });
});
