import { expect, test } from "@playwright/test";

/**
 * UC-05 — Renovar certificado [PRD §19, §33]. O seed de demonstração deixa o certificado da
 * Gama Indústria (103) vencido e aguardando o cliente; a procuração dela vence em 10 dias.
 */
const GAMA = "Empresa Fictícia Gama Indústria Ltda";
const DELTA = "Empresa Fictícia Delta Tecnologia Ltda";

function nextYear(): string {
  const d = new Date();
  d.setFullYear(d.getFullYear() + 1);
  return d.toISOString().slice(0, 10);
}

test("UC-05 filtra vencimentos, avança as etapas e renova o certificado com nova validade", async ({ page }) => {
  await page.goto("/credentials");
  await expect(page.getByRole("heading", { level: 1, name: "Procurações e Certificados" })).toBeVisible();

  // 1-2. Filtrar vencimentos.
  await page.getByRole("button", { name: "Vencidos" }).click();
  await expect(page).toHaveURL(/filter=expired/);
  const grid = page.getByRole("grid", { name: "Procurações e Certificados" });
  const gama = grid.getByRole("row", { name: new RegExp(GAMA) });
  await expect(gama).toBeVisible();
  await expect(gama.getByText("Vencido")).toBeVisible();

  const stage = page.getByLabel(`Etapa de renovação — Certificado digital — ${GAMA}`);

  // 3-4. Renovação solicitada e, depois, aguardando cliente (cada etapa é gravada).
  await stage.selectOption("RENEWAL_REQUESTED");
  await expect(stage).toBeEnabled();
  await page.reload();
  await expect(stage).toHaveValue("RENEWAL_REQUESTED");
  await stage.selectOption("AWAITING_CLIENT");
  await expect(stage).toBeEnabled();
  await expect(stage).toHaveValue("AWAITING_CLIENT");

  // 5. Renovado pede a nova validade.
  await stage.selectOption("RENEWED");
  const dialog = page.getByRole("dialog", { name: "Renovar certificado digital" });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: "Confirmar renovação" }).click();
  await expect(dialog.getByText("Informe a nova data de validade.")).toBeVisible();
  await dialog.getByLabel(/Nova data de validade/).fill(nextYear());
  await dialog.getByRole("button", { name: "Confirmar renovação" }).click();
  await expect(dialog).toBeHidden();
  await expect(page.getByText("Renovação registrada", { exact: true })).toBeVisible();

  // 6. Situação volta a Vigente: sai dos vencidos e aparece como Vigente.
  await expect(grid.getByRole("row", { name: new RegExp(GAMA) })).toHaveCount(0);
  await page.getByRole("button", { name: "Todos" }).click();
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Gama Indústria");
  const renewed = grid.getByRole("row", { name: new RegExp(GAMA) });
  // Selo de situação do certificado (a procuração da Gama vence em 15 dias).
  await expect(renewed.locator("span").filter({ hasText: /^Vigente$/ })).toBeVisible();
  await expect(page.getByLabel(`Etapa de renovação — Certificado digital — ${GAMA}`)).toHaveValue("CURRENT");
});

test("CA-020 recusa .pfx com a mensagem do backend e aceita a procuração em PDF", async ({ page }) => {
  await page.goto("/credentials?q=Delta%20Tecnologia");
  const grid = page.getByRole("grid", { name: "Procurações e Certificados" });
  await expect(grid.getByRole("rowheader")).toHaveText(DELTA);
  await grid.getByRole("button", { name: new RegExp(`Documentos e observações — ${DELTA}`) }).click();

  const dialog = page.getByRole("dialog", { name: `Documentos — ${DELTA}` });
  await expect(dialog).toBeVisible();

  // Contêiner PKCS#12 sintético (sem chave real): recusado pelo backend.
  await dialog.getByLabel(/Enviar documento — Certificado digital/).setInputFiles({
    name: "certificado.pfx",
    mimeType: "application/x-pkcs12",
    buffer: Buffer.from([0x30, 0x80, 0x02, 0x01, 0x03, 0x30, 0x80]),
  });
  await expect(dialog.getByRole("alert")).toHaveText("Chaves privadas não são armazenadas pelo sistema.");
  await expect(dialog.getByText("Nenhum documento enviado.")).toHaveCount(2);

  // PDF da procuração: aceito e listado como documento vigente.
  await dialog.getByLabel(/Enviar documento — Procuração RFB/).setInputFiles({
    name: "procuracao-delta.pdf",
    mimeType: "application/pdf",
    buffer: Buffer.from("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n%%EOF\n"),
  });
  await expect(dialog.getByText("procuracao-delta.pdf")).toBeVisible();
  await expect(dialog.getByText("Nenhum documento enviado.")).toHaveCount(1);
});

test("T-M4-11 editar a validade na ficha da empresa reflete na tela geral", async ({ page, request }) => {
  const EPSILON = "Empresa Fictícia Épsilon Logística Ltda";
  const res = await request.get("/api/v1/credential-controls");
  expect(res.ok()).toBeTruthy();
  const list = (await res.json()) as { items: { company: { id: string; legal_name: string } }[] };
  const company = list.items.find((r) => r.company.legal_name === EPSILON);
  if (!company) throw new Error("empresa 105 fora da lista de credenciais");

  await page.goto(`/companies/${company.company.id}?tab=credentials`);
  const due = page.getByLabel(`Vencimento — Certificado digital — ${EPSILON}`);
  await expect(due).toBeVisible();
  await due.fill("2027-03-15");
  await due.press("Enter");
  await expect(due).toBeEnabled();
  await expect(due).toHaveValue("2027-03-15");

  await page.goto("/credentials?q=Épsilon");
  await expect(page.getByLabel(`Vencimento — Certificado digital — ${EPSILON}`)).toHaveValue("2027-03-15");
});
