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

export async function apiJson<T>(request: APIRequestContext, method: "get" | "post" | "patch", url: string, data?: unknown): Promise<T> {
  const res = method === "get" ? await request.get(url) : method === "post" ? await request.post(url, { data }) : await request.patch(url, { data });
  expect(res.ok(), `${method.toUpperCase()} ${url}: ${res.status()} ${await res.text()}`).toBeTruthy();
  return (await res.json()) as T;
}

export async function latestCompetence(request: APIRequestContext): Promise<string> {
  return (await apiJson<{ competence: string }>(request, "get", "/api/v1/competencies/latest")).competence;
}

/** Linha da empresa na grade do módulo; reabre o período se estiver fechado. */
export async function openGridRow(request: APIRequestContext, module: string, competence: string, code: string, query = ""): Promise<GridRow> {
  const url = `/api/v1/modules/${module}/grid?competence=${competence}${query}`;
  const find = async () => {
    const row = (await apiJson<{ rows: GridRow[] }>(request, "get", url)).rows.find((r) => r.company.internal_code === code);
    if (!row) throw new Error(`empresa ${code} fora da grade ${module}`);
    return row;
  };
  const row = await find();
  if (row.period.state !== "CLOSED") return row;
  await apiJson(request, "post", `/api/v1/company-module-periods/${row.period.id}/reopen`, { version: row.period.version, reason: "Preparação do teste E2E" });
  return find();
}

export async function bulkStatus(request: APIRequestContext, ids: string[], status: string): Promise<void> {
  if (ids.length > 0) await apiJson(request, "post", "/api/v1/obligation-instances/bulk-status", { instance_ids: ids, status_code: status });
}
