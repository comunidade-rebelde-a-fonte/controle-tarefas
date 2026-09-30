import { expect, test } from "@playwright/test";

test("§53.3-11 T-M6-05 Exportar baixa o XLSX com a competência, os filtros e a busca da tela", async ({ page, request }) => {
  const { competence } = (await (await request.get("/api/v1/competencies/latest")).json()) as { competence: string };

  await page.goto(`/fiscal?competence=${competence}&group=normal&tax_regime=LUCRO_REAL&q=Alfa`);
  const link = page.getByRole("link", { name: "Exportar" });
  await expect(link).toBeVisible();

  const href = await link.getAttribute("href");
  expect(href).not.toBeNull();
  const url = new URL(href ?? "", page.url());
  expect(url.pathname).toBe("/api/v1/exports/fiscal.xlsx");
  expect(Object.fromEntries(url.searchParams)).toMatchObject({ competence, group: "normal", tax_regime: "LUCRO_REAL", q: "Alfa" });

  const [download] = await Promise.all([page.waitForEvent("download"), link.click()]);
  expect(download.suggestedFilename()).toBe(`fiscal-normal-${competence}.xlsx`);
  const path = await download.path();
  expect(path).toBeTruthy();

  const res = await request.get(href ?? "");
  expect(res.status()).toBe(200);
  expect(res.headers()["content-type"]).toBe("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet");
  const body = await res.body();
  expect(body.subarray(0, 2).toString()).toBe("PK"); // contêiner ZIP do XLSX
});
