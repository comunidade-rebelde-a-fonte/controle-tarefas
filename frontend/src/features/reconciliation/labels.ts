import type { ReconciliationDocumentStatus, ReviewBucket } from "@/services/reconciliation";

/** Estados do documento (contrato congelado da origem, D-020), em texto. */
export const DOC_STATUS: Record<ReconciliationDocumentStatus, string> = {
  pendente: "Na triagem",
  triado: "Na extração",
  extraido: "Extraído",
  importado: "Importado",
  revisao_triagem: "Revisão da triagem",
  revisao_extracao: "Revisão da extração",
  duplicado: "Duplicado",
  pendencia_periodo_fechado: "Período fechado",
};

/** Filtros de estado da lista de documentos, na ordem do fluxo. */
export const DOC_STATUS_FILTERS = Object.entries(DOC_STATUS) as [ReconciliationDocumentStatus, string][];

export const DOC_KIND: Record<string, string> = { extrato: "Extrato", comprovante: "Comprovante", desconhecido: "Desconhecido" };

/** Baldes da fila (a contagem da origem e a classificação da TASK-020). */
export const BUCKETS: [ReviewBucket, string][] = [
  ["revisao", "Revisão"],
  ["divergencia", "Divergências"],
  ["extrato_sem", "Extrato sem comprovante"],
  ["comprovante_sem", "Comprovante sem extrato"],
  ["classificacao", "Classificação"],
];

export function bucketText(b: string): string {
  return BUCKETS.find(([k]) => k === b)?.[1] ?? b;
}

/**
 * Motivos em texto (motivo_legivel e MOTIVOS_DIVERGENCIA da origem), mais os da triagem, da
 * extração, das bordas (TASK-018), do desfazer e da classificação.
 */
const REASONS: Record<string, string> = {
  parcelado: "O comprovante está marcado como parcela. Confirme se esta linha do extrato é essa parcela.",
  juros_no_boleto: "Boleto pago com juros ou multa. O extrato traz o valor com encargos; os encargos viram ajuste no razão.",
  iss_retido: "Recebimento líquido de retenção. O extrato traz o valor líquido; a retenção vira ajuste no razão.",
  mesmo_nf_varios_pdfs:
    "Mais de um comprovante para a mesma nota. Pode ser pagamento em duplicidade: case cada linha com o comprovante da mesma data.",
  varios_lancamentos_mesma_contraparte: "Há mais de um comprovante da mesma contraparte. Escolha o que corresponde a esta linha.",
  duplicidade_mesmo_valor: "Outra linha do extrato tem o mesmo valor e um descritivo parecido. Pode ser lançamento em duplicidade.",
  alerta_comprovante: "Um comprovante próximo tem um alerta. Confira antes de conciliar.",
  passe_4_extenso: "Combinações demais para a soma automática. Escolha os comprovantes à mão.",
  transferencia_interna: "Parece transferência entre contas da própria empresa. Confirme e concilie com a outra ponta.",
  estorno: "Parece estorno de um lançamento próximo (até 7 dias). Confira a operação original.",
  bruto_liquido: "Valor próximo de um comprovante (diferença de até R$ 20,00): pode ser bruto × líquido, com tarifa ou retenção.",
  conciliacao_desfeita: "A conciliação desta linha foi desfeita. Ela não volta sozinha ao automático: concilie à mão.",
  campos_a_revisar: "Comprovante com campo ilegível ou de baixa confiança. Corrija os campos para ele entrar no automático.",
  sem_regra_nem_historico: "Nenhuma regra nem histórico classificou esta linha. Escolha a conta ou peça uma sugestão.",
  confianca_baixa: "A triagem não teve certeza do tipo do documento.",
  tipo_desconhecido: "A triagem não reconheceu o documento como extrato nem como comprovante.",
  documento_ilegivel: "O documento não pôde ser lido.",
  agente_desligado: "O agente está desligado em Configurações → IA.",
  saida_invalida: "A leitura do agente veio fora do formato esperado.",
  saldo_nao_fecha: "O saldo do extrato não fecha (tolerância R$ 0,00): nada foi importado.",
  metadados_ausentes: "Faltam o banco, a agência, a conta ou o período do extrato.",
  csv_ilegivel: "O CSV do extrato não pôde ser lido.",
  empresa_nao_identificada: "A empresa do documento não foi identificada.",
  conta_de_outra_empresa: "A conta do extrato pertence a outra empresa.",
  periodo_fechado: "O Contábil da competência está fechado.",
  campos_abaixo_limiar: "Campos do comprovante abaixo do limiar de confiança.",
};

/** Texto do motivo; o código desconhecido vira texto legível (o fallback da origem). */
export function reasonText(code: string | null | undefined): string {
  if (!code) return "";
  if (REASONS[code]) return REASONS[code];
  if (code.startsWith("ambiguo")) return "Mais de um comprovante possível para esta linha. Escolha o certo.";
  const t = code.replaceAll("_", " ");
  return t.charAt(0).toUpperCase() + t.slice(1);
}

/** Métodos da conciliação (D-020). */
export function methodText(method: string): string {
  if (/^passe_\d$/.test(method)) return `Automática, passe ${method.slice(-1)}`;
  return ({ manual: "Manual", sem_comprovante: "Sem comprovante", resolvedor: "Aceite do resolvedor" } as Record<string, string>)[method] ?? method;
}

/** Origem da classificação (TASK-020). */
export function classificationText(origin: string | null | undefined, pending: boolean): string {
  if (!origin) return "Sem classificação";
  if (origin === "agente") return pending ? "À espera do agente ou do contador" : "Sugestão do agente aceita";
  return ({ regra: "Regra", historico: "Histórico", manual: "Contador" } as Record<string, string>)[origin] ?? origin;
}

/** Campos do comprovante (as chaves da origem na confiança e na origem por campo). */
export const FIELD_LABELS: Record<string, string> = {
  data: "Data",
  valor: "Valor",
  sentido: "Sentido",
  contraparte: "Contraparte",
  contraparte_doc: "CPF/CNPJ da contraparte",
  documento: "Documento",
  tipo_operacao: "Tipo de operação",
};

/** Status do lançamento do cliente. */
export const ENTRY_STATUS: Record<string, string> = {
  pendente: "Pendente",
  parcial: "Parcial (a revisar)",
  extraido: "Extraído",
  manual: "Manual",
};

/** Códigos de falha do job em texto. */
export function jobErrorText(code: string | null | undefined): string {
  return (
    ({
      TRIAGE_INVALID_OUTPUT: "a triagem veio fora do formato",
      LLM_UNAVAILABLE: "o gateway de IA não respondeu",
      RECONCILIATION_FILE_MISSING: "o original não foi encontrado no armazenamento",
    } as Record<string, string>)[code ?? ""] ?? "erro no processamento"
  );
}
