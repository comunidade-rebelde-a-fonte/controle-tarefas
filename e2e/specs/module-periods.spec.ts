import { expect, test } from "@playwright/test";
import { closePeriod, displayCompetence, gridRows, latestCompetence, openGridRow } from "../support/api";

const TETA = "Empresa Fictícia Teta Engenharia Ltda";
const KAPPA = "Empresa Fictícia Kappa Distribuidora Ltda";

test("§53.3-8 RN-006 fechar módulos: Folha e Declarações fecham por empresa, o Fiscal segue aberto e a Folha fica bloqueada (423)", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  await openGridRow(request, "payroll", competence, "108");
  await openGridRow(request, "declarations", competence, "108");
  await openGridRow(request, "fiscal", competence, "108", "&group=normal");

  for (const [path, label, heading] of [
    ["payroll", "Folha", "Folha"],
    ["declarations", "Declarações", "Declarações"],
  ] as const) {
    await page.goto(`/${path}?competence=${competence}`);
    await expect(page.getByRole("heading", { level: 1, name: heading })).toBeVisible();
    await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Teta");
    const grid = page.getByRole("grid", { name: `Grade ${label}` });
    await expect(grid.getByRole("rowheader")).toHaveCount(1);
    await expect(grid.getByRole("rowheader")).toHaveText(TETA);
    await grid.getByRole("button", { name: `Fechar ${label} de ${TETA}` }).click();
    const dialog = page.getByRole("dialog", { name: `Fechar ${label} de ${displayCompetence(competence)}` });
    await dialog.getByRole("button", { name: /^(Fechar competência|Atualizar e fechar)$/ }).click();
    await expect(dialog).toBeHidden();
    await expect(grid.getByText("Fechado", { exact: true })).toBeVisible();
    await expect(grid.getByRole("button", { name: `Reabrir ${label} de ${TETA}` })).toBeVisible();
  }

  // Fechamento independente por módulo [RN-006]: o Fiscal da mesma empresa continua aberto.
  await page.goto(`/fiscal?competence=${competence}&group=normal`);
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Teta");
  const fiscal = page.getByRole("grid", { name: "Grade Fiscal" });
  await expect(fiscal.getByRole("rowheader")).toHaveCount(1);
  await expect(fiscal.getByRole("rowheader")).toHaveText(TETA);
  await expect(fiscal.getByRole("button", { name: `Fechar Fiscal de ${TETA}` })).toBeVisible();

  // Período fechado: a API recusa editar a Folha até a reabertura [plan R9].
  const row = (await gridRows(request, "payroll", competence)).find((r) => r.company.internal_code === "108");
  const cell = Object.values(row?.cells ?? {}).find((c) => c.applicable);
  if (!row || !cell) throw new Error("108 sem obrigação aplicável na Folha");
  expect(row.period.state).toBe("CLOSED");
  const res = await request.patch(`/api/v1/obligation-instances/${cell.id}/status`, {
    data: { version: cell.version, status_code: cell.status_code === "IN_PROGRESS" ? "SENT_TO_CLIENT" : "IN_PROGRESS" },
  });
  expect(res.status()).toBe(423);
});

test("§53.3-9 RN-007 reabrir com motivo: o motivo é obrigatório e a reabertura fica no histórico da empresa", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  await closePeriod(request, await openGridRow(request, "accounting", competence, "110"));
  const reason = "Correção do balancete depois da revisão do cliente";

  await page.goto(`/accounting?competence=${competence}`);
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Kappa");
  const grid = page.getByRole("grid", { name: "Grade Contábil" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByRole("rowheader")).toHaveText(KAPPA);
  await grid.getByRole("button", { name: `Reabrir Contábil de ${KAPPA}` }).click();

  // Sem motivo, nada é enviado.
  const dialog = page.getByRole("dialog", { name: "Reabrir Contábil" });
  await dialog.getByRole("button", { name: "Reabrir" }).click();
  await expect(dialog.getByText("Informe o motivo da reabertura.")).toBeVisible();

  await dialog.getByLabel("Motivo da reabertura").fill(reason);
  await dialog.getByRole("button", { name: "Reabrir" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByText("Contábil reaberto", { exact: true })).toBeVisible();
  await expect(grid.getByRole("button", { name: `Fechar Contábil de ${KAPPA}` })).toBeVisible();

  // Histórico da empresa: evento de reabertura com o motivo [§10.6].
  await page.goto("/companies");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Kappa");
  await page.getByRole("link", { name: KAPPA }).click();
  await page.getByRole("tab", { name: "Histórico de competências" }).click();
  const history = page.getByRole("table", { name: "Histórico de competências" });
  const current = history.getByRole("row").filter({ hasText: displayCompetence(competence) });
  const events = current.getByRole("img", { name: /^Eventos: .*Fechado, Reaberto$/ });
  await expect(events).toBeVisible();
  await events.hover();
  await expect(page.getByRole("tooltip")).toContainText(`Motivo: ${reason}`);
});
