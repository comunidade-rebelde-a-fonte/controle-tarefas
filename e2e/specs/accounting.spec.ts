import { expect, test } from "@playwright/test";
import { bulkStatus, latestCompetence, openGridRow } from "../support/api";

const COMPANY = "Empresa Fictícia Alfa Comércio Ltda";
const BLOCKED = "Não é possível concluir Conciliação. A etapa Contábil digitado/processado ainda não foi concluída.";

test("CA-010 Conciliação com a etapa anterior A iniciar é bloqueada com a mensagem do §18.3", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  const row = await openGridRow(request, "accounting", competence, "101");
  const id = (code: string) => {
    const c = row.cells[code];
    if (!c) throw new Error(`101 sem ${code}`);
    return c.id;
  };
  await bulkStatus(request, [id("ACCOUNTING_PROCESSED"), id("ACCOUNTING_RECONCILIATION"), id("ACCOUNTING_TRIAL_BALANCE_CLOSED")], "NOT_STARTED");
  for (const step of ["ACCOUNTING_MOVEMENT_RECEIVED", "ACCOUNTING_PAYROLL_POSTED", "ACCOUNTING_TAXES_POSTED"]) {
    await bulkStatus(request, [id(step)], "OK");
  }

  await page.goto(`/accounting?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "Contábil" })).toBeVisible();
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Alfa");
  const grid = page.getByRole("grid", { name: "Grade Contábil" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  // Sequência visível nos cabeçalhos (1 → 6).
  await expect(grid.getByRole("columnheader", { name: "5. Conciliação" })).toBeVisible();

  // Conciliação: o OK fica desabilitado e a mensagem do bloqueio aparece.
  await grid.getByRole("button", { name: `Conciliação — ${COMPANY}: A iniciar` }).click();
  await expect(page.getByText(BLOCKED)).toBeVisible();
  await expect(page.getByRole("menuitemradio", { name: "OK" })).toHaveAttribute("aria-disabled", "true");
  await page.keyboard.press("Escape");

  // "Sem movimento" na etapa anterior satisfaz a sequência [§18.2]; o OK da Conciliação é liberado.
  await grid.getByRole("button", { name: `Contábil digitado/processado — ${COMPANY}: A iniciar` }).click();
  await page.getByRole("menuitemradio", { name: "Sem movimento" }).click();
  await expect(grid.getByRole("button", { name: `Contábil digitado/processado — ${COMPANY}: Sem movimento` })).toBeVisible();
  await grid.getByRole("button", { name: `Conciliação — ${COMPANY}: A iniciar` }).click();
  await expect(page.getByRole("menuitemradio", { name: "OK" })).not.toHaveAttribute("aria-disabled");
  await page.getByRole("menuitemradio", { name: "OK" }).click();
  await expect(grid.getByRole("button", { name: `Conciliação — ${COMPANY}: OK` })).toBeVisible();
});

const ETA = "Empresa Fictícia Eta Varejo Ltda";
const STEPS = [
  ["ACCOUNTING_MOVEMENT_RECEIVED", "Movimento recebido"],
  ["ACCOUNTING_PAYROLL_POSTED", "Contabilização da folha"],
  ["ACCOUNTING_TAXES_POSTED", "Contabilização dos impostos"],
  ["ACCOUNTING_PROCESSED", "Contábil digitado/processado"],
  ["ACCOUNTING_RECONCILIATION", "Conciliação"],
  ["ACCOUNTING_TRIAL_BALANCE_CLOSED", "Fechamento do balancete"],
] as const;

test("§53.3-7 CA-010 concluir o Contábil em sequência: cada etapa libera a seguinte até a empresa ficar pronta para fechar", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  const row = await openGridRow(request, "accounting", competence, "107");
  await bulkStatus(
    request,
    STEPS.map(([code]) => row.cells[code]?.id ?? ""),
    "NOT_STARTED",
  );

  await page.goto(`/accounting?competence=${competence}`);
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Eta Varejo");
  const grid = page.getByRole("grid", { name: "Grade Contábil" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByRole("rowheader")).toHaveText(ETA);
  await expect(grid.getByText("Pronto para fechar")).toHaveCount(0);

  for (const [i, [, name]] of STEPS.entries()) {
    const next = STEPS[i + 1];
    // A etapa seguinte ainda está bloqueada: o OK dela fica desabilitado.
    if (next) {
      await grid.getByRole("button", { name: `${next[1]} — ${ETA}: A iniciar` }).click();
      await expect(page.getByRole("menuitemradio", { name: "OK" })).toHaveAttribute("aria-disabled", "true");
      await page.keyboard.press("Escape");
    }
    await grid.getByRole("button", { name: `${name} — ${ETA}: A iniciar` }).click();
    await page.getByRole("menuitemradio", { name: "OK" }).click();
    const done = grid.getByRole("button", { name: `${name} — ${ETA}: OK` });
    await expect(done).toBeVisible();
    await expect(done).not.toHaveAttribute("aria-busy"); // confirmado pela API (a célula é otimista)
  }

  await expect(grid.getByLabel("Conclusão final: 100%")).toBeVisible();
  await expect(grid.getByText("Pronto para fechar")).toBeVisible();
});
