import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Ledger, LedgerLine, ReconciliationDetail } from "@/services/reconciliation";
import { jsonResponse, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { LedgerPage } from "./LedgerPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const COMPANY = "00000000-0000-4000-8000-0000000000c1";
const R1 = "00000000-0000-4000-8000-0000000000f1";
const R2 = "00000000-0000-4000-8000-0000000000f2";
const LINE = "00000000-0000-4000-8000-0000000000a1";
const HISTORY = "00000000-0000-4000-8000-0000000000a9";
const STATEMENT = "00000000-0000-4000-8000-0000000000d1";
const RECEIPT = "00000000-0000-4000-8000-0000000000d2";
const ENTRY = "00000000-0000-4000-8000-0000000000e1";
const MANUAL = "00000000-0000-4000-8000-0000000000e2";

function line(over: Partial<LedgerLine>): LedgerLine {
  return {
    date: "2026-08-10", competence: "2026-08", bank_account: "a_fonte 0001/12345-6", account_code: "4.1.01", account_name: "Fornecedores",
    classification_origin: "regra", classification_pending: false, history: "PIX ENVIADO FORNECEDOR ALFA", counterparty: "Fornecedor Alfa",
    debit: "1500.00", credit: "0.00", reconciliation_id: R1, reconciliation_method: "passe_1", reconciliation_origin: "auto",
    statement_transaction_id: LINE, statement_document_id: STATEMENT, entry_ids: [ENTRY], receipt_document_ids: [RECEIPT], ...over,
  };
}

const ledger: Ledger = {
  company_id: COMPANY, from: "2026-08", to: "2026-08", total_debit: "1500.00", total_credit: "320.00",
  items: [
    line({}),
    line({
      reconciliation_id: R2, date: "2026-08-12", history: "PIX RECEBIDO CLIENTE BETA", counterparty: null, debit: "0.00", credit: "320.00",
      account_code: null, account_name: null, classification_origin: null, entry_ids: [MANUAL], receipt_document_ids: [],
    }),
  ],
};

const detail: ReconciliationDetail = {
  id: R1, company_id: COMPANY, method: "passe_1", origin: "auto", pass: 1, score: "1.0000", proposal_id: null, created_by: null,
  created_at: "2026-08-15T12:00:00Z", undone_at: null,
  statement_transaction: {
    id: LINE, date: "2026-08-10", competence: "2026-08", description: "PIX ENVIADO FORNECEDOR ALFA", amount: "1500.00", direction: "D",
    bank_account: "a_fonte 0001/12345-6", statement_import_id: "00000000-0000-4000-8000-000000000011", statement_document_id: STATEMENT,
  },
  entries: [
    { id: ENTRY, date: "2026-08-10", amount: "1500.00", direction: "D", counterparty: "Fornecedor Alfa", counterparty_doc: null, document_ref: null, status: "extraido", document_id: RECEIPT },
    { id: MANUAL, date: "2026-08-10", amount: "0.00", direction: "D", counterparty: null, counterparty_doc: null, document_ref: null, status: "manual", document_id: null },
  ],
  classification: {
    account_code: "4.1.01", account_name: "Fornecedores", origin: "historico", pending: false, rule: null, history_line_id: HISTORY,
    suggestion_id: null, classified_by: null, classified_at: "2026-08-15T12:00:00Z",
  },
};

function mock() {
  return mockFetchRoutes(({ method, url }) => {
    if (method === "GET" && url.pathname === "/api/v1/companies") {
      return jsonResponse({ items: [{ id: COMPANY, internal_code: "900", legal_name: "Empresa 900" }], total: 1 });
    }
    if (method === "GET" && url.pathname === "/api/v1/competencies/latest") return jsonResponse({ competence: "2026-10", current: "2026-10" });
    if (method === "GET" && url.pathname === `/api/v1/companies/${COMPANY}/ledger`) return jsonResponse(ledger);
    if (method === "GET" && url.pathname === `/api/v1/reconciliation/reconciliations/${R1}`) return jsonResponse(detail);
    return undefined;
  });
}

describe("LedgerPage", () => {
  it("o razão do período com a conta, a origem da classificação, os totais e a exportação filtrada", async () => {
    const fetchFn = mock();
    renderWithProviders(<LedgerPage />, { route: `/reconciliation/ledger?company_id=${COMPANY}&from=2026-08&to=2026-08` });
    const table = await screen.findByRole("table", { name: "Linhas do razão" });
    const rows = within(table).getAllByRole("row");
    expect(rows[1]).toHaveTextContent("4.1.01 · Fornecedores");
    expect(rows[1]).toHaveTextContent("Regra");
    expect(rows[1]).toHaveTextContent("R$ 1.500,00");
    expect(rows[2]).toHaveTextContent("Sem classificação");
    expect(within(table).getByText("Total do período").closest("tr")).toHaveTextContent("R$ 1.500,00R$ 320,00");
    const exportLink = screen.getByRole("link", { name: "Exportar CSV" });
    expect(exportLink).toHaveAttribute("href", `/api/v1/companies/${COMPANY}/ledger/export?format=csv&from=2026-08&to=2026-08`);
    expect(screen.getByRole("link", { name: "Exportar XLSX" })).toHaveAttribute(
      "href",
      `/api/v1/companies/${COMPANY}/ledger/export?format=xlsx&from=2026-08&to=2026-08`,
    );

    await userEvent.type(screen.getByLabelText("Conta bancária ou contábil"), "4.1.01");
    await userEvent.click(screen.getByRole("button", { name: "Filtrar" }));
    await waitFor(() => expect(fetchFn.mock.calls.some(([u]) => urlOf(u).includes("account=4.1.01"))).toBe(true));
    expect(screen.getByRole("link", { name: "Exportar CSV" })).toHaveAttribute(
      "href",
      `/api/v1/companies/${COMPANY}/ledger/export?format=csv&from=2026-08&to=2026-08&account=4.1.01`,
    );
    expect(screen.getByRole("link", { name: "Exportar XLSX" })).toHaveAttribute(
      "href",
      `/api/v1/companies/${COMPANY}/ledger/export?format=xlsx&from=2026-08&to=2026-08&account=4.1.01`,
    );
  });

  it("sem período na URL vale a competência corrente", async () => {
    const fetchFn = mock();
    renderWithProviders(<LedgerPage />, { route: `/reconciliation/ledger?company_id=${COMPANY}` });
    await waitFor(() => expect(fetchFn.mock.calls.some(([u]) => urlOf(u).includes("/ledger?from=2026-10&to=2026-10"))).toBe(true));
  });

  it("período invertido não consulta", async () => {
    const fetchFn = mock();
    renderWithProviders(<LedgerPage />, { route: `/reconciliation/ledger?company_id=${COMPANY}&from=2026-09&to=2026-08` });
    expect(await screen.findByText("A competência inicial é depois da final.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Exportar CSV" })).toBeNull();
    expect(fetchFn.mock.calls.some(([u]) => urlOf(u).includes("/ledger?"))).toBe(false);
  });

  it("AC-021: o drill-down leva da linha à classificação, à conciliação, à transação e aos originais", async () => {
    mock();
    renderWithProviders(<LedgerPage />, { route: `/reconciliation/ledger?company_id=${COMPANY}&from=2026-08&to=2026-08` });
    await userEvent.click(await screen.findByRole("button", { name: "Origem da linha de 10/08/2026: PIX ENVIADO FORNECEDOR ALFA" }));
    const dialog = await screen.findByRole("dialog", { name: "Origem da linha do razão" });
    const trail = await within(dialog).findByRole("list", { name: "Rastro da linha do razão" });
    const [classification, reconciliation, transaction, documents] = within(trail)
      .getAllByRole("listitem")
      .filter((li) => li.parentElement === trail);
    if (!classification || !reconciliation || !transaction || !documents) throw new Error("rastro incompleto");
    expect(classification).toHaveTextContent("4.1.01 · Fornecedores");
    expect(classification).toHaveTextContent("Histórico");
    expect(within(classification).getByRole("link")).toHaveAttribute("href", `/reconciliation?item=${HISTORY}&kind=linha`);
    expect(reconciliation).toHaveTextContent("Automática, passe 1");
    expect(reconciliation).toHaveTextContent("pontuação 100%");
    expect(transaction).toHaveTextContent("PIX ENVIADO FORNECEDOR ALFA");
    expect(within(transaction).getByRole("link", { name: "Abrir a linha (desfazer, reclassificar)" })).toHaveAttribute(
      "href",
      `/reconciliation?item=${LINE}&kind=linha`,
    );
    expect(within(documents).getByRole("link", { name: "Extrato bancário" })).toHaveAttribute("href", `/reconciliation/documents/${STATEMENT}`);
    expect(within(documents).getByRole("link", { name: "Comprovante" })).toHaveAttribute("href", `/reconciliation/documents/${RECEIPT}`);
    expect(within(documents).getByText("Lançamento manual (sem original)")).toBeInTheDocument();
  });
});
