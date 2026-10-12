import { Percent } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { CompanyFilterChips } from "@/components/CompanyFilterChips";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { formatCnpj } from "@/lib/cnpj";
import { companyFilterFrom } from "@/lib/companyFilter";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate, isPositive } from "@/lib/decimal";
import { type FactorRPortfolio, type FactorRPortfolioLine, useFactorRPortfolio } from "@/services/factorR";
import { FactorRDisclaimer, FilterToggle, LightBadge, PANav } from "./common";
import { usePA } from "./usePA";
import { PACKAGE_LABELS } from "./labels";

function Kpi({ label, value, testId }: { label: string; value: ReactNode; testId: string }) {
  return (
    <div className="rounded-lg border bg-card px-4 py-3">
      <p className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{label}</p>
      <p className="mt-1 text-xl font-semibold tabular-nums" data-testid={testId}>
        {value}
      </p>
    </div>
  );
}

function Kpis({ data }: { data: FactorRPortfolio }) {
  const k = data.kpis;
  return (
    <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
      <Kpi label="Monitoradas" value={k.monitoradas} testId="kpi-monitoradas" />
      <Kpi label="No Anexo V" value={k.no_v} testId="kpi-no-v" />
      <Kpi label={`No limite (28% até ${formatRate(data.meta_operacional, 0)})`} value={k.no_limite} testId="kpi-no-limite" />
      <Kpi label="Seguras" value={k.seguras} testId="kpi-seguras" />
      <Kpi label="Economia em jogo (12 meses)" value={formatMoney(k.economia_em_jogo)} testId="kpi-economia" />
      <Kpi label="Receita dos pacotes (mês)" value={formatMoney(k.honorarios_pacotes)} testId="kpi-honorarios" />
    </div>
  );
}

/** Quanto a folha mensal precisa subir, com os valores do motor só rotulados [FR-07]. */
export function PayrollIncrease({ line }: { line: Pick<FactorRPortfolioLine, "reforco_mensal_28" | "reforco_mensal_meta" | "meta_operacional"> }) {
  const { reforco_mensal_28: to28, reforco_mensal_meta: toTarget, meta_operacional: target } = line;
  if (to28 === null || toTarget === null) return <span className="text-muted-foreground">—</span>;
  const targetLabel = `meta de ${formatRate(target)}`;
  if (!isPositive(toTarget)) return <span className="text-emerald-800">já na {targetLabel}</span>;
  return (
    <div className="grid gap-0.5 whitespace-nowrap">
      {isPositive(to28) ? (
        <span>
          <strong className="tabular-nums">+ {formatMoney(to28)}</strong>{" "}
          <span className="text-xs text-muted-foreground">para chegar a 28% (Anexo III)</span>
        </span>
      ) : (
        <span className="text-xs text-emerald-800">já atinge 28% (Anexo III)</span>
      )}
      <span>
        <span className="tabular-nums">+ {formatMoney(toTarget)}</span>{" "}
        <span className="text-xs text-muted-foreground">para a {targetLabel}</span>
      </span>
    </div>
  );
}

