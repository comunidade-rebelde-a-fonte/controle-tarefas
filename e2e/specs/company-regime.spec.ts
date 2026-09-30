import { expect, test } from "@playwright/test";

test("UC-04 troca de regime com vigência futura atualiza a linha do tempo", async ({ page }) => {
  await page.goto("/companies");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("00.000.001/0001-01");
  await page.getByRole("link", { name: "Empresa Fictícia Alfa Comércio Ltda" }).click();

  await expect(page.getByRole("heading", { level: 1, name: "Empresa Fictícia Alfa Comércio Ltda" })).toBeVisible();
  await page.getByRole("tab", { name: "Regime tributário" }).click();

  const timeline = page.getByRole("table", { name: "Linha do tempo de regimes" });
  await expect(timeline.getByRole("row")).toHaveCount(2); // cabeçalho + Lucro Presumido

  await page.getByLabel("Novo regime").selectOption("LUCRO_REAL");
  await page.getByLabel("Competência inicial").fill("2027-01");
  await page.getByRole("button", { name: "Incluir regime" }).click();

  await expect(timeline.getByRole("row")).toHaveCount(3);
  const presumido = timeline.getByRole("row").nth(1);
  await expect(presumido).toContainText("Lucro Presumido");
  await expect(presumido).toContainText("01/2025");
  await expect(presumido).toContainText("12/2026");
  const real = timeline.getByRole("row").nth(2);
  await expect(real).toContainText("Lucro Real");
  await expect(real).toContainText("01/2027");
  await expect(real).toContainText("Vigente");
});
