import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReviewLineDetail, ReviewQueue } from "@/services/reconciliation";
import { jsonResponse, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { reasonText } from "../labels";
import { ReviewQueuePage } from "./ReviewQueuePage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const LINE = "00000000-0000-4000-8000-0000000000a1";
const LINE2 = "00000000-0000-4000-8000-0000000000a2";
const E1 = "00000000-0000-4000-8000-0000000000e1";
const E2 = "00000000-0000-4000-8000-0000000000e2";
const DOC = "00000000-0000-4000-8000-0000000000d1";
const COMPANY = "00000000-0000-4000-8000-0000000000c1";

const queue: ReviewQueue = {
  items: [
    {
      kind: "linha", id: LINE, bucket: "divergencia", company_id: COMPANY, company_code: "900", company_name: "Empresa 900", state: "divergencia",
      date: "2026-08-10", amount: "300.00", direction: "D", summary: "PIX ENVIADO DOIS BOLETOS", reason: "ambiguo_passe_1", competence: "2026-08",
    },
    {
      kind: "linha", id: LINE2, bucket: "extrato_sem", company_id: COMPANY, company_code: "900", company_name: "Empresa 900", state: "livre",
      date: "2026-08-25", amount: "35.00", direction: "D", summary: "TARIFA PACOTE", reason: null, competence: "2026-08",
    },
  ],
  counts: { total: 2, revisao: 0, divergencia: 1, extrato_sem: 1, comprovante_sem: 0, classificacao: 0 },
};

const line: ReviewLineDetail = {
  line: {
    id: LINE, company_id: COMPANY, company_code: "900", company_name: "Empresa 900", date: "2026-08-10", competence: "2026-08",
    description: "PIX ENVIADO DOIS BOLETOS", document_ref: null, amount: "300.00", direction: "D", balance_after: null,
    bank_account: "a_fonte 0001/12345-6", statement_import_id: "00000000-0000-4000-8000-000000000011", statement_document_id: DOC,
    marks: [{ tipo: "divergencia", motivo: "ambiguo_passe_1", candidatos: [E1] }], account_code: null, classification_origin: null,
    classification_pending: false,
  },
  reconciliation: null,
  candidates: [
    { id: E1, date: "2026-08-10", amount: "120.00", direction: "D", counterparty: "Alfa", counterparty_doc: null, document_ref: null, status: "manual",
      document_id: null, operation_type: null, days_apart: 0, annotated: true },
    { id: E2, date: "2026-08-09", amount: "180.00", direction: "D", counterparty: "Alfa", counterparty_doc: null, document_ref: null, status: "extraido",
      document_id: null, operation_type: null, days_apart: 1, annotated: false },
  ],
};

function mock() {
  return mockFetchRoutes(({ method, url, body }) => {
    if (method === "GET" && url.pathname === "/api/v1/reconciliation/review-queue") {
      const bucket = url.searchParams.get("bucket");
      return jsonResponse(bucket ? { ...queue, items: queue.items.filter((i) => i.bucket === bucket) } : queue);
    }
    if (method === "GET" && url.pathname === `/api/v1/reconciliation/review/${LINE}`) return jsonResponse(line);
    if (method === "GET" && url.pathname === `/api/v1/reconciliation/review/${LINE}/proposals`) return jsonResponse({ items: [] });
    if (method === "GET" && url.pathname === `/api/v1/reconciliation/documents/${DOC}/pages`) {
      return jsonResponse({ content_type: "text/plain; charset=utf-8", pages: 0, dpi: 150 });
    }
    if (method === "GET" && url.pathname === `/api/v1/reconciliation/documents/${DOC}/file`) return new Response("data;descricao;valor\n");
    if (method === "GET" && url.pathname === "/api/v1/reconciliation/reconciliations") return jsonResponse({ items: [] });
    if (method === "GET" && url.pathname === "/api/v1/companies") return jsonResponse({ items: [], total: 0 });
    if (method === "POST" && url.pathname === `/api/v1/reconciliation/review/${LINE}/manual-match`) {
      expect(body).toEqual({ entry_ids: [E1, E2] });
      return jsonResponse({ reconciliation_id: "r1", method: "manual", entries_total: "300.00", difference: "0.00", classification: "agente" }, 201);
    }
    return undefined;
  });
}

describe("ReviewQueuePage", () => {
  it("08 §7: os baldes com a contagem e os itens com o motivo em texto", async () => {
    mock();
    renderWithProviders(<ReviewQueuePage />, { route: "/reconciliation" });
    const table = await screen.findByRole("table", { name: "Itens da fila" });
    expect(within(table).getByText("PIX ENVIADO DOIS BOLETOS")).toBeInTheDocument();
    expect(within(table).getByText(reasonText("ambiguo_passe_1"))).toBeInTheDocument();
    const buckets = screen.getByRole("group", { name: "Filtrar por balde" });
    expect(within(buckets).getByRole("button", { name: "Todos (2)" })).toHaveAttribute("aria-pressed", "true");
    expect(within(buckets).getByRole("button", { name: "Divergências (1)" })).toBeInTheDocument();
    await userEvent.click(within(buckets).getByRole("button", { name: "Extrato sem comprovante (1)" }));
    await waitFor(() => expect(within(screen.getByRole("table", { name: "Itens da fila" })).queryByText("PIX ENVIADO DOIS BOLETOS")).toBeNull());
  });

  it("abre a linha com o original ao lado, os candidatos e concilia os escolhidos (N:1)", async () => {
    const fetchFn = mock();
    renderWithProviders(<ReviewQueuePage />, { route: "/reconciliation" });
    await userEvent.click(await screen.findByRole("button", { name: "PIX ENVIADO DOIS BOLETOS" }));
    const panel = await screen.findByRole("region", { name: "Item em revisão" });
    expect(await within(panel).findByLabelText("Conteúdo do original")).toHaveTextContent("data;descricao;valor");
    expect(within(panel).getByText("anotado pelo matching")).toBeInTheDocument();
    const [first, second] = within(panel).getAllByRole("checkbox");
    if (!first || !second) throw new Error("candidatos não exibidos");
    await userEvent.click(first);
    await userEvent.click(second);
    await userEvent.click(within(panel).getByRole("button", { name: /Conciliar os 2 selecionados/ }));
    await waitFor(() => expect(fetchFn.mock.calls.some(([u, i]) => urlOf(u).includes("/manual-match") && i?.method === "POST")).toBe(true));
  });

  it("J passa para o próximo item e Esc fecha", async () => {
    mock();
    renderWithProviders(<ReviewQueuePage />, { route: `/reconciliation?item=${LINE}&kind=linha` });
    expect(await screen.findByRole("region", { name: "Item em revisão" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "PIX ENVIADO DOIS BOLETOS" })).toHaveAttribute("aria-current", "true");
    await userEvent.keyboard("j");
    await waitFor(() => expect(screen.getByRole("button", { name: "TARIFA PACOTE" })).toHaveAttribute("aria-current", "true"));
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("region", { name: "Item em revisão" })).toBeNull());
  });
});
