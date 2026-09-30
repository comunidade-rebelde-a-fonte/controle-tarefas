import { Search } from "lucide-react";
import { useEffect, useEffectEvent, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { CopyCnpjButton } from "@/components/CopyButton";
import { DataGrid, type DataGridColumn } from "@/components/DataGrid";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { formatCnpj } from "@/lib/cnpj";
import { TAX_REGIME_LABELS, type TaxRegime } from "@/lib/labels";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { filterBySearch } from "@/lib/useGridFilters";
import { cn } from "@/lib/utils";
import { type CredentialRow, type CredentialType, useCredentialList } from "@/services/credentials";
import { DocumentsCell, ExpiresCell, NotesCell, RenewalCell, SituationCell } from "./cells";
import { CredentialsContext, type CredentialsActions, editId } from "./context";
import { DocumentsDialog } from "./DocumentsDialog";
import { RenewedDialog, type RenewedTarget } from "./RenewedDialog";

const DEFAULT_THRESHOLDS = [7, 15, 30, 60];

/** Filtros rápidos do §33 (as faixas "vence em" vêm das configurações). */
function quickFilters(thresholds: number[]): { value: string; label: string }[] {
  return [
    { value: "", label: "Todos" },
    { value: "expired", label: "Vencidos" },
    ...thresholds.map((d) => ({ value: `due_${String(d)}`, label: `Vencem em até ${String(d)} dias` })),
    { value: "awaiting_client", label: "Aguardando cliente" },
    { value: "no_date", label: "Sem data cadastrada" },
  ];
}

const FILTER_RE = /^(expired|awaiting_client|no_date|due_[1-9][0-9]{0,2})$/;

function typeColumns(type: CredentialType, prefix: string, short: string): DataGridColumn<CredentialRow>[] {
  return [
    { id: `${prefix}_expires`, header: `Vencimento ${short}`, width: 140, cell: (r) => <ExpiresCell row={r} type={type} columnId={`${prefix}_expires`} /> },
    { id: `${prefix}_situation`, header: `Situação ${short}`, width: 170, cell: (r) => <SituationCell row={r} type={type} /> },
    { id: `${prefix}_renewal`, header: `Renovação ${short}`, width: 180, cell: (r) => <RenewalCell row={r} type={type} columnId={`${prefix}_renewal`} /> },
  ];
}

/** Colunas do §33, estáveis entre renders (a grade não remonta as células). */
const COLUMNS: DataGridColumn<CredentialRow>[] = [
  { id: "company", header: "Empresa", width: 280, cell: (r) => r.company.legal_name },
  { id: "code", header: "Código", width: 80, cell: (r) => r.company.internal_code },
  {
    id: "cnpj",
    header: "CNPJ",
    width: 190,
    cell: (r) => (
      <span className="flex items-center gap-1 tabular-nums">
        {formatCnpj(r.company.cnpj)}
        <CopyCnpjButton cnpj={r.company.cnpj} tabIndex={-1} />
      </span>
    ),
  },
  {
    id: "regime",
    header: "Regime",
    width: 140,
    cell: (r) => <span className="truncate text-xs">{r.company.tax_regime ? TAX_REGIME_LABELS[r.company.tax_regime as TaxRegime] : "—"}</span>,
  },
  ...typeColumns("RFB_POWER_OF_ATTORNEY", "poa", "procuração"),
  ...typeColumns("DIGITAL_CERTIFICATE", "cert", "certificado"),
  { id: "notes", header: "Observação", width: 240, cell: (r) => <NotesCell row={r} /> },
  { id: "documents", header: "Documentos", width: 110, cell: (r) => <DocumentsCell row={r} columnId="documents" /> },
];

/** Enter/Espaço na célula foca o controle editável dela. */
function activateCell(row: CredentialRow, columnId: string) {
  const el = document.querySelector<HTMLElement>(`[data-edit="${editId(row.company.id, columnId)}"]`);
  if (!el) return;
  el.focus();
  if (el instanceof HTMLButtonElement) el.click();
}

/**
 * Procurações e Certificados [PRD §19, §33, UC-05]: grade com filtros rápidos, edição de
 * vencimento e etapa na célula, "Renovado" com nova validade e documentos por empresa.
 */
export function CredentialsPage() {
  const [params, setParams] = useSearchParams();
  const rawFilter = params.get("filter") ?? "";
  const filter = FILTER_RE.test(rawFilter) ? rawFilter : "";
  const q = params.get("q") ?? "";
  const [search, setSearch] = useState(q);
  const debounced = useDebouncedValue(search, 250);
  const setParam = (key: string, value: string) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(key, value);
        else next.delete(key);
        return next;
      },
      { replace: true },
    );
  const syncSearch = useEffectEvent((value: string) => {
    if (value !== q) setParam("q", value);
  });
  useEffect(() => {
    syncSearch(debounced);
  }, [debounced]);

  const list = useCredentialList(filter);
  const rows = useMemo(() => filterBySearch(list.data?.items ?? [], q, (r) => r.company), [list.data, q]);
  const chips = quickFilters(list.data?.thresholds ?? DEFAULT_THRESHOLDS);

  const [renewed, setRenewed] = useState<RenewedTarget | null>(null);
  const [documents, setDocuments] = useState<string | null>(null);
  const actions = useMemo<CredentialsActions>(
    () => ({ openRenewed: (row, type) => setRenewed({ row, type }), openDocuments: (row) => setDocuments(row.company.id) }),
    [setRenewed, setDocuments],
  );

  return (
    <CredentialsContext.Provider value={actions}>
      <PageHeader title="Procurações e Certificados" description="Vencimentos e renovação de procurações RFB e certificados digitais." />
      <div className="grid gap-3 p-4">
        <div className="flex flex-wrap items-center gap-2">
          <div role="group" aria-label="Filtros rápidos" className="flex flex-wrap gap-1.5">
            {chips.map((c) => (
              <button
                key={c.value || "all"}
                type="button"
                aria-pressed={filter === c.value}
                onClick={() => setParam("filter", c.value)}
                className={cn(
                  "rounded-full border px-3 py-1 text-xs font-medium",
                  filter === c.value ? "border-primary bg-primary text-primary-foreground" : "bg-card text-muted-foreground hover:bg-accent",
                )}
              >
                {c.label}
              </button>
            ))}
          </div>
          <div className="relative ml-auto w-80">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
            <Input
              type="search"
              aria-label="Buscar empresa"
              placeholder="Nome, código ou CNPJ (com ou sem máscara)"
              className="pl-8"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <p className="w-full text-sm text-muted-foreground" aria-live="polite">
            {list.data ? `${String(rows.length)} ${rows.length === 1 ? "empresa" : "empresas"}` : ""}
          </p>
        </div>
        <ErrorAlert error={list.error} fallback="Não foi possível carregar procurações e certificados." />
        {list.isPending ? (
          <Skeleton className="h-96" />
        ) : (
          <DataGrid
            aria-label="Procurações e Certificados"
            rows={rows}
            columns={COLUMNS}
            getRowId={(r) => r.company.id}
            onCellActivate={activateCell}
            emptyMessage="Nenhuma empresa para os filtros selecionados."
          />
        )}
      </div>
      <RenewedDialog target={renewed} onClose={() => setRenewed(null)} />
      <DocumentsDialog companyId={documents} onClose={() => setDocuments(null)} />
    </CredentialsContext.Provider>
  );
}
