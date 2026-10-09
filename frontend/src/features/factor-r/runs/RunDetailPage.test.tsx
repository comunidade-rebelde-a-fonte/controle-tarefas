import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AIRunDetail } from "@/services/aiRuns";
import { aiRunDetail, COMPANY_ID, TRACE_ID } from "@/test/factorR";
import { jsonResponse } from "@/test/fetch";
import { mockFetchRoutes } from "@/test/fetchRouter";
import { renderWithProviders } from "@/test/render";
import { RunDetailPage } from "./RunDetailPage";

afterEach(() => {
  vi.unstubAllGlobals();
});

function mock(run: AIRunDetail, evaluation?: () => Response) {
  const posts: unknown[] = [];
  let current = run;
  mockFetchRoutes(({ method, url, body }) => {
    if (method === "GET" && url.pathname === `/api/v1/ai/runs/${TRACE_ID}`) return jsonResponse(current);
    if (method === "GET" && url.pathname === `/api/v1/companies/${COMPANY_ID}`) return jsonResponse({ id: COMPANY_ID, legal_name: "Clínica Exemplo Ltda" });
    if (method === "POST" && url.pathname === `/api/v1/ai/runs/${TRACE_ID}/evaluation`) {
      posts.push(body);
      if (evaluation) return evaluation();
      const evaluated = { id: "88888888-8888-4888-8888-888888888888", user_id: "u1", rating: "erro" as const, comment: "Fila sem o PA.", created_at: "2026-09-15T16:00:00Z" };
      current = { ...current, has_human_eval: true, human_evals: [evaluated] };
      return jsonResponse(evaluated, 201);
    }
    return undefined;
  });
  return posts;
}

const render = () => renderWithProviders(<RunDetailPage />, { route: `/factor-r/runs/${TRACE_ID}`, path: "/factor-r/runs/:traceId" });

describe("RunDetailPage", () => {
  it("AC-029: metadados, passos com as chamadas ao LLM, decisão e prompts versionados", async () => {
    mock(aiRunDetail({ company_id: COMPANY_ID, competence: "2026-09", langfuse_url: "http://localhost:3100/project/p/traces/x" }));
    render();
    expect(await screen.findByRole("heading", { level: 1, name: "Trace do Priorizador" })).toBeInTheDocument();
    expect(screen.getByTestId("trace-id")).toHaveTextContent(TRACE_ID);
    expect(screen.getByText(/alias priorizador · configuração v1/).closest("dd")).toHaveTextContent("openai/gpt-6-luna");
    expect(screen.getByText("240 → 48 tokens · US$ 0,000084")).toBeInTheDocument();
    expect(screen.getByText("fatorr/plan@2026.10.1#eddce0a92af0")).toBeInTheDocument();
    expect(screen.getByText("fatorr/render@2026.10.1#6df09143757a")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: "Clínica Exemplo Ltda" })).toHaveAttribute("href", `/companies/${COMPANY_ID}?tab=factor-r&pa=2026-09`);
    expect(screen.getByRole("link", { name: "Abrir no Langfuse" })).toHaveAttribute("target", "_blank");

    const spans = screen.getByRole("table", { name: "Passos da corrida" });
    const rows = within(spans).getAllByRole("row").slice(1);
    expect(rows.map((r) => r.getAttribute("data-span"))).toEqual(["plan", "classificar_intencao", "tool", "decide", "render"]);
    expect(rows[1]).toHaveTextContent("LLM");
    expect(rows[1]).toHaveTextContent("120 → 24 tokens");
    expect(rows[1]).toHaveTextContent("US$ 0,000042");
    expect(rows[4]).toHaveTextContent("aviso");
    expect(rows[4]).toHaveTextContent("numeros_orfaos");
    expect(screen.getByText("fila_priorizacao")).toBeInTheDocument();
    expect(screen.getByText("Sem avaliação ouro para esta corrida.")).toBeInTheDocument();
  });

  it("FR-17: nota ouro com o resultado de cada campo", async () => {
    mock(
      aiRunDetail({
        agent: "parser_pgdas",
        gold_evals: [
          { field: "rpa", expected: "50000.00", obtained: "50000.00", within_tolerance: true, status: "ok" },
          { field: "fs12", expected: "150000.00", obtained: "180000.00", within_tolerance: false, status: "erro" },
          { field: "rbt12", expected: null, obtained: "600000.00", within_tolerance: null, status: "ouro_indisponivel" },
        ],
      }),
    );
    render();
    const gold = await screen.findByRole("table", { name: "Nota ouro" });
    const rows = within(gold).getAllByRole("row").slice(1);
    expect(rows[0]).toHaveTextContent("rpa50000.0050000.00ok");
    expect(rows[1]).toHaveTextContent("fs12150000.00180000.00erro");
    expect(rows[2]).toHaveTextContent("rbt12—600000.00ouro indisponível");
  });

  it("FR-17: nota erro exige comentário antes de enviar; registrada, aparece na lista", async () => {
    const posts = mock(aiRunDetail());
    const user = userEvent.setup();
    render();
    const form = await screen.findByRole("form", { name: "Registrar nota humana" });
    await user.click(within(form).getByRole("radio", { name: "Erro" }));
    await user.click(within(form).getByRole("button", { name: "Registrar nota" }));
    expect(within(form).getByRole("alert")).toHaveTextContent("Comentário é obrigatório quando a nota é erro.");
    expect(posts).toHaveLength(0);

    await user.type(within(form).getByLabelText(/Comentário/), "Fila sem o PA.");
    await user.click(within(form).getByRole("button", { name: "Registrar nota" }));
    await waitFor(() => expect(posts).toEqual([{ rating: "erro", comment: "Fila sem o PA." }]));
    const notes = await screen.findByTestId("evals-humanas");
    await waitFor(() => expect(notes).toHaveTextContent("Fila sem o PA."));
    expect(await screen.findByText("Nota registrada")).toBeInTheDocument();
  });

  it("nota acerto vai sem comentário; recusa da API aparece", async () => {
    const posts = mock(aiRunDetail(), () => jsonResponse({ code: "NOT_FOUND", message: "Corrida não encontrada." }, 404));
    const user = userEvent.setup();
    render();
    const form = await screen.findByRole("form", { name: "Registrar nota humana" });
    await user.click(within(form).getByRole("button", { name: "Registrar nota" }));
    await waitFor(() => expect(posts).toEqual([{ rating: "acerto", comment: null }]));
    expect(await within(form).findByRole("alert")).toHaveTextContent("Corrida não encontrada.");
  });
});
