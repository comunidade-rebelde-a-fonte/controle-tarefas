import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { apiJson, closePeriod, cnpjWithCheckDigits, displayCompetence, ensureCurrent, gridRows, latestCompetence, uniqueDigits } from "../support/api";
import { statement, uploadAndWait } from "../support/pgdas";

/**
 * TEST-026 — Documentos consolidados (REQ-037, REQ-043, REQ-044): uma empresa com um extrato do
 * PGDAS-D (Fator R, armazenamento local), um documento da Conciliação (MinIO do E2E) e a
 * procuração RFB (Controle, armazenamento local). A área Documentos e a aba da ficha mostram os
 * três, cada um com o status do seu módulo, e abrir leva à tela certa (D-040). Com o axe.
 */
const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"];
const created: string[] = [];

async function audit(page: Page, where: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const blocking = violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map((v) => `${v.id} [${v.impact}] ${v.help}: ${v.nodes.slice(0, 5).map((n) => n.target.join(" ")).join(" | ")}`);
  expect.soft(blocking, `axe em ${where}`).toEqual([]);
}

// A empresa sujeita ao Fator R sai da carteira no fim: a carteira da demonstração não muda.
test.afterAll(async ({ request }) => {
  for (const id of created) {
    const res = await request.post(`/api/v1/companies/${id}/deactivate`, { data: {} });
    expect(res.ok(), await res.text()).toBeTruthy();
  }
});

test("TEST-026: os documentos dos três módulos aparecem e abrem no detalhe certo", async ({ page, request }) => {
  test.setTimeout(150_000);
  const competence = await latestCompetence(request);
  const digits = uniqueDigits();
  const company = await apiJson<{ id: string; internal_code: string; legal_name: string; cnpj: string }>(request, "post", "/api/v1/companies", {
    internal_code: `E2E-DOC-${digits}`,
    legal_name: `Clínica dos Documentos E2E ${digits} Ltda`,
    cnpj: cnpjWithCheckDigits(`63${digits}`),
    service_start_date: "2025-01-01",
    payroll_type: "PAYROLL",
    tax_regime: "SIMPLES_NACIONAL",
    factor_r: true,
    modules: ["FISCAL", "ACCOUNTING"],
  });
  created.push(company.id);
  await ensureCurrent(request);

  // Fator R: o extrato do PGDAS-D da empresa, lido e vinculado no worker.
  const pgdas = await uploadAndWait(request, "pgdas-documentos.txt", statement("txt_padrao", company.cnpj));
  expect(pgdas.status).toBe("linked");

  // Conciliação: com o Contábil fechado, o documento fica parado (sem processamento no worker).
  const row = (await gridRows(request, "accounting", competence)).find((r) => r.company.internal_code === company.internal_code);
  if (!row) throw new Error("empresa fora da grade do Contábil");
  await closePeriod(request, row);
  const recon = await request.post("/api/v1/reconciliation/documents", {
    multipart: {
      file: { name: "recibo-documentos.txt", mimeType: "text/plain", buffer: Buffer.from(`Recibo da área Documentos ${digits}.\n`, "utf-8") },
      company_id: company.id,
      competence,
    },
  });
  expect(recon.status(), await recon.text()).toBe(202);

  // Controle: a procuração RFB (o arquivo público, sem chave).
  const poa = await request.put(`/api/v1/companies/${company.id}/credential-controls/RFB_POWER_OF_ATTORNEY/file`, {
    multipart: { file: { name: "procuracao-documentos.pdf", mimeType: "application/pdf", buffer: Buffer.from("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n%%EOF\n") } },
  });
  expect(poa.status(), await poa.text()).toBe(200);

  // A área Documentos, filtrada pela empresa: os três, com o status de cada módulo.
  await page.goto(`/documents?company_id=${company.id}`);
  await expect(page.getByRole("heading", { level: 1, name: "Documentos" })).toBeVisible();
  const table = page.getByRole("table", { name: "Documentos" });
  await expect(table.locator("tbody tr")).toHaveCount(3);
  await expect(page.getByText("3 documentos", { exact: true })).toBeVisible();
  await expect(table.locator("tr[data-modulo=factor_r]")).toContainText("Vinculado");
  await expect(table.locator("tr[data-modulo=reconciliation]")).toContainText("Período fechado");
  await expect(table.locator("tr[data-modulo=reconciliation]")).toContainText("MinIO");
  await expect(table.locator("tr[data-modulo=credentials]")).toContainText("Procuração RFB");
  await audit(page, "área Documentos");

  // A situação comum filtra entre módulos: o documento parado está em revisão.
  await page.getByRole("group", { name: "Filtrar por situação" }).getByRole("button", { name: /^Em revisão/ }).click();
  await expect(table.locator("tbody tr")).toHaveCount(1);
  await expect(table.locator("tbody tr")).toContainText("recibo-documentos.txt");
  await page.getByRole("group", { name: "Filtrar por situação" }).getByRole("button", { name: "Todas as situações" }).click();
  await expect(table.locator("tbody tr")).toHaveCount(3);

  // Abrir leva à tela de cada módulo.
  await table.getByRole("link", { name: "pgdas-documentos.txt" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Extrato PGDAS-D" })).toBeVisible();
  await page.goBack();
  await table.getByRole("link", { name: "recibo-documentos.txt" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "recibo-documentos.txt" })).toBeVisible();
  await page.goBack();
  await table.getByRole("link", { name: "procuracao-documentos.pdf" }).click();
  await expect(page.getByRole("heading", { level: 1, name: company.legal_name })).toBeVisible();
  await expect(page.getByRole("tab", { name: "Procurações/Certificados" })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByText("procuracao-documentos.pdf")).toBeVisible();

  // Na ficha: a aba Documentos na competência da ficha (a procuração não tem competência) e em todas.
  await page.getByRole("tablist", { name: "Seções da ficha" }).getByRole("tab", { name: "Documentos" }).click();
  const docs = page.getByRole("table", { name: "Documentos da empresa" });
  await expect(page.getByRole("button", { name: `Da competência ${displayCompetence(competence)}` })).toHaveAttribute("aria-pressed", "true");
  await expect(docs.locator("tbody tr")).toHaveCount(2);
  await page.getByRole("button", { name: "Todas as competências" }).click();
  await expect(docs.locator("tbody tr")).toHaveCount(3);
  await audit(page, "ficha — Documentos");

  // A Visão geral mostra os documentos recentes da empresa.
  await page.getByRole("tablist", { name: "Seções da ficha" }).getByRole("tab", { name: "Visão geral" }).click();
  const recent = page.getByRole("region", { name: "Documentos recentes" });
  await expect(recent.getByRole("listitem")).toHaveCount(3);
  await expect(recent).toContainText("procuracao-documentos.pdf");
});