function PortfolioTable({ data, lines }: { data: FactorRPortfolio; lines: FactorRPortfolioLine[] }) {
  return (
    <div className="overflow-x-auto rounded-lg border bg-card">
      <Table aria-label={`Carteira do Fator R — PA ${displayCompetence(data.pa)}`}>
        <thead>
          <tr>
            <Th scope="col">Ação sugerida</Th>
            <Th scope="col">Empresa</Th>
            <Th scope="col">Semáforo</Th>
            <Th scope="col" className="text-right">
              Fator R
            </Th>
            <Th scope="col">Anexo</Th>
            <Th scope="col" className="text-right">
              RBT12
            </Th>
            <Th scope="col" className="text-right">
              FS12
            </Th>
            <Th scope="col">Aumento de folha por mês</Th>
            <Th scope="col" className="text-right">
              Economia 12 meses
            </Th>
          </tr>
        </thead>
        <tbody>
          {lines.map((l) => (
            <tr key={l.company.id} data-semaforo={lightOf(l)}>
              <Td className="font-medium whitespace-nowrap">{l.acao_texto}</Td>
              <Td>
                <Link
                  to={`/companies/${l.company.id}?tab=factor-r&pa=${data.pa}`}
                  className="inline-flex min-h-6 items-center font-medium hover:underline"
                >
                  {l.company.legal_name}
                </Link>
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <span>{l.company.internal_code}</span>
                  <span className="tabular-nums">{formatCnpj(l.company.cnpj)}</span>
                  {l.pacote_comercial ? <Badge variant="secondary">{PACKAGE_LABELS[l.pacote_comercial]}</Badge> : null}
                </div>
                {l.meses_faltantes.length > 0 ? (
                  <p className="text-xs text-red-800">Faltam: {l.meses_faltantes.map(displayCompetence).join(", ")}</p>
                ) : null}
              </Td>
              <Td>
                <LightBadge light={l.semaforo} />
              </Td>
              <Td className="text-right tabular-nums">{formatRate(l.fator_r)}</Td>
              <Td>{l.anexo ? `Anexo ${l.anexo}` : "—"}</Td>
              <Td className="text-right tabular-nums">{formatMoney(l.rbt12)}</Td>
              <Td className="text-right tabular-nums">{formatMoney(l.fs12)}</Td>
              <Td>
                <PayrollIncrease line={l} />
              </Td>
              <Td className="text-right tabular-nums">{formatMoney(l.economia_12m)}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}

// O filtro de semáforo da carteira (`semaforo` na URL, separado por vírgula): o drill-down do
// Dashboard (TASK-024, D-038). "insuficiente" são os dados insuficientes (sem semáforo).
const LIGHT_FILTERS = [
  ["vermelho", "Vermelho"],
  ["amarelo", "Amarelo"],
  ["verde", "Verde"],
  ["insuficiente", "Dados insuficientes"],
] as const;
type LightFilter = (typeof LIGHT_FILTERS)[number][0];

function lightOf(l: Pick<FactorRPortfolioLine, "semaforo">): LightFilter {
  return l.semaforo ?? "insuficiente";
}

function lightsFrom(params: URLSearchParams): LightFilter[] {
  const raw = (params.get("semaforo") ?? "").split(",");
  return LIGHT_FILTERS.map(([v]) => v).filter((v) => raw.includes(v));
}

/**
 * Carteira do Fator R [REQ-011, AC-007]: empresas ativas do Simples sujeitas ao Fator R, por
 * semáforo e economia, com a ação sugerida e os indicadores do escritório. Os valores vêm do
 * motor (backend); aqui só são formatados. Desde a TASK-024, com o filtro de semáforo e os filtros
 * de empresa que vêm do Dashboard (D-038).
 */
export function FactorRPortfolioPage() {
  const { pa, error: paError } = usePA();
  const [params, setParams] = useSearchParams();
  const lights = lightsFrom(params);
  const portfolio = useFactorRPortfolio(pa, companyFilterFrom(params));
  const data = portfolio.data;
  const lines = data ? data.linhas.filter((l) => lights.length === 0 || lights.includes(lightOf(l))) : [];
  const setLight = (v: LightFilter | null) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (v) next.set("semaforo", v);
        else next.delete("semaforo");
        return next;
      },
      { replace: true },
    );
  return (
    <>
      <PageHeader
        title="Fator R"
        description="Carteira do escritório: empresas do Simples sujeitas ao Fator R, por semáforo e economia."
        actions={<PANav />}
      />
      <div className="grid gap-4 p-4">
        <CompanyFilterChips module="Fiscal" snapshotKey="pa" />
        <ErrorAlert error={paError ?? portfolio.error} fallback="Não foi possível carregar a carteira." />
        {portfolio.isPending && pa !== null ? <Skeleton className="h-96" /> : null}
        {data ? (
          <>
            <Kpis data={data} />
            {data.linhas.length > 0 ? (
              <div className="flex flex-wrap items-center gap-2">
                <div role="group" aria-label="Filtrar por semáforo" className="flex flex-wrap gap-2">
                  <FilterToggle active={lights.length === 0} onClick={() => setLight(null)}>
                    Todas ({data.linhas.length})
                  </FilterToggle>
                  {LIGHT_FILTERS.map(([v, label]) => (
                    <FilterToggle key={v} active={lights.includes(v)} onClick={() => setLight(v)}>
                      {label} ({data.linhas.filter((l) => lightOf(l) === v).length})
                    </FilterToggle>
                  ))}
                </div>
                <p role="status" className="text-sm text-muted-foreground">
                  {lines.length === 1 ? "1 empresa" : `${lines.length} empresas`}
                </p>
              </div>
            ) : null}
            {data.linhas.length === 0 ? (
              <EmptyState
                icon={Percent}
                title="Nenhuma empresa na carteira deste PA"
                description="A carteira mostra as empresas ativas, no Simples Nacional no PA e marcadas como sujeitas ao Fator R (ficha da empresa, aba Folha)."
              />
            ) : (
              <PortfolioTable data={data} lines={lines} />
            )}
          </>
        ) : null}
        <FactorRDisclaimer />
      </div>
    </>
  );
}
