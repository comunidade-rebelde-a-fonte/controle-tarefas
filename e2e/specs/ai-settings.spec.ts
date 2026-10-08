import { expect, type Page, test } from "@playwright/test";
import { E2E_GATEWAY_KEY } from "../support/ai";

/**
 * TASK-008 (AC-027, F-010, REQ-047): Configurações → IA contra o gateway LiteLLM simulado do E2E
 * (support/fake-litellm.mjs). A troca de modelo é validada no gateway e gravada com versão; nada
 * de credencial na tela nem nas respostas da API.
 */

const row = (page: Page, alias: string) => page.getByTestId(`alias-${alias}`);

async function versionOf(page: Page, alias: string): Promise<number> {
  const text = (await row(page, alias).locator("td").nth(4).textContent()) ?? "";
  return Number(text.replace(/\D/g, ""));
}

test("troca o modelo do triador: reprova modelo sem visão, grava com versão e volta ao padrão", async ({ page }) => {
  const bodies: Promise<string>[] = [];
  page.on("response", (r) => {
    if (r.url().includes("/api/v1/settings/ai/models")) bodies.push(r.text().catch(() => ""));
  });

  await page.goto("/settings?tab=ai");
  const table = page.getByRole("table", { name: "Modelos de IA por agente" });
  await expect(table.locator("tbody tr")).toHaveCount(7);
  await expect(page.getByRole("note")).toHaveText("Credenciais são configuradas no ambiente e não ficam salvas nesta tela.");
  await expect(row(page, "triador")).toContainText("gpt-6-luna");
  await expect(row(page, "triador")).toContainText("padrão do ambiente");
  const v0 = await versionOf(page, "triador");

  // Modelo sem visão: a validação reprova e o salvar não grava.
  await page.getByRole("button", { name: "Editar triador", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Configurar triador" });
  await expect(dialog.getByLabel(/api.?key|chave|senha/i)).toHaveCount(0);
  await dialog.getByLabel("Escolher o modelo").check();
  await dialog.getByLabel("Modelo", { exact: true }).fill("gpt-6-texto");
  await dialog.getByRole("button", { name: "Validar configuração" }).click();
  const result = dialog.getByTestId("resultado-validacao");
  await expect(result).toContainText("Configuração reprovada");
  await expect(result).toContainText("Não aceita imagens, e o agente triador lê páginas digitalizadas.");
  await dialog.getByRole("button", { name: "Salvar" }).click();
  await expect(result).toContainText("Não foi gravado: o modelo não atende ao agente");

  // Modelo fora da tabela do gateway: reprovado com a orientação.
  await dialog.getByLabel("Modelo", { exact: true }).fill("gpt-6.1-sol");
  await dialog.getByRole("button", { name: "Validar configuração" }).click();
  await expect(result).toContainText("O gateway não conhece o modelo openai/gpt-6.1-sol");

  // Modelo com visão e temperature: válido e gravado; vale para as próximas execuções.
  await dialog.getByLabel("Modelo", { exact: true }).fill("gpt-6-sol");
  await dialog.getByLabel("Temperature").fill("0,2");
  await dialog.getByRole("button", { name: "Validar configuração" }).click();
  await expect(result).toContainText("Configuração válida: openai/gpt-6-sol");
  await expect(result).toContainText("Parâmetros aceitos pelo modelo: temperature, max_tokens");
  await dialog.getByRole("button", { name: "Salvar" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByText(`Configuração de triador salva (versão ${String(v0 + 1)})`, { exact: true })).toBeVisible();

  // Releitura: o valor vem do banco.
  await page.reload();
  await expect(row(page, "triador")).toContainText("gpt-6-sol");
  await expect(row(page, "triador")).toContainText("escolhido aqui");
  await expect(row(page, "triador")).toContainText("temperature 0,2");
  await expect(row(page, "triador")).toContainText(`v${String(v0 + 1)}`);

  // Volta ao padrão do ambiente, sem parâmetros.
  await page.getByRole("button", { name: "Editar triador", exact: true }).click();
  await dialog.getByLabel(/Padrão do ambiente/).check();
  await dialog.getByLabel("Temperature").fill("");
  await dialog.getByRole("button", { name: "Salvar" }).click();
  await expect(dialog).toBeHidden();
  await expect(row(page, "triador")).toContainText("gpt-6-luna");
  await expect(row(page, "triador")).toContainText("padrão do ambiente");
  await expect(row(page, "triador")).toContainText(`v${String(v0 + 2)}`);
  await expect(row(page, "triador")).not.toContainText("temperature");

  // Nenhuma resposta da tela trouxe a chave do gateway.
  const texts = await Promise.all(bodies);
  expect(texts.length).toBeGreaterThan(3);
  for (const t of texts) expect(t).not.toContain(E2E_GATEWAY_KEY);
});
