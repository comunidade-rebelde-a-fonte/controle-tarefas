import { readFileSync } from "node:fs";
import path from "node:path";
import { type APIRequestContext, expect } from "@playwright/test";
import { apiJson, cnpjWithCheckDigits, uniqueDigits } from "./api";

/**
 * Conciliação nos E2E (TASK-021a): o cenário Horizonte da origem (backend/internal/reconciliation/
 * testdata/conciliacao), lido pelos leitores determinísticos no worker, sem LLM. O MinIO é o do
 * E2E (make e2e), nunca o de dev.
 */
const SCENARIO = path.resolve(process.cwd(), "../backend/internal/reconciliation/testdata/conciliacao");

export interface ReconDocument {
  id: string;
  status: string;
}

/** Empresa do cenário: o nome fantasia "Construtora Horizonte" é o que os comprovantes citam. */
export async function createHorizonte(request: APIRequestContext): Promise<{ id: string; internal_code: string }> {
  const digits = uniqueDigits();
  return apiJson(request, "post", "/api/v1/companies", {
    internal_code: `E2E-CH-${digits}`,
    legal_name: `Construtora Horizonte E2E ${digits} Ltda`,
    trade_name: "Construtora Horizonte",
    cnpj: cnpjWithCheckDigits(`77${digits}`),
    service_start_date: "2026-01-01",
    payroll_type: "PAYROLL",
    tax_regime: "LUCRO_PRESUMIDO",
    modules: ["ACCOUNTING"],
  });
}

/** Envia o arquivo do cenário pela API e espera o worker terminar (sai de pendente e triado). */
export async function uploadAndProcess(request: APIRequestContext, file: string, companyId: string): Promise<ReconDocument> {
  const buffer = readFileSync(path.join(SCENARIO, file));
  const res = await request.post("/api/v1/reconciliation/documents", {
    multipart: { file: { name: path.basename(file), mimeType: "application/pdf", buffer }, company_id: companyId },
  });
  expect(res.status(), await res.text()).toBe(202);
  const { document_id: id } = (await res.json()) as { document_id: string };
  let doc: ReconDocument | undefined;
  await expect
    .poll(
      async () => {
        doc = (await (await request.get(`/api/v1/reconciliation/documents/${id}`)).json()) as ReconDocument;
        return doc.status;
      },
      { timeout: 60_000, message: `o worker não processou ${file}` },
    )
    .not.toMatch(/^(pendente|triado)$/);
  return doc as ReconDocument;
}
