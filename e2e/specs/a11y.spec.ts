import AxeBuilder from "@axe-core/playwright";
import { expect, type Locator, type Page, test } from "@playwright/test";
import { apiJson, bulkStatus, cnpjWithCheckDigits, fixedToday, latestCompetence, openGridRow, uniqueDigits } from "../support/api";
import { statement, uploadAndWait } from "../support/pgdas";

/**
 * Acessibilidade [PRD §28.3, §45.4]: axe (WCAG 2.2 A/AA) em todas as telas e modais, sem violações
 * serious/critical; grade operável só pelo teclado; status e atraso com texto ou ícone além da cor.
 */
const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22a", "wcag22aa"];

async function audit(page: Page, where: string): Promise<void> {
  const { violations } = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const blocking = violations
    .filter((v) => v.impact === "serious" || v.impact === "critical")
    .map((v) => `${v.id} [${v.impact}] ${v.help}: ${v.nodes.slice(0, 5).map((n) => n.target.join(" ")).join(" | ")}`);
  expect.soft(blocking, `axe em ${where}`).toEqual([]);
}

/** Espera a tela carregar (título e fim dos esqueletos) antes da varredura. */
async function ready(page: Page, heading?: string | RegExp): Promise<void> {
  await expect(page.getByRole("heading", { level: 1, ...(heading ? { name: heading } : {}) })).toBeVisible();
  await expect(page.locator(".animate-pulse")).toHaveCount(0);
}

const EPSILON = "Empresa Fictícia Épsilon Logística Ltda";

test("T-M7-04 axe: todas as telas sem violações serious/critical", async ({ page, request }) => {
  test.setTimeout(120_000); // ~30 varreduras completas
  const competence = await latestCompetence(request);
  const screens: [string, string | RegExp][] = [
    [`/dashboard?competence=${competence}`, "Dashboard"],
    ["/companies", "Empresas"],
    [`/fiscal?competence=${competence}&group=normal`, "Fiscal"],
    [`/fiscal?competence=${competence}&group=simples`, "Fiscal"],
    [`/declarations?competence=${competence}`, "Declarações"],
    [`/accounting?competence=${competence}`, "Contábil"],
    [`/payroll?competence=${competence}`, "Folha"],
    [`/dctfweb?competence=${competence}`, "DCTFWeb"],
    [`/factor-r?pa=${competence}`, "Fator R"],
    ["/factor-r/inbox", "Inbox PGDAS-D"],
    [`/factor-r/agents?pa=${competence}`, "Agentes do Fator R"],
    ["/factor-r/runs", "Observabilidade dos agentes"],
    ["/credentials", "Procurações e Certificados"],
    [`/backlog?competence=${competence}`, "Backlog"],
    [`/obligations?competence=${competence}&state=overdue`, /Atrasadas/],
    ["/imports", /Importa/],
    ["/reports", "Relatórios"],
    ["/pagina-inexistente", /./],
  ];
  for (const [url, heading] of screens) {
    await page.goto(url);
    await ready(page, heading);
    await audit(page, url);
  }

  for (const tab of ["Catálogo de obrigações", "Regras de vencimento", "Feriados", "Faixas de validade", "Responsáveis", "IA"]) {
    await page.goto("/settings");
    await ready(page, "Cadastros e Configurações");
    await page.getByRole("tab", { name: tab, exact: true }).click();
    await expect(page.locator(".animate-pulse")).toHaveCount(0);
    await audit(page, `/settings — ${tab}`);
  }

  await page.goto("/companies");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Alfa Comércio");
  await page.getByRole("link", { name: "Empresa Fictícia Alfa Comércio Ltda" }).click();
  await ready(page, "Empresa Fictícia Alfa Comércio Ltda");
  for (const tab of [
    "Dados gerais",
    "Regime tributário",
    "Módulos",
    "Obrigações/aplicabilidade",
    "Folha",
    "Fator R",
    "Responsáveis",
    "Procurações/Certificados",
    "Histórico de competências",
  ]) {
    await page.getByRole("tab", { name: tab }).click();
    await expect(page.locator(".animate-pulse")).toHaveCount(0);
    await audit(page, `ficha da empresa — ${tab}`);
  }

  // Fator R de uma empresa da carteira: painel, simulador, consultor, grade mensal editável,
  // extratos, corridas e configuração.
  await page.goto("/factor-r");
  await page.getByRole("link", { name: "Brisa Serviços Digitais Ltda" }).click();
  await ready(page, "Brisa Serviços Digitais Ltda");
  await audit(page, "ficha da empresa — Fator R (Brisa, com mês do PGDAS-D)");
  await page.getByRole("button", { name: "Simular" }).click();
  await expect(page.getByTestId("resultado-simulacao")).toBeVisible({ timeout: 15_000 });
  await audit(page, "ficha da empresa — Fator R, resultado do simulador");
});

