import { sanitizeCnpj } from "@/lib/cnpj";
import type { CompanyCreate, CompanyUpdate } from "@/services/companies";
import type { CompanyFormValues } from "./schemas";

function nullIfEmpty(v: string): string | null {
  const t = v.trim();
  return t === "" ? null : t;
}

function commonPayload(v: CompanyFormValues) {
  return {
    internal_code: v.internal_code.trim(),
    legal_name: v.legal_name.trim(),
    trade_name: nullIfEmpty(v.trade_name),
    cnpj: sanitizeCnpj(v.cnpj),
    city: nullIfEmpty(v.city),
    state: nullIfEmpty(v.state),
    state_registration: nullIfEmpty(v.state_registration),
    municipal_registration: nullIfEmpty(v.municipal_registration),
    service_start_date: v.service_start_date,
    service_end_date: v.service_end_date === "" ? null : v.service_end_date,
    payroll_type: v.payroll_type,
    factor_r: v.factor_r === "" ? null : v.factor_r === "true",
    fiscal_responsible_user_id: nullIfEmpty(v.fiscal_responsible_user_id),
    accounting_responsible_user_id: nullIfEmpty(v.accounting_responsible_user_id),
    payroll_responsible_user_id: nullIfEmpty(v.payroll_responsible_user_id),
    declarations_responsible_user_id: nullIfEmpty(v.declarations_responsible_user_id),
    notes: nullIfEmpty(v.notes),
  };
}

export function toCreatePayload(v: CompanyFormValues): CompanyCreate {
  return { ...commonPayload(v), tax_regime: v.tax_regime, modules: v.modules };
}

export function toUpdatePayload(v: CompanyFormValues, version: number): CompanyUpdate {
  return { ...commonPayload(v), version };
}
