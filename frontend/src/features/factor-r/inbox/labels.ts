import type { PgdasDocument } from "@/services/factorRInbox";

/** Campos lidos do extrato, na ordem da origem (CAMPOS da página do documento). */
export const PGDAS_FIELDS: readonly [keyof PgdasDocument["campos"], string, "texto" | "cnpj" | "reais" | "percentual"][] = [
  ["cnpj", "CNPJ", "cnpj"],
  ["pa", "PA", "texto"],
  ["rpa", "RPA (receita do PA)", "reais"],
  ["rbt12", "RBT12", "reais"],
  ["fs12", "FS12", "reais"],
  ["fator_r", "Fator r", "percentual"],
  ["das", "Valor do DAS", "reais"],
  ["anexo", "Anexo", "texto"],
];

const FIELD_NAMES: Record<string, string> = {
  cnpj: "CNPJ",
  pa: "PA",
  rpa: "RPA",
  rbt12: "RBT12",
  fs12: "FS12",
  fator_r: "Fator r",
  das: "Valor do DAS",
  anexo: "Anexo",
};

/** Motivo de revisão do documento em texto; o código continua no texto do agente. */
const REASONS: Record<string, string> = {
  cnpj_nao_encontrado: "CNPJ fora do cadastro de empresas",
  sem_camada_texto: "PDF sem camada de texto (digitalizado): não dá para ler sem OCR",
  pdf_ilegivel: "PDF ilegível",
  extracao_excedeu_limites: "A leitura passou dos limites de segurança (tempo, páginas ou memória)",
};

export function reasonText(motivo: string | null): string {
  if (!motivo) return "";
  const low = /^confianca_baixa \((.+) < (.+)\)$/.exec(motivo);
  if (low) return `Confiança baixa (${low[1] ?? ""} abaixo do limiar de ${low[2] ?? ""})`;
  return REASONS[motivo] ?? motivo;
}

/** Avisos do parser (motivos) em texto. */
export function warningText(code: string): string {
  if (code === "cnpj_digito_verificador_invalido") return "CNPJ com dígito verificador inválido";
  if (code === "fator_r_inconsistente_com_fs12_rbt12") return "Fator r declarado não bate com FS12 ÷ RBT12";
  const missing = /^(.+)_nao_encontrado$/.exec(code);
  if (missing?.[1] && FIELD_NAMES[missing[1]]) return `${FIELD_NAMES[missing[1]]} não encontrado`;
  return code;
}

/** Códigos de falha do job pgdas_parse em texto. */
const JOB_ERRORS: Record<string, string> = {
  PGDAS_PA_INVALIDO: "a competência lida do extrato é inválida",
  PGDAS_FILE_MISSING: "o arquivo original não foi encontrado no armazenamento",
  PGDAS_DOCUMENT_NOT_FOUND: "o documento não foi encontrado",
  INVALID_PAYLOAD: "o job não trouxe os dados esperados",
  WORKER_LOST: "o worker parou durante o processamento",
};

export function jobErrorText(code: string | null | undefined): string {
  if (!code) return "erro não informado";
  return JOB_ERRORS[code] ?? code;
}

/** Status do documento como filtro (contagem da lista), na ordem do fluxo. */
export const STATUS_FILTERS: readonly [PgdasDocument["status"], string][] = [
  ["received", "Recebidos"],
  ["parsed", "Extraídos"],
  ["needs_review", "Em revisão"],
  ["linked", "Vinculados"],
  ["rejected", "Rejeitados"],
];
