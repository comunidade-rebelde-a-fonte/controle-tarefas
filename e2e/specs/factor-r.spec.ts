import { expect, type Page, test } from "@playwright/test";

/**
 * Fator R (TASK-014a). O seed de demonstração, no relógio fixo do E2E (PA corrente 09/2026),
 * deixa na carteira: Íris (vermelho), Estação (vermelho, com 04/2026 e 07/2026 sem
 * lançamento), Fênix (amarelo), Brisa (verde, com 09/2025 a 12/2025 declarados no PGDAS-D) e
 * Lume (sem lançamentos).
 */
const IRIS = "Íris Clínica Veterinária Ltda";
const ESTACAO = "Estação Criativa Publicidade Ltda";
const FENIX = "Fênix Manutenção Predial Ltda";
const BRISA = "Brisa Serviços Digitais Ltda";
const LUME = "Lume Consultoria Empresarial Ltda";

async function openFactorRTab(page: Page, company: string): Promise<void> {
  await page.goto("/factor-r");
  await page.getByRole("link", { name: company }).click();
  await expect(page.getByRole("heading", { level: 1, name: company })).toBeVisible();
  await expect(page).toHaveURL(/tab=factor-r/);
  await expect(page.getByTestId("painel-fator-r")).toBeVisible();
}

test("AC-007 carteira do Fator R: ordem por semáforo e economia, semáforo com texto além da cor", async ({ page }) => {
  await page.goto("/factor-r");
  await expect(page.getByRole("heading", { level: 1, name: "Fator R" })).toBeVisible();
  const rows = page.getByRole("table", { name: "Carteira do Fator R — PA 09/2026" }).locator("tbody tr");
  await expect(rows).toHaveCount(5);
  const expected: [string, string, string][] = [
    [IRIS, "Vermelho · abaixo de 28% (Anexo V)", "reunião de correção"],
    [ESTACAO, "Vermelho · abaixo de 28% (Anexo V)", "reunião de correção"],
    [FENIX, "Amarelo · no limite", "vigiar / simular"],
    [BRISA, "Verde · na meta", "manter"],
    [LUME, "Dados insuficientes", "completar lançamentos"],
  ];
  for (const [i, [company, light, action]] of expected.entries()) {
    const row = rows.nth(i);
    await expect(row).toContainText(company);
    await expect(row).toContainText(light);
    await expect(row).toContainText(action);
  }
  await expect(rows.nth(1)).toContainText("Faltam: 04/2026, 07/2026");
  await expect(page.getByTestId("kpi-monitoradas")).toHaveText("5");
  await expect(page.getByTestId("kpi-no-v")).toHaveText("2");
  await expect(page.getByRole("note")).toContainText("O PGDAS-D da Receita Federal prevalece.");
});

test("AC-006 janela móvel: o PA seguinte tira o mês mais antigo e põe o novo, sem edição", async ({ page }) => {
  await openFactorRTab(page, IRIS);
  await expect(page.getByTestId("janela")).toHaveText("Janela: 09/2025 a 08/2026");
  const months = page.getByRole("list", { name: "Meses da janela" });
  await expect(months.getByText("09/2025", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "PA seguinte: 10/2026" }).click();
  await expect(page).toHaveURL(/pa=2026-10/);
  await expect(page.getByTestId("janela")).toHaveText("Janela: 10/2025 a 09/2026");
  await expect(months.getByText("09/2026 · faltante")).toBeVisible();
  await expect(months.getByText(/09\/2025/)).toHaveCount(0);
  // A grade acompanha a janela do PA.
  await expect(page.getByLabel("Receita bruta 09/2026")).toHaveValue("");
  await expect(page.getByLabel("Receita bruta 09/2025")).toHaveCount(0);
});

test("grade mensal: lançar um mês recalcula o Fator R e a carteira", async ({ page }) => {
  await openFactorRTab(page, FENIX);
  const panel = page.getByTestId("painel-fator-r");
  await expect(panel.getByText("Amarelo · no limite")).toBeVisible();

  await page.getByLabel("Pró-labore 08/2026").fill("25.000,00");
  await page.getByRole("button", { name: "Salvar 08/2026" }).click();
  await expect(page.getByText("Movimento de 08/2026 salvo", { exact: true })).toBeVisible();
  await expect(page.getByTestId("folha-2026-08")).toHaveText("R$ 37.380,00");
  await expect(panel.getByText("Verde · na meta")).toBeVisible();

  await page.goto("/factor-r");
  await expect(page.getByRole("row", { name: new RegExp(FENIX) })).toContainText("Verde · na meta");
});

test("FR-03: mês do PGDAS-D mostra a folha declarada e permite detalhar", async ({ page }) => {
  await openFactorRTab(page, BRISA);
  const cell = page.getByTestId("folha-pgdas-2025-09");
  await expect(cell).toContainText("Folha declarada no PGDAS-D: R$ 30.180,00");
  await expect(cell).toContainText("O extrato não separa pró-labore, salários, CPP e FGTS.");
  await expect(page.getByLabel("Pró-labore 09/2025")).toHaveCount(0);
  await cell.getByRole("button", { name: "Detalhar folha" }).click();
  await expect(page.getByLabel("Salários + 13º/férias 09/2025")).toHaveValue("30.180,00");
  await expect(page.getByLabel("Pró-labore 09/2025")).toHaveValue("0,00");
});
