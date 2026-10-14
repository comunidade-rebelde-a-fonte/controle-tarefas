import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Company } from "@/services/companies";
import type { PgdasDocument } from "@/services/factorRInbox";
import { COMPANY_ID, DOC_ID, JOB_ID, pgdasDocument, TRACE_ID } from "@/test/factorR";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes, usersRoute } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { PgdasDocumentPage } from "./PgdasDocumentPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const company = {
  id: COMPANY_ID,
  internal_code: "301",
  legal_name: "Clínica Exemplo Ltda",
  trade_name: null,
  cnpj: "11222333000181",
  active: true,
} as unknown as Company;

interface Call {
  method: string;
  path: string;
  body: unknown;
}

function mock(opts: { document?: PgdasDocument; createCompany?: () => Response } = {}) {
  const calls: Call[] = [];
  let doc = opts.document ?? pgdasDocument();
  const fetchFn = mockFetchRoutes(usersRoute, ({ method, url, body }) => {
    calls.push({ method, path: url.pathname + url.search, body });
    const base = `/api/v1/factor-r/pgdas/${DOC_ID}`;
    if (method === "GET" && url.pathname === base) return jsonResponse(doc);
    if (method === "GET" && url.pathname === "/api/v1/companies") return jsonResponse({ items: [company] });
    if (method === "GET" && url.pathname === `/api/v1/companies/${COMPANY_ID}`) return jsonResponse(company);
    if (method === "POST" && url.pathname === `${base}/link`) {
      doc = { ...doc, status: "linked", motivo: null, company_id: COMPANY_ID, sugestao_cadastro: null, texto_agente: "Extrato vinculado a Clínica Exemplo Ltda." };
      return jsonResponse(doc);
    }
    if (method === "POST" && url.pathname === `${base}/review`) {
      doc = { ...doc, status: "rejected", motivo: "Extrato de outra empresa", sugestao_cadastro: null };
      return jsonResponse(doc);
    }
    if (method === "POST" && url.pathname === `${base}/create-company`) {
      if (opts.createCompany) return opts.createCompany();
      doc = { ...doc, status: "linked", motivo: null, company_id: COMPANY_ID, sugestao_cadastro: null };
      return jsonResponse({ documento: doc, empresa: company }, 201);
    }
    if (method === "GET" && url.pathname === `/api/v1/jobs/${JOB_ID}`) {
      return jsonResponse({ id: JOB_ID, status: doc.job?.status, attempts: 3, max_attempts: 3, worker_online: true, last_error_code: doc.job?.last_error_code ?? null });
    }
    if (method === "POST" && url.pathname === `/api/v1/jobs/${JOB_ID}/retry`) {
      return jsonResponse({ id: JOB_ID, status: "pending", attempts: 0, max_attempts: 3, worker_online: true, last_error_code: null });
    }
    return undefined;
  });
  return { calls, fetchFn };
}

const render = () => renderWithProviders(<PgdasDocumentPage />, { route: `/factor-r/inbox/${DOC_ID}`, path: "/factor-r/inbox/:id" });

