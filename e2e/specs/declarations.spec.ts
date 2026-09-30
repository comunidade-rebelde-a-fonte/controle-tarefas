import { expect, test } from "@playwright/test";
import { apiJson, bulkStatus, latestCompetence, openGridRow } from "../support/api";

const COMPANY = "Empresa Fictícia Alfa Comércio Ltda";

test("T-M4-02 marca o MIT como Transmitida com protocolo e o protocolo persiste após recarregar", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  const row = await openGridRow(request, "declarations", competence, "101");
  const mit = row.cells.MIT;
  if (!mit) throw new Error("101 sem MIT");
  await bulkStatus(request, [mit.id], "NOT_STARTED");
  const fresh = await openGridRow(request, "declarations", competence, "101");
  const cell = fresh.cells.MIT;
  if (!cell) throw new Error("101 sem MIT");
  await apiJson(request, "patch", `/api/v1/obligation-instances/${cell.id}/details`, { version: cell.version, protocol: null, transmission_date: null });

  await page.goto(`/declarations?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "Declarações" })).toBeVisible();
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Alfa");
  const grid = page.getByRole("grid", { name: "Grade Declarações" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByRole("rowheader")).toHaveText(COMPANY);

  // Status inline: MIT → Transmitida.
  await grid.getByRole("button", { name: `MIT — ${COMPANY}: A iniciar` }).click();
  await page.getByRole("menuitemradio", { name: "Transmitida" }).click();
  await expect(grid.getByRole("button", { name: `MIT — ${COMPANY}: Transmitida` })).toBeVisible();

  // Protocolo e data de transmissão pelo popover da célula.
  await grid.getByRole("button", { name: `Detalhes de MIT — ${COMPANY}` }).click();
  const details = page.getByRole("dialog", { name: /Detalhes de MIT/ });
  await details.getByLabel("Data de transmissão").fill("2026-09-10");
  await details.getByLabel("Protocolo").fill("MIT-2026-0001");
  await details.getByRole("button", { name: "Salvar" }).click();
  await expect(page.getByText("Detalhes salvos", { exact: true })).toBeVisible();

  // Após recarregar, o popover mostra o protocolo gravado.
  await page.reload();
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Alfa");
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await grid.getByRole("button", { name: `Detalhes de MIT — ${COMPANY}` }).click();
  await expect(page.getByRole("dialog", { name: /Detalhes de MIT/ }).getByLabel("Protocolo")).toHaveValue("MIT-2026-0001");
  await expect(page.getByRole("dialog", { name: /Detalhes de MIT/ }).getByLabel("Data de transmissão")).toHaveValue("2026-09-10");
});
