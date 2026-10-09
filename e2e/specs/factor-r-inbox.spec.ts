import { expect, test } from "@playwright/test";
import { apiJson, cnpjWithCheckDigits, createCompany, uniqueDigits } from "../support/api";
import { formatCnpj, statement } from "../support/pgdas";

/**
 * Inbox PGDAS-D pela tela (TASK-014b; REQ-012, AC-008, AC-009, FR-08 a FR-11). O envio volta na
 * hora e o worker do E2E lê o extrato (parser determinístico, sem LLM). A empresa cadastrada pelo
 * extrato é sujeita ao Fator R: no fim ela é inativada, para a carteira da demonstração continuar
 * igual para os outros testes.
 */
test.describe.configure({ mode: "serial" });

const DISCLAIMER = "O PGDAS-D da Receita Federal prevalece.";
const createdSubjects: string[] = [];

test.afterAll(async ({ request }) => {
  for (const id of createdSubjects) {
    const res = await request.post(`/api/v1/companies/${id}/deactivate`, { data: {} });
    expect(res.ok(), await res.text()).toBeTruthy();
  }
});

test("AC-008: extrato de CNPJ cadastrado é lido, vinculado e lança a receita do PA em competência vazia", async ({ page, request }) => {
  const company = await createCompany(request, {
    name: "E2E Clínica do Inbox",
    competence: "2025-01",
    taxRegime: "SIMPLES_NACIONAL",
    payrollType: "PAYROLL",
    modules: ["FISCAL"],
    prefix: "61",
  });
  await page.goto("/factor-r/inbox");
  await expect(page.getByRole("heading", { level: 1, name: "Inbox PGDAS-D" })).toBeVisible();
  await page.getByLabel("Extratos do PGDAS-D (PDF ou TXT)").setInputFiles({ name: "extrato-conhecido.txt", mimeType: "text/plain", buffer: statement("txt_padrao", company.cnpj) });

  const sent = page.getByTestId("resultado-upload").filter({ hasText: "extrato-conhecido.txt" });
  await expect(sent).toContainText("Vinculado", { timeout: 30_000 });
  await expect(sent).toContainText(`Extrato vinculado a ${company.legal_name}.`);
  await expect(sent).toContainText("Receita do PA 2026-09 lançada na competência 2026-09");
  await expect(sent).toContainText(DISCLAIMER);

  // Detalhe: o que o parser leu, a empresa e o caminho para a ficha.
  await sent.getByRole("link", { name: "ver documento" }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Extrato PGDAS-D" })).toBeVisible();
  const fields = page.getByTestId("campos-extraidos");
  await expect(fields.locator("[data-campo=cnpj]")).toContainText(formatCnpj(company.cnpj));
  await expect(fields.locator("[data-campo=rpa]")).toContainText("R$ 50.000,00");
  await expect(fields.locator("[data-campo=pa]")).toContainText("09/2026");
  await expect(page.getByRole("link", { name: "Baixar original" })).toHaveAttribute("href", /\/api\/v1\/factor-r\/pgdas\/.+\/file$/);
  await expect(page.getByRole("button", { name: "Vincular" })).toHaveCount(0);

  // Ficha → aba Fator R: o extrato, a corrida do parser e o mês lançado com o link do extrato.
  await page.getByTestId("empresa-vinculada").getByRole("link").click();
  await expect(page).toHaveURL(/tab=factor-r&pa=2026-09/);
  await expect(page.getByRole("list", { name: "Extratos vinculados" })).toContainText("PA 09/2026");
  await expect(page.getByRole("list", { name: "Corridas da empresa" })).toContainText("Parser PGDAS-D");
  await page.getByRole("button", { name: "PA seguinte: 10/2026" }).click();
  await expect(page.getByRole("link", { name: "Abrir o extrato PGDAS-D de 09/2026" })).toBeVisible();
  await expect(page.getByLabel("Receita bruta 09/2026")).toHaveValue("50.000,00");
});

test("AC-009 e FR-11: CNPJ fora do cadastro fica em revisão, sem gravar nada, até o cadastro pelo extrato", async ({ page, request }) => {
  const digits = uniqueDigits();
  const cnpj = cnpjWithCheckDigits(`62${digits}`);
  await page.goto("/factor-r/inbox");
  await page.getByLabel("Extratos do PGDAS-D (PDF ou TXT)").setInputFiles({ name: "extrato-novo.txt", mimeType: "text/plain", buffer: statement("txt_padrao", cnpj) });

  const sent = page.getByTestId("resultado-upload").filter({ hasText: "extrato-novo.txt" });
  await expect(sent).toContainText("Em revisão", { timeout: 30_000 });
  await expect(sent).toContainText("precisa de revisão: cnpj_nao_encontrado");
  const block = sent.getByTestId("cadastro-pelo-extrato");
  await expect(block).toContainText(`Esse CNPJ não está no cadastro de empresas (${formatCnpj(cnpj)})`);
  await expect(block).toContainText("O extrato indica: CLINICA EXEMPLO LTDA.");
  // Nada é gravado sem a confirmação do analista.
  const before = await apiJson<{ items: unknown[] }>(request, "get", `/api/v1/companies?cnpj=${cnpj}`);
  expect(before.items).toHaveLength(0);

  await block.getByRole("button", { name: "Cadastrar e vincular" }).click();
  const dialog = page.getByRole("dialog", { name: "Cadastrar a empresa do extrato" });
  await expect(dialog.getByLabel("CNPJ")).toHaveValue(formatCnpj(cnpj));
  await expect(dialog.getByLabel("CNPJ")).toHaveJSProperty("readOnly", true);
  await expect(dialog.getByLabel("Razão social")).toHaveValue("CLINICA EXEMPLO LTDA");
  await expect(dialog.getByLabel("Fator R", { exact: true })).toHaveValue("true");
  await expect(dialog.getByLabel("Início da atividade (Fator R)")).toHaveValue("2019-03");
  await dialog.getByLabel("Código interno").fill(`E2E-PG-${digits}`);
  await dialog.getByLabel("Início do atendimento").fill("2026-09-01");
  await dialog.getByRole("button", { name: "Confirmar cadastro e vínculo" }).click();
  await expect(dialog).toBeHidden();
  await expect(sent).toContainText("Vinculado");
  await expect(sent).toContainText("Empresa CLINICA EXEMPLO LTDA cadastrada a partir do extrato.");

  const after = await apiJson<{ items: { id: string; legal_name: string; factor_r: boolean | null }[] }>(request, "get", `/api/v1/companies?cnpj=${cnpj}`);
  expect(after.items).toHaveLength(1);
  const created = after.items[0];
  if (!created) throw new Error("empresa não cadastrada");
  createdSubjects.push(created.id);
  expect(created).toMatchObject({ legal_name: "CLINICA EXEMPLO LTDA", factor_r: true });
  const runs = await apiJson<{ items: { trigger: string }[] }>(request, "get", `/api/v1/ai/runs?company_id=${created.id}`);
  expect(runs.items.map((r) => r.trigger)).toContain("cadastro_pelo_extrato");
});

test("FR-11: documento em revisão é rejeitado com motivo e sai da revisão", async ({ page }) => {
  await page.goto("/factor-r/inbox");
  await page.getByLabel("Extratos do PGDAS-D (PDF ou TXT)").setInputFiles({ name: "extrato-sem-cnpj.txt", mimeType: "text/plain", buffer: statement("txt_sem_cnpj") });
  const sent = page.getByTestId("resultado-upload").filter({ hasText: "extrato-sem-cnpj.txt" });
  await expect(sent).toContainText("Em revisão", { timeout: 30_000 });
  await expect(sent.getByTestId("cadastro-pelo-extrato")).toHaveCount(0);

  await sent.getByRole("link", { name: "ver documento" }).click();
  await expect(page.getByTestId("motivo")).toContainText("CNPJ fora do cadastro de empresas");
  await expect(page.getByTestId("campos-extraidos").locator("[data-campo=cnpj]")).toContainText("não encontrado");
  const reject = page.getByRole("button", { name: "Rejeitar" });
  await expect(reject).toBeDisabled();
  await page.getByLabel("Motivo da rejeição").fill("Extrato sem CNPJ: pedir o PDF original ao cliente.");
  await reject.click();
  await expect(page.getByTestId("motivo")).toContainText("Motivo da rejeição: Extrato sem CNPJ: pedir o PDF original ao cliente.");
  await expect(page.getByRole("button", { name: "Rejeitar" })).toHaveCount(0);

  // Filtro por status na lista.
  await page.getByRole("link", { name: "Inbox PGDAS-D" }).first().click();
  await page.getByRole("button", { name: /^Rejeitados \(\d+\)$/ }).click();
  await expect(page).toHaveURL(/status=rejected/);
  await expect(page.getByRole("table", { name: "Extratos do inbox" })).toContainText("extrato-sem-cnpj.txt");
  await expect(page.getByRole("note")).toContainText(DISCLAIMER);
});
