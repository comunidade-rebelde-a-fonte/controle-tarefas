import { useQueryClient } from "@tanstack/react-query";
import {
  ArrowDownLeft,
  ArrowUpRight,
  CircleAlert,
  CircleCheck,
  Clock,
  FileSearch,
  Lock,
  LoaderCircle,
  Play,
  RotateCw,
  type LucideIcon,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge, type BadgeProps } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { isApiError } from "@/lib/api";
import { isCompetenceKey } from "@/lib/competence";
import { formatMoney } from "@/lib/decimal";
import { jobActive, useJob, useRetryJob } from "@/services/jobs";
import { cn } from "@/lib/utils";
import {
  type ReconciliationDocument,
  type ReconciliationDocumentStatus,
  type ReconciliationMatching,
  type RunSummary,
  reconciliationKeys,
  useRunReconciliation,
} from "@/services/reconciliation";
import { DOC_STATUS, jobErrorText, percentText, runSummaryText } from "./labels";
import { csvLinesOf } from "./model";

/** Valor com o sentido em texto e ícone (nunca só cor): C entra, D sai. */
export function Amount({ value, direction }: { value: string | null | undefined; direction: string | null | undefined }) {
  if (!direction) return <span className="tabular-nums">{formatMoney(value)}</span>;
  const credit = direction === "C";
  const Icon = credit ? ArrowDownLeft : ArrowUpRight;
  return (
    <span className="inline-flex items-center gap-1 whitespace-nowrap tabular-nums">
      <Icon className={credit ? "size-3.5 text-emerald-700" : "size-3.5 text-red-700"} aria-hidden />
      {formatMoney(value)}
      <span className="sr-only">{credit ? "(entrada)" : "(saída)"}</span>
    </span>
  );
}

const STATUS_STYLE: Record<ReconciliationDocumentStatus, { variant: BadgeProps["variant"]; icon: LucideIcon }> = {
  pendente: { variant: "muted", icon: Clock },
  triado: { variant: "muted", icon: LoaderCircle },
  extraido: { variant: "default", icon: CircleCheck },
  importado: { variant: "success", icon: CircleCheck },
  revisao_triagem: { variant: "warning", icon: FileSearch },
  revisao_extracao: { variant: "warning", icon: FileSearch },
  duplicado: { variant: "muted", icon: CircleAlert },
  pendencia_periodo_fechado: { variant: "danger", icon: Lock },
};

/** Estado do documento com ícone e texto. */
export function DocStatusBadge({ status }: { status: ReconciliationDocumentStatus }) {
  const s = STATUS_STYLE[status];
  return (
    <Badge variant={s.variant}>
      <s.icon aria-hidden />
      {DOC_STATUS[status]}
    </Badge>
  );
}

export { CompanySelect } from "@/components/CompanySelect";

/**
 * Andamento do job que processa o documento [AC-014; 11 §9]: na fila, em execução, nova
 * tentativa, fila parada (sem worker ativo) ou falha, com o reenvio do job.
 */
export function DocumentJob({ document }: { document: ReconciliationDocument }) {
  const qc = useQueryClient();
  const job = document.job;
  const waiting = document.status === "pendente" || document.status === "triado";
  const live = useJob(job?.id, waiting);
  const retry = useRetryJob(() => void qc.invalidateQueries({ queryKey: reconciliationKeys.all }));
  if (!waiting || !job) return null;
  const status = live.data?.status ?? job.status;
  const attempts = live.data?.attempts ?? job.attempts;
  const errorCode = live.data?.last_error_code ?? job.last_error_code;
  if (status === "dead") {
    return (
      <div role="alert" className="grid gap-2 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900">
        <p className="flex items-start gap-2">
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span>
            O processamento falhou depois de {attempts} {attempts === 1 ? "tentativa" : "tentativas"}: {jobErrorText(errorCode)}.
            {errorCode ? <span className="ml-1 font-mono text-xs">({errorCode})</span> : null}
          </span>
        </p>
        <Button size="sm" variant="outline" className="w-fit" onClick={() => retry.mutate(job.id)} disabled={retry.isPending}>
          <RotateCw aria-hidden /> Tentar de novo
        </Button>
        <ErrorAlert error={retry.error} fallback="Não foi possível reenviar o job." />
      </div>
    );
  }
  if (!jobActive(status)) return null;
  if (live.data && !live.data.worker_online) {
    return (
      <p role="status" className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900">
        <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
        Fila parada: nenhum worker ativo. O documento está guardado e será processado quando o worker voltar.
      </p>
    );
  }
  return (
    <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
      {status === "running" ? (
        <>
          <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" aria-hidden />
          {document.status === "pendente" ? "Triando o documento…" : "Extraindo…"}
        </>
      ) : (
        <>
          <Clock className="size-4" aria-hidden /> Na fila, aguardando o worker.
        </>
      )}
    </p>
  );
}

