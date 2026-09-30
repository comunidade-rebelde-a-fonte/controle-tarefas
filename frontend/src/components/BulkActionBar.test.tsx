import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, requestBody } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { BulkActionBar, type GridColumn } from "./BulkActionBar";

afterEach(() => {
  vi.unstubAllGlobals();
});

const status = (code: string, label: string, sort: number) => ({
  code, label, sort_order: sort, counts_as_started: false, counts_as_operational_progress: false, is_final: false, is_blocking: false, excluded_from_denominator: false,
});

const columns: GridColumn[] = [
  {
    obligation_type: { id: "t-ret", code: "FISCAL_RETIDOS", name: "Retidos", primary_module: "FISCAL" },
    sort_order: 130, is_closing_stage: false, allows_transmission_date: false, allows_protocol: false, status_set_code: "FISCAL_ITEM",
    statuses: [status("NOT_STARTED", "A iniciar", 1), status("NO_MOVEMENT", "Sem movimento", 6)],
  },
];

describe("BulkActionBar", () => {
  it("CA-016 fluxo completo: obrigação → status → prévia → confirmar", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method, url, body }) => {
      if (method !== "POST" || url.pathname !== "/api/v1/obligation-instances/bulk-status") return undefined;
      const dry = (body as { dry_run?: boolean }).dry_run === true;
      return jsonResponse({
        dry_run: dry, selected: 3, updated: 2, soft_warnings: 0,
        skipped: [{ id: "i3", company_code: "103", company_name: "Gama Indústria", reason: "PERIOD_CLOSED", message: "Módulo fechado nesta competência." }],
      });
    });
    const onDone = vi.fn();
    const onClear = vi.fn();
    renderWithProviders(
      <BulkActionBar competence="2026-09" columns={columns} selectedCompanyIds={["c1", "c2", "c3"]} onClearSelection={onClear} onDone={onDone} />,
    );

    const bar = screen.getByRole("region", { name: "Ações em massa" });
    expect(bar).toHaveTextContent("3 empresas selecionadas");
    expect(within(bar).getByRole("button", { name: "Pré-visualizar" })).toBeDisabled();
    await user.selectOptions(screen.getByLabelText("Obrigação"), "t-ret");
    await user.selectOptions(screen.getByLabelText("Status"), "NO_MOVEMENT");
    await user.click(screen.getByRole("button", { name: "Pré-visualizar" }));

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("2 empresas → Retidos → Sem movimento");
    expect(within(dialog).getByRole("list", { name: "Empresas puladas" })).toHaveTextContent("Gama Indústria — Módulo fechado nesta competência.");
    expect(requestBody(fetchFn, 0)).toEqual({
      target: { competence: "2026-09", obligation_type_id: "t-ret", company_ids: ["c1", "c2", "c3"] },
      status_code: "NO_MOVEMENT",
      dry_run: true,
    });

    await user.click(within(dialog).getByRole("button", { name: "Confirmar" }));
    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(requestBody(fetchFn, 1)).toEqual({
      target: { competence: "2026-09", obligation_type_id: "t-ret", company_ids: ["c1", "c2", "c3"] },
      status_code: "NO_MOVEMENT",
    });
    expect(onClear).toHaveBeenCalled();
    expect(await screen.findByText("2 obrigações atualizadas")).toBeInTheDocument();
  });

  it("pré-requisitos SOFT exigem motivo da exceção", async () => {
    const user = userEvent.setup();
    const fetchFn = mockFetchRoutes(({ method, body }) =>
      method === "POST"
        ? jsonResponse({ dry_run: (body as { dry_run?: boolean }).dry_run === true, selected: 2, updated: 2, soft_warnings: 2, skipped: [] })
        : undefined,
    );
    renderWithProviders(<BulkActionBar competence="2026-09" columns={columns} selectedCompanyIds={["c1", "c2"]} onClearSelection={() => undefined} />);
    await user.selectOptions(screen.getByLabelText("Obrigação"), "t-ret");
    await user.selectOptions(screen.getByLabelText("Status"), "NO_MOVEMENT");
    await user.click(screen.getByRole("button", { name: "Pré-visualizar" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Confirmar" }));
    expect(await within(dialog).findByText("Informe o motivo da exceção.")).toBeInTheDocument();
    expect(fetchFn).toHaveBeenCalledTimes(1);

    await user.type(within(dialog).getByLabelText(/Motivo da exceção/), "Componentes transmitidos pelo cliente");
    await user.click(within(dialog).getByRole("button", { name: "Confirmar" }));
    await waitFor(() => expect(fetchFn).toHaveBeenCalledTimes(2));
    expect(requestBody(fetchFn, 1)).toMatchObject({ confirm_exception: true, exception_reason: "Componentes transmitidos pelo cliente" });
  });

  it("não aparece sem seleção", () => {
    renderWithProviders(<BulkActionBar competence="2026-09" columns={columns} selectedCompanyIds={[]} onClearSelection={() => undefined} />);
    expect(screen.queryByRole("region", { name: "Ações em massa" })).not.toBeInTheDocument();
  });
});
