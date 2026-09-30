import { expect, test } from "@playwright/test";

/** Completa 12 dígitos com os dígitos verificadores do CNPJ (dados fictícios de teste). */
function cnpjWithCheckDigits(base12: string): string {
  const digit = (base: string, weights: number[]) => {
    const sum = [...base].reduce((acc, d, i) => acc + Number(d) * (weights[i] ?? 0), 0);
    const r = sum % 11;
    return r < 2 ? "0" : String(11 - r);
  };
  const first = digit(base12, [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]);
  const second = digit(base12 + first, [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2]);
  return base12 + first + second;
}

const HEADERS = ["Código interno", "Razão social", "CNPJ", "Regime tributário inicial", "Início do atendimento", "Tipo de folha", "Módulo Fiscal"];

test("T-M6-04 importar planilha com uma linha inválida mostra o erro e importa só as válidas", async ({ page }) => {
  const csv = [
    HEADERS.join(";"),
    `801;Importada Um Ltda;${cnpjWithCheckDigits("000088010001")};Simples Nacional;01/02/2026;Sem folha;Sim`,
    `802;Importada Com Erro Ltda;00.000.001/0001-99;Lucro Real;01/02/2026;Folha;Sim`,
    `803;Importada Três Ltda;${cnpjWithCheckDigits("000088030001")};Lucro Presumido;01/03/2026;Folha;Sim`,
  ].join("\n");

  await page.goto("/imports");
  await expect(page.getByRole("heading", { level: 1, name: "Importação/Exportação" })).toBeVisible();
  await page.getByLabel(/Planilha \(\.xlsx ou \.csv/).setInputFiles({ name: "empresas.csv", mimeType: "text/csv", buffer: Buffer.from(csv, "utf-8") });
  await page.getByRole("button", { name: "Validar planilha" }).click();

  const table = page.getByRole("table", { name: "Prévia da importação" });
  await expect(table).toBeVisible();
  await expect(table.getByText("CNPJ inválido: confira os 14 dígitos e os dígitos verificadores.")).toBeVisible();
  await expect(page.getByText(/1 com erro \(não serão importadas\)/)).toBeVisible();

  await page.getByRole("button", { name: "Importar 2 empresas" }).click();
  await expect(page.getByText("Importação concluída")).toBeVisible();
  await expect(page.getByRole("link", { name: "801 — Importada Um Ltda" })).toBeVisible();
  await expect(page.getByRole("link", { name: "803 — Importada Três Ltda" })).toBeVisible();

  await page.goto("/companies?q=Importada");
  await expect(page.getByText("Importada Um Ltda")).toBeVisible();
  await expect(page.getByText("Importada Três Ltda")).toBeVisible();
  await expect(page.getByText("Importada Com Erro Ltda")).toHaveCount(0);
});
