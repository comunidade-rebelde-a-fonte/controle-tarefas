import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReconciliationCompletion, ReconciliationCompletionRecord } from "@/services/reconciliation";
import { jsonResponse, requestBody, urlOf } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { Completion } from "./Completion";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ID = "00000000-0000-4000-8000-0000000000c1";

const obligation = {
  instance_id: "00000000-0000-4000-8000-0000000000f9", status_code: "NOT_STARTED", status_label: "A iniciar", is_final: false, applicable: true,
};

function completion(over: Partial<ReconciliationCompletion>): ReconciliationCompletion {
  return {
    state: "pronta",
    ready: true,
    blocking: { missing_statement: false, divergent: 0, without_receipt: 0, review: 0, classification: 0 },
    warnings: { without_statement: 0 },
    obligation,
    current: null,
    history: [],
    ...over,
  };
}

const record: ReconciliationCompletionRecord = {
  id: "00000000-0000-4000-8000-0000000000e1", completed_by: "Ana Souza", completed_at: "2026-10-05T15:00:00Z",
  obligation_status_before: "NOT_STARTED", obligation_sync: "concluida", reopened_by: null, reopened_at: null, reopen_reason: null,
  reopen_obligation_sync: null,
};

const concluded = completion({
  state: "concluida",
  obligation: { ...obligation, status_code: "OK", status_label: "OK", is_final: true },
  current: record,
});

describe("Completion", () => {
  it("F-008: o que falta impede concluir, com o caminho para a fila; comprovante sem extrato só avisa", () => {
    renderWithProviders(
      <Completion
        companyId={ID}
        competence="2026-10"
        accountingClosed={false}
        completion={completion({
          state: "em_andamento", ready: false,
          blocking: { missing_statement: false, divergent: 2, without_receipt: 1, review: 0, classification: 3 },
          warnings: { without_statement: 1 },
        })}
      />,
    );
    const card = screen.getByRole("region", { name: "Conclusão da conciliação" });
    expect(within(card).getByText("Em andamento")).toBeInTheDocument();
    const missing = within(card).getByRole("list", { name: "O que falta para concluir" });
    expect(within(missing).getByRole("link", { name: "2 linhas com divergência." })).toHaveAttribute(
      "href",
      `/reconciliation?company_id=${ID}&competence=2026-10&bucket=divergencia`,
    );
    expect(within(missing).getByRole("link", { name: "3 linhas à espera da conta." })).toBeInTheDocument();
    expect(card).toHaveTextContent("Aviso: 1 comprovante sem linha do extrato");
    expect(within(card).getByRole("button", { name: "Concluir a conciliação" })).toBeDisabled();
    expect(card).toHaveTextContent("Obrigação Conciliação (Contábil): A iniciar");
  });

  it("AC-023: pronta, o contador confirma e a conclusão vai para a API", async () => {
    const fetchFn = mockFetchRoutes(({ method, url }) =>
      method === "POST" && url.pathname === `/api/v1/companies/${ID}/reconciliation/complete` ? jsonResponse(concluded) : undefined,
    );
    renderWithProviders(<Completion companyId={ID} competence="2026-10" accountingClosed={false} completion={completion({})} />);
    const card = screen.getByRole("region", { name: "Conclusão da conciliação" });
    expect(card).toHaveTextContent("O extrato está todo resolvido: a conciliação está pronta para concluir.");
    await userEvent.click(within(card).getByRole("button", { name: "Concluir a conciliação" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Concluir a conciliação de 10/2026?" });
    expect(confirm).toHaveTextContent("A obrigação Conciliação do Contábil vai para OK");
    await userEvent.click(within(confirm).getByRole("button", { name: "Concluir" }));
    expect(await screen.findByText("Conciliação concluída")).toBeInTheDocument();
    expect(fetchFn.mock.calls.some(([u, i]) => urlOf(u).includes("/reconciliation/complete?competence=2026-10") && i?.method === "POST")).toBe(true);
  });

  it("a recusa da API aparece (dependência do Contábil)", async () => {
    mockFetchRoutes(({ method, url }) =>
      method === "POST" && url.pathname === `/api/v1/companies/${ID}/reconciliation/complete`
        ? jsonResponse(
            { code: "DEPENDENCY_BLOCKED", message: "Não é possível concluir Conciliação. A etapa Contábil digitado/processado ainda não foi concluída." },
            422,
          )
        : undefined,
    );
    renderWithProviders(<Completion companyId={ID} competence="2026-10" accountingClosed={false} completion={completion({})} />);
    await userEvent.click(screen.getByRole("button", { name: "Concluir a conciliação" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Concluir" }));
    expect(await screen.findByText(/A etapa Contábil digitado\/processado ainda não foi concluída/)).toBeInTheDocument();
  });

  it("AC-024: concluída, mostra quem concluiu e reabre com motivo", async () => {
    const fetchFn = mockFetchRoutes(({ method, url }) =>
      method === "POST" && url.pathname === `/api/v1/companies/${ID}/reconciliation/reopen` ? jsonResponse(completion({})) : undefined,
    );
    renderWithProviders(<Completion companyId={ID} competence="2026-10" accountingClosed={false} completion={concluded} />);
    const card = screen.getByRole("region", { name: "Conclusão da conciliação" });
    expect(within(card).getByText("Concluída")).toBeInTheDocument();
    expect(within(card).getByRole("status")).toHaveTextContent("Concluída por Ana Souza");
    expect(within(card).getByRole("status")).toHaveTextContent("a obrigação Conciliação foi para OK");
    expect(within(card).queryByRole("button", { name: "Concluir a conciliação" })).toBeNull();
    await userEvent.click(within(card).getByRole("button", { name: "Reabrir a conciliação" }));
    const dialog = await screen.findByRole("dialog", { name: "Reabrir a conciliação de 10/2026" });
    const reopen = within(dialog).getByRole("button", { name: "Reabrir" });
    expect(reopen).toBeDisabled();
    await userEvent.type(within(dialog).getByLabelText(/^Motivo/), "Chegou o extrato da poupança");
    await userEvent.click(reopen);
    expect(await screen.findByText("Conciliação reaberta")).toBeInTheDocument();
    const index = fetchFn.mock.calls.findIndex(([u, i]) => urlOf(u).includes("/reconciliation/reopen") && i?.method === "POST");
    expect(requestBody(fetchFn, index)).toEqual({ reason: "Chegou o extrato da poupança" });
  });

  it("com o Contábil fechado, não conclui nem reabre; o histórico mostra as reaberturas", async () => {
    const reopened: ReconciliationCompletionRecord = {
      ...record,
      id: "00000000-0000-4000-8000-0000000000e0",
      reopened_by: "Bruno Lima", reopened_at: "2026-10-03T12:00:00Z", reopen_reason: "faltava a tarifa", reopen_obligation_sync: "pendente",
    };
    renderWithProviders(
      <Completion
        companyId={ID}
        competence="2026-10"
        accountingClosed
        completion={{ ...concluded, history: [record, reopened] }}
      />,
    );
    const card = screen.getByRole("region", { name: "Conclusão da conciliação" });
    expect(within(card).queryByRole("button", { name: "Reabrir a conciliação" })).toBeNull();
    expect(card).toHaveTextContent("Com o Contábil fechado, reabra o Contábil antes de reabrir a conciliação.");
    await userEvent.click(within(card).getByText("1 reabertura no histórico"));
    await waitFor(() => expect(card).toHaveTextContent("reaberta por Bruno Lima"));
    expect(card).toHaveTextContent("“faltava a tarifa” — a obrigação voltou para Pendente.");
  });
});
