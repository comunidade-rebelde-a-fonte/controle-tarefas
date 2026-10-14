import type { FactorRLight, FactorRMovementOrigin, FactorRPackage, FactorRReason } from "@/services/factorR";

/** Textos do semáforo (os da origem). A cor nunca é o único indicador [AC-007]. */
export const LIGHT_LABELS: Record<FactorRLight, string> = {
  vermelho: "Vermelho · abaixo de 28% (Anexo V)",
  amarelo: "Amarelo · no limite",
  verde: "Verde · na meta",
};

/** Explicação de cada motivo de dados insuficientes (origem: PainelFatorR). */
export const REASON_TEXTS: Record<FactorRReason, string> = {
  rbt12_zero: "Sem receita na janela (RBT12 = 0). Lance a receita dos 12 meses anteriores ao PA.",
  empresa_nova_regra_pendente:
    "Empresa com menos de 12 meses na janela. A regra de proporcionalização ainda está em conferência (P-05).",
  rbt12_acima_do_limite_do_simples: "RBT12 acima do limite do Simples Nacional.",
};

export const PACKAGE_LABELS: Record<FactorRPackage, string> = {
  monitoramento: "Monitoramento",
  correcao: "Correção",
  retainer: "Retainer",
};

export const PACKAGES = Object.keys(PACKAGE_LABELS) as FactorRPackage[];

export const ORIGIN_LABELS: Record<FactorRMovementOrigin, string> = {
  manual: "manual",
  pgdas: "PGDAS-D",
  folha: "folha",
  agente: "agente",
};

/** Aviso permanente de conformidade [FR-19]: o cálculo aqui não é a apuração oficial. */
export const DISCLAIMER = "Esta plataforma não é a apuração oficial. O PGDAS-D da Receita Federal prevalece.";
