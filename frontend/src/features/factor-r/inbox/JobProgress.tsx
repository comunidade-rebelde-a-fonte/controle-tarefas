import { useQueryClient } from "@tanstack/react-query";
import { CircleAlert, Clock, LoaderCircle, RotateCw } from "lucide-react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { jobActive, useJob, useRetryJob } from "@/services/jobs";
import { inboxKeys, type PgdasDocument } from "@/services/factorRInbox";
import { jobErrorText } from "./labels";

/**
 * Andamento do job `pgdas_parse` de um documento recebido [AC-014; 11 §9]: na fila, em
 * execução, nova tentativa, fila parada (sem worker ativo) ou falha, com o reenvio do job.
 */
export function JobProgress({ document }: { document: PgdasDocument }) {
  const qc = useQueryClient();
  const job = document.job;
  const live = useJob(job?.id, document.status === "received");
  // De volta à fila, o documento volta a ser acompanhado até o fim do job.
  const retry = useRetryJob(() => void qc.invalidateQueries({ queryKey: inboxKeys.all }));
  if (document.status !== "received" || !job) return null;
  const status = live.data?.status ?? job.status;
  const workerOnline = live.data?.worker_online ?? true;
  const attempts = live.data?.attempts ?? job.attempts;
  const maxAttempts = live.data?.max_attempts ?? job.max_attempts;
  const errorCode = live.data?.last_error_code ?? job.last_error_code;

  if (status === "dead") {
    return (
      <div role="alert" className="grid gap-2 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900" data-testid="job-falhou">
        <p className="flex items-start gap-2">
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span>
            O processamento falhou depois de {attempts} {attempts === 1 ? "tentativa" : "tentativas"}: {jobErrorText(errorCode)}.
            {errorCode ? <span className="ml-1 font-mono text-xs">({errorCode})</span> : null}
          </span>
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => retry.mutate(job.id)} disabled={retry.isPending}>
            <RotateCw aria-hidden /> Tentar de novo
          </Button>
          <span className="text-xs">Se o arquivo estiver errado, rejeite o documento.</span>
        </div>
        <ErrorAlert error={retry.error} fallback="Não foi possível reenviar o job." />
      </div>
    );
  }
  if (!jobActive(status)) return null;
  if (!workerOnline) {
    return (
      <p role="status" className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" data-testid="fila-parada">
        <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
        Fila parada: nenhum worker ativo. O extrato está guardado e será lido quando o worker voltar.
      </p>
    );
  }
  return (
    <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground" data-testid="job-andamento">
      {status === "running" ? (
        <>
          <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" aria-hidden /> Lendo o extrato…
        </>
      ) : status === "retry" ? (
        <>
          <RotateCw className="size-4" aria-hidden /> Nova tentativa agendada ({attempts} de {maxAttempts}): {jobErrorText(errorCode)}.
        </>
      ) : (
        <>
          <Clock className="size-4" aria-hidden /> Na fila, aguardando o worker.
        </>
      )}
    </p>
  );
}
