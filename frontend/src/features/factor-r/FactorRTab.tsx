import { TriangleAlert } from "lucide-react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Skeleton } from "@/components/ui/skeleton";
import { displayCompetence } from "@/lib/competence";
import { TAX_REGIME_LABELS } from "@/lib/labels";
import type { Company } from "@/services/companies";
import { type FactorRCalc, useFactorRCalc } from "@/services/factorR";
import { CalcPanel } from "./CalcPanel";
import { FactorRDisclaimer, PANav } from "./common";
import { usePA } from "./usePA";
import { MonthlyGrid } from "./MonthlyGrid";
import { SettingsCard } from "./SettingsCard";

/** Por que a empresa não está na carteira neste PA (o cálculo continua disponível). */
function NotMonitored({ company, calc }: { company: Company; calc: FactorRCalc }) {
  const reasons: string[] = [];
  if (!company.active) reasons.push("a empresa está inativa");
  if (company.factor_r !== true) reasons.push("não está marcada como sujeita ao Fator R");
  if (calc.regime_no_pa !== "SIMPLES_NACIONAL") {
    reasons.push(
      calc.regime_no_pa
        ? `no PA ${displayCompetence(calc.pa)} o regime é ${TAX_REGIME_LABELS[calc.regime_no_pa]}, não o Simples Nacional`
        : `não há regime tributário vigente no PA ${displayCompetence(calc.pa)}`,
    );
  }
  return (
    <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" data-testid="fora-da-carteira">
      <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
      <p>
        Esta empresa não entra na carteira do Fator R neste PA: {reasons.join("; ")}.{" "}
        {company.factor_r !== true ? (
          <Link to={`/companies/${company.id}?tab=payroll`} className="underline">
            Marcar na aba Folha
          </Link>
        ) : null}
      </p>
    </div>
  );
}

/**
 * Aba Fator R da ficha [TASK-014a]: resultado do PA com o cálculo explicado, a janela usada,
 * a grade mensal editável e a configuração da empresa. O PA fica na URL (`?pa=`).
 */
export function FactorRTab({ company }: { company: Company }) {
  const { pa, error: paError } = usePA();
  const calc = useFactorRCalc(company.id, pa);
  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">Fator R{pa ? ` — PA ${displayCompetence(pa)}` : ""}</h2>
        <PANav />
      </div>
      <ErrorAlert error={paError ?? calc.error} fallback="Não foi possível calcular o Fator R." />
      {calc.data && !calc.data.monitorada ? <NotMonitored company={company} calc={calc.data} /> : null}
      {calc.isPending && pa !== null ? <Skeleton className="h-64" /> : null}
      {calc.data ? <CalcPanel calc={calc.data} /> : null}
      {pa ? <MonthlyGrid companyId={company.id} pa={pa} /> : null}
      <SettingsCard company={company} />
      <FactorRDisclaimer />
    </div>
  );
}
