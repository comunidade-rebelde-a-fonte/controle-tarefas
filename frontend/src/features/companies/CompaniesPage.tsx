import { Ban, Building2, ExternalLink, MoreHorizontal, Pencil, Plus, Search } from "lucide-react";
import { useEffect, useEffectEvent, useState, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { CopyButton, CopyCnpjButton } from "@/components/CopyButton";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input, NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { Tooltip } from "@/components/ui/tooltip";
import { useToast } from "@/components/ui/toast-context";
import { formatCnpj } from "@/lib/cnpj";
import { formatDate } from "@/lib/format";
import { MODULE_LABELS, MODULE_SHORT, MODULES, TAX_REGIME_LABELS, TAX_REGIMES, type ModuleCode, type TaxRegime } from "@/lib/labels";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { type Company, type CompanyFilters, useCompanies } from "@/services/companies";
import { DeactivateCompanyDialog, EditCompanyDialog } from "./CompanyActions";
import { CompanyFormDialog } from "./CompanyForm";

type StatusFilter = "active" | "inactive" | "all";

/** Filtros da listagem sincronizados com a URL [AGENTS §8.5]. */
function useCompanyFilters() {
  const [params, setParams] = useSearchParams();
  const status = (params.get("status") ?? "active") as StatusFilter;
  const regime = params.get("tax_regime") ?? "";
  const module = params.get("module") ?? "";
  const q = params.get("q") ?? "";
  const set = (key: string, value: string) => {
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (value) next.set(key, value);
        else next.delete(key);
        return next;
      },
      { replace: true },
    );
  };
  return { status, regime, module, q, set };
}

function toApiFilters(q: string, status: StatusFilter, regime: string, module: string): CompanyFilters {
  const f: CompanyFilters = {};
  if (q.trim()) f.q = q.trim();
  if (status !== "all") f.active = status === "active";
  if (regime) f.tax_regime = regime as TaxRegime;
  if (module) f.module = module as ModuleCode;
  return f;
}

export function CompaniesPage({ emptyState }: { emptyState?: (openCreate: () => void) => ReactNode }) {
  const { status, regime, module, q, set } = useCompanyFilters();
  const [creating, setCreating] = useState(false);
  const [search, setSearch] = useState(q);
  const debounced = useDebouncedValue(search, 300);

  // Leva o termo para a URL somente quando a digitação estabiliza.
  const syncQuery = useEffectEvent((value: string) => {
    if (value !== q) set("q", value);
  });
  useEffect(() => {
    syncQuery(debounced);
  }, [debounced]);

  const filters = toApiFilters(debounced, status, regime, module);
  const { data, isPending, isError, isFetching } = useCompanies(filters);
  const hasFilters = !!(debounced || regime || module || status !== "active");

  return (
    <>
      <PageHeader
        title="Empresas"
        description="Cadastro único usado por todos os módulos."
        actions={
          <Button onClick={() => setCreating(true)}>
            <Plus aria-hidden /> Nova empresa
          </Button>
        }
      />
      <CompanyFormDialog open={creating} onOpenChange={setCreating} />
      <div className="space-y-4 p-6">
        <div className="flex flex-wrap items-end gap-3" role="search" aria-label="Filtros de empresas">
          <div className="relative w-96">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input
              type="search"
              aria-label="Buscar empresa"
              placeholder="Buscar por nome, código ou CNPJ (com ou sem máscara)"
              className="pl-8"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <FilterSelect label="Regime" value={regime} onChange={(v) => set("tax_regime", v)}>
            <option value="">Todos os regimes</option>
            {TAX_REGIMES.map((r) => (
              <option key={r} value={r}>
                {TAX_REGIME_LABELS[r]}
              </option>
            ))}
          </FilterSelect>
          <FilterSelect label="Situação" value={status} onChange={(v) => set("status", v === "active" ? "" : v)}>
            <option value="active">Ativas</option>
            <option value="inactive">Inativas</option>
            <option value="all">Todas</option>
          </FilterSelect>
          <FilterSelect label="Módulo" value={module} onChange={(v) => set("module", v)}>
            <option value="">Todos os módulos</option>
            {MODULES.map((m) => (
              <option key={m} value={m}>
                {MODULE_LABELS[m]}
              </option>
            ))}
          </FilterSelect>
          {hasFilters ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setSearch("");
                ["q", "tax_regime", "module", "status"].forEach((k) => set(k, ""));
              }}
            >
              Limpar filtros
            </Button>
          ) : null}
          <p className="ml-auto text-sm text-muted-foreground" aria-live="polite">
            {data ? `${String(data.total)} empresa${data.total === 1 ? "" : "s"}` : ""}
            {isFetching && data ? " · atualizando…" : ""}
          </p>
        </div>

        {isError ? (
          <p className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-800" role="alert">
            Não foi possível carregar as empresas. Tente novamente.
          </p>
        ) : isPending ? (
          <div className="space-y-2" aria-label="Carregando empresas">
            {Array.from({ length: 6 }, (_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : data.items.length === 0 ? (
          hasFilters || !emptyState ? (
            <p className="rounded-md border border-dashed bg-card p-8 text-center text-sm text-muted-foreground">
              Nenhuma empresa encontrada para os filtros selecionados.
            </p>
          ) : (
            emptyState(() => setCreating(true))
          )
        ) : (
          <CompaniesTable items={data.items} />
        )}
      </div>
    </>
  );
}

function FilterSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (v: string) => void; children: ReactNode }) {
  return (
    <NativeSelect aria-label={label} className="w-48" value={value} onChange={(e) => onChange(e.target.value)}>
      {children}
    </NativeSelect>
  );
}

