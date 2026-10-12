import AxeBuilder from "@axe-core/playwright";
import { type APIRequestContext, expect, type Page, test } from "@playwright/test";
import { apiJson, closePeriod, cnpjWithCheckDigits, displayCompetence, ensureCurrent, gridRows, latestCompetence, openGridRow, uniqueDigits } from "../support/api";

/**
 * TEST-024 — Dashboard consolidado (REQ-017, REQ-035; AC-012, AC-033): as seções Fator R e
 * Conciliação com os filtros do Dashboard (o responsável do Fiscal no Fator R e o do Contábil na
 * Conciliação; decisões do usuário, D-038) e cada número levando à lista que o explica, com o
 * mesmo total. Um responsável próprio deixa os números exatos. Depois, as áreas do menu abrem a
 * partir do Dashboard, sem outra aplicação (AC-033). Com a varredura de acessibilidade (axe).
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

// As empresas do Fator R saem da carteira no fim: a carteira da demonstração não muda.
test.afterAll(async ({ request }) => {
  for (const id of created) {
    const res = await request.post(`/api/v1/companies/${id}/deactivate`, { data: {} });
    expect(res.ok(), await res.text()).toBeTruthy();
  }
});

/** Empresa do Simples sujeita ao Fator R, com o Fiscal do responsável e 12 meses na janela do PA. */
async function factorRCompany(request: APIRequestContext, name: string, responsible: string, pa: string, payroll: string) {
  const digits = uniqueDigits();
  const company = await apiJson<{ id: string; internal_code: string; legal_name: string }>(request, "post", "/api/v1/companies", {
    internal_code: `E2E-FR-${digits}`,
    legal_name: `${name} ${digits}`,
    cnpj: cnpjWithCheckDigits(`61${digits}`),
    service_start_date: "2025-01-01",
    payroll_type: "PAYROLL",
    tax_regime: "SIMPLES_NACIONAL",
    factor_r: true,
    modules: ["FISCAL", "DECLARATIONS"],
    fiscal_responsible_user_id: responsible,
  });
  created.push(company.id);
  const [y, m] = pa.split("-").map(Number) as [number, number];
  for (let i = 12; i >= 1; i--) {
    const d = new Date(Date.UTC(y, m - 1 - i, 1));
    const month = `${String(d.getUTCFullYear())}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
    const res = await request.put(`/api/v1/companies/${company.id}/factor-r/movements/${month}`, {
      data: { version: 0, receita_bruta: "10000.00", pro_labore: payroll },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
  }
  return company;
}

test("TEST-024: o Dashboard consolida Fator R e Conciliação, e cada número abre a lista que o explica", async ({ page, request }) => {
  test.setTimeout(180_000);
  const competence = await latestCompetence(request);
  const shown = displayCompetence(competence);
  const user = await apiJson<{ id: string; name: string }>(request, "post", "/api/v1/users", {
    name: `Responsável E2E ${uniqueDigits()}`,
    role: "OPERATOR",
  });

  // Fator R: uma empresa no Anexo V (10% de folha) e outra na meta (40%), com o Fiscal do responsável.
  const red = await factorRCompany(request, "Clínica Vermelha E2E", user.id, competence, "1000.00");
  const green = await factorRCompany(request, "Estúdio Verde E2E", user.id, competence, "4000.00");

  // Conciliação: uma empresa com o Contábil do responsável; um documento chega com o Contábil
  // fechado (fica parado) e o Contábil é reaberto: a empresa usa a Conciliação e o Contábil não
  // fecha enquanto a obrigação Conciliação não estiver concluída.
  const digits = uniqueDigits();
  const recon = await apiJson<{ id: string; internal_code: string; legal_name: string }>(request, "post", "/api/v1/companies", {
    internal_code: `E2E-CO-${digits}`,
    legal_name: `Construtora Painel E2E ${digits} Ltda`,
    cnpj: cnpjWithCheckDigits(`62${digits}`),
    service_start_date: "2026-01-01",
    payroll_type: "PAYROLL",
    tax_regime: "LUCRO_PRESUMIDO",
    modules: ["ACCOUNTING"],
    accounting_responsible_user_id: user.id,
  });
  await ensureCurrent(request);
  const row = (await gridRows(request, "accounting", competence)).find((r) => r.company.internal_code === recon.internal_code);
  if (!row) throw new Error("empresa fora da grade do Contábil");
  await closePeriod(request, row);
  const late = await request.post("/api/v1/reconciliation/documents", {
    multipart: {
      file: { name: "recibo-painel.txt", mimeType: "text/plain", buffer: Buffer.from(`Recibo do painel E2E ${digits}.\n`, "utf-8") },
      company_id: recon.id,
      competence,
    },
  });
  expect(late.status(), await late.text()).toBe(202);
  await openGridRow(request, "accounting", competence, recon.internal_code);

  // O Dashboard filtrado pelo responsável.
  await page.goto("/dashboard");
  await expect(page.getByRole("heading", { level: 1, name: "Dashboard" })).toBeVisible();
  await page.getByRole("combobox", { name: "Responsável", exact: true }).selectOption({ label: user.name });
  await expect(page).toHaveURL(new RegExp(`responsible_id=${user.id}`));
  const fr = page.getByRole("region", { name: "Fator R" });
  await expect(fr).toContainText(`Carteira no PA ${shown}`);
  await expect(fr.getByRole("link", { name: "Monitoradas: 2" })).toBeVisible();
  await expect(fr.getByRole("link", { name: "No Anexo V (vermelho): 1" })).toBeVisible();
  await expect(fr.getByRole("link", { name: "Seguras (verde): 1" })).toBeVisible();
  await expect(fr.getByRole("link", { name: "Exigem ação: 1" })).toBeVisible();
  const priorities = fr.getByRole("list", { name: "Prioridades do Fator R" });
  await expect(priorities.getByRole("listitem")).toHaveCount(1);
  await expect(priorities).toContainText(red.legal_name);
  await expect(priorities).toContainText("reunião de correção");
  const rc = page.getByRole("region", { name: "Conciliação" });
  await expect(rc.getByRole("link", { name: "Parados por período fechado: 1" })).toBeVisible();
  await expect(rc.getByRole("link", { name: "Processando: 0" })).toBeVisible();
  await expect(rc.getByRole("link", { name: "Na fila de revisão: 1" })).toBeVisible();
  await expect(rc.getByRole("link", { name: "Prontas para concluir: 0" })).toBeVisible();
  await expect(rc.getByRole("link", { name: "Contábil bloqueado pela Conciliação: 1" })).toBeVisible();
  await audit(page, "Dashboard com as seções Fator R e Conciliação");
  const dashboard = page.url();

  // Fator R → a carteira filtrada pelo semáforo e pelo responsável: a mesma empresa.
  await fr.getByRole("link", { name: "No Anexo V (vermelho): 1" }).click();
  await expect(page).toHaveURL(/\/factor-r\?.*semaforo=vermelho/);
  const portfolio = page.getByRole("table", { name: `Carteira do Fator R — PA ${shown}` });
  await expect(portfolio.getByRole("row")).toHaveCount(2);
  await expect(portfolio.getByRole("row").nth(1)).toContainText(red.legal_name);
  await expect(page.getByText("1 empresa", { exact: true })).toBeVisible();
  await expect(page.getByRole("group", { name: "Filtros do Dashboard" })).toContainText(`Responsável do Fiscal: ${user.name}`);
  await page.getByRole("group", { name: "Filtrar por semáforo" }).getByRole("button", { name: "Verde (1)" }).click();
  await expect(portfolio.getByRole("row").nth(1)).toContainText(green.legal_name);
  await audit(page, "carteira filtrada pelo Dashboard");

  // Conciliação → a lista das empresas com o Contábil bloqueado.
  await page.goto(dashboard);
  await rc.getByRole("link", { name: "Contábil bloqueado pela Conciliação: 1" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Empresas" })).toBeVisible();
  const companies = page.getByRole("table", { name: "Conciliação por empresa" });
  await expect(companies.getByRole("row")).toHaveCount(2);
  await expect(companies.getByRole("row").nth(1)).toContainText(recon.legal_name);
  await expect(companies.getByRole("row").nth(1)).toContainText("Não fecha: a Conciliação não está concluída");
  await expect(page.getByText("1 empresa", { exact: true })).toBeVisible();
  await audit(page, "Conciliação por empresa");
  await companies.getByRole("link", { name: recon.legal_name }).click();
  await expect(page.getByRole("region", { name: "Fechamento do Contábil" })).toContainText("o Contábil só fecha com a obrigação Conciliação concluída");

  // Conciliação → os documentos parados e a fila da competência.
  await page.goto(dashboard);
  await rc.getByRole("link", { name: "Parados por período fechado: 1" }).click();
  await expect(page.getByText("1 documento", { exact: true })).toBeVisible();
  await expect(page.getByRole("table", { name: "Documentos da Conciliação" })).toContainText("recibo-painel.txt");
  await page.goto(dashboard);
  await rc.getByRole("link", { name: "Na fila de revisão: 1" }).click();
  const queue = page.getByRole("table", { name: "Itens da fila" });
  await expect(queue.getByRole("row")).toHaveCount(2);
  await expect(queue).toContainText("recibo-painel.txt");
  await expect(page.getByRole("group", { name: "Filtros do Dashboard" })).toContainText(`Responsável do Contábil: ${user.name}`);

  // AC-033: do Dashboard, cada área do menu abre na mesma aplicação.
  await page.goto("/dashboard");
  const menu = page.getByRole("complementary", { name: "Menu principal" });
  for (const [label, heading] of [
    ["Empresas", "Empresas"],
    ["Backlog", "Backlog"],
    ["Fiscal", "Fiscal"],
    ["Declarações", "Declarações"],
    ["Contábil", "Contábil"],
    ["Folha", "Folha"],
    ["DCTFWeb", "DCTFWeb"],
    ["Fator R", "Fator R"],
    ["Conciliação", "Fila de revisão"],
    ["Procurações e Certificados", "Procurações e Certificados"],
    ["Cadastros e Configurações", "Cadastros e Configurações"],
    ["Importação/Exportação", /Importa/],
    ["Relatórios", "Relatórios"],
    ["Dashboard", "Dashboard"],
  ] as const) {
    await menu.getByRole("link", { name: label, exact: true }).click();
    await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
  }
});
