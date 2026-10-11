import { readFileSync } from "node:fs";
import AxeBuilder from "@axe-core/playwright";
import { expect, type Page, test } from "@playwright/test";
import { apiJson } from "../support/api";
import { createHorizonte, uploadAndProcess } from "../support/reconciliation";

/**
 * TEST-021b — Lançamentos, razão, plano de contas e regras da Conciliação (REQ-023, REQ-026 a
 * REQ-028; AC-020 a AC-022): o extrato do Banco Beta do cenário Horizonte lido no worker (o do
 * Banco A Fonte é do TEST-021a: o mesmo arquivo e a mesma conta não entram duas vezes), os
 * lançamentos pela tela (formulário e CSV), a rodada, o plano e as regras, e o razão com o
 * drill-down até o original e a exportação CSV, com a varredura de acessibilidade (axe).
 */
const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"];
const ESCOLA_CNPJ = "45.678.901/0001-23";

async function audit(page: Page, where: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const blocking = violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map((v) => `${v.id} [${v.impact}] ${v.help}: ${v.nodes.slice(0, 5).map((n) => n.target.join(" ")).join(" | ")}`);
  expect.soft(blocking, `axe em ${where}`).toEqual([]);
}

function csv(name: string, text: string) {
  return { name, mimeType: "text/csv", buffer: Buffer.from(text, "utf-8") };
}

