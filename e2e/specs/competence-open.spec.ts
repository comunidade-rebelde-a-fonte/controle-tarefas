import { expect, test } from "@playwright/test";
import { createCompany, displayCompetence, ensureCurrent, fixedToday, gridRows, latestCompetence } from "../support/api";

test("§53.3-3 UC-01 abrir competência: a geração inclui a empresa nova, é idempotente e a grade mostra as obrigações", async ({ page, request }) => {
  // A competência corrente é o mês de "hoje" no relógio fixo do E2E.
  const competence = await latestCompetence(request);
  expect(competence).toBe(fixedToday().slice(0, 7));

  const company = await createCompany(request, {
    prefix: "03",
    name: "Empresa Fictícia Abertura E2E",
    competence,
    taxRegime: "SIMPLES_NACIONAL",
    payrollType: "PAYROLL",
    modules: ["FISCAL", "DECLARATIONS", "ACCOUNTING", "PAYROLL", "CREDENTIALS"],
  });
  const inGrid = async (module: string, query = "") =>
    (await gridRows(request, module, competence, query)).filter((r) => r.company.internal_code === company.internal_code);

  // Cadastrada depois da geração: ainda fora da competência (sem snapshot).
  expect(await inGrid("fiscal", "&group=simples")).toHaveLength(0);

  // ensureCurrentCompetence (boot, verificação diária ou acesso) completa a competência.
  const first = await ensureCurrent(request);
  expect(first.competence).toBe(competence);
  expect(first.companies_processed).toBeGreaterThanOrEqual(1);
  expect(first.instances_created).toBeGreaterThan(0);

  // Idempotente: a segunda execução não cria nada e a empresa aparece uma única vez [CA-005].
  const again = await ensureCurrent(request);
  expect(again.companies_processed).toBe(0);
  expect(again.instances_created).toBe(0);
  for (const [module, query] of [["fiscal", "&group=simples"], ["payroll", ""], ["accounting", ""], ["declarations", ""]] as const) {
    const rows = await inGrid(module, query);
    expect(rows, `${module}: uma linha para a empresa nova`).toHaveLength(1);
    expect(rows[0]?.period.state).toBe("OPEN");
  }

  // Na tela: a competência aberta é a corrente e a empresa tem as obrigações "A iniciar".
  await page.goto("/fiscal");
  await expect(page.getByRole("navigation", { name: "Competência" }).getByLabel("Selecionar competência")).toHaveValue(competence);
  await page.getByRole("button", { name: "Simples Nacional" }).click();
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill(company.internal_code);
  const grid = page.getByRole("grid", { name: "Grade Fiscal" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByRole("rowheader")).toHaveText(company.legal_name);
  await expect(grid.getByRole("button", { name: new RegExp(`— ${company.legal_name}: A iniciar$`) }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: `Competência anterior: ${displayCompetence(prevOf(competence))}` })).toBeVisible();
});

function prevOf(key: string): string {
  const [y, m] = key.split("-").map(Number);
  const d = new Date(Date.UTC(y ?? 0, (m ?? 1) - 2, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}