/**
 * A recusa do CSV (tudo ou nada): a mensagem e o erro de cada linha; nada foi gravado. Outras
 * falhas aparecem como erro comum.
 */
export function CsvImportError({ error, fallback }: { error: unknown; fallback: string }) {
  const lines = csvLinesOf(error);
  if (lines.length === 0) return <ErrorAlert error={error} fallback={fallback} />;
  const shown = lines.slice(0, 50);
  return (
    <div role="alert" className="grid gap-1.5 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900">
      <p className="flex items-start gap-2 font-medium">
        <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
        {isApiError(error) ? error.message : fallback} Corrija as linhas e envie o arquivo de novo.
      </p>
      <ul className="grid gap-0.5 pl-6 text-xs" aria-label="Linhas com erro">
        {shown.map((l) => (
          <li key={`${String(l.line)}-${l.message}`}>
            Linha {l.line}: {l.message}
          </li>
        ))}
      </ul>
      {lines.length > shown.length ? <p className="pl-6 text-xs">E mais {lines.length - shown.length} linhas com erro.</p> : null}
    </div>
  );
}

/**
 * Roda o matching da competência da empresa e a classificação por regra e histórico (AC-018,
 * AC-020); o que sobra vai para a fila. Idempotente: rodar de novo não duplica nada.
 */
export function RunReconciliation({ companyId, competence, showQueueLink = false }: { companyId: string; competence: string; showQueueLink?: boolean }) {
  const run = useRunReconciliation();
  const [result, setResult] = useState<RunSummary | null>(null);
  const ready = !!companyId && isCompetenceKey(competence);
  const current = result && result.company_id === companyId && result.competence === competence ? result : null;
  return (
    <div className="grid gap-2">
      <Button
        size="sm"
        className="w-fit"
        disabled={!ready || run.isPending}
        onClick={() => run.mutate({ companyId, competence }, { onSuccess: setResult })}
      >
        <Play aria-hidden /> {run.isPending ? "Rodando a conciliação…" : "Rodar a conciliação da competência"}
      </Button>
      {current ? (
        <p role="status" className="max-w-3xl text-xs text-muted-foreground">
          Rodada concluída: {runSummaryText(current)}
          {showQueueLink ? (
            <>
              {" "}
              <Link
                to={`/reconciliation?company_id=${companyId}&competence=${competence}`}
                className="font-medium text-foreground underline underline-offset-4"
              >
                Abrir a fila de revisão
              </Link>
            </>
          ) : null}
        </p>
      ) : null}
      <ErrorAlert error={run.error} fallback="Não foi possível rodar a conciliação." />
    </div>
  );
}

/** Largura de barra (0 a 100) a partir do percentual da API; só para desenhar. */
function barWidth(value: string | null | undefined): number {
  const n = Number(value ?? 0);
  return Number.isFinite(n) ? Math.min(Math.max(n, 0), 100) : 0;
}

/**
 * O percentual automático contra a meta (a régua do /metricas da origem): o número, as linhas e
 * a barra com a marca da meta. A barra é só desenho; o texto diz tudo.
 */
export function AutoRate({ matching }: { matching: ReconciliationMatching }) {
  const below = matching.auto_percent !== null && barWidth(matching.auto_percent) < barWidth(matching.target_percent);
  return (
    <div className="grid gap-2">
      <p className="text-3xl font-semibold tabular-nums" data-testid="percentual-automatico">
        {percentText(matching.auto_percent)}
      </p>
      <p className="text-xs text-muted-foreground">
        {matching.auto} de {matching.lines} linhas do extrato conciliadas pelo automático · meta {percentText(matching.target_percent)}
      </p>
      <div className="relative h-2 rounded bg-muted" aria-hidden>
        <div
          className={cn("h-2 rounded", below ? "bg-amber-500" : "bg-emerald-600")}
          style={{ width: `${String(barWidth(matching.auto_percent))}%` }}
        />
        <div className="absolute -top-1 h-4 w-0.5 bg-foreground" style={{ left: `${String(barWidth(matching.target_percent))}%` }} />
      </div>
    </div>
  );
}

/** Barra da taxa de sucesso (playbooks), com o percentual em texto ao lado. */
export function RateBar({ value }: { value: string | null | undefined }) {
  if (value === null || value === undefined) return <span className="text-muted-foreground">—</span>;
  return (
    <span className="inline-flex items-center gap-2">
      <span className="inline-block h-1.5 w-20 rounded bg-muted" aria-hidden>
        <span className="block h-1.5 rounded bg-emerald-600" style={{ width: `${String(barWidth(value))}%` }} />
      </span>
      <span className="tabular-nums">{percentText(value)}</span>
    </span>
  );
}
