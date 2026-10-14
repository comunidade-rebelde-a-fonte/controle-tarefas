import { Calculator, ScrollText, TriangleAlert } from "lucide-react";
import { type SubmitEvent, useState } from "react";
import { Link } from "react-router";
import { CompetenceInput } from "@/components/CompetenceInput";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { isApiError } from "@/lib/api";
import { displayCompetence, isCompetenceKey } from "@/lib/competence";
import { formatMoney, formatRate, parsePercentInput } from "@/lib/decimal";
import { formatDateTime } from "@/lib/format";
import { type FactorRSimulation, useCreateFactorRSimulation, useFactorRSimulations } from "@/services/factorRAgents";
import { VerdictBadge } from "./VerdictBadge";

/** Valor do JSON da simulação como texto decimal (o motor manda decimais como string). */
function text(v: unknown): string | null {
  if (typeof v === "string") return v;
  if (typeof v === "number") return String(v);
  return null;
}

type FieldName = "pa" | "meta" | "fracao_pro_labore" | "inss" | "irrf" | "horizonte_meses";
const FIELDS: readonly FieldName[] = ["pa", "meta", "fracao_pro_labore", "inss", "irrf", "horizonte_meses"];

function Line({ label, value, testId }: { label: string; value: string; testId?: string }) {
  return (
    <div className="flex justify-between gap-4 border-b py-1 last:border-0">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium tabular-nums" data-testid={testId}>
        {value}
      </dd>
    </div>
  );
}

/** Parâmetros usados na simulação (os padrões entram quando o campo fica vazio). */
function usedParams(s: FactorRSimulation): string {
  const p = s.parametros;
  const parts = [
    `meta ${formatRate(text(p.meta))}`,
    `${formatRate(text(p.fracao_pro_labore), 0)} do reforço em pró-labore`,
    `INSS ${formatRate(text(p.inss))}`,
    `IRRF ${formatRate(text(p.irrf))}`,
    `${text(p.horizonte_meses) ?? "—"} meses`,
  ];
  const floor = text(p.piso_economia_anual);
  if (floor) parts.push(`piso de ${formatMoney(floor)} por ano`);
  return parts.join(" · ");
}

/** Resultado do simulador [FR-12, AC-010]: só formata o que o motor calculou. */
function Result({ s }: { s: FactorRSimulation }) {
  const r = s.resultado;
  return (
    <section aria-label="Resultado da simulação" className="grid gap-2 rounded-lg border bg-card p-3 text-sm" data-testid="resultado-simulacao">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span data-testid="veredito">
          <VerdictBadge verdict={s.veredito} />
        </span>
        <Link to={`/factor-r/runs/${s.trace_id}`} className="inline-flex items-center gap-1 text-xs underline underline-offset-4" data-testid="link-trace-simulacao">
          <ScrollText className="size-3.5" aria-hidden /> ver trace
        </Link>
      </div>
      <p className="whitespace-pre-line">{s.texto}</p>
      <dl>
        <Line label="Fator R atual" value={formatRate(text(r.fator_atual))} />
        <Line label="Reforço de folha (12 meses)" value={formatMoney(text(r.reforco_12m))} />
        <Line label="Reforço por mês" value={formatMoney(text(r.reforco_mensal))} />
        <Line label="Pró-labore extra por mês" value={formatMoney(text(r.pro_labore_extra_mensal))} />
        <Line label="Custo INSS do sócio" value={formatMoney(text(r.custo_inss))} />
        <Line label="Custo IRRF" value={formatMoney(text(r.custo_irrf))} />
        <Line label="Economia de DAS no horizonte" value={formatMoney(text(r.economia_horizonte))} />
        <Line label="Líquido" value={formatMoney(text(r.liquido))} testId="liquido" />
      </dl>
      <p className="text-xs text-muted-foreground">Parâmetros: {usedParams(s)}.</p>
    </section>
  );
}

