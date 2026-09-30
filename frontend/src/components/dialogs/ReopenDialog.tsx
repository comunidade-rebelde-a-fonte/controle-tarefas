import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast-context";
import { displayCompetence } from "@/lib/competence";
import { type ModulePeriod, useReopenPeriod } from "@/services/periods";
import { ReasonField } from "./ReasonField";

/** Período de empresa + módulo + competência alvo de fechamento ou reabertura. */
export interface PeriodTarget {
  periodId: string;
  version: number;
  companyName: string;
  moduleLabel: string;
  competence: string;
}

/** Reabre um módulo fechado; o motivo é obrigatório e fica no histórico [PRD §10.6, CA-015]. */
export function ReopenDialog({
  target,
  onClose,
  onReopened,
}: {
  target: PeriodTarget | null;
  onClose: () => void;
  onReopened?: (period: ModulePeriod) => void;
}) {
  return (
    <Dialog open={!!target} onOpenChange={(open) => !open && onClose()}>
      <DialogContent size="sm">{target ? <ReopenForm target={target} onClose={onClose} onReopened={onReopened} /> : null}</DialogContent>
    </Dialog>
  );
}

function ReopenForm({ target, onClose, onReopened }: { target: PeriodTarget; onClose: () => void; onReopened?: ((p: ModulePeriod) => void) | undefined }) {
  const toast = useToast();
  const reopen = useReopenPeriod();
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string>();

  const submit = () => {
    const text = reason.trim();
    if (!text) {
      setError("Informe o motivo da reabertura.");
      return;
    }
    reopen.mutate(
      { id: target.periodId, version: target.version, reason: text },
      {
        onSuccess: (p) => {
          toast({ title: `${target.moduleLabel} reaberto`, description: `${target.companyName} — ${displayCompetence(target.competence)}.`, variant: "success" });
          onReopened?.(p);
          onClose();
        },
      },
    );
  };

  return (
    <>
      <DialogHeader
        title={`Reabrir ${target.moduleLabel}`}
        description={`${target.companyName} — ${displayCompetence(target.competence)}. A reabertura libera a edição das obrigações do módulo e fica registrada com o motivo.`}
      />
      <ReasonField
        label="Motivo da reabertura"
        value={reason}
        onChange={(v) => {
          setReason(v);
          setError(undefined);
        }}
        error={error}
        placeholder="Ex.: correção de obrigação transmitida com informação incorreta"
      />
      <ErrorAlert error={reopen.error} />
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Cancelar
        </Button>
        <Button onClick={submit} disabled={reopen.isPending}>
          {reopen.isPending ? "Reabrindo…" : "Reabrir"}
        </Button>
      </DialogFooter>
    </>
  );
}
