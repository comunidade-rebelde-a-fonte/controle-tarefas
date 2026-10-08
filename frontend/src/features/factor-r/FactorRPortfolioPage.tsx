import { Percent } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { formatCnpj } from "@/lib/cnpj";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate, isPositive } from "@/lib/decimal";
import { type FactorRPortfolio, type FactorRPortfolioLine, useFactorRPortfolio } from "@/services/factorR";
import { FactorRDisclaimer, LightBadge, PANav } from "./common";
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

function PortfolioTable({ data }: { data: FactorRPortfolio }) {
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
          {data.linhas.map((l) => (
            <tr key={l.company.id} data-semaforo={l.semaforo ?? "insuficiente"}>
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

/**
 * Carteira do Fator R [REQ-011, AC-007]: empresas ativas do Simples sujeitas ao Fator R, por
 * semáforo e economia, com a ação sugerida e os indicadores do escritório. Os valores vêm do
 * motor (backend); aqui só são formatados.
 */
export function FactorRPortfolioPage() {
  const { pa, error: paError } = usePA();
  const portfolio = useFactorRPortfolio(pa);
  const data = portfolio.data;
  return (
    <>
      <PageHeader
        title="Fator R"
        description="Carteira do escritório: empresas do Simples sujeitas ao Fator R, por semáforo e economia."
        actions={<PANav />}
      />
      <div className="grid gap-4 p-4">
        <ErrorAlert error={paError ?? portfolio.error} fallback="Não foi possível carregar a carteira." />
        {portfolio.isPending && pa !== null ? <Skeleton className="h-96" /> : null}
        {data ? (
          <>
            <Kpis data={data} />
            {data.linhas.length === 0 ? (
              <EmptyState
                icon={Percent}
                title="Nenhuma empresa na carteira deste PA"
                description="A carteira mostra as empresas ativas, no Simples Nacional no PA e marcadas como sujeitas ao Fator R (ficha da empresa, aba Folha)."
              />
            ) : (
              <PortfolioTable data={data} />
            )}
          </>
        ) : null}
        <FactorRDisclaimer />
      </div>
    </>
  );
}