test("TEST-021b: lançamentos, plano, regras e o razão com drill-down e exportação", async ({ page, request }) => {
  test.setTimeout(150_000);
  const company = await createHorizonte(request);
  const statement = await uploadAndProcess(request, "extratos/extrato_banco_beta_2026-08.pdf", company.id);
  expect(statement.status).toBe("importado");

  // Lançamentos do cliente: o recebimento da Escola Crescer pelo formulário (com o CNPJ) e o boleto
  // da Madeireira Pinheiro pelo CSV, que antes é recusado inteiro por uma linha inválida.
  await page.goto(`/reconciliation/entries?company_id=${company.id}`);
  await expect(page.getByRole("heading", { level: 1, name: "Lançamentos" })).toBeVisible();
  const form = page.getByRole("form", { name: "Novo lançamento" });
  await form.getByLabel(/^Data/).fill("2026-08-26");
  await form.getByLabel(/^Valor/).fill("20000,00");
  await form.getByLabel(/^Sentido/).selectOption("C");
  await form.getByLabel(/^Contraparte/).fill("Escola Crescer Ltda");
  await form.getByLabel(/^CPF\/CNPJ/).fill(ESCOLA_CNPJ);
  await form.getByRole("button", { name: "Incluir" }).click();
  await expect(page.getByText("Lançamento incluído", { exact: true })).toBeVisible();
  const entries = page.getByRole("table", { name: "Lançamentos do cliente" });
  await expect(entries.getByRole("row").filter({ hasText: "Escola Crescer Ltda" })).toContainText("45678901000123");

  const file = page.getByLabel("Ou importe o CSV do cliente");
  await file.setInputFiles(csv("cliente.csv", "data;valor;sentido;contraparte\n2026-08-10;12640,00;X;Madeireira Pinheiro Ltda\n"));
  await page.getByRole("button", { name: "Importar CSV" }).click();
  await expect(page.getByRole("list", { name: "Linhas com erro" })).toContainText("Linha 2: Sentido inválido: use C (entrada) ou D (saída).");
  await file.setInputFiles(csv("cliente.csv", "data;valor;sentido;contraparte\n2026-08-10;12640,00;D;Madeireira Pinheiro Ltda\n"));
  await page.getByRole("button", { name: "Importar CSV" }).click();
  await expect(page.getByText("1 lançamento importado", { exact: true })).toBeVisible();
  await expect(entries.getByText("Madeireira Pinheiro Ltda")).toBeVisible();
  await audit(page, "lançamentos");

  // A rodada da competência casa as duas linhas pelo passe 1.
  const run = page.getByRole("region", { name: "Conciliar a competência" });
  await run.getByLabel("Competência").fill("2026-08");
  await run.getByRole("button", { name: "Rodar a conciliação da competência" }).click();
  await expect(run.getByText(/^Rodada concluída:/)).toBeVisible();
  await expect(run.getByRole("link", { name: "Abrir a fila de revisão" })).toHaveAttribute(
    "href",
    `/reconciliation?company_id=${company.id}&competence=2026-08`,
  );

  // A tarifa do banco não tem comprovante: confirmada pela API (a tela é do TEST-021a).
  const queue = await apiJson<{ items: { id: string; kind: string; summary: string }[] }>(
    request,
    "get",
    `/api/v1/reconciliation/review-queue?company_id=${company.id}&competence=2026-08&bucket=extrato_sem`,
  );
  const tariff = queue.items.find((i) => i.kind === "linha" && /tarifa/i.test(i.summary));
  expect(tariff, "a tarifa não está na fila").toBeDefined();
  await apiJson(request, "post", `/api/v1/reconciliation/review/${tariff?.id ?? ""}/confirm-no-receipt`);

  // Plano de contas: uma conta pelo formulário e duas pelo CSV; desativar pede confirmação.
  await page.getByRole("navigation", { name: "Seções da Conciliação" }).getByRole("link", { name: "Plano de contas" }).click();
  await expect(page).toHaveURL(new RegExp(`/reconciliation/chart\\?company_id=${company.id}$`));
  await expect(page.getByText("Plano vazio")).toBeVisible();
  const accountForm = page.getByRole("form", { name: "Nova conta" });
  await accountForm.getByLabel(/^Código/).fill("4.1.01");
  await accountForm.getByLabel(/^Nome/).fill("Fornecedores");
  await accountForm.getByRole("button", { name: "Incluir" }).click();
  await expect(page.getByText("Conta 4.1.01 incluída", { exact: true })).toBeVisible();
  await page.getByLabel("Arquivo do plano (codigo;nome)").setInputFiles(csv("plano.csv", "codigo;nome\n1.1.02;Clientes\n5.1.01;Despesas bancárias\n"));
  await page.getByRole("button", { name: "Importar CSV" }).click();
  await expect(page.getByText("2 contas novas e 0 atualizadas.", { exact: true })).toBeVisible();
  const chart = page.getByRole("table", { name: "Contas do plano" });
  await expect(chart.getByRole("row")).toHaveCount(4);
  await chart.getByRole("button", { name: "Desativar a conta 1.1.02" }).click();
  await page.getByRole("alertdialog", { name: "Desativar a conta 1.1.02?" }).getByRole("button", { name: "Desativar" }).click();
  await expect(chart.getByRole("row").filter({ hasText: "1.1.02" })).toContainText("Desativada");
  await chart.getByRole("button", { name: "Reativar a conta 1.1.02" }).click();
  await expect(chart.getByRole("row").filter({ hasText: "1.1.02" })).toContainText("Ativa");
  await audit(page, "plano de contas");

  // Regras: contraparte, CNPJ e descritivo; aplicadas às linhas conciliadas que esperam conta.
  await page.getByRole("navigation", { name: "Seções da Conciliação" }).getByRole("link", { name: "Regras" }).click();
  const ruleForm = page.getByRole("form", { name: "Nova regra" });
  for (const [type, pattern, account] of [
    ["contraparte", "Madeireira Pinheiro Ltda", "4.1.01"],
    ["cnpj", ESCOLA_CNPJ, "1.1.02"],
    ["regex_descritivo", "tarifa", "5.1.01"],
  ] as const) {
    await ruleForm.getByLabel(/^Tipo/).selectOption(type);
    await ruleForm.getByLabel(/^Padrão/).fill(pattern);
    await ruleForm.getByLabel(/^Conta/).selectOption(account);
    await ruleForm.getByRole("button", { name: "Confirmar regra" }).click();
    await expect(page.getByRole("table", { name: "Regras da empresa" }).getByRole("row")).toHaveCount(
      1 + ["contraparte", "cnpj", "regex_descritivo"].indexOf(type) + 1,
    );
  }
  // A mais nova primeiro (a ordem em que valem); o CNPJ fica só com os dígitos.
  const rules = page.getByRole("table", { name: "Regras da empresa" });
  await expect(rules.getByRole("row").nth(1)).toContainText("tarifa");
  await expect(rules.getByRole("row").nth(2)).toContainText("45678901000123");
  const apply = page.getByRole("region", { name: "Criar e aplicar regras" });
  await apply.getByLabel("Competência").fill("2026-08");
  await apply.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.getByText("Linhas: 3 por regra, 0 por histórico e 0 à espera do agente ou do contador.", { exact: true })).toBeVisible();
  await audit(page, "regras");

  // Razão: as três conciliações, com a conta e a origem; filtro por conta e totais.
  await page.goto(`/reconciliation/ledger?company_id=${company.id}&from=2026-08&to=2026-08`);
  const ledger = page.getByRole("table", { name: "Linhas do razão" });
  await expect(ledger.getByRole("row")).toHaveCount(5); // cabeçalho, 3 linhas e o total
  await expect(ledger.getByRole("row").filter({ hasText: /madeireira/i })).toContainText("4.1.01 · Fornecedores");
  await expect(ledger.getByRole("row").filter({ hasText: /escola crescer/i })).toContainText("1.1.02 · Clientes");
  await expect(ledger.getByRole("row").filter({ hasText: /tarifa/i })).toContainText("5.1.01 · Despesas bancárias");
  await expect(ledger.getByRole("row").filter({ hasText: "Total do período" })).toContainText("R$ 12.675,00R$ 20.000,00");
  await audit(page, "razão");
  await page.getByLabel("Conta bancária ou contábil").fill("5.1.01");
  await page.getByRole("button", { name: "Filtrar" }).click();
  await expect(ledger.getByRole("row")).toHaveCount(3);
  await page.getByLabel("Conta bancária ou contábil").fill("");
  await page.getByRole("button", { name: "Filtrar" }).click();
  await expect(ledger.getByRole("row")).toHaveCount(5);

  // Exportação CSV do razão filtrado (AC-022): o arquivo traz as três linhas com a conta e a origem.
  const downloading = page.waitForEvent("download");
  await page.getByRole("link", { name: "Exportar CSV" }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toMatch(/^razao-\d{14}-2026-08-2026-08\.csv$/);
  const lines = readFileSync(await download.path(), "utf-8").trim().split(/\r?\n/);
  expect(lines[0]).toBe("data;conta;historico;debito;credito;competencia;conta_contabil;nome_conta;contraparte;origem_classificacao;origem_conciliacao");
  expect(lines).toHaveLength(4);
  expect(lines.find((l) => /madeireira/i.test(l))).toMatch(/^2026-08-10;.*;12640\.00;0\.00;2026-08;4\.1\.01;Fornecedores;Madeireira Pinheiro Ltda;regra;auto$/);

  // Drill-down (AC-021): classificação → conciliação → transação → documento original.
  await ledger.getByRole("row").filter({ hasText: /madeireira/i }).getByRole("button", { name: /^Origem da linha de 10\/08\/2026/ }).click();
  const dialog = page.getByRole("dialog", { name: "Origem da linha do razão" });
  const trail = dialog.getByRole("list", { name: "Rastro da linha do razão" });
  await expect(trail).toContainText("Pela regra Contraparte Madeireira Pinheiro Ltda");
  await expect(trail).toContainText("Automática, passe 1");
  await expect(trail).toContainText("Lançamento manual (sem original)");
  await audit(page, "drill-down do razão");
  await dialog.getByRole("link", { name: "Extrato bancário" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "extrato_banco_beta_2026-08.pdf" })).toBeVisible();
});