test("TASK-014b axe: documento do inbox, cadastro pelo extrato, agentes e trace", async ({ page, request }) => {
  test.setTimeout(90_000);
  // Extrato de um CNPJ fora do cadastro: fica em revisão, com a oferta de cadastro.
  const doc = await uploadAndWait(request, "extrato-a11y.txt", statement("txt_padrao", cnpjWithCheckDigits(`63${uniqueDigits()}`)));
  expect(doc.status).toBe("needs_review");
  await page.goto(`/factor-r/inbox/${doc.id}`);
  await ready(page, "Extrato PGDAS-D");
  await audit(page, "documento do inbox em revisão");
  await page.getByRole("button", { name: "Cadastrar e vincular" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await audit(page, "diálogo Cadastrar a empresa do extrato");
  await page.keyboard.press("Escape");

  // Chat do priorizador com a fila e o trace da corrida.
  await page.goto("/factor-r/agents");
  await ready(page, "Agentes do Fator R");
  const chat = page.getByTestId("chat-priorizador");
  await chat.getByLabel("Pergunta ao Priorizador").fill("O que priorizar esta semana?");
  await chat.getByRole("button", { name: "Enviar" }).click();
  await expect(chat.getByTestId("fila-priorizador")).toBeVisible({ timeout: 15_000 });
  await audit(page, "agentes — fila do priorizador");
  await chat.getByRole("link", { name: "ver trace" }).click();
  await ready(page, "Trace do Priorizador");
  await audit(page, "trace da corrida");
});

test("T-M7-04 axe: diálogos, menus e popovers sem violações serious/critical", async ({ page, request }) => {
  test.setTimeout(90_000);
  const competence = await latestCompetence(request);
  await openGridRow(request, "fiscal", competence, "105", "&group=normal");

  // Cadastro de empresa.
  await page.goto("/companies");
  await ready(page, "Empresas");
  await page.getByRole("button", { name: "Nova empresa" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await audit(page, "diálogo Nova empresa");
  await page.keyboard.press("Escape");

  // Menu de ações da empresa e edição.
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Épsilon");
  await page.getByRole("button", { name: `Ações de ${EPSILON}` }).click();
  await expect(page.getByRole("menu")).toBeVisible();
  await audit(page, "menu de ações da empresa");
  await page.getByRole("menuitem", { name: "Editar" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.getByRole("dialog").locator(".animate-pulse")).toHaveCount(0);
  await audit(page, "diálogo Editar empresa");
  await page.keyboard.press("Escape");

  // Grade: menu de status, detalhes, vencimento, histórico, observação e fechamento.
  await page.goto(`/fiscal?competence=${competence}&group=normal`);
  await ready(page, "Fiscal");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Épsilon");
  const grid = page.getByRole("grid", { name: "Grade Fiscal" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);

  await grid.getByRole("button", { name: new RegExp(`^Entradas — ${EPSILON}: `) }).click();
  await expect(page.getByRole("menu")).toBeVisible();
  await audit(page, "menu de status da célula");
  await page.keyboard.press("Escape");

  await grid.getByRole("button", { name: `Detalhes de Entradas — ${EPSILON}` }).click();
  const details = page.getByRole("dialog", { name: "Detalhes de Entradas" });
  await expect(details).toBeVisible();
  await audit(page, "popover de detalhes da célula");
  await details.getByRole("button", { name: "Alterar vencimento" }).click();
  await expect(page.getByRole("dialog", { name: "Alterar vencimento" })).toBeVisible();
  await audit(page, "diálogo Alterar vencimento");
  await page.keyboard.press("Escape");

  await grid.getByRole("button", { name: `Detalhes de Entradas — ${EPSILON}` }).click();
  await page.getByRole("dialog", { name: "Detalhes de Entradas" }).getByRole("button", { name: "Ver histórico" }).click();
  await expect(page.getByRole("dialog", { name: "Histórico" })).toBeVisible();
  await audit(page, "painel de histórico");
  await page.keyboard.press("Escape");

  await grid.getByRole("button", { name: new RegExp(`^Observação de ${EPSILON}`) }).click();
  await expect(grid.getByRole("textbox", { name: `Observação de ${EPSILON}` })).toBeFocused();
  await audit(page, "edição da observação");
  await page.keyboard.press("Escape");

  await grid.getByRole("button", { name: `Fechar Fiscal de ${EPSILON}` }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await audit(page, "diálogo Fechar módulo");
  await page.keyboard.press("Escape");

  // Seleção de linhas e barra de ações em massa.
  await grid.getByRole("checkbox", { name: `Selecionar ${EPSILON}` }).check();
  await expect(page.getByRole("region", { name: "Ações em massa" })).toBeVisible();
  await audit(page, "barra de ações em massa");

  // Reabertura (período fechado pela API).
  const row = await openGridRow(request, "accounting", competence, "105");
  await apiJson(request, "post", `/api/v1/company-module-periods/${row.period.id}/close`, { version: row.period.version, confirm_set_closing: true });
  await page.goto(`/accounting?competence=${competence}`);
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Épsilon");
  await page.getByRole("button", { name: `Reabrir Contábil de ${EPSILON}` }).click();
  await expect(page.getByRole("dialog", { name: "Reabrir Contábil" })).toBeVisible();
  await audit(page, "diálogo Reabrir módulo");
  await page.keyboard.press("Escape");
});

test("T-M7-04 axe: diálogos de empresas, DCTFWeb, configurações, credenciais e importação", async ({ page, request }) => {
  test.setTimeout(90_000);
  const competence = await latestCompetence(request);

  // Inativar empresa (cancelado).
  await page.goto("/companies");
  await ready(page, "Empresas");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Épsilon");
  await page.getByRole("button", { name: `Ações de ${EPSILON}` }).click();
  await page.getByRole("menuitem", { name: "Inativar" }).click();
  await expect(page.getByRole("alertdialog")).toBeVisible();
  await audit(page, "diálogo Inativar empresa");
  await page.keyboard.press("Escape");

  // DCTFWeb com eSocial pendente: aviso de pré-requisitos (cancelado, nada é gravado).
  await openGridRow(request, "payroll", competence, "105");
  const dctf = await openGridRow(request, "dctfweb", competence, "105");
  await bulkStatus(request, [dctf.cells.ESOCIAL?.id ?? ""], "NOT_STARTED");
  await bulkStatus(request, [dctf.cells.DCTFWEB?.id ?? ""], "NOT_STARTED");
  await page.goto(`/dctfweb?competence=${competence}`);
  await ready(page, "DCTFWeb");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Épsilon");
  const grid = page.getByRole("grid", { name: "Grade DCTFWeb" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);
  await grid.getByRole("button", { name: `DCTFWeb — ${EPSILON}: A iniciar` }).click();
  await page.getByRole("menuitemradio", { name: "Transmitida" }).click();
  await expect(page.getByRole("dialog", { name: "Pré-requisitos pendentes" })).toBeVisible();
  await audit(page, "diálogo Pré-requisitos pendentes");
  await page.keyboard.press("Escape");

  // Configurações: edição de regra de vencimento e de tipo do catálogo (canceladas).
  await page.goto("/settings");
  await ready(page, "Cadastros e Configurações");
  await page.getByRole("tab", { name: "Regras de vencimento" }).click();
  await page.getByRole("table", { name: "Regras de vencimento" }).getByRole("button", { name: /^Editar / }).first().click();
  await expect(page.getByRole("dialog", { name: "Editar regra de vencimento" })).toBeVisible();
  await audit(page, "diálogo Editar regra de vencimento");
  await page.keyboard.press("Escape");
  await page.getByRole("tab", { name: "Catálogo de obrigações" }).click();
  await page.getByRole("table", { name: /^Catálogo — / }).first().getByRole("button", { name: /^Editar / }).first().click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await audit(page, "diálogo Editar tipo de obrigação");
  await page.keyboard.press("Escape");

  // Configurações → IA: edição do alias com o resultado da validação (cancelada, nada gravado).
  await page.getByRole("tab", { name: "IA", exact: true }).click();
  await page.getByRole("button", { name: "Editar extrator", exact: true }).click();
  const aiDialog = page.getByRole("dialog", { name: "Configurar extrator" });
  await aiDialog.getByLabel("Escolher o modelo").check();
  await aiDialog.getByRole("button", { name: "Validar configuração" }).click();
  await expect(aiDialog.getByTestId("resultado-validacao")).toBeVisible();
  await audit(page, "diálogo Configurar alias de IA (validação aprovada)");
  await aiDialog.getByLabel("Modelo", { exact: true }).fill("gpt-6-texto");
  await aiDialog.getByRole("button", { name: "Validar configuração" }).click();
  await expect(aiDialog.getByTestId("resultado-validacao")).toContainText("Configuração reprovada");
  await audit(page, "diálogo Configurar alias de IA (validação reprovada)");
  await page.keyboard.press("Escape");

  // Credenciais: documentos e observações; "Renovado" pede a nova validade (cancelado).
  await page.goto("/credentials");
  await ready(page, "Procurações e Certificados");
  await page.getByRole("button", { name: new RegExp(`^Documentos e observações — ${EPSILON}`) }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await audit(page, "diálogo Documentos e observações");
  await page.keyboard.press("Escape");
  await page.getByLabel(`Etapa de renovação — Certificado digital — ${EPSILON}`).selectOption({ label: "Renovado" });
  await expect(page.getByRole("dialog")).toBeVisible();
  await audit(page, "diálogo Renovado");
  await page.keyboard.press("Escape");

  // Importação: prévia da validação (sem importar).
  const csv = [
    "Código interno;Razão social;CNPJ;Regime tributário inicial;Início do atendimento;Tipo de folha;Módulo Fiscal",
    "A11Y-1;Empresa Fictícia Acessibilidade Ltda;00.000.001/0001-99;Lucro Real;01/02/2026;Folha;Sim",
  ].join("\n");
  await page.goto("/imports");
  await ready(page, /Importa/);
  await page.getByLabel(/Planilha \(\.xlsx ou \.csv/).setInputFiles({ name: "empresas.csv", mimeType: "text/csv", buffer: Buffer.from(csv, "utf-8") });
  await page.getByRole("button", { name: "Validar planilha" }).click();
  await expect(page.getByRole("table", { name: "Prévia da importação" })).toBeVisible();
  await audit(page, "prévia da importação");
});

/** Move o foco pela grade só com o teclado até a célula que contém o botão com o nome dado. */
async function arrowTo(page: Page, key: "ArrowRight" | "ArrowDown", target: RegExp): Promise<void> {
  for (let i = 0; i < 40; i++) {
    const labels = await page.evaluate(() =>
      Array.from(document.activeElement?.querySelectorAll("button") ?? []).map((b) => b.getAttribute("aria-label") ?? ""),
    );
    if (labels.some((l) => target.test(l))) return;
    await page.keyboard.press(key);
  }
  throw new Error(`célula ${target} não alcançada pelo teclado`);
}

function focusedMenuItem(page: Page): Locator {
  return page.locator("[role=menuitemradio]:focus");
}

test("T-M7-04 teclado na grade: navegar, abrir o menu de status e alterar sem mouse", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  const row = await openGridRow(request, "fiscal", competence, "105", "&group=normal");
  await bulkStatus(request, [row.cells.FISCAL_ENTRADAS?.id ?? ""], "NOT_STARTED");

  await page.goto(`/fiscal?competence=${competence}&group=normal`);
  await ready(page, "Fiscal");
  const search = page.getByRole("searchbox", { name: "Buscar empresa" });
  await search.focus();
  await page.keyboard.type("Épsilon");
  const grid = page.getByRole("grid", { name: "Grade Fiscal" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);

  // Tab até as células da grade (tabindex rotativo: o corpo da grade é uma única parada de Tab).
  const inGridBody = () => page.evaluate(() => !!document.activeElement?.matches("[role=grid] tbody [data-cell]"));
  for (let i = 0; i < 30 && !(await inGridBody()); i++) await page.keyboard.press("Tab");
  expect(await inGridBody()).toBe(true);

  // Setas até a célula Entradas; Enter abre o menu de status.
  await arrowTo(page, "ArrowRight", new RegExp(`^Entradas — ${EPSILON}: A iniciar$`));
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu")).toBeVisible();

  // O foco entra no menu. Aberto pelo teclado, o Radix pode já focar o primeiro item (depende de
  // ele ter visto um evento de ponteiro antes); as setas seguem a ordem a partir de onde o foco está.
  await expect.poll(() => page.evaluate(() => !!document.activeElement?.closest("[role=menu]"))).toBe(true);
  const order = ["A iniciar", "Baixadas", "Lançadas", "Fechadas"];
  const start = await page.evaluate(() =>
    document.activeElement?.getAttribute("role") === "menuitemradio" ? (document.activeElement.textContent ?? "").trim() : "",
  );
  for (const item of order.slice(order.indexOf(start) + 1)) {
    await page.keyboard.press("ArrowDown");
    await expect(focusedMenuItem(page)).toHaveText(item);
  }
  await page.keyboard.press("Enter");
  await expect(page.getByRole("menu")).toBeHidden();

  const updated = grid.getByRole("button", { name: `Entradas — ${EPSILON}: Fechadas` });
  await expect(updated).toBeVisible();
  await expect(updated).not.toHaveAttribute("aria-busy");
  // O foco volta para a célula editada (não para o botão interno), e as setas seguem navegando.
  const focusedCell = () => page.evaluate(() => document.activeElement?.getAttribute("data-cell") ?? "");
  const edited = await focusedCell();
  expect(edited).not.toBe("");
  await expect(page.locator(`[data-cell="${edited}"]`).getByRole("button", { name: `Entradas — ${EPSILON}: Fechadas` })).toBeVisible();
  await page.keyboard.press("ArrowRight");
  expect(await inGridBody()).toBe(true);
  expect(await focusedCell()).not.toBe(edited);
});

test("T-M7-04 §28.3 status, atraso, prazo alterado e histórico têm texto ou ícone com rótulo além da cor", async ({ page, request }) => {
  const competence = await latestCompetence(request);
  const row = await openGridRow(request, "fiscal", competence, "105", "&group=normal");
  const cell = row.cells.FISCAL_ENTRADAS;
  if (!cell) throw new Error("105 sem Entradas");
  await bulkStatus(request, [cell.id], "NOT_STARTED");
  const fresh = await openGridRow(request, "fiscal", competence, "105", "&group=normal");
  const entradas = fresh.cells.FISCAL_ENTRADAS;
  if (!entradas) throw new Error("105 sem Entradas");
  // Vencimento 5 dias antes de "hoje" (relógio fixo): a obrigação fica atrasada.
  const today = new Date(`${fixedToday()}T12:00:00Z`);
  today.setUTCDate(today.getUTCDate() - 5);
  const due = today.toISOString().slice(0, 10);
  const reason = "Vencimento antecipado para o teste de acessibilidade";
  await apiJson(request, "patch", `/api/v1/obligation-instances/${entradas.id}/details`, {
    version: entradas.version,
    due_date_effective: due,
    due_date_override_reason: reason,
  });
  const [y, m, d] = due.split("-");

  await page.goto(`/fiscal?competence=${competence}&group=normal`);
  await ready(page, "Fiscal");
  await page.getByRole("searchbox", { name: "Buscar empresa" }).fill("Épsilon");
  const grid = page.getByRole("grid", { name: "Grade Fiscal" });
  await expect(grid.getByRole("rowheader")).toHaveCount(1);

  // Status: texto visível e nome acessível, não só a cor.
  const status = grid.getByRole("button", { name: `Entradas — ${EPSILON}: A iniciar` });
  await expect(status).toHaveText(/A iniciar/);
  // Atraso, prazo alterado e histórico: ícones com rótulo textual.
  await expect(grid.getByRole("img", { name: `Atrasada: venceu em ${d}/${m}/${y}` })).toBeVisible();
  await expect(grid.getByRole("img", { name: `Prazo alterado manualmente: ${reason}` })).toBeVisible();
  await expect(grid.getByRole("img", { name: "Possui histórico de alterações" }).first()).toBeVisible();
  // Contagem de atrasadas da linha com texto.
  await expect(grid.getByLabel(/^\d+ obrigaç(ão atrasada|ões atrasadas)$/)).toBeVisible();

  // Estado do período e progresso: texto, não só cor.
  await expect(grid.getByLabel(/^Progresso operacional: \d+%$/)).toBeVisible();
  await expect(grid.getByRole("button", { name: `Fechar Fiscal de ${EPSILON}` })).toHaveText("Fechar");
});
