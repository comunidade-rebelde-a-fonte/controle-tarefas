import AxeBuilder from "@axe-core/playwright";
import { type APIRequestContext, expect, type Page, test } from "@playwright/test";
import { apiJson, bulkStatus, displayCompetence, ensureCurrent, gridRows, latestCompetence, uniqueDigits } from "../support/api";
import { createHorizonte } from "../support/reconciliation";

/**
 * TEST-023 — O fechamento do Contábil é a trava da Conciliação (REQ-033, REQ-034; AC-025): fechado
 * pela aba Conciliação da ficha, as mutações da Conciliação na competência são recusadas e o
 * documento que chega vira pendência; reaberto com motivo, as mutações voltam e o documento segue
 * pendente até alguém reprocessar. Usando a Conciliação na competência, o Contábil só fecha com a
 * obrigação Conciliação final (decisões do usuário, D-037). Com a varredura de acessibilidade (axe).
 * A competência é a corrente do E2E, a única em que a API gera a obrigação.
 */
const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"];

async function audit(page: Page, where: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const blocking = violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map((v) => `${v.id} [${v.impact}] ${v.help}: ${v.nodes.slice(0, 5).map((n) => n.target.join(" ")).join(" | ")}`);
  expect.soft(blocking, `axe em ${where}`).toEqual([]);
}

/** Um lançamento manual pela tela; devolve o formulário, para conferir o resultado. */
async function addEntry(page: Page, companyId: string, date: string) {
  await page.goto(`/reconciliation/entries?company_id=${companyId}`);
  const form = page.getByRole("form", { name: "Novo lançamento" });
  await form.getByLabel(/^Data/).fill(date);
  await form.getByLabel(/^Valor/).fill("150,00");
  await form.getByLabel(/^Sentido/).selectOption("D");
  await form.getByLabel(/^Contraparte/).fill("Papelaria Central Ltda");
  await form.getByRole("button", { name: "Incluir" }).click();
  return form;
}

/** Um documento de texto único (o mesmo arquivo não entra duas vezes) para a competência. */
async function uploadLate(request: APIRequestContext, companyId: string, competence: string) {
  const text = `Recibo de pagamento à Papelaria Central Ltda, enviado ao E2E ${uniqueDigits()}.\n`;
  const res = await request.post("/api/v1/reconciliation/documents", {
    multipart: { file: { name: "recibo-tardio.txt", mimeType: "text/plain", buffer: Buffer.from(text, "utf-8") }, company_id: companyId, competence },
  });
  expect(res.status(), await res.text()).toBe(202);
  return (await res.json()) as { document_id: string; status: string };
}

