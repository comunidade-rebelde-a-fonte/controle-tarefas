import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { apiJson, latestCompetence } from "../support/api";

/**
 * TEST-025 — A ficha da empresa como hub (REQ-002, REQ-036; AC-002, AC-034): as seções da ficha
 * com a competência do cabeçalho valendo para todas, a troca de empresa mantendo a aba e a
 * competência sem nenhuma aba mostrar a empresa anterior, os links antigos e os indicadores da
 * listagem. Na demonstração (PA corrente 09/2026), Íris está no vermelho do Fator R e Fênix no
 * amarelo. Com a varredura de acessibilidade (axe).
 */
const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"];
const IRIS = "Íris Clínica Veterinária Ltda";
const FENIX = "Fênix Manutenção Predial Ltda";

async function audit(page: Page, where: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const blocking = violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map((v) => `${v.id} [${v.impact}] ${v.help}: ${v.nodes.slice(0, 5).map((n) => n.target.join(" ")).join(" | ")}`);
  expect.soft(blocking, `axe em ${where}`).toEqual([]);
}

interface CompanyRef {
  id: string;
  internal_code: string;
  legal_name: string;
}

test("TEST-025: abas, empresa e competência trocam sem identidade divergente", async ({ page, request }) => {
  test.setTimeout(120_000);
  const competence = await latestCompetence(request);
  const find = async (name: string) =>
    (await apiJson<{ items: CompanyRef[] }>(request, "get", `/api/v1/companies?q=${encodeURIComponent(name)}`)).items.find((c) => c.legal_name === name);
  const iris = await find(IRIS);
  const fenix = await find(FENIX);
  if (!iris || !fenix) throw new Error("empresas da demonstração ausentes");

  // A listagem mostra os indicadores compactos da competência atual.
  await page.goto("/companies");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Íris");
  const row = page.getByRole("row").filter({ hasText: IRIS });
  await expect(row.getByRole("list", { name: "Indicadores" })).toContainText("Fator R vermelho");
  await audit(page, "listagem de empresas com indicadores");

  // A ficha abre na Visão geral da competência do cabeçalho.
  await row.getByRole("link", { name: IRIS }).click();
  await expect(page.getByRole("heading", { level: 1, name: IRIS })).toBeVisible();
  const sections = page.getByRole("tablist", { name: "Seções da ficha" });
  await expect(sections.getByRole("tab", { name: "Visão geral" })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByLabel("Selecionar competência")).toHaveValue(competence);
  const fr = page.getByRole("region", { name: "Fator R da empresa" });
  await expect(fr).toContainText(`Fator R — PA ${competence.slice(5)}/${competence.slice(0, 4)}`);
  await expect(fr).toContainText("Vermelho");
  await expect(fr.getByRole("list", { name: "Tendência do Fator R" })).toBeVisible();
  for (const link of await page.getByRole("region", { name: "Progresso por módulo" }).getByRole("link", { name: "Abrir a grade" }).all()) {
    await expect(link).toHaveAttribute("href", new RegExp(`q=${iris.internal_code}`));
  }
  await audit(page, "ficha — Visão geral");

  // A competência do cabeçalho vale para o Fator R (o PA) e para a Conciliação.
  await page.getByRole("button", { name: "Competência anterior: 08/2026" }).click();
  await expect(page).toHaveURL(/competence=2026-08/);
  await sections.getByRole("tab", { name: "Fator R" }).click();
  await expect(page.getByRole("heading", { name: "Fator R — PA 08/2026" })).toBeVisible();
  await sections.getByRole("tab", { name: "Conciliação" }).click();
  await expect(page.getByRole("heading", { name: "Conciliação — 08/2026" })).toBeVisible();

  // Trocar de empresa: a aba e a competência ficam, e tudo passa a ser da Fênix.
  await page.getByLabel("Trocar de empresa").selectOption({ label: `${fenix.internal_code} · ${FENIX}` });
  await expect(page.getByRole("heading", { level: 1, name: FENIX })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/companies/${fenix.id}\\?.*competence=2026-08`));
  await expect(sections.getByRole("tab", { name: "Conciliação" })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("heading", { name: "Conciliação — 08/2026" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Conciliação da empresa" }).getByRole("link", { name: "Documentos" })).toHaveAttribute(
    "href",
    `/reconciliation/documents?company_id=${fenix.id}`,
  );
  // Nenhum conteúdo da ficha é da Íris (o nome dela só fica entre as opções do seletor de empresa).
  await expect(page.getByRole("tabpanel", { name: "Conciliação" })).not.toContainText(IRIS);

  // Obrigações da competência: só da Fênix, com o mesmo total da API.
  await sections.getByRole("tab", { name: "Obrigações" }).click();
  const list = page.getByRole("table", { name: "Obrigações da empresa na competência" });
  const open = await apiJson<{ total: number }>(request, "get", `/api/v1/backlog?competence=2026-08&company_id=${fenix.id}&state=open&page_size=1`);
  await expect(list.locator("tbody tr")).toHaveCount(Math.min(open.total, 50));
  for (const link of await list.getByRole("link").all()) {
    await expect(link).toHaveAttribute("href", new RegExp(`q=${fenix.internal_code}`));
  }
  await audit(page, "ficha — Obrigações da competência");

  // Documentos: a lista consolidada da empresa (TASK-026) e os atalhos de cada módulo.
  await sections.getByRole("tab", { name: "Documentos" }).click();
  await expect(page.getByRole("button", { name: "Da competência 08/2026" })).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("link", { name: "Documentos da Conciliação" })).toHaveAttribute("href", `/reconciliation/documents?company_id=${fenix.id}`);
  await expect(page.getByRole("link", { name: "Extratos do PGDAS-D" })).toHaveAttribute("href", `/companies/${fenix.id}?tab=factor-r&competence=2026-08`);
  await audit(page, "ficha — Documentos");

  // Link antigo: a subaba e o PA viram a seção e a competência da ficha.
  await page.goto(`/companies/${fenix.id}?tab=regime&pa=2026-07`);
  await expect(sections.getByRole("tab", { name: "Dados gerais" })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("tablist", { name: "Dados gerais" }).getByRole("tab", { name: "Regime tributário" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page).toHaveURL(/competence=2026-07/);
  await expect(page).not.toHaveURL(/pa=/);
  await expect(page.getByRole("table", { name: "Linha do tempo de regimes" })).toBeVisible();
});
