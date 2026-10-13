import { Upload } from "lucide-react";
import { Link } from "react-router";
import { CompanySelect } from "@/components/CompanySelect";
import { CompetenceInput } from "@/components/CompetenceInput";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Input, NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { FilterToggle } from "@/features/factor-r/common";
import { type DocumentIndex, useDocumentIndex } from "@/services/documents";
import { DocumentIndexTable } from "./DocumentIndexTable";
import { KIND_LABELS, KINDS_BY_MODULE, MODULES, SITUATIONS, statusText } from "./labels";
import { ALL_KINDS, useDocumentFilters } from "./useDocumentFilters";

/** A soma das contagens que passam no filtro dado. */
function sum(counts: DocumentIndex["counts"] | undefined, keep: (c: DocumentIndex["counts"][number]) => boolean): number {
  return (counts ?? []).filter(keep).reduce((n, c) => n + c.total, 0);
}

/** Os botões de origem, situação e status do módulo, com as quantidades. */
export function DocumentFilterToggles({ data, f }: { data: DocumentIndex | undefined; f: ReturnType<typeof useDocumentFilters> }) {
  const counts = data?.counts;
  return (
    <div className="grid gap-2">
      <div role="group" aria-label="Filtrar por origem" className="flex flex-wrap gap-2">
        <FilterToggle active={!f.module} onClick={() => f.update({ module: null, status: null, kind: null })}>
          Todas as origens ({sum(counts, () => true)})
        </FilterToggle>
        {MODULES.map(([m, label]) => (
          <FilterToggle key={m} active={f.module === m} onClick={() => f.update({ module: m, status: null, kind: null })}>
            {label} ({sum(counts, (c) => c.module === m)})
          </FilterToggle>
        ))}
      </div>
      <div role="group" aria-label="Filtrar por situação" className="flex flex-wrap gap-2">
        <FilterToggle active={!f.situation} onClick={() => f.update({ situation: null })}>
          Todas as situações
        </FilterToggle>
        {SITUATIONS.map(([s, label]) => (
          <FilterToggle key={s} active={f.situation === s} onClick={() => f.update({ situation: s })}>
            {label} ({sum(counts, (c) => c.situation === s && (!f.module || c.module === f.module))})
          </FilterToggle>
        ))}
      </div>
      {f.module ? (
        <div role="group" aria-label="Filtrar pelo status do módulo" className="flex flex-wrap gap-2">
          <FilterToggle active={!f.status} onClick={() => f.update({ status: null })}>
            Todos os status
          </FilterToggle>
          {(counts ?? [])
            .filter((c) => c.module === f.module)
            .map((c) => (
              <FilterToggle key={c.status} active={f.status === c.status} onClick={() => f.update({ status: c.status })}>
                {statusText(c.module, c.status)} ({c.total})
              </FilterToggle>
            ))}
        </div>
      ) : null}
    </div>
  );
}

/**
 * A área Documentos (08 §8; TASK-026, D-040): um agregador de navegação sobre os documentos do
 * Fator R, da Conciliação e das procurações e certificados, cada um no seu armazenamento e com o
 * seu estado. "Abrir" leva à tela do módulo; o envio continua em cada módulo.
 */
export function DocumentsAreaPage() {
  const f = useDocumentFilters();
  const index = useDocumentIndex(f.query);
  const data = index.data;
  const kinds = f.module ? KINDS_BY_MODULE[f.module] : ALL_KINDS;
  return (
    <>
      <PageHeader
        title="Documentos"
        description="Os documentos de todos os módulos num lugar só, cada um no seu armazenamento; abrir leva à tela do módulo."
        actions={
          <>
            <Link to="/factor-r/inbox" className="inline-flex min-h-6 items-center gap-1 text-sm underline underline-offset-4">
              <Upload className="size-4" aria-hidden /> Enviar extrato do PGDAS-D
            </Link>
            <Link to="/reconciliation/documents" className="inline-flex min-h-6 items-center gap-1 text-sm underline underline-offset-4">
              <Upload className="size-4" aria-hidden /> Enviar extrato bancário ou comprovante
            </Link>
          </>
        }
      />
      <div className="grid gap-4 p-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Empresa
            <CompanySelect value={f.companyId ?? ""} onChange={(id) => f.update({ company_id: id || null })} allLabel="Todas as empresas" />
          </label>
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Competência ou período
            <CompetenceInput value={f.competence ?? ""} onChange={(v) => f.update({ competence: v || null })} />
          </label>
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Tipo
            <NativeSelect className="w-52" value={f.kind ?? ""} onChange={(e) => f.update({ kind: e.target.value || null })}>
              <option value="">Todos os tipos</option>
              {kinds.map((k) => (
                <option key={k} value={k}>
                  {KIND_LABELS[k]}
                </option>
              ))}
            </NativeSelect>
          </label>
          {!f.module || f.module === "reconciliation" ? (
            <label className="grid gap-1 text-xs font-medium text-muted-foreground">
              Banco
              <Input
                className="w-44"
                defaultValue={f.bank ?? ""}
                placeholder="Ex.: beta"
                onBlur={(e) => f.update({ bank: e.target.value.trim() || null })}
                onKeyDown={(e) => {
                  if (e.key === "Enter") f.update({ bank: e.currentTarget.value.trim() || null });
                }}
              />
            </label>
          ) : null}
        </div>
        <DocumentFilterToggles data={data} f={f} />
        <ErrorAlert error={index.error} fallback="Não foi possível carregar os documentos." />
        {index.isPending ? <Skeleton className="h-64" /> : null}
        {data ? (
          <>
            <p role="status" className="text-sm text-muted-foreground">
              {data.total === 1 ? "1 documento" : `${String(data.total)} documentos`}
            </p>
            <DocumentIndexTable data={data} offset={f.offset} onOffset={(o) => f.update({ offset: o > 0 ? String(o) : null })} aria-label="Documentos" />
          </>
        ) : null}
      </div>
    </>
  );
}
