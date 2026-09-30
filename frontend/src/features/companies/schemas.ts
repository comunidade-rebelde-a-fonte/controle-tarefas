import { z } from "zod";
import { sanitizeCnpj } from "@/lib/cnpj";
import { MODULES, PAYROLL_TYPES, TAX_REGIMES } from "@/lib/labels";

const optionalText = (max: number) => z.string().trim().max(max, `Máximo de ${String(max)} caracteres.`);
const date = z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Informe uma data válida.");

/**
 * Validação de formulário (UX). A API revalida tudo, inclusive os dígitos verificadores do
 * CNPJ e a unicidade de CNPJ/código [AGENTS §6.5].
 */
export const companyFormSchema = z
  .object({
    internal_code: z.string().trim().min(1, "Informe o código interno.").max(30, "Máximo de 30 caracteres."),
    legal_name: z.string().trim().min(1, "Informe a razão social.").max(200, "Máximo de 200 caracteres."),
    trade_name: optionalText(200),
    cnpj: z.string().refine((v) => sanitizeCnpj(v).length === 14, "O CNPJ deve ter 14 dígitos."),
    tax_regime: z.enum(TAX_REGIMES, { error: "Selecione o regime tributário." }),
    service_start_date: date,
    service_end_date: z.union([date, z.literal("")]),
    city: optionalText(100),
    state: z.string(),
    state_registration: optionalText(30),
    municipal_registration: optionalText(30),
    payroll_type: z.enum(PAYROLL_TYPES, { error: "Selecione o tipo de folha." }),
    factor_r: z.enum(["", "true", "false"]),
    fiscal_responsible_user_id: z.string(),
    accounting_responsible_user_id: z.string(),
    payroll_responsible_user_id: z.string(),
    declarations_responsible_user_id: z.string(),
    notes: optionalText(4000),
    modules: z.array(z.enum(MODULES)),
  })
  .refine((v) => !v.service_end_date || v.service_end_date >= v.service_start_date, {
    path: ["service_end_date"],
    message: "O fim do atendimento deve ser igual ou posterior ao início.",
  });

export type CompanyFormValues = z.infer<typeof companyFormSchema>;

export const emptyCompanyForm: CompanyFormValues = {
  internal_code: "",
  legal_name: "",
  trade_name: "",
  cnpj: "",
  tax_regime: "SIMPLES_NACIONAL",
  service_start_date: "",
  service_end_date: "",
  city: "",
  state: "",
  state_registration: "",
  municipal_registration: "",
  payroll_type: "PAYROLL",
  factor_r: "",
  fiscal_responsible_user_id: "",
  accounting_responsible_user_id: "",
  payroll_responsible_user_id: "",
  declarations_responsible_user_id: "",
  notes: "",
  modules: [...MODULES],
};
