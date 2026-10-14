import { Building2, Lock } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { CompanyFilterChips } from "@/components/CompanyFilterChips";
import { CompetenceInput } from "@/components/CompetenceInput";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { FilterToggle } from "@/features/factor-r/common";
import { companyFilterFrom } from "@/lib/companyFilter";
import { isCompetenceKey } from "@/lib/competence";
import { useLatestCompetence } from "@/services/competencies";
import { type ReconciliationOverviewState, useReconciliationOverview } from "@/services/reconciliation";
import { blockingText, COMPLETION_STATES } from "../labels";

const STATES = COMPLETION_STATES;

function isState(v: string | null): v is ReconciliationOverviewState {
  return STATES.some(([s]) => s === v);
}

/**
 * A Conciliação por empresa na competência (TASK-024, D-038): o estado da conclusão de cada
 * empresa, o que falta, o Contábil e a obrigação Conciliação. É a lista que explica as prontas
 * para concluir e o Contábil bloqueado do Dashboard, com os mesmos filtros.
 */
export function CompaniesPage() {
  const [params, setParams] = useSearchParams();
  const latest = useLatestCompetence();
  const chosen = params.get("competence");
  const competence = isCompetenceKey(chosen) ? chosen : (latest.data?.current ?? null);
  const rawState = params.get("state");
  const state = isState(rawState) ? rawState : undefined;
  const blocked = params.get("blocked") === "true";
  const overview = useReconciliationOverview(competence, { state, blocked, filter: companyFilterFrom(params) });
  const data = overview.data;
  const counts = data?.counts;
  const update = (patch: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        return next;
      },
      { replace: true },
    );
  return (
    <>
      <PageHeader
        title="Empresas"
        description="A Conciliação de cada empresa na competência: o que falta para concluir, o Contábil e a obrigação Conciliação."
      />
      <div className="grid gap-4 p-4">
        <label className="grid w-fit gap-1 text-xs font-medium text-muted-foreground">
          Competência
          <CompetenceInput value={competence ?? ""} onChange={(v) => update({ competence: v || null })} />
        </label>
        <CompanyFilterChips module="Contábil" />
        <div role="group" aria-label="Filtrar por estado" className="flex flex-wrap gap-2">
          <FilterToggle active={!state && !blocked} onClick={() => update({ state: null, blocked: null })}>
            Todas ({counts?.total ?? 0})
          </FilterToggle>
          {STATES.map(([s, label]) => (
            <FilterToggle key={s} active={state === s && !blocked} onClick={() => update({ state: s, blocked: null })}>
              {label} ({counts?.[s] ?? 0})
            </FilterToggle>
          ))}
          <FilterToggle active={blocked} onClick={() => update({ state: null, blocked: blocked ? null : "true" })}>
            Bloqueando o Contábil ({counts?.blocks_accounting ?? 0})
          </FilterToggle>
        </div>
        <ErrorAlert error={latest.error ?? overview.error} fallback="Não foi possível carregar a Conciliação por empresa." />
        {overview.isPending && competence ? <Skeleton className="h-64" /> : null}
        {data ? (
          <p role="status" className="text-sm text-muted-foreground">
            {data.items.length === 1 ? "1 empresa" : `${String(data.items.length)} empresas`}
          </p>
        ) : null}
        {data && data.items.length === 0 ? (
          <EmptyState icon={Building2} title="Nenhuma empresa" description="Com os filtros escolhidos, nenhuma empresa usa a Conciliação nesta competência." />
        ) : null}
        {data && data.items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Conciliação por empresa">
              <thead>
                <tr>
                  <Th scope="col">Empresa</Th>
                  <Th scope="col">Conciliação</Th>
                  <Th scope="col">O que falta</Th>
                  <Th scope="col">Contábil</Th>
                  <Th scope="col">Obrigação Conciliação</Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((it) => {
                  const st = STATES.find(([s]) => s === it.state);
                  return (
                    <tr key={it.company.id}>
                      <Td>
                        <Link
                          to={`/companies/${it.company.id}?${new URLSearchParams({ tab: "reconciliation", competence: data.competence }).toString()}`}
                          className="inline-flex min-h-6 items-center font-medium hover:underline"
                        >
                          {it.company.legal_name}
                        </Link>
                        <div className="text-xs text-muted-foreground">{it.company.internal_code}</div>
                      </Td>
                      <Td>{st ? <Badge variant={st[2]}>{st[1]}</Badge> : it.state}</Td>
                      <Td className="text-xs">{blockingText(it.blocking)}</Td>
                      <Td className="text-xs whitespace-nowrap">
                        {it.accounting_state === "CLOSED" ? "Fechado" : it.accounting_state === "OPEN" ? "Aberto" : "—"}
                        {it.blocks_accounting ? (
                          <span className="mt-0.5 flex items-center gap-1 text-warning">
                            <Lock className="size-3" aria-hidden /> Não fecha: a Conciliação não está concluída
                          </span>
                        ) : null}
                      </Td>
                      <Td className="text-xs">
                        {it.obligation ? (it.obligation.applicable ? it.obligation.status_label : "Não aplicável") : "Não gerada"}
                      </Td>
                    </tr>
                  );
                })}
              </tbody>
            </Table>
          </div>
        ) : null}
      </div>
    </>
  );
}