function History({ companyId }: { companyId: string }) {
  const history = useFactorRSimulations(companyId);
  const items = history.data?.items ?? [];
  return (
    <section aria-label="Últimas simulações" className="grid gap-2 text-sm">
      <h3 className="text-xs font-semibold tracking-wide text-muted-foreground uppercase">Últimas simulações</h3>
      <ErrorAlert error={history.error} fallback="Não foi possível carregar o histórico." />
      {history.data && items.length === 0 ? <p className="text-muted-foreground">Nenhuma simulação ainda.</p> : null}
      <ul className="grid gap-1" data-testid="historico-simulacoes">
        {items.map((s) => (
          <li key={s.simulation_id} className="flex flex-wrap items-center gap-2">
            <VerdictBadge verdict={s.veredito} />
            <span>PA {displayCompetence(s.pa)}</span>
            <span className="text-muted-foreground">líquido {formatMoney(text((s.resultado).liquido))}</span>
            <span className="text-xs text-muted-foreground">{formatDateTime(s.created_at)}</span>
            <Link to={`/factor-r/runs/${s.trace_id}`} className="text-xs underline underline-offset-4">
              trace
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * Simulador de correção [REQ-013, FR-12, AC-010]: quanto a folha precisa subir até a meta, o
 * custo de INSS e IRRF do pró-labore extra e a economia de DAS. Todo valor vem do motor; a
 * simulação fica gravada no trace (é ela que autoriza um "corrigir").
 */
export function Simulator({ companyId, pa: initialPA, policyMeta }: { companyId: string; pa: string; policyMeta?: string | undefined }) {
  const create = useCreateFactorRSimulation(companyId);
  const [pa, setPA] = useState(initialPA);
  const [meta, setMeta] = useState("");
  const [share, setShare] = useState("100");
  const [inss, setInss] = useState("11");
  const [irrf, setIrrf] = useState("27,5");
  const [horizon, setHorizon] = useState("12");
  const [errors, setErrors] = useState<Partial<Record<FieldName, string>>>({});
  const [result, setResult] = useState<FactorRSimulation | null>(null);

  function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    const found: Partial<Record<FieldName, string>> = {};
    const percent = (field: FieldName, value: string) => {
      const rate = parsePercentInput(value);
      if (rate === null) found[field] = "Use um percentual de 0 a 100.";
      return rate;
    };
    const metaRate = meta.trim() === "" ? null : percent("meta", meta);
    const body = {
      pa,
      meta: metaRate,
      fracao_pro_labore: percent("fracao_pro_labore", share),
      inss: percent("inss", inss),
      irrf: percent("irrf", irrf),
      horizonte_meses: Number.parseInt(horizon, 10),
    };
    if (!isCompetenceKey(pa)) found.pa = "Informe o PA.";
    if (!/^\d{1,2}$/.test(horizon.trim()) || body.horizonte_meses < 1 || body.horizonte_meses > 60) found.horizonte_meses = "De 1 a 60 meses.";
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    create.mutate(body, {
      onSuccess: setResult,
      onError: (err) => {
        if (isApiError(err) && err.code === "VALIDATION_ERROR" && err.details.fields && typeof err.details.fields === "object") {
          const fields = err.details.fields as Record<string, unknown>;
          setErrors(Object.fromEntries(FIELDS.filter((f) => typeof fields[f] === "string").map((f) => [f, fields[f] as string])));
        }
      },
    });
  }

  const fieldError = (f: FieldName) => errors[f];
  return (
    <Card>
      <CardHeader
        title="Simulador de correção"
        description="Quanto a folha precisa subir até a meta, o custo do pró-labore extra e a economia de DAS. A simulação fica gravada no trace."
      />
      <CardBody className="grid grid-cols-1 items-start gap-4 lg:grid-cols-2">
        <form onSubmit={submit} noValidate className="grid grid-cols-2 items-start gap-3" aria-label="Parâmetros da simulação">
          <Field label="PA" error={fieldError("pa")}>
            {(aria) => <CompetenceInput {...aria} value={pa} onChange={setPA} name="sim-pa" />}
          </Field>
          <Field label="Meta (%)" error={fieldError("meta")} hint={`Vazio = meta do escritório${policyMeta ? ` (${formatRate(policyMeta)})` : ""}; mínimo 28%.`}>
            <Input name="sim-meta" inputMode="decimal" value={meta} onChange={(e) => setMeta(e.target.value)} placeholder="30" />
          </Field>
          <Field label="Reforço em pró-labore (%)" error={fieldError("fracao_pro_labore")}>
            <Input name="sim-fracao" inputMode="decimal" value={share} onChange={(e) => setShare(e.target.value)} />
          </Field>
          <Field label="Horizonte no Anexo III (meses)" error={fieldError("horizonte_meses")}>
            <Input name="sim-horizonte" inputMode="numeric" value={horizon} onChange={(e) => setHorizon(e.target.value)} />
          </Field>
          <Field label="INSS do sócio (%)" error={fieldError("inss")}>
            <Input name="sim-inss" inputMode="decimal" value={inss} onChange={(e) => setInss(e.target.value)} />
          </Field>
          <Field label="IRRF marginal (%)" error={fieldError("irrf")}>
            <Input name="sim-irrf" inputMode="decimal" value={irrf} onChange={(e) => setIrrf(e.target.value)} />
          </Field>
          <p className="col-span-2 flex items-start gap-2 text-xs text-warning" data-testid="aviso-inss">
            <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
            INSS sem teto nesta versão: o custo pode ficar superestimado para pró-labore alto.
          </p>
          <div className="col-span-2 grid gap-2">
            {create.error && !(isApiError(create.error) && create.error.code === "VALIDATION_ERROR") ? (
              <ErrorAlert error={create.error} fallback="Não foi possível simular." />
            ) : null}
            <Button type="submit" className="w-fit" disabled={create.isPending}>
              <Calculator aria-hidden /> {create.isPending ? "Simulando…" : "Simular"}
            </Button>
          </div>
        </form>
        <div className="grid gap-4">
          {result ? <Result s={result} /> : null}
          <History companyId={companyId} />
        </div>
      </CardBody>
    </Card>
  );
}
