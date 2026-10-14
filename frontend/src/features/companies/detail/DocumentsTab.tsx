import { Upload } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Skeleton } from "@/components/ui/skeleton";
import { FilterToggle } from "@/features/factor-r/common";
import { DocumentIndexTable } from "@/features/documents/DocumentIndexTable";
import { DocumentFilterToggles } from "@/features/documents/DocumentsAreaPage";
import { useDocumentFilters } from "@/features/documents/useDocumentFilters";
import { displayCompetence } from "@/lib/competence";
import { useDocumentIndex } from "@/services/documents";
import { fichaHref } from "./fichaHref";

const linkClass = "inline-flex min-h-6 items-center gap-1 text-sm underline underline-offset-4";

/**
 * Os documentos da empresa em todos os módulos (TASK-026, D-040; na TASK-025 eram só atalhos): a
 * lista da área Documentos só da empresa, por padrão na competência da ficha (as procurações e os
 * certificados, sem competência, aparecem em "Todas as competências"). Abrir leva à tela do módulo.
 */
export function DocumentsTab({ companyId, competence }: { companyId: string; competence: string | null }) {
  const [params, setParams] = useSearchParams();
  const all = params.get("doc_all") === "1";
  const f = useDocumentFilters({ companyId, competence: all ? null : competence });
  const index = useDocumentIndex(f.query);
  const data = index.data;
  const setAll = (v: boolean) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (v) next.set("doc_all", "1");
        else next.delete("doc_all");
        next.delete("offset");
        return next;
      },
      { replace: true },
    );
  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <div role="group" aria-label="Período dos documentos" className="flex flex-wrap gap-2">
          <FilterToggle active={!all} onClick={() => setAll(false)}>
            Da competência{competence ? ` ${displayCompetence(competence)}` : ""}
          </FilterToggle>
          <FilterToggle active={all} onClick={() => setAll(true)}>
            Todas as competências
          </FilterToggle>
        </div>
        <div className="ml-auto flex flex-wrap gap-3">
          <Link to={fichaHref(companyId, params, "factor-r")} className={linkClass}>
            <Upload className="size-4" aria-hidden /> Extratos do PGDAS-D
          </Link>
          <Link to={`/reconciliation/documents?${new URLSearchParams({ company_id: companyId }).toString()}`} className={linkClass}>
            <Upload className="size-4" aria-hidden /> Documentos da Conciliação
          </Link>
          <Link to={fichaHref(companyId, params, "credentials")} className={linkClass}>
            <Upload className="size-4" aria-hidden /> Procurações e certificados
          </Link>
        </div>
      </div>
      <DocumentFilterToggles data={data} f={f} />
      <ErrorAlert error={index.error} fallback="Não foi possível carregar os documentos da empresa." />
      {index.isPending ? <Skeleton className="h-48" /> : null}
      {data ? (
        <DocumentIndexTable
          data={data}
          offset={f.offset}
          onOffset={(o) => f.update({ offset: o > 0 ? String(o) : null })}
          showCompany={false}
          aria-label="Documentos da empresa"
        />
      ) : null}
    </div>
  );
}
