import { CheckCheck, CircleAlert, Lock, RotateCcw } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { ConfirmDialog, Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Textarea } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { displayCompetence } from "@/lib/competence";
import { formatDateTime } from "@/lib/format";
import {
  type ReconciliationCompletion,
  type ReconciliationCompletionRecord,
  useCompleteReconciliation,
  useReopenReconciliation,
} from "@/services/reconciliation";

/** O que a conclusão fez com a obrigação Conciliação do Contábil. */
const SYNC_TEXT: Record<ReconciliationCompletionRecord["obligation_sync"], string> = {
  concluida: "a obrigação Conciliação foi para OK",
  ja_final: "a obrigação Conciliação já estava concluída e ficou como estava",
  nao_aplicavel: "a obrigação Conciliação não se aplica à empresa nesta competência",
  sem_obrigacao: "a empresa não tem a obrigação Conciliação gerada nesta competência",
};

const REOPEN_SYNC_TEXT: Record<NonNullable<ReconciliationCompletionRecord["reopen_obligation_sync"]>, string> = {
  pendente: "a obrigação voltou para Pendente",
  mantida: "a obrigação ficou como estava (foi mudada no Controle depois da conclusão, ou já era final)",
  nao_aplicavel: "a obrigação não se aplica",
  sem_obrigacao: "sem obrigação gerada",
};

const STATE_BADGE: Record<ReconciliationCompletion["state"], { label: string; variant: "muted" | "warning" | "success" }> = {
  em_andamento: { label: "Em andamento", variant: "muted" },
  pronta: { label: "Pronta para concluir", variant: "warning" },
  concluida: { label: "Concluída", variant: "success" },
};

