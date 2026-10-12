import { X } from "lucide-react";
import { useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { displayCompetence } from "@/lib/competence";
import { COMPANY_FILTER_KEYS, companyFilterFrom, hasCompanyFilter } from "@/lib/companyFilter";
import { TAX_REGIME_LABELS } from "@/lib/labels";
import { useUsers } from "@/services/users";

const GROUP_LABELS = { NORMAL: "Normais", SIMPLES: "Simples Nacional" } as const;

/**
 * Os filtros de empresa que vieram do Dashboard (TASK-024, D-038): agrupamento, regime e o
 * responsável do módulo da tela (o do Fiscal na carteira do Fator R, o do Contábil na
 * Conciliação). Mostra o que está filtrando e limpa os filtros; sem filtro, não aparece.
 */
export function CompanyFilterChips({ module, snapshotKey = "competence" }: { module: "Fiscal" | "Contábil"; snapshotKey?: string }) {
  const [params, setParams] = useSearchParams();
  const f = companyFilterFrom(params);
  const users = useUsers();
  if (!hasCompanyFilter(f)) return null;
  const user = f.responsible_id ? users.data?.items.find((u) => u.id === f.responsible_id) : undefined;
  const parts: string[] = [];
  if (f.regime_group) parts.push(`Agrupamento: ${GROUP_LABELS[f.regime_group]}`);
  if (f.tax_regime) parts.push(`Regime: ${TAX_REGIME_LABELS[f.tax_regime]}`);
  if (f.responsible_id) parts.push(`Responsável do ${module}: ${user?.name ?? "…"}`);
  const snapshot = params.get(snapshotKey);
  const clear = () =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const k of [...COMPANY_FILTER_KEYS, "filter_competence"]) next.delete(k);
        return next;
      },
      { replace: true },
    );
  return (
    <div role="group" aria-label="Filtros do Dashboard" className="flex flex-wrap items-center gap-2 rounded-md border bg-accent/40 px-3 py-2 text-sm">
      <span className="font-medium">Filtros do Dashboard{snapshot ? ` (empresas de ${displayCompetence(snapshot)})` : ""}:</span>
      {parts.map((p) => (
        <span key={p} className="rounded border bg-card px-2 py-0.5 text-xs">
          {p}
        </span>
      ))}
      <Button size="sm" variant="ghost" onClick={clear}>
        <X aria-hidden /> Limpar filtros do Dashboard
      </Button>
    </div>
  );
}
