import { expect, test } from "@playwright/test";
import { displayCompetence, latestCompetence } from "../support/api";

function keyOf(display: string): string {
  const [m, y] = display.split("/");
  return `${y ?? ""}-${m ?? ""}`;
}

test("§53.3-10 Backlog (R8): pendências só de competências anteriores, filtro por módulo e ida à grade da competência de origem", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  await page.goto(`/backlog?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "Backlog" })).toBeVisible();
  await expect(page.getByText(`Pendências de competências anteriores a ${displayCompetence(competence)}.`)).toBeVisible();

  const list = page.getByRole("table", { name: "Backlog" });
  const rows = list.getByRole("row");
  await expect(rows.nth(1)).toBeVisible();

  // Filtro por módulo: o do item mais atrasado; todos os itens passam a ser dele.
  const moduleLabel = (await rows.nth(1).getByRole("cell").nth(4).innerText()).trim();
  const filtered = page.waitForResponse((r) => r.url().includes("/api/v1/backlog") && r.url().includes("module="));
  await page.getByRole("combobox", { name: "Módulo" }).selectOption({ label: moduleLabel });
  await filtered;
  const columns = () =>
    list.locator("tbody tr").evaluateAll((trs) =>
      trs.map((tr) => ({ competence: tr.children[3]?.textContent?.trim() ?? "", module: tr.children[4]?.textContent?.trim() ?? "" })),
    );
  await expect.poll(async () => (await columns()).every((c) => c.module === moduleLabel)).toBe(true);
  const items = await columns();
  expect(items.length).toBeGreaterThan(0);
  // Só competências anteriores à selecionada [plan R8].
  for (const c of items) expect(keyOf(c.competence) < competence, `${c.competence} antes de ${displayCompetence(competence)}`).toBeTruthy();

  // Item → grade do módulo na competência de origem, com a busca pelo código da empresa
  // (a busca é por trecho de nome, código ou CNPJ, então outra empresa pode coincidir).
  const first = rows.nth(1);
  const company = (await first.getByRole("link").innerText()).trim();
  const origin = keyOf((await first.getByRole("cell").nth(3).innerText()).trim());
  await first.getByRole("link").click();
  const grid = page.getByRole("grid", { name: `Grade ${moduleLabel}` });
  await expect(grid.getByRole("rowheader", { name: company, exact: true })).toBeVisible();
  await expect(page.getByLabel("Selecionar competência")).toHaveValue(origin);
});