test("TEST-023: fechar o Contábil trava a Conciliação; reaberto com motivo, destrava", async ({ page, request }) => {
  test.setTimeout(120_000);
  const company = await createHorizonte(request);
  await ensureCurrent(request);
  const competence = await latestCompetence(request);
  const shown = displayCompetence(competence);
  const row = (await gridRows(request, "accounting", competence)).find((r) => r.company.internal_code === company.internal_code);
  const reconciliation = row?.cells.ACCOUNTING_RECONCILIATION;
  expect(reconciliation?.applicable, "a empresa sem a obrigação Conciliação na competência corrente").toBe(true);

  // Sem Conciliação na competência, o Contábil fecha como sempre, pela aba da ficha.
  await page.goto(`/companies/${company.id}?tab=reconciliation`);
  await page.getByLabel("Competência", { exact: true }).fill(competence);
  const closure = page.getByRole("region", { name: "Fechamento do Contábil" });
  // O selo "Contábil fechado" da aba tem o mesmo texto do aviso: o aviso é conferido nas notificações.
  const toasts = page.getByRole("region", { name: /^Notifications/ });
  await expect(closure.getByText("Aberto", { exact: true })).toBeVisible();
  await expect(closure).not.toContainText("A empresa usa a Conciliação nesta competência");
  await audit(page, "ficha — Contábil aberto");
  await closure.getByRole("button", { name: "Fechar o Contábil" }).click();
  const closeDialog = page.getByRole("dialog", { name: `Fechar Contábil de ${shown}` });
  await audit(page, "fechamento do Contábil");
  await closeDialog.getByRole("button", { name: "Fechar competência" }).click();
  await expect(toasts.getByText("Contábil fechado", { exact: true })).toBeVisible();
  await expect(closure.getByText("Fechado", { exact: true })).toBeVisible();
  await expect(closure).toContainText("Fechado por");

  // Fechado, a Conciliação trava: o lançamento e a rodada são recusados; o documento que chega
  // vira pendência, sem processamento.
  const blocked = await addEntry(page, company.id, `${competence}-10`);
  await expect(blocked.getByText(`O Contábil está fechado em ${shown}. Reabra o fechamento do módulo Contábil para alterar a conciliação.`)).toBeVisible();
  const run = await request.post(`/api/v1/companies/${company.id}/reconciliation/run?competence=${competence}`);
  expect(run.status()).toBe(423);
  expect(((await run.json()) as { code: string }).code).toBe("PERIOD_CLOSED");
  const late = await uploadLate(request, company.id, competence);
  expect(late.status).toBe("pendencia_periodo_fechado");

  // Reabrir pede motivo; a reabertura fica no histórico do fechamento, com o motivo.
  const reason = "Teste E2E: chegou o recibo da papelaria depois do fechamento.";
  await page.goto(`/companies/${company.id}?tab=reconciliation`);
  await page.getByLabel("Competência", { exact: true }).fill(competence);
  await closure.getByRole("button", { name: "Reabrir o Contábil" }).click();
  const reopenDialog = page.getByRole("dialog", { name: "Reabrir Contábil" });
  await reopenDialog.getByRole("button", { name: "Reabrir" }).click();
  await expect(reopenDialog.getByText("Informe o motivo da reabertura.")).toBeVisible();
  await reopenDialog.getByLabel("Motivo da reabertura").fill(reason);
  await audit(page, "reabertura do Contábil");
  await reopenDialog.getByRole("button", { name: "Reabrir" }).click();
  await expect(toasts.getByText("Contábil reaberto", { exact: true })).toBeVisible();
  await expect(closure.getByText("Aberto", { exact: true })).toBeVisible();
  await closure.getByText("Histórico do fechamento").click();
  const history = closure.getByRole("list", { name: "Histórico do fechamento do Contábil" });
  await expect(history.getByRole("listitem").first()).toContainText("Reaberto por");
  await expect(history.getByRole("listitem").first()).toContainText(`“${reason}”`);
  await expect(history.getByRole("listitem").nth(1)).toContainText("Fechado por");
  // O documento tardio é da competência: agora a empresa usa a Conciliação nela.
  await expect(closure).toContainText("A empresa usa a Conciliação nesta competência: o Contábil só fecha com a obrigação Conciliação concluída.");
  await audit(page, "ficha — Contábil reaberto");

  // Reaberto, as mutações voltam; o documento tardio segue pendente até alguém reprocessar.
  const allowed = await addEntry(page, company.id, `${competence}-10`);
  await expect(page.getByText("Lançamento incluído", { exact: true })).toBeVisible();
  await expect(allowed.getByText(/Contábil está fechado/)).toHaveCount(0);
  await page.goto(`/reconciliation/documents/${late.document_id}`);
  await expect(page.getByRole("heading", { level: 1, name: "recibo-tardio.txt" })).toBeVisible();
  await expect(page.getByText("Período fechado", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Reprocessar" })).toBeVisible();
  const doc = await apiJson<{ status: string }>(request, "get", `/api/v1/reconciliation/documents/${late.document_id}`);
  expect(doc.status).toBe("pendencia_periodo_fechado");

  // Usando a Conciliação, o Contábil só fecha com a obrigação Conciliação final.
  await page.goto(`/companies/${company.id}?tab=reconciliation`);
  await page.getByLabel("Competência", { exact: true }).fill(competence);
  await closure.getByRole("button", { name: "Fechar o Contábil" }).click();
  await closeDialog.getByRole("button", { name: "Fechar competência" }).click();
  await expect(closeDialog.getByText(`A Conciliação de ${shown} não está concluída (a obrigação Conciliação está como A iniciar). Conclua a conciliação antes de fechar o Contábil.`)).toBeVisible();
  await audit(page, "fechamento recusado");

  // A obrigação Conciliação concluída (com as etapas anteriores, dependências HARD): fecha.
  const cells = row?.cells ?? {};
  for (const step of ["ACCOUNTING_MOVEMENT_RECEIVED", "ACCOUNTING_PAYROLL_POSTED", "ACCOUNTING_TAXES_POSTED", "ACCOUNTING_PROCESSED", "ACCOUNTING_RECONCILIATION"]) {
    await bulkStatus(request, [cells[step]?.id ?? ""], "OK");
  }
  await closeDialog.getByRole("button", { name: "Fechar competência" }).click();
  await expect(toasts.getByText("Contábil fechado", { exact: true })).toBeVisible();
  await expect(closure.getByText("Fechado", { exact: true })).toBeVisible();
});
