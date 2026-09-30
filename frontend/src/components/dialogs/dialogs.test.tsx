import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api";
import type { ObligationInstance } from "@/services/obligations";
import type { ModulePeriod } from "@/services/periods";
import { jsonResponse, requestBody } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CellDetailsPopover } from "./CellDetailsPopover";
import { CloseModuleDialog } from "./CloseModuleDialog";
import { DependencyWarningDialog } from "./DependencyWarningDialog";
import { DueDateOverrideDialog } from "./DueDateOverrideDialog";
import { HistoryDrawer } from "./HistoryDrawer";
import { ReopenDialog } from "./ReopenDialog";

afterEach(() => {
  vi.unstubAllGlobals();
});

const instance: ObligationInstance = {
  id: "i1",
  company_id: "c1",
  competence: "2026-09",
  obligation_type: { id: "t1", code: "DCTFWEB", name: "DCTFWeb", primary_module: "DECLARATIONS" },
  status_code: "NOT_STARTED",
  status_label: "A iniciar",
  applicable: true,
  counts_in_progress: true,
  is_final: false,
  is_started: false,
  is_blocking: false,
  is_overdue: false,
  due_date_original: "2026-10-15",
  due_date_effective: "2026-10-15",
  due_overridden: false,
  due_date_override_reason: null,
  transmission_date: null,
  protocol: null,
  has_history: false,
  version: 4,
  last_changed_at: null,
  last_changed_by: null,
};

const changeResult = {
  instance: { ...instance, status_code: "TRANSMITTED", version: 5 },
  row: { module: "DECLARATIONS", progress: { denominator: 1, operational_done: 1, final_done: 1, blocking: 0, operational_progress: 1, final_completion: 1, ready_to_close: true }, overdue: 0, nearest_deadline: null },
};

const period = (patch: Partial<ModulePeriod> = {}): ModulePeriod => ({
  id: "p1",
  company_id: "c1",
  competence: "2026-09",
  module: "FISCAL",
  state: "OPEN",
  closed_at: null,
  closed_by: null,
  general_note: null,
  version: 2,
  ...patch,
});

const target = { periodId: "p1", version: 2, companyName: "Alfa Comércio", moduleLabel: "Fiscal", competence: "2026-09" };

describe("ReopenDialog", () => {
  it("CA-015 não confirma a reabertura sem motivo", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method, url }) =>
      method === "POST" && url.pathname === "/api/v1/company-module-periods/p1/reopen" ? jsonResponse(period({ version: 3 })) : undefined,
    );
    const onReopened = vi.fn();
    const onClose = vi.fn();
    renderWithProviders(<ReopenDialog target={target} onClose={onClose} onReopened={onReopened} />);

    await user.click(screen.getByRole("button", { name: "Reabrir" }));
    expect(await screen.findByText("Informe o motivo da reabertura.")).toBeInTheDocument();
    await user.type(screen.getByLabelText(/Motivo da reabertura/), "   ");
    await user.click(screen.getByRole("button", { name: "Reabrir" }));
    expect(screen.getByText("Informe o motivo da reabertura.")).toBeInTheDocument();
    expect(fetchFn).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText(/Motivo da reabertura/), "Correção de obrigação transmitida");
    await user.click(screen.getByRole("button", { name: "Reabrir" }));
    await waitFor(() => expect(onReopened).toHaveBeenCalled());
    expect(requestBody(fetchFn)).toEqual({ version: 2, reason: "Correção de obrigação transmitida" });
    expect(onClose).toHaveBeenCalled();
  });
});

describe("DependencyWarningDialog", () => {
  const softError = new ApiError(409, {
    code: "DEPENDENCY_WARNING",
    message: "DCTFWeb possui pré-requisitos pendentes: eSocial.",
    details: { pending: [{ obligation_type_id: "t2", code: "ESOCIAL", name: "eSocial", status_code: "NOT_STARTED" }] },
  });

  it("CA-011 não conclui com exceção sem motivo e registra o motivo informado", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method, url }) =>
      method === "PATCH" && url.pathname === "/api/v1/obligation-instances/i1/status" ? jsonResponse(changeResult) : undefined,
    );
    const onResolved = vi.fn();
    renderWithProviders(
      <DependencyWarningDialog event={{ error: softError, instance, statusCode: "TRANSMITTED" }} onClose={() => undefined} onResolved={onResolved} />,
    );

    expect(screen.getByText("DCTFWeb possui pré-requisitos pendentes: eSocial.")).toBeInTheDocument();
    expect(screen.getByRole("list", { name: "Pré-requisitos pendentes" })).toHaveTextContent("eSocial");
    await user.click(screen.getByRole("button", { name: "Concluir com exceção" }));
    expect(await screen.findByText("Informe o motivo da exceção.")).toBeInTheDocument();
    expect(fetchFn).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText(/Motivo da exceção/), "eSocial transmitido pelo cliente");
    await user.click(screen.getByRole("button", { name: "Concluir com exceção" }));
    await waitFor(() => expect(onResolved).toHaveBeenCalledWith(changeResult));
    expect(requestBody(fetchFn)).toEqual({
      status_code: "TRANSMITTED",
      version: 4,
      confirm_exception: true,
      exception_reason: "eSocial transmitido pelo cliente",
    });
  });

  it("CA-010 dependência HARD informa as etapas e não oferece exceção", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const hard = new ApiError(422, {
      code: "DEPENDENCY_BLOCKED",
      message: "Não é possível concluir Conciliação. A etapa Contábil digitado/processado ainda não foi concluída.",
      details: { pending: [{ obligation_type_id: "t3", code: "ACCOUNTING_PROCESSED", name: "Contábil digitado/processado", status_code: "NOT_STARTED" }] },
    });
    renderWithProviders(<DependencyWarningDialog event={{ error: hard, instance, statusCode: "OK" }} onClose={onClose} />);
    expect(screen.getByRole("list", { name: "Pré-requisitos pendentes" })).toHaveTextContent("Contábil digitado/processado");
    expect(screen.queryByLabelText(/Motivo/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Concluir com exceção" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Entendi" }));
    expect(onClose).toHaveBeenCalled();
  });
});