describe("PgdasDocumentPage", () => {
  it("FR-09: campos lidos com a confiança, avisos, meses declarados, original e trace", async () => {
    mock({
      document: pgdasDocument({
        motivo: "confianca_baixa (0.3500 < 0.400000)",
        motivos: ["cnpj_digito_verificador_invalido", "das_nao_encontrado"],
        campos: { cnpj: "45723174000199", pa: "2026-09", rbt12: "600000.00", rpa: "50000.00", fs12: "180000.00", fator_r: "0.30", das: null, anexo: "III" },
        confianca_campos: { cnpj: "0.3", pa: "1", rbt12: "1", rpa: "1", fs12: "1", fator_r: "1", das: "0", anexo: "1" },
        series_anteriores: { receitas: { "2025-09": "50000.00" }, folhas: { "2025-09": "15000.00" }, receitas_conferem: true, folhas_conferem: false },
        sugestao_cadastro: null,
      }),
    });
    render();
    const fields = await screen.findByRole("table", { name: "Campos lidos do extrato" });
    expect(within(fields).getByRole("row", { name: /CNPJ/ })).toHaveTextContent("45.723.174/0001-9930%");
    expect(within(fields).getByRole("row", { name: /Valor do DAS/ })).toHaveTextContent("não encontrado0%");
    expect(within(fields).getByRole("row", { name: /RPA/ })).toHaveTextContent("R$ 50.000,00100%");
    expect(screen.getByText("CNPJ com dígito verificador inválido")).toBeInTheDocument();
    expect(screen.getByText("Valor do DAS não encontrado")).toBeInTheDocument();
    expect(screen.getByTestId("motivo")).toHaveTextContent("Confiança baixa (0.3500 abaixo do limiar de 0.400000)");
    expect(screen.getByRole("table", { name: "Meses anteriores declarados" })).toHaveTextContent("09/2025R$ 50.000,00R$ 15.000,00");
    expect(screen.getByText(/Receitas conferem com o RBT12; folhas não conferem com a FS12/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Baixar original" })).toHaveAttribute("href", `/api/v1/factor-r/pgdas/${DOC_ID}/file`);
    expect(screen.getByRole("link", { name: "Ver trace" })).toHaveAttribute("href", `/factor-r/runs/${TRACE_ID}`);
    expect(screen.getByRole("note")).toHaveTextContent("O PGDAS-D da Receita Federal prevalece.");
  });

  it("FR-11: vincula a uma empresa encontrada pela busca", async () => {
    const { calls } = mock();
    const user = userEvent.setup();
    render();
    await user.click(await screen.findByRole("radio", { name: /Clínica Exemplo Ltda/ }));
    await user.click(screen.getByRole("button", { name: "Vincular" }));
    await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.path.endsWith("/link"))).toBe(true));
    expect(calls.find((c) => c.path.endsWith("/link"))?.body).toEqual({ company_id: COMPANY_ID });
    expect(await screen.findByTestId("empresa-vinculada")).toHaveTextContent("Vinculado a 301 — Clínica Exemplo Ltda");
    expect(screen.queryByRole("button", { name: "Vincular" })).not.toBeInTheDocument();
  });

  it("FR-11: rejeita com motivo de pelo menos 3 caracteres", async () => {
    const { calls } = mock();
    const user = userEvent.setup();
    render();
    const reject = await screen.findByRole("button", { name: "Rejeitar" });
    expect(reject).toBeDisabled();
    await user.type(screen.getByLabelText("Motivo da rejeição"), "Extrato de outra empresa");
    await user.click(reject);
    await waitFor(() => expect(calls.some((c) => c.path.endsWith("/review"))).toBe(true));
    expect(calls.find((c) => c.path.endsWith("/review"))?.body).toEqual({ decisao: "rejeitar", motivo: "Extrato de outra empresa" });
    expect(await screen.findByTestId("motivo")).toHaveTextContent("Motivo da rejeição: Extrato de outra empresa");
  });

  it("falha do job: mostra o motivo e devolve o job à fila", async () => {
    const { calls } = mock({
      document: pgdasDocument({
        status: "received",
        motivo: null,
        sugestao_cadastro: null,
        texto_agente: null,
        job: { id: JOB_ID, status: "dead", attempts: 3, max_attempts: 3, last_error_code: "PGDAS_PA_INVALIDO" },
      }),
    });
    render();
    const failed = await screen.findByTestId("job-falhou");
    expect(failed).toHaveTextContent("a competência lida do extrato é inválida");
    expect(failed).toHaveTextContent("PGDAS_PA_INVALIDO");
    await userEvent.click(within(failed).getByRole("button", { name: "Tentar de novo" }));
    await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.path === `/api/v1/jobs/${JOB_ID}/retry`)).toBe(true));
    // Documento recebido com o job encerrado ainda pode ser rejeitado; vínculo só em revisão.
    expect(screen.getByRole("button", { name: "Rejeitar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Vincular" })).not.toBeInTheDocument();
  });

  it("cadastro pelo extrato: formulário canônico com o CNPJ lido; nada é gravado antes da confirmação", async () => {
    const { calls } = mock();
    const user = userEvent.setup();
    render();
    await user.click(await screen.findByRole("button", { name: "Cadastrar e vincular" }));
    const dialog = await screen.findByRole("dialog", { name: "Cadastrar a empresa do extrato" });
    const cnpj = within(dialog).getByLabelText(/CNPJ/);
    expect(cnpj).toHaveValue("45.723.174/0001-10");
    expect(cnpj).toHaveAttribute("readonly");
    expect(within(dialog).getByLabelText(/Razão social/)).toHaveValue("CLINICA EXEMPLO LTDA");
    expect(within(dialog).getByLabelText(/Regime inicial/)).toHaveValue("SIMPLES_NACIONAL");
    expect(within(dialog).getByLabelText("Fator R")).toHaveValue("true");
    expect(within(dialog).getByLabelText("Início da atividade (Fator R)")).toHaveValue("2019-03");
    expect(calls.some((c) => c.method === "POST")).toBe(false);

    await user.type(within(dialog).getByLabelText(/Código interno/), "301");
    await user.type(within(dialog).getByLabelText(/Início do atendimento/), "2026-09-01");
    await user.click(within(dialog).getByRole("button", { name: "Confirmar cadastro e vínculo" }));
    await waitFor(() => expect(calls.some((c) => c.path.endsWith("/create-company"))).toBe(true));
    expect(calls.find((c) => c.path.endsWith("/create-company"))?.body).toMatchObject({
      empresa: { internal_code: "301", legal_name: "CLINICA EXEMPLO LTDA", cnpj: "45723174000110", tax_regime: "SIMPLES_NACIONAL", factor_r: true, service_start_date: "2026-09-01" },
      inicio_atividade: "2019-03",
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByTestId("empresa-vinculada")).toBeInTheDocument();
  });

  it("cadastro pelo extrato: CNPJ já cadastrado oferece vincular à empresa existente", async () => {
    const { calls } = mock({
      createCompany: () =>
        jsonResponse(
          {
            code: "CNPJ_CONFLICT",
            message: "Já existe uma empresa com este CNPJ.",
            details: { conflicting_company: { id: COMPANY_ID, internal_code: "301", legal_name: "Clínica Exemplo Ltda" } },
          },
          409,
        ),
    });
    const user = userEvent.setup();
    render();
    await user.click(await screen.findByRole("button", { name: "Cadastrar e vincular" }));
    const dialog = await screen.findByRole("dialog", { name: "Cadastrar a empresa do extrato" });
    await user.type(within(dialog).getByLabelText(/Código interno/), "302");
    await user.type(within(dialog).getByLabelText(/Início do atendimento/), "2026-09-01");
    await user.click(within(dialog).getByRole("button", { name: "Confirmar cadastro e vínculo" }));
    expect(await within(dialog).findByText("Já existe uma empresa com este CNPJ.")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Vincular o extrato a esta empresa" }));
    await waitFor(() => expect(calls.find((c) => c.path.endsWith("/link"))?.body).toEqual({ company_id: COMPANY_ID }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
