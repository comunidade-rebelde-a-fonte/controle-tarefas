import { mkdirSync } from "node:fs";
import path from "node:path";
import { expect, type Page, test } from "@playwright/test";
import { apiJson, latestCompetence } from "../support/api";

/**
 * TEST-029 — Design System InovaSala (REQ-051, REQ-052, AC-035; D-043). As telas principais com o
 * tema aplicado: o fundo preto do material, a Bricolage Grotesque nos títulos e a Inter no texto
 * (servidas pelo próprio sistema), o logo InovaSala by Ceath e o acento laranja no item ativo do
 * menu. Grava as capturas para a revisão visual em e2e/.cache/visual; as revisadas ficam em
 * docs/consolidacao/evidencias/task-029. O axe de todas as telas é o do a11y.spec.ts.
 */
const OUT = path.resolve(process.cwd(), ".cache/visual");

/** Espera o título e o fim dos esqueletos e grava a captura da janela. */
async function capture(page: Page, name: string, heading: string | RegExp): Promise<void> {
  await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
  await expect(page.locator(".animate-pulse")).toHaveCount(0);
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: path.join(OUT, `${name}.jpg`), type: "jpeg", quality: 85 });
}

test("TEST-029: o tema InovaSala nas telas principais, com as capturas para a revisão visual", async ({ page, request }) => {
  test.setTimeout(180_000);
  mkdirSync(OUT, { recursive: true });
  const competence = await latestCompetence(request);

  await page.goto(`/dashboard?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "Dashboard" })).toBeVisible();
  const theme = await page.evaluate(async () => {
    await document.fonts.ready;
    const loaded = (family: string) => {
      let ok = false;
      document.fonts.forEach((f) => {
        ok = ok || (f.family.includes(family) && f.status === "loaded");
      });
      return ok;
    };
    const h1 = document.querySelector("h1");
    const logo = document.querySelector<HTMLImageElement>('img[alt="InovaSala by Ceath"]');
    const active = document.querySelector('nav a[aria-current="page"] svg');
    return {
      background: getComputedStyle(document.body).backgroundColor,
      heading: h1 ? getComputedStyle(h1).fontFamily : "",
      body: getComputedStyle(document.body).fontFamily,
      fonts: loaded("Bricolage Grotesque") && loaded("Inter"),
      logo: !!logo && logo.complete && logo.naturalWidth > 0,
      activeIcon: active ? getComputedStyle(active).color : "",
    };
  });
  expect(theme.background).toBe("rgb(5, 5, 5)");
  expect(theme.heading).toContain("Bricolage Grotesque");
  expect(theme.body).toContain("Inter");
  expect(theme.fonts, "as fontes do material carregadas pelo próprio sistema").toBe(true);
  expect(theme.logo, "o logo InovaSala by Ceath carregado").toBe(true);
  // O orange-400 da paleta Tailwind do material (o Tailwind 4 calcula as cores em OKLCH).
  expect(theme.activeIcon, "o acento laranja (orange-400) no item ativo do menu").toBe("oklch(0.75 0.183 55.934)");
  await capture(page, "01-dashboard", "Dashboard");

  await page.goto("/companies");
  await capture(page, "02-empresas", "Empresas");

  await page.goto(`/fiscal?competence=${competence}&group=normal`);
  await capture(page, "03-fiscal", "Fiscal");

  await page.goto(`/accounting?competence=${competence}`);
  await capture(page, "04-contabil", "Contábil");

  // A ficha da Construtora Horizonte (111), a empresa do cenário da Conciliação no seed.
  const companies = await apiJson<{ items: { id: string; internal_code: string; legal_name: string }[] }>(request, "get", "/api/v1/companies?q=111");
  const horizonte = companies.items.find((c) => c.internal_code === "111");
  if (!horizonte) throw new Error("a 111 deveria estar no seed");
  await page.goto(`/companies/${horizonte.id}?competence=${competence}`);
  await capture(page, "05-ficha", horizonte.legal_name);

  await page.goto(`/factor-r?pa=${competence}`);
  await capture(page, "06-fator-r", "Fator R");

  // A fila de revisão da primeira empresa que usa a Conciliação em 08/2026 (as dos testes da
  // Conciliação, na suíte inteira); sem nenhuma, a lista de empresas da Conciliação.
  const recon = await apiJson<{ items: { company: { id: string } }[] }>(request, "get", "/api/v1/reconciliation/companies?competence=2026-08");
  if (recon.items.length > 0) {
    await page.goto(`/reconciliation?company_id=${recon.items[0].company.id}&competence=2026-08`);
    await capture(page, "07-conciliacao", "Fila de revisão");
  } else {
    await page.goto("/reconciliation/companies?competence=2026-08");
    await capture(page, "07-conciliacao", "Empresas");
  }

  await page.goto("/documents");
  await capture(page, "08-documentos", "Documentos");

  await page.goto("/settings");
  await capture(page, "09-configuracoes", "Cadastros e Configurações");
});
