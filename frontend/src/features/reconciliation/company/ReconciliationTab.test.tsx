import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Company } from "@/services/companies";
import type { BankAccountList, CompanyReconciliationSummary } from "@/services/reconciliation";
import { jsonResponse, requestBody, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { ReconciliationTab } from "./ReconciliationTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ID = "00000000-0000-4000-8000-0000000000c1";
const DOC = "00000000-0000-4000-8000-0000000000d1";

const company = {
  id: ID,
  internal_code: "900",
  legal_name: "Construtora Horizonte Ltda",
  cnpj: "11222333000181",
  active: true,
  version: 1,
} as unknown as Company;

function summary(over: Partial<CompanyReconciliationSummary>): CompanyReconciliationSummary {
  return {
    company_id: ID,
    competence: "2026-10",
    accounting_closed: false,
    statements: [
      {
        import_id: "00000000-0000-4000-8000-0000000000e1", document_id: DOC, file_name: "extrato.pdf", bank_account: "beta 0021/33412-7",
        period_start: "2026-10-01", period_end: "2026-10-31", opening_balance: "100.00", closing_balance: "250.50", attempts: 2, transactions: 3,
      },
    ],
    matching: { lines: 3, reconciled: 2, auto: 1, manual: 1, free: 1, auto_percent: "33.3", target_percent: "80", by_method: {}, undone_auto: 0 },
    buckets: { total: 2, revisao: 0, divergencia: 1, extrato_sem: 1, comprovante_sem: 0, classificacao: 0 },
    entries: { total: 4, partial: 1 },
    classification: { classified: 1, pending: 1 },
    ...over,
  };
}

const accounts: BankAccountList = {
  items: [
    {
      id: "00000000-0000-4000-8000-0000000000a1", company_id: ID, bank: "a_fonte", branch: "0001", account: "12345-6", label: "a_fonte 0001/12345-6",
      active: true, statements: 2, last_period_end: "2026-09-30", created_by: null, created_at: "2026-09-01T12:00:00Z",
    },
  ],
  read: [{ bank: "beta", branch: "0021", account: "33412-7", label: "beta 0021/33412-7", statements: 1, last_period_end: "2026-10-31" }],
};

function mock(closed = false) {
  return mockFetchRoutes(({ method, url, body }) => {
    if (method === "GET" && url.pathname === "/api/v1/competencies/latest") return jsonResponse({ competence: "2026-10", current: "2026-10" });
    if (method === "GET" && url.pathname === `/api/v1/companies/${ID}/reconciliation`) {
      const competence = url.searchParams.get("competence") ?? "";
      return jsonResponse(summary({ competence, accounting_closed: closed }));
    }
    if (method === "GET" && url.pathname === `/api/v1/companies/${ID}/bank-accounts`) return jsonResponse(accounts);
    if (method === "POST" && url.pathname === `/api/v1/companies/${ID}/bank-accounts`) {
      const b = body as { bank: string; branch: string; account: string };
      if (b.bank === "itau") {
        return jsonResponse({ code: "BANK_ACCOUNT_EXISTS", message: "Essa conta já está cadastrada na empresa 901 · Outra." }, 409);
      }
      return jsonResponse({ ...accounts.items[0], label: `${b.bank} ${b.branch}/${b.account}` }, 201);
    }
    return undefined;
  });
}

describe("ReconciliationTab", () => {
  it("AC-034: a competência corrente, o percentual, a fila, os extratos e os atalhos filtrados pela empresa", async () => {
    const fetchFn = mock();
    renderWithProviders(<ReconciliationTab company={company} />, { route: `/companies/${ID}?tab=reconciliation` });
    expect(await screen.findByText("Contábil aberto")).toBeInTheDocument();
    expect(fetchFn.mock.calls.some(([u]) => urlOf(u).includes(`/companies/${ID}/reconciliation?competence=2026-10`))).toBe(true);
    expect(screen.getByRole("region", { name: "Conciliação da competência" })).toHaveTextContent("33,3%");
    expect(screen.getByRole("button", { name: "Rodar a conciliação da competência" })).toBeEnabled();
    const queue = screen.getByRole("region", { name: "Fila de revisão da competência" });
    expect(within(queue).getByRole("link", { name: "Divergências" })).toHaveAttribute(
      "href",
      `/reconciliation?company_id=${ID}&competence=2026-10&bucket=divergencia`,
    );
    expect(screen.getByRole("region", { name: "Classificação e lançamentos" })).toHaveTextContent("4 lançamentos do cliente, 1 parcial (a revisar).");
    const statements = screen.getByRole("table", { name: "Extratos importados da competência" });
    expect(statements).toHaveTextContent("beta 0021/33412-7");
    expect(statements).toHaveTextContent("agente, 2ª tentativa");
    expect(statements).toHaveTextContent("R$ 250,50");
    expect(within(statements).getByRole("link", { name: "Abrir o extrato extrato.pdf" })).toHaveAttribute("href", `/reconciliation/documents/${DOC}`);
    const nav = screen.getByRole("navigation", { name: "Conciliação da empresa" });
    expect(within(nav).getByRole("link", { name: "Razão" })).toHaveAttribute(
      "href",
      `/reconciliation/ledger?company_id=${ID}&from=2026-10&to=2026-10`,
    );
    expect(within(nav).getByRole("link", { name: "Plano de contas" })).toHaveAttribute("href", `/reconciliation/chart?company_id=${ID}`);

    fireEvent.change(screen.getByLabelText("Competência"), { target: { value: "2026-08" } });
    await waitFor(() => expect(fetchFn.mock.calls.some(([u]) => urlOf(u).includes(`/companies/${ID}/reconciliation?competence=2026-08`))).toBe(true));
  });

  it("com o Contábil fechado, a trava aparece e não há como rodar a conciliação", async () => {
    mock(true);
    renderWithProviders(<ReconciliationTab company={company} />, { route: `/companies/${ID}?tab=reconciliation` });
    expect(await screen.findByText("Contábil fechado")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("O Contábil da competência está fechado: a Conciliação fica travada.");
    expect(screen.queryByRole("button", { name: "Rodar a conciliação da competência" })).toBeNull();
  });

  it("CC-11: cadastra a conta lida no extrato com um clique e mostra a recusa da conta de outra empresa", async () => {
    const fetchFn = mock();
    renderWithProviders(<ReconciliationTab company={company} />, { route: `/companies/${ID}?tab=reconciliation` });
    const card = await screen.findByRole("region", { name: "Contas bancárias" });
    expect(await within(card).findByRole("table", { name: "Contas cadastradas" })).toHaveTextContent("a_fonte 0001/12345-6");
    const read = within(card).getByRole("region", { name: "Contas lidas nos extratos" });
    expect(read).toHaveTextContent("1 extrato, até 31/10/2026");
    await userEvent.click(within(read).getByRole("button", { name: "Cadastrar a conta beta 0021/33412-7" }));
    expect(await screen.findByText("Conta beta 0021/33412-7 cadastrada")).toBeInTheDocument();
    const index = fetchFn.mock.calls.findIndex(([u, i]) => urlOf(u).endsWith("/bank-accounts") && i?.method === "POST");
    expect(requestBody(fetchFn, index)).toEqual({ bank: "beta", branch: "0021", account: "33412-7" });

    const form = within(card).getByRole("form", { name: "Nova conta bancária" });
    await userEvent.type(within(form).getByLabelText(/^Banco/), " itau ");
    await userEvent.type(within(form).getByLabelText(/^Agência/), "1234");
    await userEvent.type(within(form).getByLabelText(/^Conta/), "99999-9");
    await userEvent.click(within(form).getByRole("button", { name: "Cadastrar conta" }));
    expect(await within(card).findByText("Essa conta já está cadastrada na empresa 901 · Outra.")).toBeInTheDocument();
  });
});
