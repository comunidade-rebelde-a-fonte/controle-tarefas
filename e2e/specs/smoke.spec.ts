import { expect, test } from "@playwright/test";

test("SPA abre no Dashboard, navega entre as áreas e enxerga a API", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/dashboard$/);
  await expect(page.getByRole("heading", { level: 1, name: "Dashboard" })).toBeVisible();

  const nav = page.getByRole("navigation");
  // 12 áreas do PRD §6 + Fator R (TASK-014a), Conciliação (TASK-021a) e Documentos (TASK-026).
  await expect(nav.getByRole("link")).toHaveCount(15);
  await expect(page.getByRole("status", { name: "API disponível" })).toBeVisible();

  await nav.getByRole("link", { name: "Fiscal" }).click();
  await expect(page).toHaveURL(/\/fiscal$/);
  await expect(page.getByRole("heading", { level: 1, name: "Fiscal" })).toBeVisible();
});

test("health da API via proxy responde 200", async ({ request }) => {
  const res = await request.get("/api/v1/health");
  expect(res.status()).toBe(200);
  expect(await res.json()).toEqual({ status: "ok", database: "ok" });
});
