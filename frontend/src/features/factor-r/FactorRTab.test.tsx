import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Company } from "@/services/companies";
import type { FactorRCalc, FactorRMovement, FactorRSettings } from "@/services/factorR";
import { jsonResponse, requestedUrls } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { FactorRTab } from "./FactorRTab";

afterEach(() => {
  vi.unstubAllGlobals();
});

const ID = "22222222-2222-4222-8222-222222222222";

const company: Company = {
  id: ID,
  internal_code: "202",
  legal_name: "Brisa Serviços Digitais Ltda",
  trade_name: "Brisa Digital",
  cnpj: "00100002000102",
  city: null,
  state: null,
  state_registration: null,
  municipal_registration: null,
  service_start_date: "2025-01-01",
  service_end_date: null,
  active: true,
  payroll_type: "PAYROLL",
  factor_r: true,
  responsibles: { fiscal: null, accounting: null, payroll: null, declarations: null },
  notes: null,
  current_tax_regime: "SIMPLES_NACIONAL",
  current_regime_group: "SIMPLES",
  active_modules: [],
  version: 3,
} as unknown as Company;

const calc: FactorRCalc = {
  pa: "2026-09",
  janela_inicio: "2025-09",
  janela_fim: "2026-08",
  meses_preenchidos: ["2025-09", "2025-10", "2025-11", "2025-12", "2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07"],
  meses_faltantes: ["2026-08"],
  status: "ok",
  motivo: null,
  rbt12: "600000.00",
  fs12: "120000.00",
  fator_r: "0.2",
  anexo: "V",
  semaforo: "vermelho",
  folha_minima_28: "168000.0000",
  folha_minima_meta: "180000.00000000",
  gap_12m_28: "48000.0000",
  reforco_mensal_28: "4000.0000",
  gap_12m_meta: "60000.00000000",
  reforco_mensal_meta: "5000.00000000",
  aliquota_efetiva_iii: "0.105600",
  aliquota_efetiva_v: "0.178500",
  economia_12m: "43740.00000000",
  meta_operacional: "0.300000",
  cpp_integra_fs12: true,
  tabela_vigencia_inicio: "2018-01-01",
  regime_no_pa: "SIMPLES_NACIONAL",
  monitorada: true,
};

function movement(competence: string, patch: Partial<FactorRMovement> = {}): FactorRMovement {
  return {
    competence,
    receita_bruta: "50000.00",
    pro_labore: "10000.00",
    salarios_13_ferias: "0.00",
    cpp_recolhida: "0.00",
    fgts_recolhido: "0.00",
    folha_total_declarada_pgdas: null,
    folha_mes: "10000.00",
    origem: "manual",
    observacao: null,
    pgdas_document_id: null,
    version: 1,
    updated_at: "2026-09-01T12:00:00Z",
    ...patch,
  };
}

const settings: FactorRSettings = {
  company_id: ID,
  cnae: "6201-5/01",
  atividade: "Serviços digitais",
  quantidade_socios: 3,
  contato: null,
  pacote_comercial: "retainer",
  honorario_mensal: "900.00",
  inicio_atividade: "2021-06",
  notas: null,
  version: 1,
  updated_at: "2026-09-01T12:00:00Z",
};

function mock(overrides: { calc?: FactorRCalc; movements?: FactorRMovement[] } = {}) {
  const puts: { path: string; body: unknown }[] = [];
  const patches: unknown[] = [];
  const fetchFn = mockFetchRoutes(({ method, url, body }) => {
    const base = `/api/v1/companies/${ID}/factor-r`;
    if (method === "GET" && url.pathname === "/api/v1/competencies/latest") return jsonResponse({ competence: "2026-09", current: "2026-09" });
    if (method === "GET" && url.pathname === `${base}/calc`) return jsonResponse(overrides.calc ?? calc);
    if (method === "GET" && url.pathname === `${base}/movements`) {
      return jsonResponse({
        items: overrides.movements ?? [
          movement("2025-09", { origem: "pgdas", pro_labore: "0.00", folha_total_declarada_pgdas: "14000.00", folha_mes: "14000.00", observacao: "Total declarado no PGDAS-D" }),
          movement("2025-10"),
        ],
      });
    }
    if (method === "GET" && url.pathname === `${base}/settings`) return jsonResponse(settings);
    if (method === "PUT" && url.pathname.startsWith(`${base}/movements/`)) {
      puts.push({ path: url.pathname, body });
      return jsonResponse(movement(url.pathname.split("/").pop() ?? ""));
    }
    if (method === "PATCH" && url.pathname === `${base}/settings`) {
      patches.push(body);
      return jsonResponse({ ...settings, version: 2 });
    }
    return undefined;
  });
  return { fetchFn, puts, patches };
}

