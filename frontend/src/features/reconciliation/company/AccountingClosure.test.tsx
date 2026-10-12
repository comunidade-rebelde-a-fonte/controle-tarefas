import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Company } from "@/services/companies";
import type { ReconciliationClosure } from "@/services/reconciliation";
import { jsonResponse, requestBody, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { AccountingClosure } from "./AccountingClosure";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ID = "00000000-0000-4000-8000-0000000000c1";
const PERIOD = "00000000-0000-4000-8000-0000000000a9";
const company = { id: ID, internal_code: "900", legal_name: "Construtora Horizonte Ltda" } as unknown as Company;

function period(state: "OPEN" | "CLOSED") {
  return {
    id: PERIOD, company_id: ID, competence: "2026-10", module: "ACCOUNTING", state,
    closed_at: state === "CLOSED" ? "2026-10-31T18:00:00Z" : null,
    closed_by: state === "CLOSED" ? { id: "u1", name: "Ana Souza" } : null, general_note: null, version: 3,
  };
}

function closure(state: "OPEN" | "CLOSED", over: Partial<ReconciliationClosure> = {}): ReconciliationClosure {
  return {
    company_id: ID, competence: "2026-10", period: period(state) as ReconciliationClosure["period"], requires_reconciliation: true,
    events: [
      { id: "e2", event_type: "REOPENED", reason: "Faltou a tarifa", user: { id: "u2", name: "Bruno Lima" }, created_at: "2026-10-20T12:00:00Z" },
      { id: "e1", event_type: "OPENED", reason: null, user: { id: "u1", name: "Ana Souza" }, created_at: "2026-10-01T12:00:00Z" },
    ],
    ...over,
  };
}

describe("AccountingClosure", () => {
  it("aberto: avisa a exigência da Conciliação, mostra o histórico e fecha pelo endpoint do Contábil", async () => {
    let state: "OPEN" | "CLOSED" = "OPEN";
    const fetchFn = mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === `/api/v1/companies/${ID}/reconciliation/closure`) return jsonResponse(closure(state));
      if (method === "POST" && url.pathname === `/api/v1/company-module-periods/${PERIOD}/close`) {
        state = "CLOSED";
        return jsonResponse(period("CLOSED"));
      }
      return undefined;
    });
    renderWithProviders(<AccountingClosure company={company} competence="2026-10" />);
    const card = await screen.findByRole("region", { name: "Fechamento do Contábil" });
    expect(await within(card).findByText("Aberto")).toBeInTheDocument();
    expect(card).toHaveTextContent("o Contábil só fecha com a obrigação Conciliação concluída");
    await userEvent.click(within(card).getByText("Histórico do fechamento"));
    expect(within(card).getByRole("list", { name: "Histórico do fechamento do Contábil" })).toHaveTextContent("Reaberto por Bruno Lima");
    expect(card).toHaveTextContent("“Faltou a tarifa”");

    await userEvent.click(within(card).getByRole("button", { name: "Fechar o Contábil" }));
    const dialog = await screen.findByRole("dialog", { name: "Fechar Contábil de 10/2026" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Fechar competência" }));
    expect(await screen.findByText("Contábil fechado")).toBeInTheDocument();
    const index = fetchFn.mock.calls.findIndex(([u, i]) => urlOf(u).endsWith(`/company-module-periods/${PERIOD}/close`) && i?.method === "POST");
    expect(requestBody(fetchFn, index)).toMatchObject({ version: 3 });
    expect(await within(card).findByText("Fechado")).toBeInTheDocument();
    expect(card).toHaveTextContent("Fechado por Ana Souza");
  });

  it("a recusa do fechamento com a Conciliação não concluída aparece no diálogo", async () => {
    mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === `/api/v1/companies/${ID}/reconciliation/closure`) return jsonResponse(closure("OPEN"));
      if (method === "POST" && url.pathname === `/api/v1/company-module-periods/${PERIOD}/close`) {
        return jsonResponse(
          {
            code: "RECONCILIATION_NOT_FINAL",
            message: "A Conciliação de 10/2026 não está concluída (a obrigação Conciliação está como A iniciar). Conclua a conciliação antes de fechar o Contábil.",
          },
          422,
        );
      }
      return undefined;
    });
    renderWithProviders(<AccountingClosure company={company} competence="2026-10" />);
    await userEvent.click(await screen.findByRole("button", { name: "Fechar o Contábil" }));
    const dialog = await screen.findByRole("dialog", { name: "Fechar Contábil de 10/2026" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Fechar competência" }));
    expect(await within(dialog).findByText(/Conclua a conciliação antes de fechar o Contábil/)).toBeInTheDocument();
  });

  it("fechado: reabre com motivo pelo endpoint do Contábil", async () => {
    const fetchFn = mockFetchRoutes(({ method, url }) => {
      if (method === "GET" && url.pathname === `/api/v1/companies/${ID}/reconciliation/closure`) return jsonResponse(closure("CLOSED"));
      if (method === "POST" && url.pathname === `/api/v1/company-module-periods/${PERIOD}/reopen`) return jsonResponse(period("OPEN"));
      return undefined;
    });
    renderWithProviders(<AccountingClosure company={company} competence="2026-10" />);
    const card = await screen.findByRole("region", { name: "Fechamento do Contábil" });
    expect(await within(card).findByText("Fechado por Ana Souza em", { exact: false })).toBeInTheDocument();
    await userEvent.click(within(card).getByRole("button", { name: "Reabrir o Contábil" }));
    const dialog = await screen.findByRole("dialog", { name: "Reabrir Contábil" });
    await userEvent.type(within(dialog).getByLabelText(/^Motivo da reabertura/), "Chegou o extrato da poupança");
    await userEvent.click(within(dialog).getByRole("button", { name: "Reabrir" }));
    const index = await vi.waitFor(() => {
      const i = fetchFn.mock.calls.findIndex(([u, init]) => urlOf(u).endsWith(`/company-module-periods/${PERIOD}/reopen`) && init?.method === "POST");
      if (i < 0) throw new Error("sem reabertura");
      return i;
    });
    expect(requestBody(fetchFn, index)).toMatchObject({ reason: "Chegou o extrato da poupança" });
  });

  it("sem período gerado, só avisa", async () => {
    mockFetchRoutes(({ method, url }) =>
      method === "GET" && url.pathname === `/api/v1/companies/${ID}/reconciliation/closure`
        ? jsonResponse(closure("OPEN", { period: null, events: [], requires_reconciliation: false }))
        : undefined,
    );
    renderWithProviders(<AccountingClosure company={company} competence="2026-10" />);
    expect(await screen.findByText("O Contábil não tem período gerado para a empresa nesta competência.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Fechar o Contábil" })).toBeNull();
  });
});
