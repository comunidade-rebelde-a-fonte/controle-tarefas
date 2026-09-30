import { CompetenceNav } from "@/components/CompetenceNav";
import { ObligationList } from "@/components/ObligationList";
import { PageHeader } from "@/components/PageHeader";
import { displayCompetence } from "@/lib/competence";
import { useCompetence } from "@/lib/useCompetence";
import { useUrlParams } from "@/lib/useUrlParams";
import { useBacklog } from "@/services/dashboard";
import { BacklogFilters } from "./BacklogFilters";
import { backlogQueryFromParams } from "./query";

/**
 * Backlog [PRD §25, plan R8]: obrigações aplicáveis não finais e módulos ainda abertos das
 * competências anteriores à selecionada, mais atrasadas primeiro. O item abre a grade do módulo.
 */
export function BacklogPage() {
  const { competence } = useCompetence();
  const { params, set } = useUrlParams();
  const before = params.get("before") ?? competence;
  const query = before ? backlogQueryFromParams(params, { before }) : null;
  const backlog = useBacklog(query);
  return (
    <>
      <PageHeader
        title="Backlog"
        description={before ? `Pendências de competências anteriores a ${displayCompetence(before)}.` : "Pendências de competências anteriores."}
        actions={<CompetenceNav />}
      />
      <div className="grid gap-3 p-4">
        <BacklogFilters />
        <ObligationList
          aria-label="Backlog"
          data={backlog.data}
          isLoading={backlog.isPending}
          error={backlog.error}
          onPage={(page) => set({ page: String(page) }, { resetPage: false })}
          emptyMessage="Nenhuma pendência de competências anteriores para os filtros selecionados."
        />
      </div>
    </>
  );
}
