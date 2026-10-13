import { screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CompanyOverview } from "@/services/companies";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { OverviewTab } from "./OverviewTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ID = "00000000-0000-4000-8000-00000000000a";

const data: CompanyOverview = {
  company: { id: ID, internal_code: "101", legal_name: "Alfa Comércio Ltda" },
  competence: "2026-09",
  competence_generated: true,
  modules: [
    {
      module: "ACCOUNTING", label: "Contábil", period_state: "CLOSED", applicable: 6, done: 6, pending: 0, overdue: 0,
      operational_progress: 1, final_completion: 1, drilldown: { path: "/accounting", query: { competence: "2026-09", q: "101" } },
    },
  ],
  next_due: [
    {
      instance_id: "00000000-0000-4000-8000-0000000000d1", module: "FISCAL", module_label: "Fiscal", obligation: "ICMS", status_label: "A iniciar",
      due_date: "2026-09-10", overdue: true, drilldown: { path: "/fiscal", query: { competence: "2026-09", q: "101", group: "normal" } },
    },
  ],
  factor_r: {
    pa: "2026-09", status: "ok", motivo: null, semaforo: "vermelho", fator_r: "0.2", anexo: "V", economia_12m: "43740.00000000",
    acao: "reuniao_correcao", acao_texto: "reunião de correção",
    trend: [
      { pa: "2026-08", semaforo: "amarelo", fator_r: "0.29" },
      { pa: "2026-09", semaforo: "vermelho", fator_r: "0.2" },
    ],
  },
  reconciliation: {
    company: { id: ID, internal_code: "101", legal_name: "Alfa Comércio Ltda" }, state: "em_andamento", statements: 1,
    blocking: { missing_statement: false, divergent: 2, without_receipt: 0, review: 1, classification: 0 }, without_statement: 0,
    uses_reconciliation: true, accounting_state: "OPEN",
    obligation: { instance_id: "00000000-0000-4000-8000-0000000000b1", status_code: "NOT_STARTED", status_label: "A iniciar", is_final: false, applicable: true },
    blocks_accounting: true,
  },
  attention: [
    {
      key: "OVERDUE", label: "Obrigações atrasadas", count: 1,
      sample: [{ company: { id: ID, internal_code: "101", legal_name: "Alfa Comércio Ltda" }, title: "Fiscal — ICMS", detail: "A iniciar · venceu em 10/09/2026 (5 dias)", date: "2026-09-10", drilldown: null }],
      drilldown: { path: `/companies/${ID}`, query: { competence: "2026-09", tab: "obligations", state: "overdue" } },
    },
  ],
};

function mock(body: CompanyOverview) {
  return mockFetchRoutes(({ method, url }) => {
    if (method !== "GET") return undefined;
    if (url.pathname === `/api/v1/companies/${ID}/overview`) return jsonResponse(body);
    if (url.pathname === "/api/v1/documents") {
      return jsonResponse({
        items: [
          {
            id: "00000000-0000-4000-8000-0000000000f1", module: "factor_r", storage_backend: "local", kind: "pgdas", status: "linked",
            situation: "concluido", company: { id: ID, internal_code: "101", legal_name: "Alfa Comércio Ltda" }, competence: "2026-09",
            period_start: null, period_end: null, bank: null, file_name: "pgdas-setembro.txt", expires_at: null,
            received_at: "2026-09-20T12:00:00Z", updated_at: "2026-09-20T12:00:00Z",
            detail: { path: "/factor-r/inbox/00000000-0000-4000-8000-0000000000f1", query: {} },
          },
        ],
        total: 1,
        counts: [],
      });
    }
    return undefined;
  });
}

describe("OverviewTab", () => {
  it("TEST-025: módulos, vencimentos, Fator R com a tendência, Conciliação e pendências da mesma empresa", async () => {
    mock(data);
    renderWithProviders(<OverviewTab companyId={ID} competence="2026-09" />, { route: `/companies/${ID}?competence=2026-09` });
    const modules = await screen.findByRole("region", { name: "Progresso por módulo" });
    expect(within(modules).getByRole("group", { name: "Módulo Contábil" })).toHaveTextContent("Fechado");
    const due = screen.getByRole("list", { name: "Próximos vencimentos da empresa" });
    expect(due).toHaveTextContent("Fiscal — ICMS");
    expect(due).toHaveTextContent("Atrasada");
    expect(within(due).getByRole("link", { name: "Fiscal — ICMS" })).toHaveAttribute("href", "/fiscal?competence=2026-09&q=101&group=normal");
    const fr = screen.getByRole("region", { name: "Fator R da empresa" });
    expect(fr).toHaveTextContent("Fator R — PA 09/2026");
    expect(fr).toHaveTextContent("Ação sugerida: reunião de correção · economia de R$ 43.740,00 em 12 meses");
    expect(within(fr).getByRole("list", { name: "Tendência do Fator R" })).toHaveTextContent("08/2026: 29,00% (amarelo)");
    expect(within(fr).getByRole("link", { name: "Abrir o Fator R" })).toHaveAttribute("href", `/companies/${ID}?tab=factor-r&competence=2026-09`);
    const rec = screen.getByRole("region", { name: "Conciliação da empresa" });
    expect(rec).toHaveTextContent("Em andamento");
    expect(rec).toHaveTextContent("O que falta: 2 com divergência · 1 em revisão");
    expect(rec).toHaveTextContent("O Contábil não fecha enquanto a conciliação não estiver concluída.");
    const pending = screen.getByRole("region", { name: "Pendências da empresa" });
    expect(within(pending).getByRole("listitem", { name: "Obrigações atrasadas: 1" })).toHaveTextContent("Fiscal — ICMS · A iniciar");
    expect(within(pending).getByRole("link", { name: "Ver todas" })).toHaveAttribute(
      "href",
      `/companies/${ID}?competence=2026-09&tab=obligations&state=overdue`,
    );
    const docs = screen.getByRole("region", { name: "Documentos recentes" });
    expect(await within(docs).findByRole("link", { name: "pgdas-setembro.txt" })).toHaveAttribute(
      "href",
      "/factor-r/inbox/00000000-0000-4000-8000-0000000000f1",
    );
    expect(docs).toHaveTextContent("Extrato do PGDAS-D · Vinculado");
    expect(within(docs).getByRole("link", { name: "Ver todos" })).toHaveAttribute("href", `/companies/${ID}?tab=documents&competence=2026-09&doc_all=1`);
  });

  it("sem Fator R, sem Conciliação e numa competência não gerada, explica cada caso", async () => {
    mock({ ...data, competence: "2026-10", competence_generated: false, modules: [], next_due: [], factor_r: null, reconciliation: null, attention: [] });
    renderWithProviders(<OverviewTab companyId={ID} competence="2026-10" />);
    expect(await screen.findByRole("status")).toHaveTextContent("A competência 10/2026 ainda não foi gerada");
    expect(screen.getByText("A empresa não está na carteira do Fator R neste PA.")).toBeInTheDocument();
    expect(screen.getByText("A empresa não usa a Conciliação nesta competência.")).toBeInTheDocument();
    expect(screen.getByText("Nenhuma pendência nesta competência.")).toBeInTheDocument();
  });
});
