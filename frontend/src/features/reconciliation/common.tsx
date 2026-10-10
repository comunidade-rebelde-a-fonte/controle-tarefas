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
  RotateCw,
  type LucideIcon,
} from "lucide-react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge, type BadgeProps } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/input";
import { formatMoney } from "@/lib/decimal";
import { useCompanies } from "@/services/companies";
import { jobActive, useJob, useRetryJob } from "@/services/jobs";
import { type ReconciliationDocument, type ReconciliationDocumentStatus, reconciliationKeys } from "@/services/reconciliation";
import { DOC_STATUS, jobErrorText } from "./labels";

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

/** Empresa ativa do cadastro canônico (todas, ou a escolhida). */
export function CompanySelect({
  value,
  onChange,
  allLabel,
  ...props
}: {
  value: string;
  onChange: (id: string) => void;
  allLabel?: string;
  id?: string;
  "aria-invalid"?: boolean | undefined;
  "aria-describedby"?: string | undefined;
  "aria-label"?: string;
}) {
  const companies = useCompanies({ active: true });
  return (
    <NativeSelect value={value} onChange={(e) => onChange(e.target.value)} {...props}>
      <option value="">{allLabel ?? "Escolha a empresa"}</option>
      {(companies.data?.items ?? []).map((c) => (
        <option key={c.id} value={c.id}>
          {c.internal_code} · {c.legal_name}
        </option>
      ))}
    </NativeSelect>
  );
}

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
