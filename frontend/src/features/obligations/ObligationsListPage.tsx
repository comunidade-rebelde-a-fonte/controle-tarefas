import { ArrowLeft } from "lucide-react";
import { Link } from "react-router";
import { ObligationList } from "@/components/ObligationList";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { BacklogFilters } from "@/features/backlog/BacklogFilters";
import { STATE_LABELS } from "@/features/backlog/labels";
import { backlogQueryFromParams } from "@/features/backlog/query";
import { displayCompetence, isCompetenceKey } from "@/lib/competence";
import { TAX_REGIME_LABELS, type TaxRegime } from "@/lib/labels";
import { useUrlParams } from "@/lib/useUrlParams";
import { useBacklog } from "@/services/dashboard";

/**
 * Lista filtrada do drill-down do dashboard [PRD §24.7]: as obrigações da competência que
 * compõem o número clicado (ex.: "Atrasadas: 14" → as 14 obrigações), com os filtros na URL.
 */
export function ObligationsListPage() {
  const { params, get, set } = useUrlParams();
  const competence = params.get("competence");
  const valid = isCompetenceKey(competence);
  const query = valid ? backlogQueryFromParams(params, { competence }) : null;
  const list = useBacklog(query);
  const state = get("state") || "open";
  const group = get("regime_group");
  const regime = get("tax_regime");
  const title = `${STATE_LABELS[state] ?? "Obrigações"}${valid ? ` — ${displayCompetence(competence)}` : ""}`;
  return (
    <>
      <PageHeader
        title={title}
        description="Obrigações que compõem o número selecionado no dashboard."
        actions={
          <Link className="inline-flex items-center gap-1 text-sm text-primary hover:underline" to={`/dashboard${valid ? `?competence=${competence}` : ""}`}>
            <ArrowLeft aria-hidden className="size-4" /> Voltar ao dashboard
          </Link>
        }
      />
      <div className="grid gap-3 p-4">
        {group || regime ? (
          <div className="flex gap-2 text-sm">
            {group ? <Badge variant="outline">{group === "SIMPLES" ? "Simples Nacional" : "Normais"}</Badge> : null}
            {regime ? <Badge variant="outline">{regime in TAX_REGIME_LABELS ? TAX_REGIME_LABELS[regime as TaxRegime] : regime}</Badge> : null}
          </div>
        ) : null}
        <BacklogFilters showOverdue={state === "overdue"} />
        <ObligationList
          aria-label={title}
          data={list.data}
          isLoading={list.isPending}
          error={valid ? list.error : new Error("competência inválida")}
          onPage={(page) => set({ page: String(page) }, { resetPage: false })}
        />
      </div>
    </>
  );
}
