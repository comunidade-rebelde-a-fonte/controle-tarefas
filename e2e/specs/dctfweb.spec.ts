import { expect, test } from "@playwright/test";
import { bulkStatus, latestCompetence, openGridRow } from "../support/api";

const COMPANY = "Empresa Fictícia Alfa Comércio Ltda";

test("§53.3-6 CA-011 concluir a DCTFWeb com eSocial pendente exige exceção com motivo, registrado no histórico", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  // Períodos de Declarações e Folha abertos (a DCTFWeb reúne instâncias dos dois módulos).
  await openGridRow(request, "payroll", competence, "101");
  const row = await openGridRow(request, "dctfweb", competence, "101");
  const id = (code: string) => {
    const c = row.cells[code];
    if (!c) throw new Error(`101 sem ${code}`);
    return c.id;
  };
  await bulkStatus(request, [id("MIT"), id("REINF_RETIDOS"), id("REINF_LUCROS")], "TRANSMITTED");
  await bulkStatus(request, [id("ESOCIAL"), id("DCTFWEB")], "NOT_STARTED");

  await page.goto(`/dctfweb?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "DCTFWeb" })).toBeVisible();
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Alfa");
  const grid = page.getByRole("grid", { name: "Grade DCTFWeb" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByText("Pendente: eSocial")).toBeVisible();
  // Sem fechamento próprio (plan R1).
  await expect(grid.getByRole("button", { name: /^Fechar DCTFWeb/ })).toHaveCount(0);

  // Concluir a DCTFWeb: alerta forte com o pré-requisito pendente.
  await grid.getByRole("button", { name: `DCTFWeb — ${COMPANY}: A iniciar` }).click();
  await page.getByRole("menuitemradio", { name: "Transmitida" }).click();
  const dialog = page.getByRole("dialog", { name: "Pré-requisitos pendentes" });
  await expect(dialog.getByRole("alert")).toContainText("DCTFWeb possui pré-requisitos pendentes: eSocial.");

  // Sem motivo não conclui; com motivo, conclui com exceção.
  await dialog.getByRole("button", { name: "Concluir com exceção" }).click();
  await expect(dialog.getByText("Informe o motivo da exceção.")).toBeVisible();
  const reason = "eSocial transmitido pelo cliente em outro sistema";
  await dialog.getByLabel(/Motivo da exceção/).fill(reason);
  await dialog.getByRole("button", { name: "Concluir com exceção" }).click();
  await expect(dialog).toBeHidden();
  await expect(grid.getByRole("button", { name: `DCTFWeb — ${COMPANY}: Transmitida` })).toBeVisible();

  // O histórico mostra o motivo da exceção.
  await grid.getByRole("button", { name: `Detalhes de DCTFWeb — ${COMPANY}` }).click();
  await page.getByRole("button", { name: "Ver histórico" }).click();
  const history = page.getByRole("list", { name: "Alterações" });
  await expect(history).toContainText("Status: A iniciar → Transmitida");
  await expect(history).toContainText(`Motivo: ${reason}`);
});