describe("CloseModuleDialog", () => {
  it("pede a confirmação do Fechamento no Fiscal e fecha no mesmo ato", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method, url, body }) => {
      if (method !== "POST" || url.pathname !== "/api/v1/company-module-periods/p1/close") return undefined;
      const confirmed = (body as { confirm_set_closing?: boolean }).confirm_set_closing;
      return confirmed
        ? jsonResponse(period({ state: "CLOSED", version: 3 }))
        : jsonResponse(
            { code: "CLOSING_STATUS_REQUIRED", message: "Fechamento está como Conferido. Confirme para marcá-lo como Encerrado e fechar a competência.", details: {} },
            409,
          );
    });
    const onClosed = vi.fn();
    renderWithProviders(<CloseModuleDialog target={target} onClose={() => undefined} onClosed={onClosed} />);

    expect(screen.getByText(/O fechamento vale só para este módulo/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Fechar competência" }));
    expect(await screen.findByText(/Fechamento está como Conferido/)).toBeInTheDocument();
    expect(onClosed).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Atualizar e fechar" }));
    await waitFor(() => expect(onClosed).toHaveBeenCalled());
    expect([requestBody(fetchFn, 0), requestBody(fetchFn, 1)]).toEqual([
      { version: 2, confirm_set_closing: false },
      { version: 2, confirm_set_closing: true },
    ]);
  });
});

describe("DueDateOverrideDialog", () => {
  it("exige motivo e envia a nova data", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method, url }) =>
      method === "PATCH" && url.pathname === "/api/v1/obligation-instances/i1/details" ? jsonResponse(changeResult) : undefined,
    );
    const onSaved = vi.fn();
    renderWithProviders(<DueDateOverrideDialog instance={instance} onClose={() => undefined} onSaved={onSaved} />);

    const date = screen.getByLabelText(/Novo vencimento/);
    await user.clear(date);
    await user.type(date, "2026-10-20");
    await user.click(screen.getByRole("button", { name: "Salvar vencimento" }));
    expect(await screen.findByText("Informe o motivo da alteração do vencimento.")).toBeInTheDocument();
    expect(fetchFn).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText(/Motivo da alteração/), "Prorrogação pela Receita");
    await user.click(screen.getByRole("button", { name: "Salvar vencimento" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(requestBody(fetchFn)).toEqual({
      version: 4,
      due_date_effective: "2026-10-20",
      due_date_override_reason: "Prorrogação pela Receita",
    });
  });
});

describe("HistoryDrawer", () => {
  it("lista as alterações com rótulos, origem e motivo", async () => {
    mockFetchRoutes(({ method, url }) =>
      method === "GET" && url.pathname === "/api/v1/obligation-instances/i1/history"
        ? jsonResponse({
            items: [
              {
                id: "h2", field: "STATUS", old_value: "NOT_STARTED", new_value: "TRANSMITTED", old_label: "A iniciar", new_label: "Transmitida",
                change_source: "BULK", override_reason: "Cliente transmitiu", user: { id: "u1", name: "Administrador" }, created_at: "2026-09-10T13:00:00Z",
              },
              {
                id: "h1", field: "DUE_DATE", old_value: "2026-10-15", new_value: "2026-10-20", old_label: null, new_label: null,
                change_source: "INLINE", override_reason: "Prorrogação", user: { id: "u1", name: "Administrador" }, created_at: "2026-09-09T13:00:00Z",
              },
            ],
          })
        : undefined,
    );
    renderWithProviders(<HistoryDrawer instanceId="i1" title="DCTFWeb — Alfa Comércio" open onOpenChange={() => undefined} />);
    const list = await screen.findByRole("list", { name: "Alterações" });
    expect(list).toHaveTextContent("Status: A iniciar → Transmitida");
    expect(list).toHaveTextContent("Administrador · Em massa");
    expect(list).toHaveTextContent("Motivo: Cliente transmitiu");
    expect(list).toHaveTextContent("Vencimento: 15/10/2026 → 20/10/2026");
  });
});

describe("CellDetailsPopover", () => {
  it("salva só o protocolo alterado", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method, url }) =>
      method === "PATCH" && url.pathname === "/api/v1/obligation-instances/i1/details" ? jsonResponse(changeResult) : undefined,
    );
    const onChanged = vi.fn();
    renderWithProviders(
      <CellDetailsPopover instance={instance} allowsTransmissionDate allowsProtocol onChanged={onChanged}>
        <button type="button">Detalhes</button>
      </CellDetailsPopover>,
    );
    await user.click(screen.getByRole("button", { name: "Detalhes" }));
    await user.type(await screen.findByLabelText("Protocolo"), "  ABC123 ");
    await user.click(screen.getByRole("button", { name: "Salvar" }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    expect(requestBody(fetchFn)).toEqual({ version: 4, protocol: "ABC123" });
  });

  it("módulo fechado mostra somente leitura", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <CellDetailsPopover instance={{ ...instance, protocol: "XYZ" }} allowsTransmissionDate allowsProtocol locked onEditDueDate={() => undefined}>
        <button type="button">Detalhes</button>
      </CellDetailsPopover>,
    );
    await user.click(screen.getByRole("button", { name: "Detalhes" }));
    expect(await screen.findByText("XYZ")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Salvar" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Alterar vencimento/ })).not.toBeInTheDocument();
  });
});
