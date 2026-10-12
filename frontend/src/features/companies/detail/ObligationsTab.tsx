import { X } from "lucide-react";
import { useState } from "react";
import { useSearchParams } from "react-router";
import { ObligationList } from "@/components/ObligationList";
import { Button } from "@/components/ui/button";
import { FilterToggle } from "@/features/factor-r/common";
import { type BacklogQuery, useBacklog } from "@/services/dashboard";

type State = NonNullable<BacklogQuery["state"]>;

const STATES: [State, string][] = [
  ["open", "Em aberto"],
  ["overdue", "Atrasadas"],
  ["pending", "Pendentes"],
  ["done", "Concluídas"],
  ["applicable", "Todas"],
];

// Os status que o "Exige atenção" da Visão geral filtra (documentos faltantes e erro).
const STATUS_LABELS: Record<string, string> = { MISSING_DOCUMENTS: "Documentos faltantes", ERROR: "Erro" };

function isState(v: string | null): v is State {
  return STATES.some(([s]) => s === v);
}

/**
 * As obrigações da empresa na competência da ficha (TASK-025, D-039), com a lista do Backlog e
 * do drill-down do Dashboard só da empresa; cada obrigação abre a grade do módulo filtrada nela.
 * A classe e o status ficam na URL (o "ver todos" das pendências da Visão geral).
 */
export function ObligationsTab({ companyId, competence }: { companyId: string; competence: string | null }) {
  const [params, setParams] = useSearchParams();
  const raw = params.get("state");
  const state: State = isState(raw) ? raw : "open";
  const status = params.get("status") ?? undefined;
  const [page, setPage] = useState(1);
  const list = useBacklog(
    competence ? { competence, company_id: companyId, state, ...(status ? { status } : {}), sort: "due_date", page, page_size: 50 } : null,
  );
  const update = (patch: Record<string, string | null>) => {
    setPage(1);
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
  };
  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <div role="group" aria-label="Filtrar obrigações" className="flex flex-wrap gap-2">
          {STATES.map(([s, label]) => (
            <FilterToggle key={s} active={state === s && !status} onClick={() => update({ state: s === "open" ? null : s, status: null })}>
              {label}
            </FilterToggle>
          ))}
        </div>
        {status ? (
          <span className="inline-flex items-center gap-1 rounded border bg-card px-2 py-0.5 text-xs">
            Status: {STATUS_LABELS[status] ?? status}
            <Button size="sm" variant="ghost" onClick={() => update({ status: null })} aria-label="Limpar o filtro de status">
              <X aria-hidden />
            </Button>
          </span>
        ) : null}
      </div>
      <ObligationList
        aria-label="Obrigações da empresa na competência"
        data={list.data}
        isLoading={list.isPending}
        error={list.error}
        onPage={setPage}
        showCompany={false}
        emptyMessage="Nenhuma obrigação da empresa nesta competência com este filtro."
      />
    </div>
  );
}