describe("FactorRTab", () => {
  it("AC-005/AC-006: mostra o resultado do PA, a janela e o cálculo explicado", async () => {
    const { fetchFn } = mock();
    renderWithProviders(<FactorRTab company={company} />, { route: "/companies/x?tab=factor-r&pa=2026-09" });

    const panel = await screen.findByTestId("painel-fator-r");
    expect(within(panel).getByText("Vermelho · abaixo de 28% (Anexo V)")).toBeInTheDocument();
    expect(screen.getByTestId("janela")).toHaveTextContent("Janela: 09/2025 a 08/2026");
    expect(screen.getByTestId("fator-r")).toHaveTextContent("20,00%");
    expect(screen.getByTestId("anexo")).toHaveTextContent("Anexo V");
    expect(screen.getByTestId("rbt12")).toHaveTextContent("R$ 600.000,00");
    expect(screen.getByTestId("aliquota-iii")).toHaveTextContent("10,56%");
    expect(screen.getByTestId("economia")).toHaveTextContent("R$ 43.740,00");
    expect(screen.getByText("Folha mínima 30,00% (meta)")).toBeInTheDocument();
    expect(screen.getByTestId("politica-cpp")).toHaveTextContent("CPP do DAS integra a FS12");
    expect(screen.getByTestId("vigencia-tabela")).toHaveTextContent("01/01/2018");
    const missing = within(screen.getByRole("list", { name: "Meses da janela" })).getByText(/08\/2026/);
    expect(missing).toHaveTextContent("08/2026 · faltante");
    expect(screen.getByText("Como foi calculado")).toBeInTheDocument();
    expect(screen.getByRole("note")).toHaveTextContent("O PGDAS-D da Receita Federal prevalece.");
    expect(requestedUrls(fetchFn).some((u) => u.includes("/movements?from=2025-09&to=2026-08"))).toBe(true);
  });

  it("dados insuficientes: mostra o motivo e não inventa valores", async () => {
    mock({
      calc: {
        ...calc,
        status: "dados_insuficientes",
        motivo: "empresa_nova_regra_pendente",
        semaforo: null,
        fator_r: null,
        anexo: null,
        aliquota_efetiva_iii: null,
        aliquota_efetiva_v: null,
        economia_12m: null,
      },
    });
    renderWithProviders(<FactorRTab company={company} />, { route: "/?pa=2026-09" });
    expect(await screen.findByTestId("dados-insuficientes")).toHaveTextContent("menos de 12 meses na janela");
    expect(screen.getByTestId("fator-r")).toHaveTextContent("—");
    expect(screen.getByTestId("economia")).toHaveTextContent("—");
  });

  it("explica por que a empresa não está na carteira", async () => {
    mock({ calc: { ...calc, monitorada: false, regime_no_pa: "LUCRO_PRESUMIDO" } });
    renderWithProviders(<FactorRTab company={{ ...company, factor_r: null }} />, { route: "/?pa=2026-09" });
    const notice = await screen.findByTestId("fora-da-carteira");
    expect(notice).toHaveTextContent("não está marcada como sujeita ao Fator R");
    expect(notice).toHaveTextContent("o regime é Lucro Presumido");
    expect(within(notice).getByRole("link", { name: "Marcar na aba Folha" })).toBeInTheDocument();
  });

  it("grade mensal: lança um mês com valores pt-BR e a versão atual", async () => {
    const { puts } = mock();
    renderWithProviders(<FactorRTab company={company} />, { route: "/?pa=2026-09" });
    const revenue = await screen.findByLabelText("Receita bruta 10/2025");
    expect(revenue).toHaveValue("50.000,00");
    fireEvent.change(revenue, { target: { value: "51.234,5" } });
    fireEvent.change(screen.getByLabelText("FGTS recolhido 10/2025"), { target: { value: "160" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar 10/2025" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]).toEqual({
      path: `/api/v1/companies/${ID}/factor-r/movements/2025-10`,
      body: {
        version: 1,
        receita_bruta: "51234.50",
        pro_labore: "10000.00",
        salarios_13_ferias: "0.00",
        cpp_recolhida: "0.00",
        fgts_recolhido: "160.00",
        observacao: null,
      },
    });
    expect(await screen.findByText("Movimento de 10/2025 salvo")).toBeInTheDocument();
  });

  it("grade mensal: mês vazio cria com versão 0 e valor inválido não é enviado", async () => {
    const { puts } = mock();
    renderWithProviders(<FactorRTab company={company} />, { route: "/?pa=2026-09" });
    fireEvent.change(await screen.findByLabelText("Receita bruta 08/2026"), { target: { value: "12,345" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar 08/2026" }));
    expect(await screen.findByText("Receita bruta: valor inválido (use 1.234,56)")).toBeInTheDocument();
    expect(puts).toHaveLength(0);
    fireEvent.change(screen.getByLabelText("Receita bruta 08/2026"), { target: { value: "48000" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar 08/2026" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body).toMatchObject({ version: 0, receita_bruta: "48000.00", pro_labore: "0.00" });
  });

  it("FR-03: mês do PGDAS-D mostra a folha declarada e permite detalhar", async () => {
    const { puts } = mock();
    renderWithProviders(<FactorRTab company={company} />, { route: "/?pa=2026-09" });
    const cell = await screen.findByTestId("folha-pgdas-2025-09");
    expect(cell).toHaveTextContent("Folha declarada no PGDAS-D: R$ 14.000,00");
    expect(cell).toHaveTextContent("O extrato não separa pró-labore, salários, CPP e FGTS.");
    expect(screen.queryByLabelText("Pró-labore 09/2025")).not.toBeInTheDocument();

    fireEvent.click(within(cell).getByRole("button", { name: "Detalhar folha" }));
    expect(screen.getByLabelText("Salários + 13º/férias 09/2025")).toHaveValue("14.000,00");
    fireEvent.change(screen.getByLabelText("Pró-labore 09/2025"), { target: { value: "4.000,00" } });
    fireEvent.change(screen.getByLabelText("Salários + 13º/férias 09/2025"), { target: { value: "10.000,00" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar 09/2025" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body).toMatchObject({ version: 1, pro_labore: "4000.00", salarios_13_ferias: "10000.00", observacao: "Total declarado no PGDAS-D" });
  });

  it("configuração: grava pacote, honorário e início de atividade com a versão", async () => {
    const { patches } = mock();
    renderWithProviders(<FactorRTab company={company} />, { route: "/?pa=2026-09" });
    const fee = await screen.findByLabelText("Honorário mensal do pacote (R$)");
    expect(fee).toHaveValue("900,00");
    expect(screen.getByText("sim")).toBeInTheDocument();
    fireEvent.change(fee, { target: { value: "1.250,00" } });
    fireEvent.change(screen.getByLabelText("Pacote comercial"), { target: { value: "correcao" } });
    fireEvent.click(screen.getByRole("button", { name: "Salvar configuração" }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({
      version: 1,
      cnae: "6201-5/01",
      atividade: "Serviços digitais",
      quantidade_socios: 3,
      contato: null,
      pacote_comercial: "correcao",
      honorario_mensal: "1250.00",
      inicio_atividade: "2021-06",
      notas: null,
    });
  });
});
