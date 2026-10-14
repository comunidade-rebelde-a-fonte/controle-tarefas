import { addMonths, displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate } from "@/lib/decimal";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import type { FactorRCalc } from "@/services/factorR";
import { LightBadge } from "./common";
import { REASON_TEXTS } from "./labels";

function Value({ label, value, testId }: { label: string; value: string; testId?: string }) {
  return (
    <div className="rounded-lg border bg-card px-3 py-2.5">
      <dt className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">{label}</dt>
      <dd className="mt-1 text-lg font-semibold tabular-nums" data-testid={testId}>
        {value}
      </dd>
    </div>
  );
}

type MonthState = "preenchido" | "faltante" | "fora";

const MONTH_STYLE: Record<MonthState, string> = {
  preenchido: "border-success/30 bg-success/10 text-success",
  faltante: "border-destructive/30 bg-destructive/10 text-destructive",
  fora: "border-border bg-muted text-muted-foreground",
};

const MONTH_SUFFIX: Record<MonthState, string> = {
  preenchido: "",
  faltante: " · faltante",
  fora: " · antes do início",
};

/** Os 12 meses da janela com a situação de cada um (texto além da cor). */
function WindowMonths({ calc }: { calc: FactorRCalc }) {
  const filled = new Set(calc.meses_preenchidos);
  const missing = new Set(calc.meses_faltantes);
  const months = Array.from({ length: 12 }, (_, i) => addMonths(calc.janela_inicio, i));
  return (
    <ul aria-label="Meses da janela" className="flex flex-wrap gap-1">
      {months.map((m) => {
        const state: MonthState = missing.has(m) ? "faltante" : filled.has(m) ? "preenchido" : "fora";
        return (
          <li key={m} data-mes={m} data-situacao={state} className={cn("rounded border px-2 py-0.5 text-xs", MONTH_STYLE[state])}>
            {displayCompetence(m)}
            {MONTH_SUFFIX[state]}
          </li>
        );
      })}
    </ul>
  );
}

/** Passo a passo das contas com os números do PA: o cálculo é explicável (08 §6). */
function HowCalculated({ calc }: { calc: FactorRCalc }) {
  const cpp = calc.cpp_integra_fs12 ? " + CPP recolhida (política do escritório)" : "";
  const filled = calc.meses_preenchidos.length;
  return (
    <details className="rounded-lg border bg-card px-3 py-2 text-sm">
      <summary className="cursor-pointer font-medium">Como foi calculado</summary>
      <ol className="mt-2 grid list-decimal gap-1 pl-5 text-muted-foreground">
        <li>
          Janela do PA {displayCompetence(calc.pa)}: os 12 meses anteriores, de {displayCompetence(calc.janela_inicio)} a{" "}
          {displayCompetence(calc.janela_fim)}. {filled} {filled === 1 ? "mês lançado" : "meses lançados"}
          {calc.meses_faltantes.length > 0 ? "; mês faltante conta como zero." : "."}
        </li>
        <li>
          RBT12 = soma da receita bruta da janela = <strong className="text-foreground">{formatMoney(calc.rbt12)}</strong>.
        </li>
        <li>
          FS12 = pró-labore + salários + FGTS{cpp} = <strong className="text-foreground">{formatMoney(calc.fs12)}</strong>. Mês com a folha
          total declarada no PGDAS-D entra com esse total.
        </li>
        {calc.status === "ok" ? (
          <>
            <li>
              Fator R = FS12 ÷ RBT12 = <strong className="text-foreground">{formatRate(calc.fator_r)}</strong>:{" "}
              {calc.anexo === "III" ? "a partir de 28%, Anexo III" : "abaixo de 28%, Anexo V"}. A meta operacional de{" "}
              {formatRate(calc.meta_operacional)} só muda o semáforo, nunca o anexo.
            </li>
            <li>
              Alíquota efetiva = (RBT12 × alíquota nominal − parcela a deduzir) ÷ RBT12, na faixa do RBT12 das tabelas vigentes desde{" "}
              {formatDate(calc.tabela_vigencia_inicio)}: Anexo III {formatRate(calc.aliquota_efetiva_iii)}, Anexo V{" "}
              {formatRate(calc.aliquota_efetiva_v)}.
            </li>
            <li>
              Economia estimada de DAS em 12 meses = (efetiva V − efetiva III) × RBT12 ={" "}
              <strong className="text-foreground">{formatMoney(calc.economia_12m)}</strong>.
            </li>
          </>
        ) : null}
      </ol>
    </details>
  );
}

/** Resultado do Fator R no PA [AC-005, AC-006]: só formata o que o motor calculou. */
export function CalcPanel({ calc }: { calc: FactorRCalc }) {
  return (
    <section aria-label="Resultado do Fator R" className="grid gap-3" data-testid="painel-fator-r">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
        <LightBadge light={calc.semaforo} />
        <span data-testid="janela">
          Janela: {displayCompetence(calc.janela_inicio)} a {displayCompetence(calc.janela_fim)}
        </span>
        <span className="text-muted-foreground" data-testid="politica-cpp">
          Política do escritório: CPP do DAS {calc.cpp_integra_fs12 ? "integra" : "não integra"} a FS12
        </span>
        <span className="text-muted-foreground" data-testid="vigencia-tabela">
          Tabelas do Simples vigentes desde {formatDate(calc.tabela_vigencia_inicio)}
        </span>
      </div>
      <WindowMonths calc={calc} />
      {calc.status === "dados_insuficientes" && calc.motivo ? (
        <p data-testid="dados-insuficientes" className="rounded-lg border bg-muted/60 p-3 text-sm">
          <strong>Dados insuficientes.</strong> {REASON_TEXTS[calc.motivo]}
        </p>
      ) : null}
      <dl className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Value label="Fator R" value={formatRate(calc.fator_r)} testId="fator-r" />
        <Value label="Anexo" value={calc.anexo ? `Anexo ${calc.anexo}` : "—"} testId="anexo" />
        <Value label="RBT12" value={formatMoney(calc.rbt12)} testId="rbt12" />
        <Value label="FS12" value={formatMoney(calc.fs12)} testId="fs12" />
        <Value label="Folha mínima 28% (12 meses)" value={formatMoney(calc.folha_minima_28)} />
        <Value label={`Folha mínima ${formatRate(calc.meta_operacional)} (meta)`} value={formatMoney(calc.folha_minima_meta)} />
        <Value label="Gap até 28% (12 meses)" value={formatMoney(calc.gap_12m_28)} />
        <Value label="Reforço mensal até 28%" value={formatMoney(calc.reforco_mensal_28)} />
        <Value label="Gap até a meta (12 meses)" value={formatMoney(calc.gap_12m_meta)} />
        <Value label="Reforço mensal até a meta" value={formatMoney(calc.reforco_mensal_meta)} />
        <Value label="Alíquota efetiva Anexo III" value={formatRate(calc.aliquota_efetiva_iii)} testId="aliquota-iii" />
        <Value label="Alíquota efetiva Anexo V" value={formatRate(calc.aliquota_efetiva_v)} testId="aliquota-v" />
        <Value label="Economia estimada de DAS (III vs V, 12 meses)" value={formatMoney(calc.economia_12m)} testId="economia" />
      </dl>
      <HowCalculated calc={calc} />
    </section>
  );
}
