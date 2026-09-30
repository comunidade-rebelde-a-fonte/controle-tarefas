import { type APIRequestContext, expect } from "@playwright/test";

/** Apoio aos E2E: prepara o estado via API (proxy /api da SPA de teste, banco controle_e2e). */

export interface GridCell {
  id: string;
  applicable: boolean;
  version: number;
  status_code: string;
  obligation_type: { code: string };
}

export interface GridRow {
  company: { internal_code: string; legal_name: string };
  period: { id: string; state: string; version: number };
  cells: Record<string, GridCell>;
}

export interface CreatedCompany {
  id: string;
  internal_code: string;
  legal_name: string;
  cnpj: string;
}

export async function apiJson<T>(request: APIRequestContext, method: "get" | "post" | "patch", url: string, data?: unknown): Promise<T> {
  const res = method === "get" ? await request.get(url) : method === "post" ? await request.post(url, { data }) : await request.patch(url, { data });
  expect(res.ok(), `${method.toUpperCase()} ${url}: ${res.status()} ${await res.text()}`).toBeTruthy();
  return (await res.json()) as T;
}

/**
 * "Hoje" do E2E em AAAA-MM-DD, no fuso do escritório: o mesmo instante fixo (E2E_FIXED_TIME)
 * usado pela API e pelo seed, para que as datas dos testes não dependam do relógio real.
 */
export function fixedToday(): string {
  const raw = process.env.E2E_FIXED_TIME;
  if (!raw) throw new Error("E2E_FIXED_TIME não definido (use `make e2e`).");
  return new Intl.DateTimeFormat("en-CA", { timeZone: "America/Sao_Paulo" }).format(new Date(raw));
}

/** Competência AAAA-MM exibida como MM/AAAA. */
export function displayCompetence(key: string): string {
  const [y, m] = key.split("-");
  return `${m ?? ""}/${y ?? ""}`;
}

/** CNPJ fictício com dígitos verificadores válidos a partir de 12 dígitos-base [RN-014]. */
export function cnpjWithCheckDigits(base: string): string {
  if (!/^\d{12}$/.test(base)) throw new Error(`base do CNPJ deve ter 12 dígitos: ${base}`);
  const digit = (digits: string) => {
    const weights = digits.length === 12 ? [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2] : [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    const sum = weights.reduce((acc, w, i) => acc + w * Number(digits[i]), 0);
    const rest = sum % 11;
    return rest < 2 ? "0" : String(11 - rest);
  };
  const first = base + digit(base);
  return first + digit(first);
}

/**
 * Sufixo numérico único por execução: as empresas criadas pelos testes não colidem entre si
 * nem com uma segunda execução sobre o mesmo banco (código e CNPJ são únicos, RN-001/RN-002).
 */
export function uniqueDigits(): string {
  return String(Date.now()).slice(-10);
}

/** Cadastra uma empresa fictícia pela API, com o atendimento iniciado na competência dada. */
export async function createCompany(
  request: APIRequestContext,
  opts: { name: string; competence: string; taxRegime: string; payrollType: string; modules: string[]; prefix: string },
): Promise<CreatedCompany> {
  const digits = uniqueDigits();
  return apiJson<CreatedCompany>(request, "post", "/api/v1/companies", {
    internal_code: `E2E-${opts.prefix}-${digits}`,
    legal_name: `${opts.name} ${digits}`,
    cnpj: cnpjWithCheckDigits(`${opts.prefix.padStart(2, "0").slice(-2)}${digits}`),
    service_start_date: `${opts.competence}-01`,
    payroll_type: opts.payrollType,
    tax_regime: opts.taxRegime,
    ...(opts.taxRegime === "SIMPLES_NACIONAL" ? { factor_r: false } : {}),
    modules: opts.modules,
  });
}

export async function latestCompetence(request: APIRequestContext): Promise<string> {
  return (await apiJson<{ competence: string }>(request, "get", "/api/v1/competencies/latest")).competence;
}

/** Aciona o ensureCurrentCompetence, como fazem o boot e a verificação diária [PRD §10.2]. */
export async function ensureCurrent(request: APIRequestContext): Promise<{ competence: string; companies_processed: number; instances_created: number }> {
  return apiJson(request, "post", "/api/v1/competencies/ensure-current");
}

export async function gridRows(request: APIRequestContext, module: string, competence: string, query = ""): Promise<GridRow[]> {
  return (await apiJson<{ rows: GridRow[] }>(request, "get", `/api/v1/modules/${module}/grid?competence=${competence}${query}`)).rows;
}

/** Linha da empresa na grade do módulo; reabre o período se estiver fechado. */
export async function openGridRow(request: APIRequestContext, module: string, competence: string, code: string, query = ""): Promise<GridRow> {
  const find = async () => {
    const row = (await gridRows(request, module, competence, query)).find((r) => r.company.internal_code === code);
    if (!row) throw new Error(`empresa ${code} fora da grade ${module}`);
    return row;
  };
  const row = await find();
  if (row.period.state !== "CLOSED") return row;
  await apiJson(request, "post", `/api/v1/company-module-periods/${row.period.id}/reopen`, { version: row.period.version, reason: "Preparação do teste E2E" });
  return find();
}

/** Fecha o período do módulo pela API (confirmando o estágio de fechamento, se houver). */
export async function closePeriod(request: APIRequestContext, row: GridRow): Promise<void> {
  await apiJson(request, "post", `/api/v1/company-module-periods/${row.period.id}/close`, { version: row.period.version, confirm_set_closing: true });
}

export async function bulkStatus(request: APIRequestContext, ids: string[], status: string): Promise<void> {
  if (ids.length > 0) await apiJson(request, "post", "/api/v1/obligation-instances/bulk-status", { instance_ids: ids, status_code: status });
}
