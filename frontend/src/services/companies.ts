import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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

export type CompanyCreate = Schemas["CompanyCreate"];
export type CompanyUpdate = Schemas["CompanyUpdate"];

function useInvalidateCompanies() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: companyKeys.all });
}

export function useCreateCompany() {
  const invalidate = useInvalidateCompanies();
  return useMutation({
    mutationFn: (body: CompanyCreate) => api.post("/companies", { body }),
    onSuccess: () => invalidate(),
  });
}

export function useUpdateCompany(id: string) {
  const invalidate = useInvalidateCompanies();
  return useMutation({
    mutationFn: (body: CompanyUpdate) => api.patch("/companies/{id}", { path: { id }, body }),
    onSuccess: () => invalidate(),
  });
}

export function useDeactivateCompany() {
  const invalidate = useInvalidateCompanies();
  return useMutation({
    mutationFn: ({ id, version }: { id: string; version: number }) =>
      api.post("/companies/{id}/deactivate", { path: { id }, body: { version } }),
    onSettled: () => invalidate(),
  });
}

export type TaxRegimeTimeline = Schemas["TaxRegimeTimeline"];
export type CompanyModules = Schemas["CompanyModules"];
export type ObligationOverrides = Schemas["ObligationOverrides"];

export const companySettingsKeys = {
  regimes: (id: string) => ["companies", "detail", id, "regimes"] as const,
  modules: (id: string) => ["companies", "detail", id, "modules"] as const,
  overrides: (id: string) => ["companies", "detail", id, "overrides"] as const,
};

export function useTaxRegimes(id: string) {
  return useQuery({
    queryKey: companySettingsKeys.regimes(id),
    queryFn: ({ signal }) => api.get("/companies/{id}/tax-regimes", { path: { id }, signal }),
  });
}

export function useChangeTaxRegime(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["TaxRegimeChange"]) => api.post("/companies/{id}/tax-regimes", { path: { id }, body }),
    onSuccess: (data) => {
      qc.setQueryData(companySettingsKeys.regimes(id), data);
      void qc.invalidateQueries({ queryKey: companyKeys.all });
    },
  });
}

export function useCompanyModules(id: string) {
  return useQuery({
    queryKey: companySettingsKeys.modules(id),
    queryFn: ({ signal }) => api.get("/companies/{id}/modules", { path: { id }, signal }),
  });
}

export function useSetCompanyModule(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ module, ...body }: Schemas["CompanyModuleChange"] & { module: Schemas["ModuleCode"] }) =>
      api.put("/companies/{id}/modules/{module}", { path: { id, module }, body }),
    onSuccess: (data) => {
      qc.setQueryData(companySettingsKeys.modules(id), data);
      void qc.invalidateQueries({ queryKey: companyKeys.all });
    },
  });
}

export function useObligationOverrides(id: string) {
  return useQuery({
    queryKey: companySettingsKeys.overrides(id),
    queryFn: ({ signal }) => api.get("/companies/{id}/obligation-overrides", { path: { id }, signal }),
  });
}

export function useSetObligationOverride(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ obligationTypeId, ...body }: Schemas["ObligationOverrideChange"] & { obligationTypeId: string }) =>
      api.put("/companies/{id}/obligation-overrides/{obligationTypeId}", { path: { id, obligationTypeId }, body }),
    onSuccess: (data) => {
      qc.setQueryData(companySettingsKeys.overrides(id), data);
    },
  });
}
