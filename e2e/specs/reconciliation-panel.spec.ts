import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { apiJson, uniqueDigits } from "../support/api";
import { createHorizonte } from "../support/reconciliation";

/**
 * TEST-021c — A aba Conciliação da ficha, as métricas e os playbooks (REQ-022, REQ-036, AC-034,
 * 11-observability §7, CC-11): a competência com a trava do Contábil, a rodada, os atalhos, o
 * cadastro da conta bancária e a varredura de acessibilidade (axe). Não usa extrato do cenário
 * (os dois são do TEST-021a e do TEST-021b); os números das métricas são conferidos nos testes de
 * integração.
 */
const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"];

async function audit(page: Page, where: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const blocking = violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map((v) => `${v.id} [${v.impact}] ${v.help}: ${v.nodes.slice(0, 5).map((n) => n.target.join(" ")).join(" | ")}`);
  expect.soft(blocking, `axe em ${where}`).toEqual([]);
}

test("TEST-021c: aba Conciliação da ficha, contas bancárias, métricas e playbooks", async ({ page, request }) => {
  test.setTimeout(120_000);
  const company = await createHorizonte(request);
  await apiJson(request, "post", "/api/v1/reconciliation/entries", {
    company_id: company.id, date: "2026-08-12", amount: "1500,00", direction: "D", counterparty: "Fornecedor E2E",
  });

  // A ficha abre na aba pela URL; a competência começa na corrente (setembro, pelo relógio fixo).
  await page.goto(`/companies/${company.id}?tab=reconciliation`);
  await expect(page.getByRole("tab", { name: "Conciliação", selected: true })).toBeVisible();
  const competence = page.getByLabel("Competência", { exact: true });
  await expect(competence).toHaveValue("2026-09");
  await competence.fill("2026-08");
  await expect(page.getByText("Contábil aberto")).toBeVisible();
  await expect(page.getByRole("region", { name: "Classificação e lançamentos" })).toContainText("1 lançamento do cliente");
  await expect(page.getByText("Nenhum extrato importado cobre a competência.")).toBeVisible();
  await page.getByRole("button", { name: "Rodar a conciliação da competência" }).click();
  await expect(page.getByText(/^Rodada concluída:/)).toBeVisible();
  const shortcuts = page.getByRole("navigation", { name: "Conciliação da empresa" });
  await expect(shortcuts.getByRole("link", { name: "Lançamentos" })).toHaveAttribute(
    "href",
    `/reconciliation/entries?company_id=${company.id}&competence=2026-08`,
  );

  // Conta bancária à mão (CC-11): cadastrada uma vez; a segunda é recusada.
  const accounts = page.getByRole("region", { name: "Contas bancárias" });
  await expect(accounts.getByText("Nenhuma conta cadastrada.")).toBeVisible();
  const digits = uniqueDigits();
  const form = accounts.getByRole("form", { name: "Nova conta bancária" });
  for (const attempt of [1, 2]) {
    await form.getByLabel(/^Banco/).fill("e2e");
    await form.getByLabel(/^Agência/).fill("0001");
    await form.getByLabel(/^Conta/).fill(`${digits}-0`);
    await form.getByRole("button", { name: "Cadastrar conta" }).click();
    if (attempt === 1) {
      await expect(page.getByText(`Conta e2e 0001/${digits}-0 cadastrada`, { exact: true })).toBeVisible();
      await expect(accounts.getByRole("table", { name: "Contas cadastradas" })).toContainText(`e2e 0001/${digits}-0`);
    }
  }
  await expect(accounts.getByText("Essa conta já está cadastrada nesta empresa.")).toBeVisible();
  await audit(page, "ficha — aba Conciliação");

  // Os atalhos levam às telas já filtradas: as métricas da empresa na competência.
  await shortcuts.getByRole("link", { name: "Métricas" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Métricas" })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`/reconciliation/metrics\\?company_id=${company.id}&competence=2026-08$`));
  await expect(page.getByRole("region", { name: "Conciliação automática" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Depois do fechamento" })).toContainText("meta zero");
  await audit(page, "métricas");

  // Playbooks (somente leitura): com ou sem versões, conforme os extratos já lidos na execução.
  await page.getByRole("navigation", { name: "Seções da Conciliação" }).getByRole("link", { name: "Playbooks" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Playbooks por banco" })).toBeVisible();
  await expect(page.getByRole("table", { name: "Playbooks por banco" }).or(page.getByText("Nenhum playbook ainda"))).toBeVisible();
  await audit(page, "playbooks");
});
