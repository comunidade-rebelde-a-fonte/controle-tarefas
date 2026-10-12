import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Dashboard } from "@/services/dashboard";
import { renderWithProviders } from "@/test/render";
import { FactorRSection } from "./FactorRSection";
import { ReconciliationSection } from "./ReconciliationSection";

const R = "00000000-0000-4000-8000-00000000a001";
const link = (path: string, query: Record<string, string>) => ({ path, query });
const fr = (semaforo?: string) => link("/factor-r", { pa: "2026-09", responsible_id: R, ...(semaforo ? { semaforo } : {}) });

const factorR: Dashboard["factor_r"] = {
  pa: "2026-09",
  unavailable: null,
  meta_operacional: "0.300000",
  monitored: { value: 12, drilldown: fr() },
  red: { value: 3, drilldown: fr("vermelho") },
  yellow: { value: 2, drilldown: fr("amarelo") },
  green: { value: 6, drilldown: fr("verde") },
  insufficient: { value: 1, drilldown: fr("insuficiente") },
  savings_at_play: { value: "104580.00000000", drilldown: fr("vermelho,amarelo") },
  action_required: { value: 6, drilldown: fr("vermelho,amarelo,insuficiente") },
  priorities: [
    {
      company: { id: "c1", internal_code: "208", legal_name: "Clínica Horizonte Ltda", trade_name: null, cnpj: "00100008000108" },
      semaforo: "vermelho",
      fator_r: "0.2",
      economia_12m: "43740.00000000",
      acao: "reuniao_correcao",
      acao_texto: "reunião de correção",
      drilldown: link("/companies/c1", { tab: "factor-r", pa: "2026-09" }),
    },
    {
      company: { id: "c2", internal_code: "210", legal_name: "Estúdio Sem Dados ME", trade_name: null, cnpj: "00100008000109" },
      semaforo: null,
      fator_r: null,
      economia_12m: null,
      acao: "completar_lancamentos",
      acao_texto: "completar lançamentos",
      drilldown: link("/companies/c2", { tab: "factor-r", pa: "2026-09" }),
    },
  ],
};

const rq = (path: string, extra: Record<string, string>) => link(path, { responsible_id: R, ...extra });

const reconciliation: Dashboard["reconciliation"] = {
  processing: { value: 2, drilldown: rq("/reconciliation/documents", { status: "pendente,triado", filter_competence: "2026-09" }) },
  extraction_review: { value: 1, drilldown: rq("/reconciliation/documents", { status: "revisao_triagem,revisao_extracao", filter_competence: "2026-09" }) },
  closed_period: { value: 0, drilldown: rq("/reconciliation/documents", { status: "pendencia_periodo_fechado", filter_competence: "2026-09" }) },
  queue: { value: 7, drilldown: rq("/reconciliation", { competence: "2026-09" }) },
  divergent: { value: 1, drilldown: rq("/reconciliation", { competence: "2026-09", bucket: "divergencia" }) },
  without_receipt: { value: 4, drilldown: rq("/reconciliation", { competence: "2026-09", bucket: "extrato_sem" }) },
  without_statement: { value: 2, drilldown: rq("/reconciliation", { competence: "2026-09", bucket: "comprovante_sem" }) },
  ready: { value: 1, drilldown: rq("/reconciliation/companies", { competence: "2026-09", state: "pronta" }) },
  blocks_accounting: { value: 3, drilldown: rq("/reconciliation/companies", { competence: "2026-09", blocked: "true" }) },
};

describe("FactorRSection", () => {
  it("números do motor com o drill-down para a carteira filtrada e as prioridades com a ação", () => {
    renderWithProviders(<FactorRSection section={factorR} />);
    const card = screen.getByRole("region", { name: "Fator R" });
    expect(card).toHaveTextContent("Carteira no PA 09/2026, meta operacional de 30%.");
    expect(within(card).getByRole("link", { name: "No Anexo V (vermelho): 3" })).toHaveAttribute(
      "href",
      `/factor-r?pa=2026-09&responsible_id=${R}&semaforo=vermelho`,
    );
    expect(within(card).getByRole("link", { name: "Economia em jogo (12 meses): R$ 104.580,00" })).toHaveAttribute(
      "href",
      `/factor-r?pa=2026-09&responsible_id=${R}&semaforo=vermelho%2Camarelo`,
    );
    expect(within(card).getByRole("link", { name: "Exigem ação: 6" })).toHaveAttribute(
      "href",
      `/factor-r?pa=2026-09&responsible_id=${R}&semaforo=vermelho%2Camarelo%2Cinsuficiente`,
    );
    expect(within(card).getByRole("link", { name: "Abrir a carteira" })).toHaveAttribute("href", `/factor-r?pa=2026-09&responsible_id=${R}`);
    const list = within(card).getByRole("list", { name: "Prioridades do Fator R" });
    const [first, second] = within(list).getAllByRole("listitem");
    expect(first).toHaveTextContent("reunião de correção · Fator R 20,00% · economia de R$ 43.740,00 em 12 meses");
    expect(within(first as HTMLElement).getByRole("link", { name: "Clínica Horizonte Ltda" })).toHaveAttribute(
      "href",
      "/companies/c1?tab=factor-r&pa=2026-09",
    );
    expect(second).toHaveTextContent("Dados insuficientes");
    expect(second).toHaveTextContent("completar lançamentos");
  });

  it("sem tabela do Simples, mostra o motivo no lugar dos números", () => {
    renderWithProviders(
      <FactorRSection
        section={{ ...factorR, unavailable: "Não há tabela do Simples vigente para o PA 12/2017.", meta_operacional: "", priorities: [] }}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Não há tabela do Simples vigente para o PA 12/2017.");
    expect(screen.queryByRole("link", { name: /Monitoradas/ })).toBeNull();
  });
});

describe("ReconciliationSection", () => {
  it("documentos de todas as competências, a fila e as empresas da competência, com o drill-down", () => {
    renderWithProviders(<ReconciliationSection section={reconciliation} competence="2026-09" />);
    const card = screen.getByRole("region", { name: "Conciliação" });
    const docs = within(card).getByRole("group", { name: "Documentos da Conciliação" });
    expect(within(docs).getByRole("link", { name: "Processando: 2" })).toHaveAttribute(
      "href",
      `/reconciliation/documents?responsible_id=${R}&status=pendente%2Ctriado&filter_competence=2026-09`,
    );
    expect(docs).toHaveTextContent("todas as competências");
    const queue = within(card).getByRole("group", { name: "Fila de revisão de 09/2026" });
    expect(within(queue).getByRole("link", { name: "Divergências: 1" })).toHaveAttribute(
      "href",
      `/reconciliation?responsible_id=${R}&competence=2026-09&bucket=divergencia`,
    );
    const companies = within(card).getByRole("group", { name: "Empresas de 09/2026" });
    expect(within(companies).getByRole("link", { name: "Prontas para concluir: 1" })).toHaveAttribute(
      "href",
      `/reconciliation/companies?responsible_id=${R}&competence=2026-09&state=pronta`,
    );
    expect(within(companies).getByRole("link", { name: "Contábil bloqueado pela Conciliação: 3" })).toHaveAttribute(
      "href",
      `/reconciliation/companies?responsible_id=${R}&competence=2026-09&blocked=true`,
    );
  });
});
