import { useQueryClient } from "@tanstack/react-query";
import { Lock, LockOpen } from "lucide-react";
import { useState } from "react";
import { CloseModuleDialog } from "@/components/dialogs/CloseModuleDialog";
import { type PeriodTarget, ReopenDialog } from "@/components/dialogs/ReopenDialog";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDateTime } from "@/lib/format";
import type { Company } from "@/services/companies";
import { reconciliationKeys, useReconciliationClosure } from "@/services/reconciliation";

const EVENT_TEXT: Record<string, string> = { OPENED: "Aberto", CLOSED: "Fechado", REOPENED: "Reaberto" };

/**
 * O fechamento do Contábil na aba Conciliação (TASK-023, D-037): a única trava da Conciliação
 * (ADR-003). Mostra o estado e o histórico, e fecha e reabre com os mesmos diálogos e endpoints da
 * grade do Contábil. Usando a Conciliação, o Contábil só fecha com a obrigação Conciliação final.
 */
export function AccountingClosure({ company, competence }: { company: Company; competence: string }) {
  const qc = useQueryClient();
  const closure = useReconciliationClosure(company.id, competence);
  const [closing, setClosing] = useState<PeriodTarget | null>(null);
  const [reopening, setReopening] = useState<PeriodTarget | null>(null);
  const c = closure.data;
  const p = c?.period ?? null;
  const target: PeriodTarget | null = p
    ? { periodId: p.id, version: p.version, companyName: company.legal_name, moduleLabel: "Contábil", competence }
    : null;
  const refresh = () => void qc.invalidateQueries({ queryKey: reconciliationKeys.all });
  const closed = p?.state === "CLOSED";
  return (
    <Card aria-label="Fechamento do Contábil">
      <CardHeader
        title="Fechamento do Contábil"
        description="A única trava da Conciliação: fechado, nada muda na conciliação da competência até a reabertura com motivo."
        actions={
          p ? (
            closed ? (
              <Badge variant="danger">
                <Lock aria-hidden /> Fechado
              </Badge>
            ) : (
              <Badge variant="success">
                <LockOpen aria-hidden /> Aberto
              </Badge>
            )
          ) : null
        }
      />
      <CardBody className="grid gap-3 text-sm">
        <ErrorAlert error={closure.error} fallback="Não foi possível carregar o fechamento do Contábil." />
        {closure.isPending ? <Skeleton className="h-16" /> : null}
        {c && !p ? <p className="text-muted-foreground">O Contábil não tem período gerado para a empresa nesta competência.</p> : null}
        {p && target ? (
          <>
            {closed ? (
              <p>
                Fechado por {p.closed_by?.name ?? "—"} em {formatDateTime(p.closed_at)}.
              </p>
            ) : c?.requires_reconciliation ? (
              <p className="text-xs text-muted-foreground">
                A empresa usa a Conciliação nesta competência: o Contábil só fecha com a obrigação Conciliação concluída.
              </p>
            ) : null}
            {closed ? (
              <Button size="sm" variant="outline" className="w-fit" onClick={() => setReopening(target)}>
                <LockOpen aria-hidden /> Reabrir o Contábil
              </Button>
            ) : (
              <Button size="sm" variant="outline" className="w-fit" onClick={() => setClosing(target)}>
                <Lock aria-hidden /> Fechar o Contábil
              </Button>
            )}
            {c && c.events.length > 0 ? (
              <details className="text-xs">
                <summary className="inline-flex min-h-6 cursor-pointer items-center underline underline-offset-4">Histórico do fechamento</summary>
                <ul className="mt-1 grid gap-1" aria-label="Histórico do fechamento do Contábil">
                  {c.events.map((ev) => (
                    <li key={ev.id}>
                      {EVENT_TEXT[ev.event_type] ?? ev.event_type} por {ev.user.name} em {formatDateTime(ev.created_at)}
                      {ev.reason ? `: “${ev.reason}”` : ""}
                    </li>
                  ))}
                </ul>
              </details>
            ) : null}
          </>
        ) : null}
        <CloseModuleDialog target={closing} onClose={() => setClosing(null)} onClosed={refresh} />
        <ReopenDialog target={reopening} onClose={() => setReopening(null)} onReopened={refresh} />
      </CardBody>
    </Card>
  );
}
