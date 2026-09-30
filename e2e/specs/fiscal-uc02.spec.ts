import { expect, type APIRequestContext, test } from "@playwright/test";

const COMPANY = "Empresa Fictícia Alfa Comércio Ltda";

interface GridCell {
  id: string;
  applicable: boolean;
  obligation_type: { code: string };
}
interface GridRow {
  company: { internal_code: string };
  period: { id: string; state: string; version: number };
  cells: Record<string, GridCell>;
}

async function json<T>(request: APIRequestContext, method: "get" | "post", url: string, data?: unknown): Promise<T> {
  const res = method === "get" ? await request.get(url) : await request.post(url, { data });
  expect(res.ok(), `${method.toUpperCase()} ${url}: ${res.status()} ${await res.text()}`).toBeTruthy();
  return (await res.json()) as T;
}

/** Deixa a Alfa (101) com todos os controles fiscais encerrados, exceto Entradas "A iniciar". */
async function prepare(request: APIRequestContext): Promise<string> {
  const { competence } = await json<{ competence: string }>(request, "get", "/api/v1/competencies/latest");
  const gridUrl = `/api/v1/modules/fiscal/grid?competence=${competence}&group=normal`;
  let row = (await json<{ rows: GridRow[] }>(request, "get", gridUrl)).rows.find((r) => r.company.internal_code === "101");
  if (!row) throw new Error("empresa 101 fora da grade Normal");
  if (row.period.state === "CLOSED") {
    await json(request, "post", `/api/v1/company-module-periods/${row.period.id}/reopen`, { version: row.period.version, reason: "Preparação do teste E2E" });
    row = (await json<{ rows: GridRow[] }>(request, "get", gridUrl)).rows.find((r) => r.company.internal_code === "101");
    if (!row) throw new Error("empresa 101 fora da grade Normal");
  }
  const cells = Object.values(row.cells).filter((c) => c.applicable && c.obligation_type.code !== "FISCAL_FECHAMENTO");
  const others = cells.filter((c) => c.obligation_type.code !== "FISCAL_ENTRADAS").map((c) => c.id);
  const entradas = cells.find((c) => c.obligation_type.code === "FISCAL_ENTRADAS");
  if (!entradas) throw new Error("Entradas não aplicável para a 101");
  await json(request, "post", "/api/v1/obligation-instances/bulk-status", { instance_ids: others, status_code: "CLOSED" });
  await json(request, "post", "/api/v1/obligation-instances/bulk-status", { instance_ids: [entradas.id], status_code: "NOT_STARTED" });
  const fechamento = row.cells.FISCAL_FECHAMENTO;
  if (fechamento) await json(request, "post", "/api/v1/obligation-instances/bulk-status", { instance_ids: [fechamento.id], status_code: "CHECKED" });
  return competence;
}

test("§53.3-4 UC-02 / CA-014 filtra, altera status inline, vê o progresso mudar e fecha a empresa pronta", async ({ page, request }) => {
  const competence = await prepare(request);

  await page.goto(`/fiscal?competence=${competence}`);
  await expect(page.getByRole("heading", { level: 1, name: "Fiscal" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Normais", pressed: true })).toBeVisible();

  // Filtrar: busca livre pela empresa.
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Alfa");
  const grid = page.getByRole("grid", { name: "Grade Fiscal" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await expect(grid.getByRole("rowheader")).toHaveText(COMPANY);
  await expect(grid.getByLabel(/^Progresso operacional: /)).not.toHaveText("100%");
  await expect(grid.getByText("Pronto para fechar")).toHaveCount(0);

  // Alterar status inline: Entradas → Fechadas.
  await grid.getByRole("button", { name: `Entradas — ${COMPANY}: A iniciar` }).click();
  await page.getByRole("menuitemradio", { name: "Fechadas" }).click();
  await expect(grid.getByRole("button", { name: `Entradas — ${COMPANY}: Fechadas` })).toBeVisible();

  // O progresso muda e a empresa fica pronta para fechar [CA-014].
  await expect(grid.getByLabel("Progresso operacional: 100%")).toBeVisible();
  await expect(grid.getByLabel("Conclusão final: 100%")).toBeVisible();
  await expect(grid.getByText("Pronto para fechar")).toBeVisible();

  // Fechar: a coluna Fechamento está "Conferido" → a API pede a confirmação do Encerrado [§13.6].
  await grid.getByRole("button", { name: `Fechar Fiscal de ${COMPANY}` }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "Fechar competência" }).click();
  await expect(dialog.getByRole("alert")).toContainText("Confirme para marcá-lo como Encerrado");
  await dialog.getByRole("button", { name: "Atualizar e fechar" }).click();
  await expect(dialog).toBeHidden();

  await expect(grid.getByText("Fechado", { exact: true })).toBeVisible();
  await expect(grid.getByRole("button", { name: `Reabrir Fiscal de ${COMPANY}` })).toBeVisible();
  await expect(grid.getByRole("button", { name: `Fechamento — ${COMPANY}: Encerrado` })).toBeVisible();
});
