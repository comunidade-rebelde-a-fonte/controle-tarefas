import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { COMPANY_ID, simulation, TRACE_ID } from "@/test/factorR";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { Simulator } from "./Simulator";

afterEach(() => {
  vi.unstubAllGlobals();
});

const BASE = `/api/v1/companies/${COMPANY_ID}/factor-r/simulations`;

function mock(post?: () => Response) {
  const posts: unknown[] = [];
  let history = [simulation({ simulation_id: "s-old", veredito: "nao_forcar", created_at: "2026-09-10T12:00:00Z" })];
  mockFetchRoutes(({ method, url, body }) => {
    if (method === "GET" && url.pathname === BASE) return jsonResponse({ items: history });
    if (method === "POST" && url.pathname === BASE) {
      posts.push(body);
      if (post) return post();
      const created = simulation();
      history = [created, ...history];
      return jsonResponse(created, 201);
    }
    return undefined;
  });
  return posts;
}

const render = () => renderWithProviders(<Simulator companyId={COMPANY_ID} pa="2026-09" policyMeta="0.300000" />);

describe("Simulator", () => {
  it("AC-010: padrões da origem viram frações exatas; o resultado só formata o motor", async () => {
    const posts = mock();
    const user = userEvent.setup();
    render();
    expect(screen.getByText(/Vazio = meta do escritório \(30,00%\)/)).toBeInTheDocument();
    expect(screen.getByTestId("aviso-inss")).toHaveTextContent("INSS sem teto");
    await user.click(screen.getByRole("button", { name: "Simular" }));
    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toEqual({ pa: "2026-09", meta: null, fracao_pro_labore: "1", inss: "0.11", irrf: "0.275", horizonte_meses: 12 });

    const result = await screen.findByTestId("resultado-simulacao");
    expect(within(result).getByTestId("veredito")).toHaveTextContent("Corrigir");
    expect(within(result).getByTestId("liquido")).toHaveTextContent("R$ 5.130,54");
    expect(result).toHaveTextContent("Fator R atual13,34%");
    expect(result).toHaveTextContent("Reforço por mêsR$ 8.743,00");
    expect(result).toHaveTextContent("meta 30,00% · 100% do reforço em pró-labore · INSS 11,00% · IRRF 27,50% · 12 meses · piso de R$ 6.000,00 por ano");
    expect(within(result).getByRole("link", { name: "ver trace" })).toHaveAttribute("href", `/factor-r/runs/${TRACE_ID}`);
    // O histórico recarrega com a simulação nova (e já trazia a anterior).
    const history = screen.getByTestId("historico-simulacoes");
    await waitFor(() => expect(within(history).getAllByRole("listitem")).toHaveLength(2));
    expect(history).toHaveTextContent("Não forçar");
  });

  it("parâmetros digitados: percentual com vírgula e meta própria", async () => {
    const posts = mock();
    const user = userEvent.setup();
    render();
    await user.type(screen.getByLabelText("Meta (%)"), "31,5");
    await user.clear(screen.getByLabelText("Reforço em pró-labore (%)"));
    await user.type(screen.getByLabelText("Reforço em pró-labore (%)"), "60");
    await user.clear(screen.getByLabelText("Horizonte no Anexo III (meses)"));
    await user.type(screen.getByLabelText("Horizonte no Anexo III (meses)"), "24");
    await user.click(screen.getByRole("button", { name: "Simular" }));
    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toMatchObject({ meta: "0.315", fracao_pro_labore: "0.6", horizonte_meses: 24 });
  });

  it("valida antes de enviar e mostra os erros de campo da API", async () => {
    const posts = mock(() =>
      jsonResponse({ code: "VALIDATION_ERROR", message: "Dados inválidos.", details: { fields: { meta: "A meta vai de 28% a menos de 100%." } } }, 400),
    );
    const user = userEvent.setup();
    render();
    await user.type(screen.getByLabelText("INSS do sócio (%)"), "0");
    await user.click(screen.getByRole("button", { name: "Simular" }));
    expect(await screen.findByText("Use um percentual de 0 a 100.")).toBeInTheDocument();
    expect(posts).toHaveLength(0);

    await user.clear(screen.getByLabelText("INSS do sócio (%)"));
    await user.type(screen.getByLabelText("INSS do sócio (%)"), "11");
    await user.type(screen.getByLabelText("Meta (%)"), "20");
    await user.click(screen.getByRole("button", { name: "Simular" }));
    expect(await screen.findByText("A meta vai de 28% a menos de 100%.")).toBeInTheDocument();
    expect(posts).toHaveLength(1);
  });

  it("recusa de domínio (sem tabela do Simples vigente) aparece com a mensagem da API", async () => {
    mock(() => jsonResponse({ code: "SIMPLES_TABLE_NOT_IN_FORCE", message: "Não há tabela do Simples Nacional vigente no PA." }, 422));
    const user = userEvent.setup();
    render();
    await user.click(screen.getByRole("button", { name: "Simular" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Não há tabela do Simples Nacional vigente no PA.");
  });
});
