import { expect, test } from "@playwright/test";
import { createCompany, displayCompetence, latestCompetence } from "../support/api";

function nextCompetence(key: string): string {
  const [y, m] = key.split("-").map(Number);
  const d = new Date(Date.UTC(y ?? 0, m ?? 1, 1)); // mês seguinte (Date.UTC usa mês 0-based)
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

test("§53.3-2 configurar regime e módulos: nova vigência de regime e ativação da Folha a partir da próxima competência (UC-04, R6)", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  const next = nextCompetence(competence);
  const company = await createCompany(request, {
    prefix: "02",
    name: "Empresa Fictícia Configuração E2E",
    competence,
    taxRegime: "SIMPLES_NACIONAL",
    payrollType: "NO_PAYROLL",
    modules: ["FISCAL", "DECLARATIONS", "CREDENTIALS"],
  });

  await page.goto(`/companies/${company.id}`);
  await expect(page.getByRole("heading", { level: 1, name: company.legal_name })).toBeVisible();

  // Regime: Simples desde o início; Lucro Presumido a partir da próxima competência.
  await page.getByRole("tab", { name: "Regime tributário" }).click();
  const timeline = page.getByRole("table", { name: "Linha do tempo de regimes" });
  await expect(timeline.getByRole("row")).toHaveCount(2);
  await page.getByLabel("Novo regime").selectOption("LUCRO_PRESUMIDO");
  await page.getByLabel("Competência inicial").fill(next);
  await page.getByRole("button", { name: "Incluir regime" }).click();
  await expect(timeline.getByRole("row")).toHaveCount(3);
  await expect(timeline.getByRole("row").nth(1)).toContainText("Simples Nacional");
  await expect(timeline.getByRole("row").nth(1)).toContainText(displayCompetence(competence));
  await expect(timeline.getByRole("row").nth(2)).toContainText("Lucro Presumido");
  await expect(timeline.getByRole("row").nth(2)).toContainText(displayCompetence(next));

  // Módulos: a vigência sugerida é a próxima competência ainda não gerada [R6, RN-004].
  await page.getByRole("tab", { name: "Módulos" }).click();
  const modules = page.getByRole("table", { name: "Módulos" });
  const payroll = modules.getByRole("row", { name: /^Folha/ });
  await expect(payroll).toContainText("Inativo");
  await payroll.getByRole("button", { name: `Ativar a partir de ${displayCompetence(next)}` }).click();
  await expect(page.getByText(`Folha ativado a partir de ${displayCompetence(next)}`)).toBeVisible();
  await expect(payroll).toContainText(`${displayCompetence(next)} – atual: ativo`);

  // Persistido: a configuração volta igual depois de recarregar.
  await page.reload();
  await expect(modules.getByRole("row", { name: /^Folha/ })).toContainText(`${displayCompetence(next)} – atual: ativo`);
  await page.getByRole("tab", { name: "Regime tributário" }).click();
  await expect(timeline.getByRole("row")).toHaveCount(3);
});
