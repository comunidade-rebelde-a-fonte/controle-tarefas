import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";

export type Company = Schemas["Company"];
export type CompanyList = Schemas["CompanyList"];

export interface CompanyFilters {
  q?: string;
  tax_regime?: Schemas["TaxRegime"];
  active?: boolean;
  module?: Schemas["ModuleCode"];
}

export const companyKeys = {
  all: ["companies"] as const,
  list: (f: CompanyFilters) => ["companies", "list", f] as const,
  detail: (id: string) => ["companies", "detail", id] as const,
};

/** Lista de empresas com busca global e filtros (resolvidos na API). */
export function useCompanies(filters: CompanyFilters) {
  return useQuery({
    queryKey: companyKeys.list(filters),
    queryFn: ({ signal }) => api.get("/companies", { query: filters, signal }),
    placeholderData: keepPreviousData,
  });
}

export function useCompany(id: string | undefined) {
  return useQuery({
    queryKey: companyKeys.detail(id ?? ""),
    queryFn: ({ signal }) => api.get("/companies/{id}", { path: { id: id ?? "" }, signal }),
    enabled: !!id,
  });
}
