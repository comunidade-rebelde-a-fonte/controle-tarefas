/** Rótulos pt-BR dos códigos da API (somente exibição). */
import type { Schemas } from "./api";

export type TaxRegime = Schemas["TaxRegime"];
export type ModuleCode = Schemas["ModuleCode"];
export type PayrollType = Schemas["PayrollType"];

export const TAX_REGIME_LABELS: Record<TaxRegime, string> = {
  SIMPLES_NACIONAL: "Simples Nacional",
  LUCRO_PRESUMIDO: "Lucro Presumido",
  LUCRO_REAL: "Lucro Real",
  LUCRO_ARBITRADO: "Lucro Arbitrado",
  IMUNE_ISENTA: "Imune/Isenta",
};

export const TAX_REGIMES = Object.keys(TAX_REGIME_LABELS) as TaxRegime[];

export const MODULE_LABELS: Record<ModuleCode, string> = {
  FISCAL: "Fiscal",
  DECLARATIONS: "Declarações",
  ACCOUNTING: "Contábil",
  PAYROLL: "Folha",
  CREDENTIALS: "Procurações/Certificados",
};

export const MODULE_SHORT: Record<ModuleCode, string> = {
  FISCAL: "FIS",
  DECLARATIONS: "DEC",
  ACCOUNTING: "CTB",
  PAYROLL: "FOL",
  CREDENTIALS: "CER",
};

export const MODULES = Object.keys(MODULE_LABELS) as ModuleCode[];

export const PAYROLL_TYPE_LABELS: Record<PayrollType, string> = {
  NO_PAYROLL: "Sem folha",
  PRO_LABORE: "Pró-labore",
  PAYROLL: "Folha",
};

export const PAYROLL_TYPES = Object.keys(PAYROLL_TYPE_LABELS) as PayrollType[];

export const UFS = [
  "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG", "PA", "PB",
  "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO",
] as const;

export type HistoryField = Schemas["HistoryEntry"]["field"];
export type ChangeSource = Schemas["HistoryEntry"]["change_source"];

export const HISTORY_FIELD_LABELS: Record<HistoryField, string> = {
  STATUS: "Status",
  DUE_DATE: "Vencimento",
  APPLICABLE: "Aplicabilidade",
  TRANSMISSION_DATE: "Data de transmissão",
  PROTOCOL: "Protocolo",
};

export const CHANGE_SOURCE_LABELS: Record<ChangeSource, string> = {
  INLINE: "Individual",
  BULK: "Em massa",
  IMPORT: "Importação",
  SYSTEM: "Sistema",
  SYSTEM_INTEGRATION: "Integração entre módulos",
};
