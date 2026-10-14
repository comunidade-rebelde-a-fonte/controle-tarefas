import { expect, type Page, test } from "@playwright/test";
import { apiJson } from "../support/api";

/**
 * Simulador, agentes e observabilidade pela tela (TASK-014b; REQ-013 a REQ-015, AC-010, AC-029,
 * AC-031, FR-12 a FR-19). O LLM é o do gateway simulado do E2E (support/fake-litellm.mjs): o
 * caminho com LLM roda de ponta a ponta sem provedor real (FR-20). Os números vêm sempre do motor.
 */
test.describe.configure({ mode: "serial" });

const IRIS = "Íris Clínica Veterinária Ltda";
const ESTACAO = "Estação Criativa Publicidade Ltda";
const DISCLAIMER = "O PGDAS-D da Receita Federal prevalece.";
/** Texto que o gateway simulado redige: chegar à tela prova que passou pelo validador. */
const LLM_TEXT = "Resposta redigida pelo modelo de teste a partir da decisão calculada pelo sistema.";

async function openIris(page: Page): Promise<void> {
  await page.goto("/factor-r");
  await page.getByRole("link", { name: IRIS }).click();
  await expect(page.getByRole("heading", { level: 1, name: IRIS })).toBeVisible();
  await expect(page.getByTestId("painel-fator-r")).toBeVisible();
}

test("AC-010: simulador na ficha calcula pelo motor e grava a simulação no trace do consultor", async ({ page }) => {
  await openIris(page);
  await expect(page.getByTestId("aviso-inss")).toContainText("INSS sem teto");
  await page.getByRole("button", { name: "Simular" }).click();
  const result = page.getByTestId("resultado-simulacao");
  await expect(result.getByTestId("veredito")).toHaveText(/Corrigir|Não forçar|Já na meta/, { timeout: 15_000 });
  await expect(result).toContainText("Líquido");
  await expect(result).toContainText(DISCLAIMER);
  await expect(page.getByTestId("historico-simulacoes")).toContainText("PA 09/2026");

  await result.getByRole("link", { name: "ver trace" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Trace do Consultor" })).toBeVisible();
  for (const span of ["tool", "decide", "render"]) await expect(page.locator(`[data-span='${span}']`)).toBeVisible();
  // O simulador da ficha não chama o LLM.
  await expect(page.locator("[data-span='classificar_intencao']")).toHaveCount(0);
  const traceId = (await page.getByTestId("trace-id").textContent()) ?? "";
  const run = await apiJson<{ trigger: string; decision: { simulation_id?: string } }>(page.request, "get", `/api/v1/ai/runs/${traceId}`);
  expect(run.trigger).toBe("ficha");
  expect(run.decision.simulation_id).toBeTruthy();
});

test("FR-13: consultor na ficha responde pelo LLM com texto validado, disclaimer e trace", async ({ page }) => {
  await openIris(page);
  const chat = page.getByTestId("chat-consultor-ficha");
  await chat.getByLabel(`Pergunta ao Consultor · ${IRIS}`).fill("Como está a empresa?");
  await chat.getByRole("button", { name: "Enviar" }).click();
  const answer = chat.locator("[data-autor=agente]").last();
  await expect(answer).toContainText(LLM_TEXT, { timeout: 15_000 });
  await expect(answer).toContainText(DISCLAIMER);

  await answer.getByRole("link", { name: "ver trace" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Trace do Consultor" })).toBeVisible();
  for (const span of ["plan", "classificar_intencao", "tool", "decide", "render", "redigir_resposta"]) {
    await expect(page.locator(`[data-span='${span}']`)).toBeVisible();
  }
  await expect(page.locator("[data-span='redigir_resposta']")).toContainText("openai/gpt-6-luna");
  // Prompts versionados gravados na corrida (prompt-spec §1).
  await expect(page.locator("dt", { hasText: "Prompts" }).locator("xpath=following-sibling::dd")).toContainText("fatorr/render@2026.10.1");
});

test("FR-14: priorizador devolve a fila dos vermelhos e amarelos do PA com o trace", async ({ page }) => {
  await page.goto("/factor-r/agents");
  await expect(page.getByRole("heading", { level: 1, name: "Agentes do Fator R" })).toBeVisible();
  const chat = page.getByTestId("chat-priorizador");
  await chat.getByLabel("Pergunta ao Priorizador").fill("O que priorizar esta semana?");
  await chat.getByRole("button", { name: "Enviar" }).click();
  const queue = chat.getByRole("table", { name: "Fila do priorizador — PA 09/2026" });
  await expect(queue).toBeVisible({ timeout: 15_000 });
  for (const company of [IRIS, ESTACAO]) {
    await expect(queue.getByRole("row", { name: new RegExp(company) })).toContainText("reunião de correção");
  }
  await expect(chat.locator("[data-autor=agente]").last()).toContainText(DISCLAIMER);

  await chat.getByRole("link", { name: "ver trace" }).last().click();
  await expect(page.getByRole("heading", { level: 1, name: "Trace do Priorizador" })).toBeVisible();
  for (const span of ["plan", "tool", "decide", "render"]) await expect(page.locator(`[data-span='${span}']`)).toBeVisible();
});

test("FR-17 e FR-18: painel e nota humana no trace (erro exige comentário)", async ({ page }) => {
  await page.goto("/factor-r/runs");
  await expect(page.getByRole("heading", { level: 1, name: "Observabilidade dos agentes" })).toBeVisible();
  await expect(page.getByTestId("obs-total")).not.toHaveText("0");
  await expect(page.getByRole("table", { name: "Corridas por agente" })).toContainText("Priorizador");

  await page.getByLabel("Agente", { exact: true }).selectOption("priorizador");
  const withoutEval = page.getByRole("button", { name: "Só sem nota humana" });
  await withoutEval.click();
  await expect(withoutEval).toHaveAttribute("aria-pressed", "true");
  await expect(page).toHaveURL(/agent=priorizador/);
  await expect(page).toHaveURL(/sem_nota=1/);
  const row = page.getByRole("table", { name: "Corridas dos agentes" }).locator("tbody tr").first();
  await expect(row).toContainText("Priorizador");
  const traceId = (await row.getAttribute("data-trace")) ?? "";
  await row.getByRole("link").click();
  await expect(page.getByTestId("trace-id")).toHaveText(traceId);

  const form = page.getByRole("form", { name: "Registrar nota humana" });
  await form.getByRole("radio", { name: "Erro" }).check();
  await form.getByRole("button", { name: "Registrar nota" }).click();
  await expect(form.getByRole("alert")).toContainText("Comentário é obrigatório quando a nota é erro.");
  await form.getByLabel(/Comentário/).fill("Conferência E2E: a fila deveria citar o motivo de cada empresa.");
  await form.getByRole("button", { name: "Registrar nota" }).click();
  await expect(page.getByText("Nota registrada", { exact: true })).toBeVisible();
  await expect(page.getByTestId("evals-humanas")).toContainText("Conferência E2E");

  const run = await apiJson<{ has_human_eval: boolean; human_evals: { rating: string }[] }>(page.request, "get", `/api/v1/ai/runs/${traceId}`);
  expect(run.has_human_eval).toBe(true);
  expect(run.human_evals.map((h) => h.rating)).toEqual(["erro"]);
});

test("FR-19: telas novas do Fator R afirmam que o PGDAS-D prevalece", async ({ page }) => {
  for (const url of ["/factor-r/inbox", "/factor-r/agents", "/factor-r/runs"]) {
    await page.goto(url);
    await expect(page.getByRole("note"), url).toContainText(DISCLAIMER);
  }
});
