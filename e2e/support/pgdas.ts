import { readFileSync } from "node:fs";
import path from "node:path";
import { type APIRequestContext, expect } from "@playwright/test";

/**
 * Extratos PGDAS-D de teste (TASK-014b): as fixtures sintéticas do parser (as mesmas da origem,
 * em backend/internal/factorr/pgdas/testdata/fixtures), com o CNPJ trocado. O Playwright roda a
 * partir de e2e/ (`make e2e`).
 */
const FIXTURES = path.resolve(process.cwd(), "../backend/internal/factorr/pgdas/testdata/fixtures");
const FIXTURE_CNPJ = "11.222.333/0001-81";

let seq = 0;

/** 14 dígitos → 00.000.000/0000-00. */
export function formatCnpj(digits: string): string {
  return digits.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, "$1.$2.$3/$4-$5");
}

/**
 * Conteúdo do extrato com o CNPJ dado (ou o da fixture). Uma linha final única por chamada evita
 * que o inbox trate o envio como o mesmo arquivo (dedupe por SHA-256).
 */
export function statement(fixture: string, cnpjDigits?: string): Buffer {
  const raw = readFileSync(path.join(FIXTURES, fixture, "documento.txt"), "utf8");
  const text = cnpjDigits ? raw.replaceAll(FIXTURE_CNPJ, formatCnpj(cnpjDigits)) : raw;
  seq += 1;
  return Buffer.from(`${text}\nArquivo de teste E2E ${String(Date.now())}-${String(seq)}\n`, "utf-8");
}

export interface PgdasDoc {
  id: string;
  status: string;
  motivo: string | null;
  company_id: string | null;
  trace_id: string | null;
  sugestao_cadastro: { cnpj: string } | null;
}

/** Envia o extrato pela API e espera o worker terminar a leitura. */
export async function uploadAndWait(request: APIRequestContext, name: string, content: Buffer): Promise<PgdasDoc> {
  const res = await request.post("/api/v1/factor-r/pgdas", { multipart: { file: { name, mimeType: "text/plain", buffer: content } } });
  expect(res.status(), await res.text()).toBe(202);
  const { document_id: id } = (await res.json()) as { document_id: string };
  let doc: PgdasDoc | undefined;
  await expect
    .poll(
      async () => {
        doc = (await (await request.get(`/api/v1/factor-r/pgdas/${id}`)).json()) as PgdasDoc;
        return doc.status;
      },
      { timeout: 30_000, message: "o worker não leu o extrato" },
    )
    .not.toBe("received");
  return doc as PgdasDoc;
}