/** Reabrir com motivo: a obrigação volta para Pendente e a competência destrava. */
function ReopenButton({ companyId, competence }: { companyId: string; competence: string }) {
  const toast = useToast();
  const reopen = useReopenReconciliation(companyId);
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <RotateCcw aria-hidden /> Reabrir a conciliação
      </Button>
      <DialogContent size="sm">
        <DialogHeader
          title={`Reabrir a conciliação de ${displayCompetence(competence)}`}
          description="A obrigação Conciliação do Contábil volta para Pendente (se ninguém a mudou depois da conclusão) e a competência destrava. Fica no histórico quem reabriu, quando e por quê."
        />
        <Field label="Motivo" required hint="Até 1.000 caracteres.">
          <Textarea rows={3} maxLength={1000} value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <ErrorAlert error={reopen.error} fallback="Não foi possível reabrir." />
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancelar
          </Button>
          <Button
            disabled={!reason.trim() || reopen.isPending}
            onClick={() =>
              reopen.mutate(
                { competence, reason: reason.trim() },
                {
                  onSuccess: () => {
                    setOpen(false);
                    setReason("");
                    toast({ title: "Conciliação reaberta", variant: "success" });
                  },
                },
              )
            }
          >
            Reabrir
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * A conclusão da conciliação na competência (TASK-022, F-008): o que ainda impede, o aviso, a
 * obrigação Conciliação do Contábil, concluir (o contador confirma; a obrigação vai para OK) e
 * reabrir com motivo (volta para Pendente). Com o Contábil fechado, nada muda aqui.
 */
export function Completion({
  companyId,
  competence,
  completion,
  accountingClosed,
}: {
  companyId: string;
  competence: string;
  completion: ReconciliationCompletion;
  accountingClosed: boolean;
}) {
  const toast = useToast();
  const complete = useCompleteReconciliation(companyId);
  const [confirm, setConfirm] = useState(false);
  const { blocking, warnings, obligation, current } = completion;
  const queue = (bucket: string) => `/reconciliation?${new URLSearchParams({ company_id: companyId, competence, bucket }).toString()}`;
  const pending: [string, string][] = [];
  if (blocking.missing_statement) pending.push(["Falta importar o extrato da competência.", `/reconciliation/documents?company_id=${companyId}`]);
  if (blocking.divergent > 0) pending.push([`${String(blocking.divergent)} linhas com divergência.`, queue("divergencia")]);
  if (blocking.without_receipt > 0) pending.push([`${String(blocking.without_receipt)} linhas sem comprovante.`, queue("extrato_sem")]);
  if (blocking.review > 0) pending.push([`${String(blocking.review)} documentos ou comprovantes em revisão.`, queue("revisao")]);
  if (blocking.classification > 0) pending.push([`${String(blocking.classification)} linhas à espera da conta.`, queue("classificacao")]);
  const badge = STATE_BADGE[completion.state];
  const past = completion.history.filter((h) => h.reopened_at);
  return (
    <Card aria-label="Conclusão da conciliação">
      <CardHeader
        title="Conclusão da conciliação"
        description="Concluída, a obrigação Conciliação do Contábil vai para OK e a competência trava até ser reaberta com motivo."
        actions={<Badge variant={badge.variant}>{badge.label}</Badge>}
      />
      <CardBody className="grid gap-3 text-sm">
        <p>
          <span className="font-medium">Obrigação Conciliação (Contábil):</span>{" "}
          {obligation
            ? obligation.applicable
              ? obligation.status_label
              : "não se aplica nesta competência"
            : "não gerada para a empresa nesta competência"}
        </p>
        {current ? (
          <>
            <p className="flex items-start gap-2" role="status">
              <CheckCheck className="mt-0.5 size-4 shrink-0 text-success" aria-hidden />
              <span>
                Concluída por {current.completed_by} em {formatDateTime(current.completed_at)}; {SYNC_TEXT[current.obligation_sync]}. As
                mudanças na conciliação desta competência ficam travadas até a reabertura.
              </span>
            </p>
            {accountingClosed ? (
              <p className="flex items-center gap-2 text-xs text-muted-foreground">
                <Lock className="size-3.5" aria-hidden /> Com o Contábil fechado, reabra o Contábil antes de reabrir a conciliação.
              </p>
            ) : (
              <ReopenButton key={competence} companyId={companyId} competence={competence} />
            )}
          </>
        ) : (
          <>
            {pending.length > 0 ? (
              <div className="grid gap-1">
                <p className="font-medium">Falta para concluir:</p>
                <ul className="grid gap-0.5 pl-1" aria-label="O que falta para concluir">
                  {pending.map(([text, to]) => (
                    <li key={text} className="flex items-start gap-2">
                      <CircleAlert className="mt-0.5 size-3.5 shrink-0 text-warning" aria-hidden />
                      <Link to={to} className="underline underline-offset-4">
                        {text}
                      </Link>
                    </li>
                  ))}
                </ul>
              </div>
            ) : (
              <p>O extrato está todo resolvido: a conciliação está pronta para concluir.</p>
            )}
            {warnings.without_statement > 0 ? (
              <p className="text-xs text-muted-foreground">
                Aviso: {warnings.without_statement} {warnings.without_statement === 1 ? "comprovante" : "comprovantes"} sem linha do extrato
                (podem ser de outra conta ou de outro mês). Não impedem a conclusão.
              </p>
            ) : null}
            {accountingClosed ? (
              <p className="flex items-center gap-2 text-xs text-muted-foreground">
                <Lock className="size-3.5" aria-hidden /> Com o Contábil fechado, a conciliação não pode ser concluída.
              </p>
            ) : (
              <Button size="sm" className="w-fit" disabled={!completion.ready || complete.isPending} onClick={() => setConfirm(true)}>
                <CheckCheck aria-hidden /> Concluir a conciliação
              </Button>
            )}
            <ErrorAlert error={complete.error} fallback="Não foi possível concluir a conciliação." />
            <ConfirmDialog
              open={confirm}
              onOpenChange={setConfirm}
              title={`Concluir a conciliação de ${displayCompetence(competence)}?`}
              description="A obrigação Conciliação do Contábil vai para OK (com origem de sistema no histórico), e a conciliação desta competência fica travada até ser reaberta com motivo."
              confirmLabel="Concluir"
              pending={complete.isPending}
              onConfirm={() =>
                complete.mutate(competence, {
                  onSuccess: () => {
                    setConfirm(false);
                    toast({ title: "Conciliação concluída", variant: "success" });
                  },
                  onError: () => setConfirm(false),
                })
              }
            />
          </>
        )}
        {past.length > 0 ? (
          <details className="text-xs">
            <summary className="inline-flex min-h-6 cursor-pointer items-center underline underline-offset-4">
              {past.length === 1 ? "1 reabertura" : `${String(past.length)} reaberturas`} no histórico
            </summary>
            <ul className="mt-1 grid gap-1">
              {past.map((h) => (
                <li key={h.id}>
                  Concluída por {h.completed_by} em {formatDateTime(h.completed_at)}; reaberta por {h.reopened_by ?? "—"} em{" "}
                  {formatDateTime(h.reopened_at)}: “{h.reopen_reason}” — {h.reopen_obligation_sync ? REOPEN_SYNC_TEXT[h.reopen_obligation_sync] : ""}.
                </li>
              ))}
            </ul>
          </details>
        ) : null}
      </CardBody>
    </Card>
  );
}
