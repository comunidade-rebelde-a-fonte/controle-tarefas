import { expect, test } from "@playwright/test";
import { cnpjWithCheckDigits, uniqueDigits } from "../support/api";

function mask(cnpj: string): string {
  return cnpj.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, "$1.$2.$3/$4-$5");
}

test("§53.3-1 cadastrar empresa: a API recusa CNPJ inválido, a empresa é criada e o CNPJ é copiado sem máscara (RN-014)", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const digits = uniqueDigits();
  const code = `E2E-01-${digits}`;
  const name = `Empresa Fictícia Cadastro E2E ${digits}`;
  const cnpj = cnpjWithCheckDigits(`91${digits}`);
  const wrongDigit = cnpj.slice(0, 13) + String((Number(cnpj[13]) + 1) % 10);

  await page.goto("/companies");
  await page.getByRole("button", { name: "Nova empresa" }).click();
  const form = page.getByRole("form", { name: "Cadastrar empresa" });
  await form.getByLabel("Código interno").fill(code);
  await form.getByLabel("CNPJ").fill(wrongDigit);
  await form.getByLabel("Razão social").fill(name);
  await form.getByLabel("Regime inicial").selectOption("LUCRO_PRESUMIDO");
  await form.getByLabel("Início do atendimento").fill("2026-01-01");
  await form.getByLabel("Tipo de folha").selectOption("PAYROLL");

  // Dígito verificador errado: a validação é da API e a mensagem aparece no campo.
  await form.getByRole("button", { name: "Cadastrar empresa" }).click();
  await expect(form.getByText("CNPJ inválido: confira os 14 dígitos e os dígitos verificadores.")).toBeVisible();

  await form.getByLabel("CNPJ").fill(mask(cnpj));
  await expect(form.getByLabel("CNPJ")).toHaveValue(mask(cnpj));
  await form.getByRole("button", { name: "Cadastrar empresa" }).click();

  // Criada: a ficha abre direto na empresa nova.
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
  await expect(page.getByText("Empresa cadastrada", { exact: true })).toBeVisible();

  // Na lista, a busca pelo CNPJ sem máscara encontra a empresa, exibida com máscara.
  await page.goto("/companies");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill(cnpj);
  const table = page.getByRole("table", { name: "Empresas" });
  await expect(table.getByRole("link", { name })).toBeVisible();
  await expect(table.getByRole("link")).toHaveCount(1);
  const row = table.getByRole("row", { name: new RegExp(code) });
  await expect(row).toContainText(mask(cnpj));
  await expect(row).toContainText("Lucro Presumido");

  // Copiar CNPJ: a área de transferência recebe só os 14 dígitos [RN-014].
  await row.getByRole("button", { name: "Copiar CNPJ" }).click();
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(cnpj);
});
