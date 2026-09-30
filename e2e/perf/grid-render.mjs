// Medição ponta a ponta das grades no navegador [PRD §45.1, T-M3-10, T-M7-01]: do início da
// navegação até a grade exibir as linhas, com o build de produção do frontend (vite preview)
// e a API ligada ao banco controle_perf. Uso: node perf/grid-render.mjs (ver docs/perf.md).
import { chromium } from "@playwright/test";

const web = process.env.PERF_WEB_URL ?? "http://localhost:5175";
const competence = process.env.PERF_COMPETENCE ?? "";
const runs = Number(process.env.PERF_RUNS ?? "5");

const grids = [
  { label: "Fiscal simples", path: "/fiscal?group=simples", name: "Grade Fiscal" },
  { label: "Fiscal normal", path: "/fiscal?group=normal", name: "Grade Fiscal" },
  { label: "Declarações", path: "/declarations?", name: "Grade Declarações" },
  { label: "Folha", path: "/payroll?", name: "Grade Folha" },
  { label: "Contábil", path: "/accounting?", name: "Grade Contábil" },
  { label: "DCTFWeb", path: "/dctfweb?", name: "Grade DCTFWeb" },
];

const browser = await chromium.launch();
const results = {};
for (const g of grids) {
  const times = [];
  let rows = "";
  for (let i = 0; i < runs; i++) {
    // Contexto novo por rodada: sem cache HTTP nem estado do TanStack Query.
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: "pt-BR" });
    const page = await context.newPage();
    const start = performance.now();
    await page.goto(`${web}${g.path}${competence ? `&competence=${competence}` : ""}`);
    const grid = page.getByRole("grid", { name: g.name });
    await grid.getByRole("rowheader").first().waitFor({ timeout: 30_000 });
    times.push(performance.now() - start);
    rows = (await grid.getAttribute("aria-rowcount")) ?? "";
    await context.close();
  }
  times.sort((a, b) => a - b);
  results[g.label] = { rows: Number(rows) - 1, min: Math.round(times[0]), median: Math.round(times[Math.floor(times.length / 2)]), max: Math.round(times.at(-1)) };
}
await browser.close();
console.log("| Grade no navegador (cold) | Linhas | n | mín | mediana | máx |\n|---|---|---|---|---|---|");
for (const [label, r] of Object.entries(results)) {
  console.log(`| ${label} | ${r.rows} | ${runs} | ${r.min} ms | ${r.median} ms | ${r.max} ms |`);
}
