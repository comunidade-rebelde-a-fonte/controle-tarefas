import { TriangleAlert } from "lucide-react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Skeleton } from "@/components/ui/skeleton";
import { displayCompetence } from "@/lib/competence";
import { TAX_REGIME_LABELS } from "@/lib/labels";
import type { Company } from "@/services/companies";
import { type FactorRCalc, useFactorRCalc } from "@/services/factorR";
import { CompanyConsultant } from "./agents/CompanyConsultant";
import { CalcPanel } from "./CalcPanel";
import { FactorRDisclaimer } from "./common";
import { CompanyStatementsCard } from "./inbox/CompanyStatementsCard";
import { MonthlyGrid } from "./MonthlyGrid";
import { CompanyRunsCard } from "./runs/CompanyRunsCard";
import { SettingsCard } from "./SettingsCard";
import { Simulator } from "./simulator/Simulator";

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
          <Link to={`/companies/${company.id}?tab=payroll&competence=${calc.pa}`} className="underline">
            Marcar na aba Folha
          </Link>
        ) : null}
      </p>
    </div>
  );
}

/**
 * Aba Fator R da ficha [TASK-014a/b]: resultado do PA com o cálculo explicado, a janela usada,
 * o simulador e o consultor (empresas sujeitas ao Fator R, como na origem), a grade mensal
 * editável, os extratos e as corridas da empresa e a configuração. O PA é a competência da
 * ficha, escolhida no cabeçalho (TASK-025, D-039).
 */
export function FactorRTab({ company, pa }: { company: Company; pa: string | null }) {
  const calc = useFactorRCalc(company.id, pa);
  const subject = company.factor_r === true;
  return (
    <div className="grid grid-cols-1 gap-4">
      <h2 className="text-sm font-semibold">Fator R{pa ? ` — PA ${displayCompetence(pa)}` : ""}</h2>
      <ErrorAlert error={calc.error} fallback="Não foi possível calcular o Fator R." />
      {calc.data && !calc.data.monitorada ? <NotMonitored company={company} calc={calc.data} /> : null}
      {calc.isPending && pa !== null ? <Skeleton className="h-64" /> : null}
      {calc.data ? <CalcPanel calc={calc.data} /> : null}
      {subject && pa ? <Simulator key={pa} companyId={company.id} pa={pa} policyMeta={calc.data?.meta_operacional} /> : null}
      {subject ? <CompanyConsultant company={company} /> : null}
      {pa ? <MonthlyGrid companyId={company.id} pa={pa} /> : null}
      <div className="grid grid-cols-1 items-start gap-4 xl:grid-cols-2">
        <CompanyStatementsCard companyId={company.id} />
        <CompanyRunsCard companyId={company.id} />
      </div>
      <SettingsCard company={company} />
      <FactorRDisclaimer />
    </div>
  );
}
