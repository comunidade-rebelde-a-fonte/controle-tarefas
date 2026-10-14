import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { createHorizonte, uploadAndProcess } from "../support/reconciliation";

/**
 * TEST-021a — Fila de revisão e documentos da Conciliação (08 §7, REQ-018 a REQ-024): o extrato e
 * um comprovante do cenário Horizonte lidos no worker, a fila com os baldes, conciliar à mão,
 * desfazer com motivo, "não há comprovante", o original como imagem e a varredura de
 * acessibilidade (axe) nas telas.
 */
const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"];
// O extrato em PDF do cenário traz o descritivo em caixa de título (a leitura do texto nativo).
const ACO = "Pix Enviado Aco Brasil Distribuidora";

async function audit(page: Page, where: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const blocking = violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map((v) => `${v.id} [${v.impact}] ${v.help}: ${v.nodes.slice(0, 5).map((n) => n.target.join(" ")).join(" | ")}`);
  expect.soft(blocking, `axe em ${where}`).toEqual([]);
}

test("TEST-021a: fila de revisão — conciliar, desfazer e sem comprovante", async ({ page, request }) => {
  test.setTimeout(150_000);
  const company = await createHorizonte(request);
  const statement = await uploadAndProcess(request, "extratos/extrato_construtora_horizonte_2026-08.pdf", company.id);
  expect(statement.status).toBe("importado");
  const receipt = await uploadAndProcess(request, "comprovantes/06_pix_enviado_aco_brasil_1de2.pdf", company.id);
  expect(receipt.status).toBe("importado");
  // A rodada do matching roda no worker depois do comprovante: espera a linha da Aço Brasil na fila.
  await expect
    .poll(
      async () => {
        const q = (await (await request.get(`/api/v1/reconciliation/review-queue?company_id=${company.id}&competence=2026-08`)).json()) as {
          items: { summary: string; date: string | null; bucket: string }[];
        };
        return q.items.some((i) => i.summary === ACO && i.date === "2026-08-06" && i.bucket === "divergencia");
      },
      { timeout: 60_000, message: "a rodada não pôs a linha na fila" },
    )
    .toBe(true);

  await page.goto(`/reconciliation?company_id=${company.id}&competence=2026-08`);
  await expect(page.getByRole("heading", { level: 1, name: "Fila de revisão" })).toBeVisible();
  const buckets = page.getByRole("group", { name: "Filtrar por balde" });
  await expect(buckets.getByRole("button", { name: /^Divergências \(\d+\)$/ })).toBeVisible();

  // A linha da Aço Brasil de 06/08: o pré-filtro da origem a tirou do automático (o comprovante
  // é a parcela 1 de 2); o comprovante lido aparece como candidato.
  const table = page.getByRole("table", { name: "Itens da fila" });
  const row = table.getByRole("row").filter({ hasText: "06/08/2026" }).filter({ hasText: ACO });
  await row.getByRole("button", { name: ACO }).click();
  const panel = page.getByRole("region", { name: "Item em revisão" });
  await expect(panel.getByText("O comprovante está marcado como parcela", { exact: false })).toBeVisible();
  await expect(panel.getByRole("img", { name: /^Página 1 de \d+ do original$/ })).toBeVisible();
  await audit(page, "fila com a linha aberta");
  const candidates = panel.getByRole("region", { name: "Candidatos" });
  await candidates.getByRole("checkbox").first().check();
  await candidates.getByRole("button", { name: "Conciliar o selecionado" }).click();
  await expect(page.getByText("Linha conciliada", { exact: true })).toBeVisible();
  await expect(panel.getByText("Conciliada", { exact: true })).toBeVisible();
  await expect(panel.getByRole("region", { name: "Classificação" })).toBeVisible();

  // Desfazer com motivo: a linha volta para a fila, marcada, e não volta sozinha ao automático.
  await panel.getByRole("button", { name: "Desfazer" }).click();
  const dialog = page.getByRole("dialog", { name: "Desfazer a conciliação" });
  await audit(page, "diálogo de desfazer");
  await dialog.getByLabel("Motivo").fill("Teste E2E: o comprovante é de outra linha.");
  await dialog.getByRole("button", { name: "Desfazer" }).click();
  await expect(page.getByText("Conciliação desfeita", { exact: true })).toBeVisible();
  await expect(panel.getByText("A conciliação desta linha foi desfeita", { exact: false })).toBeVisible();
  await expect(panel.getByText("Motivo do desfazer: Teste E2E: o comprovante é de outra linha.")).toBeVisible();

  // A tarifa do banco: "não há comprovante".
  await page.getByRole("button", { name: "Fechar o item" }).click();
  await buckets.getByRole("button", { name: /^Extrato sem comprovante/ }).click();
  await table.getByRole("button", { name: "Tarifa Manutencao Conta Pj" }).click();
  await panel.getByRole("button", { name: "Não há comprovante" }).click();
  await page.getByRole("alertdialog", { name: "Confirmar sem comprovante" }).getByRole("button", { name: "Confirmar" }).click();
  await expect(page.getByText("Linha confirmada sem comprovante", { exact: true })).toBeVisible();
  await expect(panel.getByText("Sem comprovante.")).toBeVisible();

  // Documentos: o extrato importado e o comprovante extraído; o comprovante com o original.
  await page.goto(`/reconciliation/documents?company_id=${company.id}`);
  const docs = page.getByRole("table", { name: "Documentos da Conciliação" });
  await expect(docs.getByRole("row").filter({ hasText: "extrato_construtora_horizonte_2026-08.pdf" }).getByText("Importado")).toBeVisible();
  await audit(page, "documentos");
  await page.goto(`/reconciliation/documents/${receipt.id}`);
  await expect(page.getByRole("heading", { level: 1, name: "06_pix_enviado_aco_brasil_1de2.pdf" })).toBeVisible();
  await expect(page.getByRole("img", { name: "Página 1 de 1 do original" })).toBeVisible();
  await expect(page.getByRole("form", { name: "Campos do comprovante" })).toBeVisible();
  await audit(page, "documento");
});
