import { ArrowDownRight, ArrowUpRight, Minus } from "lucide-react";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { displayCompetence } from "@/lib/competence";
import { formatPercent } from "@/lib/format";
import type { DashboardHistory } from "@/services/dashboard";

// Série única no laranja do Design System (orange-500, contraste ≥ 3:1 sobre o fundo escuro); a
// grade em neutral-800 e os eixos em neutral-400 (TASK-029).
const SERIES = "#f97316";
const GRID = "#262626";
const AXIS = "#a3a3a3";

/** Variação com ícone e texto (nunca só cor); `higherIsBetter` define o sentido bom. */
function Delta({ value, unit, higherIsBetter, format }: { value: number; unit: string; higherIsBetter: boolean; format: (n: number) => string }) {
  if (value === 0) {
    return (
      <span className="inline-flex items-center gap-0.5 text-xs text-muted-foreground">
        <Minus aria-hidden className="size-3" /> sem variação
      </span>
    );
  }
  const up = value > 0;
  const good = up === higherIsBetter;
  const Icon = up ? ArrowUpRight : ArrowDownRight;
  return (
    <span className={`inline-flex items-center gap-0.5 text-xs font-medium ${good ? "text-success" : "text-destructive"}`}>
      <Icon aria-hidden className="size-3" />
      {up ? "+" : "−"}
      {format(Math.abs(value))} {unit}
    </span>
  );
}

const pp = (n: number) => (n * 100).toLocaleString("pt-BR", { maximumFractionDigits: 1 });

/** Comparação com a competência anterior e evolução da conclusão [PRD §24.8]. */
export function HistoryComparison({ history }: { history: DashboardHistory }) {
  const { current, previous, delta, series } = history;
  const data = series.map((p) => ({ label: displayCompetence(p.competence), conclusao: Math.round(p.final_completion * 1000) / 10 }));
  const vs = previous ? `vs ${displayCompetence(previous.competence)}` : "";
  return (
    <Card>
      <CardHeader
        title="Comparação histórica"
        description={previous ? `${displayCompetence(current.competence)} comparada com ${displayCompetence(previous.competence)}.` : "Sem competência anterior para comparar."}
      />
      <CardBody className="grid gap-4">
        <dl className="grid grid-cols-3 gap-3">
          <div>
            <dt className="text-xs text-muted-foreground">Conclusão final</dt>
            <dd className="text-xl font-semibold tabular-nums">{formatPercent(current.final_completion)}</dd>
            {delta ? (
              <dd>
                <Delta value={delta.final_completion} unit={`p.p. ${vs}`} higherIsBetter format={pp} />
              </dd>
            ) : null}
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Atrasos</dt>
            <dd className="text-xl font-semibold tabular-nums">{current.overdue}</dd>
            {delta ? (
              <dd>
                <Delta value={delta.overdue} unit={vs} higherIsBetter={false} format={String} />
              </dd>
            ) : null}
          </div>
          <div>
            <dt className="text-xs text-muted-foreground">Pendências</dt>
            <dd className="text-xl font-semibold tabular-nums">{current.pending}</dd>
            {delta ? (
              <dd>
                <Delta value={delta.pending} unit={vs} higherIsBetter={false} format={String} />
              </dd>
            ) : null}
          </div>
        </dl>
        {data.length > 1 ? (
          <figure className="grid gap-1">
            <figcaption className="text-xs font-medium text-muted-foreground">Conclusão final por competência (%)</figcaption>
            {/* Gráfico só visual (sem foco no SVG); a tabela sr-only abaixo tem os mesmos dados. */}
            <div className="h-44" aria-hidden>
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={data} margin={{ top: 8, right: 16, bottom: 0, left: 0 }} accessibilityLayer={false}>
                  <CartesianGrid stroke={GRID} vertical={false} />
                  <XAxis dataKey="label" tick={{ fill: AXIS, fontSize: 11 }} tickLine={false} axisLine={{ stroke: GRID }} />
                  <YAxis domain={[0, 100]} tick={{ fill: AXIS, fontSize: 11 }} tickLine={false} axisLine={false} width={40} />
                  <Tooltip formatter={(v) => [`${String(v)}%`, "Conclusão final"]} cursor={{ stroke: GRID }} />
                  <Line type="monotone" dataKey="conclusao" stroke={SERIES} strokeWidth={2} dot={{ r: 4, fill: SERIES }} activeDot={{ r: 5 }} isAnimationActive={false} />
                </LineChart>
              </ResponsiveContainer>
            </div>
            <table className="sr-only">
              <caption>Conclusão final por competência</caption>
              <thead>
                <tr>
                  <th>Competência</th>
                  <th>Conclusão final</th>
                </tr>
              </thead>
              <tbody>
                {data.map((d) => (
                  <tr key={d.label}>
                    <td>{d.label}</td>
                    <td>{d.conclusao}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </figure>
        ) : null}
      </CardBody>
    </Card>
  );
}
