import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { GridFilters, type GridFiltersProps } from "./GridFilters";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ANA = "7f3c2a10-1b2c-4d5e-8f90-1234567890ab";
const ICMS = "0b1c2d3e-4f50-4617-a8b9-c0d1e2f3a4b5";

function mockUsers() {
  mockFetchRoutes(({ method, url }) =>
    method === "GET" && url.pathname === "/api/v1/users"
      ? jsonResponse({ items: [{ id: ANA, name: "Ana Souza", email: null, role: "OPERATOR", active: true, created_at: "", updated_at: "" }] })
      : undefined,
  );
}

const props: GridFiltersProps = {
  statusOptions: [
    { value: "NOT_STARTED", label: "A iniciar" },
    { value: "MISSING_DOCUMENTS", label: "Documentos faltantes" },
  ],
  columnOptions: [{ value: ICMS, label: "ICMS" }],
  closingStatusOptions: [{ value: "CHECKED", label: "Conferido" }],
};

function renderFilters(route: string, p: Partial<GridFiltersProps> = {}) {
  return renderWithProviders(<GridFilters {...props} {...p} />, { route, path: "/fiscal" });
}

const paramsOf = (router: ReturnType<typeof renderFilters>["router"]) => Object.fromEntries(new URLSearchParams(router.state.location.search));

describe("GridFilters", () => {
  it("aplicar filtros grava tudo na URL e recarregar a página mantém os filtros", async () => {
    mockUsers();
    const user = userEvent.setup();
    const first = renderFilters("/fiscal?competence=2026-09");

    await user.selectOptions(screen.getByLabelText("Regime"), "SIMPLES_NACIONAL");
    await screen.findByRole("option", { name: "Ana Souza" });
    await user.selectOptions(screen.getByLabelText("Responsável"), "Ana Souza");
    await user.selectOptions(screen.getByLabelText("Status da obrigação"), "Documentos faltantes");
    await user.selectOptions(screen.getByLabelText("Coluna do status"), "ICMS");
    await user.selectOptions(screen.getByLabelText("Status de fechamento"), "Conferido");
    await user.selectOptions(screen.getByLabelText("Módulo aberto ou fechado"), "Aberto");
    await user.selectOptions(screen.getByLabelText("Observação"), "Sem observação");
    await user.click(screen.getByRole("checkbox", { name: "Somente atrasadas" }));
    await user.click(screen.getByRole("checkbox", { name: "Somente prontas para fechar" }));
    await user.type(screen.getByRole("searchbox", { name: "Buscar empresa" }), "alfa");
    await waitFor(() => expect(paramsOf(first.router).q).toBe("alfa"));

    const url = first.router.state.location.pathname + first.router.state.location.search;
    expect(paramsOf(first.router)).toEqual({
      competence: "2026-09",
      tax_regime: "SIMPLES_NACIONAL",
      responsible_id: ANA,
      status: "MISSING_DOCUMENTS",
      obligation_type_id: ICMS,
      closing_status: "CHECKED",
      period_state: "OPEN",
      note: "without",
      overdue: "true",
      ready_to_close: "true",
      q: "alfa",
    });

    // "Recarrega": desmonta e abre de novo com a mesma URL.
    first.unmount();
    renderFilters(url);
    expect(screen.getByLabelText("Regime")).toHaveValue("SIMPLES_NACIONAL");
    await waitFor(() => expect(screen.getByLabelText("Responsável")).toHaveValue(ANA));
    expect(screen.getByLabelText("Status da obrigação")).toHaveValue("MISSING_DOCUMENTS");
    expect(screen.getByLabelText("Coluna do status")).toHaveValue(ICMS);
    expect(screen.getByLabelText("Status de fechamento")).toHaveValue("CHECKED");
    expect(screen.getByLabelText("Módulo aberto ou fechado")).toHaveValue("OPEN");
    expect(screen.getByLabelText("Observação")).toHaveValue("without");
    expect(screen.getByRole("checkbox", { name: "Somente atrasadas" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Somente pendentes" })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Somente prontas para fechar" })).toBeChecked();
    expect(screen.getByRole("searchbox", { name: "Buscar empresa" })).toHaveValue("alfa");
  });

  it("'Limpar filtros' remove os filtros e mantém competência e agrupamento", async () => {
    mockUsers();
    const user = userEvent.setup();
    const { router } = renderFilters("/fiscal?competence=2026-09&group=simples&q=beta&overdue=true&note=with&tax_regime=LUCRO_REAL");
    expect(screen.getByRole("searchbox", { name: "Buscar empresa" })).toHaveValue("beta");

    await user.click(screen.getByRole("button", { name: "Limpar filtros" }));
    expect(paramsOf(router)).toEqual({ competence: "2026-09", group: "simples" });
    expect(screen.getByRole("searchbox", { name: "Buscar empresa" })).toHaveValue("");
    expect(screen.getByRole("checkbox", { name: "Somente atrasadas" })).not.toBeChecked();
    expect(screen.getByRole("button", { name: "Limpar filtros" })).toBeDisabled();
  });

  it("filtros opcionais só aparecem quando fazem sentido", () => {
    mockUsers();
    renderFilters("/fiscal", { closingStatusOptions: undefined, showRegime: false });
    expect(screen.queryByLabelText("Regime")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Status de fechamento")).not.toBeInTheDocument();
    // A coluna do status só aparece com um status escolhido.
    expect(screen.queryByLabelText("Coluna do status")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Limpar filtros" })).toBeDisabled();
  });

  it("anuncia o total de empresas exibidas", () => {
    mockUsers();
    renderFilters("/fiscal", { resultCount: 1 });
    expect(screen.getByText("1 empresa")).toBeInTheDocument();
  });
});
