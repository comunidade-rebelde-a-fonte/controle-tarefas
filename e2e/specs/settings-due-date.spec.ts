import { expect, test } from "@playwright/test";

test("edita uma regra de vencimento e relê o valor atualizado", async ({ page }) => {
  await page.goto("/settings?tab=due-dates");
  const table = page.getByRole("table", { name: "Regras de vencimento" });
  const row = table.getByRole("row", { name: /Provisória — dia 20 do mês seguinte/ });
  await expect(row).toContainText("Dia 20 do mês seguinte");

  await row.getByRole("button", { name: /Editar/ }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel(/Dia-base/).fill("22");
  await dialog.getByRole("button", { name: "Salvar regra" }).click();
  await expect(dialog).toBeHidden();

  // Releitura: recarrega a página e confere o valor vindo da API.
  await page.reload();
  await expect(page.getByRole("table", { name: "Regras de vencimento" }).getByRole("row", { name: /Provisória — dia 20 do mês seguinte/ })).toContainText(
    "Dia 22 do mês seguinte",
  );
});
