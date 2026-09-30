import { expect, test } from "@playwright/test";
import { latestCompetence } from "../support/api";

function display(key: string): string {
  const [y, m] = key.split("-");
  return `${m ?? ""}/${y ?? ""}`;
}

test("§53.3-10 UC-03 dashboard → Atrasadas: N → lista com N itens → grade da empresa → atualizar status", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  await page.goto(`/dashboard?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "Dashboard" })).toBeVisible();

  // KPI clicável "Atrasadas: N" [PRD §24.7].
  const kpis = page.getByRole("region", { name: "Indicadores principais" });
  const tile = kpis.getByRole("link", { name: /^Atrasadas: \d+$/ });
  await expect(tile).toBeVisible();
  const n = Number((await tile.getAttribute("aria-label"))?.split(": ")[1]);
  expect(n).toBeGreaterThan(0);
  await tile.click();

  // Lista filtrada com os N itens.
  await expect(page.getByRole("heading", { level: 1, name: `Atrasadas — ${display(competence)}` })).toBeVisible();
  const list = page.getByRole("table", { name: `Atrasadas — ${display(competence)}` });
  await expect(list.getByRole("row")).toHaveCount(Math.min(n, 100) + 1);

  // Item → grade do módulo na competência, filtrada na empresa.
  const first = list.getByRole("row").nth(1);
  const company = (await first.getByRole("link").innerText()).trim();
  const obligation = (await first.getByRole("cell").nth(5).innerText()).trim();
  await expect(first.getByRole("cell").nth(4)).toHaveText("Fiscal");
  await first.getByRole("link").click();
  const grid = page.getByRole("grid", { name: "Grade Fiscal" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByRole("rowheader")).toHaveText(company);

  // Atualizar o status da obrigação atrasada.
  const cell = grid.getByRole("button", { name: new RegExp(`^${obligation} — ${company}: `) });
  await cell.click();
  await page.getByRole("menuitemradio", { name: "Fechadas" }).click();
  const updated = grid.getByRole("button", { name: `${obligation} — ${company}: Fechadas` });
  await expect(updated).toBeVisible();
  await expect(updated).not.toHaveAttribute("aria-busy"); // confirmado pela API (a célula é otimista)

  // De volta ao dashboard, uma atrasada a menos.
  await page.goto(`/dashboard?competence=${competence}`);
  await expect(kpis.getByRole("link", { name: `Atrasadas: ${n - 1}` })).toBeVisible();
});
