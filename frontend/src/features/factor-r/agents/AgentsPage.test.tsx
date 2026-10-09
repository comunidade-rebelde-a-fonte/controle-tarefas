import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FactorRAgentResponse } from "@/services/factorRAgents";
import { TRACE_ID } from "@/test/factorR";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { AgentsPage } from "./AgentsPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

const DISCLAIMER = "Esta plataforma não é a apuração oficial. O PGDAS-D da Receita Federal prevalece.";

const queue: FactorRAgentResponse = {
  texto: `PA 2026-09: 2 empresa(s) na fila.\n\n${DISCLAIMER}`,
  trace_id: TRACE_ID,
  decisao: {
    intencao: "priorizar",
    pa: "2026-09",
    fila: [
      { posicao: 1, company_id: "c-fenix", empresa: "Fênix Manutenção Predial Ltda", semaforo: "amarelo", fator_r: "0.288498", economia_12m: "56686.50", acao: "vigiar / simular" },
      { posicao: 2, company_id: "c-iris", empresa: "Íris Clínica Veterinária Ltda", semaforo: "vermelho", fator_r: "0.133393", economia_12m: "45523.20", acao: "reunião de correção" },
    ],
  },
};

function mock(consult: () => Response) {
  const posts: { path: string; body: unknown }[] = [];
  mockFetchRoutes(({ method, url, body }) => {
    if (method === "GET" && url.pathname === "/api/v1/competencies/latest") return jsonResponse({ competence: "2026-09", current: "2026-09" });
    if (method === "POST" && url.pathname === "/api/v1/factor-r/agents/prioritize") {
      posts.push({ path: url.pathname, body });
      return jsonResponse(queue);
    }
    if (method === "POST" && url.pathname === "/api/v1/factor-r/agents/consult") {
      posts.push({ path: url.pathname, body });
      return consult();
    }
    return undefined;
  });
  return posts;
}

describe("AgentsPage", () => {
  it("FR-14: priorizador manda o PA da URL e mostra a fila do motor com o trace", async () => {
    const posts = mock(() => jsonResponse({}));
    const user = userEvent.setup();
    renderWithProviders(<AgentsPage />, { route: "/factor-r/agents?pa=2026-10" });
    const chat = screen.getByTestId("chat-priorizador");
    await user.type(within(chat).getByLabelText("Pergunta ao Priorizador"), "O que priorizar esta semana?");
    await user.click(within(chat).getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]?.body).toEqual({ mensagem: "O que priorizar esta semana?", pa: "2026-10" });

    const table = await within(chat).findByRole("table", { name: "Fila do priorizador — PA 09/2026" });
    const rows = within(table).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("1Fênix Manutenção Predial Ltda Amarelo · no limite28,85%R$ 56.686,50vigiar / simular");
    expect(rows[1]).toHaveTextContent("Vermelho · abaixo de 28% (Anexo V)");
    expect(within(rows[1] as HTMLElement).getByRole("link")).toHaveAttribute("href", "/companies/c-iris?tab=factor-r&pa=2026-09");
    expect(within(chat).getByText(/PA 2026-09: 2 empresa\(s\) na fila\./)).toBeInTheDocument();
    expect(within(chat).getByRole("link", { name: "ver trace" })).toHaveAttribute("href", `/factor-r/runs/${TRACE_ID}`);
    expect(within(chat).getByLabelText("Pergunta ao Priorizador")).toHaveValue("");
  });

  it("consultor: recusa de domínio aparece com a mensagem da API", async () => {
    mock(() => jsonResponse({ code: "RECOMMENDATION_WITHOUT_SIMULATION", message: "Recomendar a correção exige uma simulação gravada." }, 409));
    const user = userEvent.setup();
    renderWithProviders(<AgentsPage />, { route: "/factor-r/agents" });
    const chat = screen.getByTestId("chat-consultor");
    await user.type(within(chat).getByLabelText("Pergunta ao Consultor"), "Devo corrigir a Íris?");
    await user.click(within(chat).getByRole("button", { name: "Enviar" }));
    expect(await within(chat).findByRole("alert")).toHaveTextContent("Recomendar a correção exige uma simulação gravada.");
    expect(within(chat).getByText("Devo corrigir a Íris?")).toBeInTheDocument();
  });

  it("consultor: falha de rede não inventa resposta", async () => {
    mock(() => jsonResponse({ code: "INTERNAL", message: "erro" }, 500));
    const user = userEvent.setup();
    renderWithProviders(<AgentsPage />, { route: "/factor-r/agents" });
    const chat = screen.getByTestId("chat-consultor");
    await user.type(within(chat).getByLabelText("Pergunta ao Consultor"), "Como está a Brisa?");
    await user.click(within(chat).getByRole("button", { name: "Enviar" }));
    expect(await within(chat).findByRole("alert")).toHaveTextContent("O agente não conseguiu responder agora. Tente de novo.");
    expect(within(chat).queryByRole("link", { name: "ver trace" })).not.toBeInTheDocument();
  });
});
