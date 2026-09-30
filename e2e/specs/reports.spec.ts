import { expect, type APIRequestContext, type Locator, type Page, test } from "@playwright/test";

const REPORTS = [
  "Situação geral da competência",
  "Pendências por módulo",
  "Obrigações atrasadas",
  "Backlog",
  "Procurações e certificados a vencer",
  "Controle completo de um módulo",
];

async function expectPdf(request: APIRequestContext, link: Locator) {
  const href = await link.getAttribute("href");
  expect(href, "link sem href").toBeTruthy();
  const res = await request.get(href ?? "");
  expect(res.status(), `${href}: ${res.status()}`).toBe(200);
  expect(res.headers()["content-type"]).toBe("application/pdf");
  expect((await res.body()).subarray(0, 5).toString()).toBe("%PDF-");
}

async function downloadName(page: Page, link: Locator): Promise<string> {
  const [download] = await Promise.all([page.waitForEvent("download"), link.click()]);
  return download.suggestedFilename();
}

test("T-M6-08 a tela de Relatórios baixa cada um dos 6 PDFs", async ({ page, request }) => {
  await page.goto("/reports");
  await expect(page.getByRole("heading", { level: 1, name: "Relatórios" })).toBeVisible();

  for (const title of REPORTS) {
    const card = page.getByRole("region", { name: title });
    await expectPdf(request, card.getByRole("link", { name: `Baixar PDF — ${title}` }));
  }

  // Parâmetros próprios: controle completo da Folha.
  const module = page.getByRole("region", { name: "Controle completo de um módulo" });
  await module.getByLabel("Módulo", { exact: true }).selectOption("payroll");
  const link = module.getByRole("link", { name: "Baixar PDF — Controle completo de um módulo" });
  await expect(link).toHaveAttribute("href", /module=payroll/);
  await expectPdf(request, link);
  expect(await downloadName(page, link)).toMatch(/^relatorio-module-payroll-\d{4}-\d{2}\.pdf$/);
});

test("T-M6-08 atalhos Gerar PDF no Dashboard e no Backlog levam os filtros atuais", async ({ page, request }) => {
  await page.goto("/dashboard?regime_group=SIMPLES");
  const dash = page.getByRole("link", { name: "Gerar PDF — Situação geral da competência" });
  await expect(dash).toHaveAttribute("href", /\/api\/v1\/reports\/general\.pdf\?.*regime_group=SIMPLES/);
  await expectPdf(request, dash);

  await page.goto("/backlog?module=FISCAL");
  const backlog = page.getByRole("link", { name: "Gerar PDF — Backlog" });
  await expect(backlog).toHaveAttribute("href", /\/api\/v1\/reports\/backlog\.pdf\?.*module=FISCAL/);
  await expectPdf(request, backlog);
  expect(await downloadName(page, backlog)).toMatch(/^relatorio-backlog-\d{4}-\d{2}\.pdf$/);
});
