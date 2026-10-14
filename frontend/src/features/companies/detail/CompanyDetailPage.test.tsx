import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Company, CompanyOverview } from "@/services/companies";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { CompanyDetailPage } from "./CompanyDetailPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const A = "00000000-0000-4000-8000-00000000000a";
const B = "00000000-0000-4000-8000-00000000000b";

function company(id: string, code: string, name: string): Company {
  return {
    id, internal_code: code, legal_name: name, trade_name: null, cnpj: "00000001000101", city: null, state: null,
    state_registration: null, municipal_registration: null, service_start_date: "2025-01-01", service_end_date: null, active: true,
    payroll_type: "PAYROLL", factor_r: null, responsibles: { fiscal: null, accounting: null, payroll: null, declarations: null }, notes: null,
    current_tax_regime: "LUCRO_PRESUMIDO", current_regime_group: "NORMAL", active_modules: ["FISCAL"], version: 3,
  } as unknown as Company;
}

const companies: Record<string, Company> = { [A]: company(A, "101", "Alfa Comércio Ltda"), [B]: company(B, "102", "Beta Serviços Ltda") };

function overview(id: string, competence: string): CompanyOverview {
  const c = companies[id] ?? company(id, "000", "?");
  return {
    company: { id, internal_code: c.internal_code, legal_name: c.legal_name },
    competence,
    competence_generated: competence <= "2026-09",
    modules: [
      {
        module: "FISCAL", label: "Fiscal", period_state: "OPEN", applicable: 4, done: 1, pending: 1, overdue: 2,
        operational_progress: 0.5, final_completion: 0.25, drilldown: { path: "/fiscal", query: { competence, q: c.internal_code, group: "normal" } },
      },
    ],
    next_due: [],
    factor_r: null,
    reconciliation: null,
    attention: [],
  };
}

function mock() {
  return mockFetchRoutes(({ method, url }) => {
    if (method !== "GET") return undefined;
    if (url.pathname === "/api/v1/competencies/latest") return jsonResponse({ competence: "2026-09", current: "2026-09" });
    if (url.pathname === "/api/v1/companies") return jsonResponse({ items: Object.values(companies), total: 2 });
    for (const id of [A, B]) {
      if (url.pathname === `/api/v1/companies/${id}`) return jsonResponse(companies[id]);
      if (url.pathname === `/api/v1/companies/${id}/overview`) return jsonResponse(overview(id, url.searchParams.get("competence") ?? ""));
      if (url.pathname === `/api/v1/companies/${id}/tax-regimes`) return jsonResponse({ items: [] });
    }
    if (url.pathname === "/api/v1/backlog") {
      return jsonResponse({ items: [], total: 0, page: 1, page_size: 50 });
    }
    return undefined;
  });
}

function renderPage(route: string) {
  return renderWithProviders(<CompanyDetailPage />, { route, routes: [{ path: "/companies/:id", element: <CompanyDetailPage /> }] });
}

const search = (router: ReturnType<typeof renderPage>["router"]) => new URLSearchParams(router.state.location.search);

describe("CompanyDetailPage", () => {
  it("TEST-025: as 8 seções da ficha, abrindo na Visão geral da competência", async () => {
    mock();
    renderPage(`/companies/${A}`);
    expect(await screen.findByRole("heading", { level: 1, name: "Alfa Comércio Ltda" })).toBeInTheDocument();
    const sections = screen.getByRole("tablist", { name: "Seções da ficha" });
    expect(within(sections).getAllByRole("tab").map((t) => t.textContent)).toEqual([
      "Visão geral",
      "Dados gerais",
      "Obrigações",
      "Fator R",
      "Conciliação",
      "Documentos",
      "Procurações/Certificados",
      "Histórico de competências",
    ]);
    expect(within(sections).getByRole("tab", { name: "Visão geral" })).toHaveAttribute("aria-selected", "true");
    const modules = await screen.findByRole("region", { name: "Progresso por módulo" });
    expect(modules).toHaveTextContent("1 de 4 concluídas · 1 pendentes · 2 atrasadas");
    expect(within(modules).getByRole("link", { name: "Abrir a grade" })).toHaveAttribute("href", "/fiscal?competence=2026-09&q=101&group=normal");
    expect(screen.getByLabelText("Selecionar competência")).toHaveValue("2026-09");
  });

  it("links antigos: ?tab=regime abre Dados gerais › Regime tributário, e ?pa vira a competência da ficha", async () => {
    mock();
    const { router } = renderPage(`/companies/${A}?tab=regime&pa=2026-08`);
    await screen.findByRole("heading", { level: 1, name: "Alfa Comércio Ltda" });
    expect(screen.getByRole("tab", { name: "Dados gerais" })).toHaveAttribute("aria-selected", "true");
    expect(within(screen.getByRole("tablist", { name: "Dados gerais" })).getByRole("tab", { name: "Regime tributário" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await waitFor(() => expect(search(router).get("competence")).toBe("2026-08"));
    expect(search(router).get("pa")).toBeNull();
    expect(search(router).get("tab")).toBe("regime");

    // A seção abre na primeira subaba; os filtros de uma aba não passam para outra.
    await userEvent.click(screen.getByRole("tab", { name: "Obrigações" }));
    expect(search(router).get("tab")).toBe("obligations");
    expect(within(screen.getByRole("tablist", { name: "Obrigações" })).getByRole("tab", { name: "Da competência" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("TEST-025: trocar de empresa mantém a aba e a competência e mostra só a empresa nova", async () => {
    const fetchFn = mock();
    const { router } = renderPage(`/companies/${A}?competence=2026-08`);
    await screen.findByRole("region", { name: "Progresso por módulo" });
    await userEvent.selectOptions(screen.getByLabelText("Trocar de empresa"), B);
    expect(await screen.findByRole("heading", { level: 1, name: "Beta Serviços Ltda" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/companies/${B}`);
    expect(search(router).get("competence")).toBe("2026-08");
    const modules = await screen.findByRole("region", { name: "Progresso por módulo" });
    await waitFor(() => expect(within(modules).getByRole("link", { name: "Abrir a grade" })).toHaveAttribute("href", "/fiscal?competence=2026-08&q=102&group=normal"));
    expect(screen.queryByText("Alfa Comércio Ltda")).toBeNull();
    expect(requestedUrls(fetchFn).some((u) => u.includes(`/companies/${B}/overview?competence=2026-08`))).toBe(true);
  });

  it("a competência da ficha passa de PA futuro: a Visão geral avisa que ela não foi gerada", async () => {
    mock();
    const { router } = renderPage(`/companies/${A}`);
    await screen.findByRole("region", { name: "Progresso por módulo" });
    await userEvent.click(screen.getByRole("button", { name: "Próxima competência: 10/2026" }));
    expect(search(router).get("competence")).toBe("2026-10");
    expect(await screen.findByText(/A competência 10\/2026 ainda não foi gerada/)).toBeInTheDocument();
  });
});
