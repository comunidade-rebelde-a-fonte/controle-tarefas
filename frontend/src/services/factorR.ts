import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";
import type { CompanyFilter } from "@/lib/companyFilter";

export type FactorRPortfolio = Schemas["FactorRPortfolio"];
export type FactorRPortfolioLine = Schemas["FactorRPortfolioLine"];
export type FactorRCalc = Schemas["FactorRCalc"];
export type FactorRMovement = Schemas["FactorRMovement"];
export type FactorRMovementInput = Schemas["FactorRMovementInput"];
export type FactorRSettings = Schemas["FactorRSettings"];
export type FactorRSettingsUpdate = Schemas["FactorRSettingsUpdate"];
export type FactorRLight = Schemas["FactorRLight"];
export type FactorRReason = Schemas["FactorRReason"];
export type FactorRPackage = Schemas["FactorRPackage"];
export type FactorRMovementOrigin = Schemas["FactorRMovementOrigin"];

export const factorRKeys = {
  all: ["factor-r"] as const,
  portfolio: (pa: string, filter: CompanyFilter = {}) => ["factor-r", "portfolio", pa, filter] as const,
  company: (id: string) => ["factor-r", "company", id] as const,
  calc: (id: string, pa: string) => ["factor-r", "company", id, "calc", pa] as const,
  movements: (id: string, from: string, to: string) => ["factor-r", "company", id, "movements", from, to] as const,
  settings: (id: string) => ["factor-r", "company", id, "settings"] as const,
};

/** GET /factor-r/portfolio — carteira do PA. */
export function useFactorRPortfolio(pa: string | null, filter: CompanyFilter = {}) {
  return useQuery({
    queryKey: factorRKeys.portfolio(pa ?? "", filter),
    queryFn: ({ signal }) => api.get("/factor-r/portfolio", { query: { pa: pa ?? "", ...filter }, signal }),
    enabled: !!pa,
    placeholderData: keepPreviousData,
  });
}

/** GET /companies/{id}/factor-r/calc — Fator R da empresa no PA. */
export function useFactorRCalc(companyId: string, pa: string | null) {
  return useQuery({
    queryKey: factorRKeys.calc(companyId, pa ?? ""),
    queryFn: ({ signal }) => api.get("/companies/{id}/factor-r/calc", { path: { id: companyId }, query: { pa: pa ?? "" }, signal }),
    enabled: !!pa,
    placeholderData: keepPreviousData,
  });
}

/** GET /companies/{id}/factor-r/movements — meses de `from` a `to` (inclusive). */
export function useFactorRMovements(companyId: string, from: string, to: string) {
  return useQuery({
    queryKey: factorRKeys.movements(companyId, from, to),
    queryFn: ({ signal }) => api.get("/companies/{id}/factor-r/movements", { path: { id: companyId }, query: { from, to }, signal }),
    placeholderData: keepPreviousData,
  });
}

/** GET /companies/{id}/factor-r/settings. */
export function useFactorRSettings(companyId: string) {
  return useQuery({
    queryKey: factorRKeys.settings(companyId),
    queryFn: ({ signal }) => api.get("/companies/{id}/factor-r/settings", { path: { id: companyId }, signal }),
  });
}

/** PUT /companies/{id}/factor-r/movements/{competence}: muda série, cálculo e carteira. */
export function usePutFactorRMovement(companyId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ competence, body }: { competence: string; body: FactorRMovementInput }) =>
      api.put("/companies/{id}/factor-r/movements/{competence}", { path: { id: companyId, competence }, body }),
    onSettled: () => qc.invalidateQueries({ queryKey: factorRKeys.all }),
  });
}

/** PATCH /companies/{id}/factor-r/settings: início de atividade e honorário mudam cálculo e carteira. */
export function useUpdateFactorRSettings(companyId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: FactorRSettingsUpdate) => api.patch("/companies/{id}/factor-r/settings", { path: { id: companyId }, body }),
    onSettled: () => qc.invalidateQueries({ queryKey: factorRKeys.all }),
  });
}
