import { expect, test } from "@playwright/test";
import { bulkStatus, latestCompetence, openGridRow } from "../support/api";

const COMPANY = "Empresa Fictícia Alfa Comércio Ltda";

test("§53.3-5 T-M4-03 altera o eSocial na Folha e a DCTFWeb lê a mesma instância", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  const row = await openGridRow(request, "payroll", competence, "101");
  const esocial = row.cells.ESOCIAL;
  if (!esocial) throw new Error("101 sem eSocial");
  await bulkStatus(request, [esocial.id], "NOT_STARTED");

  await page.goto(`/payroll?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "Folha" })).toBeVisible();
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Alfa");
  const grid = page.getByRole("grid", { name: "Grade Folha" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByRole("columnheader", { name: "Tipo de Folha" })).toBeVisible();
  await expect(grid.getByRole("columnheader", { name: "Fator R" })).toBeVisible();

  await grid.getByRole("button", { name: `eSocial — ${COMPANY}: A iniciar` }).click();
  // A célula muda de forma otimista: espera o PATCH gravar antes de ler a API.
  const saved = page.waitForResponse((r) => r.url().includes(`/obligation-instances/${esocial.id}/status`) && r.request().method() === "PATCH");
  await page.getByRole("menuitemradio", { name: "Fechada" }).click();
  expect((await saved).ok()).toBeTruthy();
  await expect(grid.getByRole("button", { name: `eSocial — ${COMPANY}: Fechada` })).toBeVisible();

  // Mesma instância na visão DCTFWeb [CA-009].
  const dctf = await openGridRow(request, "dctfweb", competence, "101");
  expect(dctf.cells.ESOCIAL?.id).toBe(esocial.id);
  expect(dctf.cells.ESOCIAL?.status_code).toBe("CLOSED");
});