function CompaniesTable({ items }: { items: Company[] }) {
  return (
    <div className="max-h-[calc(100vh-13rem)] overflow-auto rounded-lg border bg-card">
      <Table aria-label="Empresas" className="min-w-[1120px] [&_td]:whitespace-nowrap [&_th]:whitespace-nowrap">
        <thead>
          <tr>
            <Th>Código</Th>
            <Th className="w-full">Razão social</Th>
            <Th>CNPJ</Th>
            <Th>Regime vigente</Th>
            <Th>Município/UF</Th>
            <Th>Módulos</Th>
            <Th>Responsável</Th>
            <Th>Situação</Th>
            <Th>Atendimento</Th>
            <Th className="right-0 z-20 shadow-[-1px_0_0_var(--color-border)]">
              <span className="sr-only">Ações</span>
            </Th>
          </tr>
        </thead>
        <tbody>
          {items.map((c) => (
            <CompanyRow key={c.id} company={c} />
          ))}
        </tbody>
      </Table>
    </div>
  );
}

function CompanyRow({ company: c }: { company: Company }) {
  const toast = useToast();
  const [dialog, setDialog] = useState<"edit" | "deactivate" | null>(null);
  const responsible = c.responsibles.fiscal ?? c.responsibles.accounting ?? c.responsibles.payroll ?? c.responsibles.declarations;
  const copy = async (value: string, what: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast({ title: `${what} copiado`, variant: "success" });
    } catch {
      toast({ title: `Não foi possível copiar o ${what.toLowerCase()}`, variant: "error" });
    }
  };
  return (
    <tr className="hover:bg-accent/40">
      <Td>
        <span className="inline-flex items-center gap-0.5 font-mono text-xs">
          {c.internal_code}
          <CopyButton value={c.internal_code} label="Copiar código" />
        </span>
      </Td>
      <Td className="max-w-0 min-w-52">
        <Link to={`/companies/${c.id}`} className="block truncate font-medium hover:underline" title={c.legal_name}>
          {c.legal_name}
        </Link>
        {c.trade_name ? <p className="truncate text-xs text-muted-foreground">{c.trade_name}</p> : null}
      </Td>
      <Td>
        <span className="inline-flex items-center gap-0.5 font-mono text-xs tabular-nums">
          {formatCnpj(c.cnpj)}
          <CopyCnpjButton cnpj={c.cnpj} />
        </span>
      </Td>
      <Td>{c.current_tax_regime ? TAX_REGIME_LABELS[c.current_tax_regime] : <span className="text-muted-foreground">—</span>}</Td>
      <Td>{[c.city, c.state].filter(Boolean).join("/") || <span className="text-muted-foreground">—</span>}</Td>
      <Td>
        <div className="flex gap-0.5">
          {c.active_modules.length === 0 ? <span className="text-xs text-muted-foreground">Nenhum</span> : null}
          {MODULES.filter((m) => c.active_modules.includes(m)).map((m) => (
            <Tooltip key={m} content={MODULE_LABELS[m]}>
              <Badge variant="secondary" aria-label={MODULE_LABELS[m]}>
                {MODULE_SHORT[m]}
              </Badge>
            </Tooltip>
          ))}
        </div>
      </Td>
      <Td className="text-sm">{responsible?.name ?? <span className="text-muted-foreground">—</span>}</Td>
      <Td>{c.active ? <Badge variant="success">Ativa</Badge> : <Badge variant="muted">Inativa</Badge>}</Td>
      <Td className="text-xs tabular-nums">
        <span aria-label="Início do atendimento">{formatDate(c.service_start_date)}</span>
        {" – "}
        <span aria-label="Fim do atendimento">{c.service_end_date ? formatDate(c.service_end_date) : "atual"}</span>
      </Td>
      <Td className="sticky right-0 bg-card shadow-[-1px_0_0_var(--color-border)]">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={`Ações de ${c.legal_name}`}>
              <MoreHorizontal aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuLabel>{c.internal_code}</DropdownMenuLabel>
            <DropdownMenuItem asChild>
              <Link to={`/companies/${c.id}`}>
                <ExternalLink aria-hidden /> Abrir ficha
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => void copy(c.internal_code, "Código")}>
              <Building2 aria-hidden /> Copiar código
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => void copy(c.cnpj.replace(/\D/g, ""), "CNPJ")}>
              <Building2 aria-hidden /> Copiar CNPJ sem máscara
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setDialog("edit")}>
              <Pencil aria-hidden /> Editar
            </DropdownMenuItem>
            {c.active ? (
              <DropdownMenuItem onSelect={() => setDialog("deactivate")} className="text-destructive">
                <Ban aria-hidden /> Inativar
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
        {dialog === "edit" ? <EditCompanyDialog companyId={c.id} onClose={() => setDialog(null)} /> : null}
        {dialog === "deactivate" ? <DeactivateCompanyDialog company={c} onClose={() => setDialog(null)} /> : null}
      </Td>
    </tr>
  );
}
