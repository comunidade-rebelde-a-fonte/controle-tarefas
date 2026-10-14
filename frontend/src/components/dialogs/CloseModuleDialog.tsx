import { TriangleAlert } from "lucide-react";
import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { displayCompetence } from "@/lib/competence";
import { CLOSING_STATUS_REQUIRED, type ModulePeriod, useClosePeriod } from "@/services/periods";
import type { PeriodTarget } from "./ReopenDialog";

/**
 * Fecha o módulo da empresa na competência [PRD §10.4, RN-006]. No Fiscal, se a coluna
 * Fechamento ainda não estiver encerrada, a API pede confirmação para atualizá-la no mesmo ato
 * (409 CLOSING_STATUS_REQUIRED) e o diálogo mostra esse segundo passo [§13.6].
 */
export function CloseModuleDialog({
  target,
  onClose,
  onClosed,
}: {
  target: PeriodTarget | null;
  onClose: () => void;
  onClosed?: (period: ModulePeriod) => void;
}) {
  return (
    <Dialog open={!!target} onOpenChange={(open) => !open && onClose()}>
      <DialogContent size="sm">{target ? <CloseForm target={target} onClose={onClose} onClosed={onClosed} /> : null}</DialogContent>
    </Dialog>
  );
}

function CloseForm({ target, onClose, onClosed }: { target: PeriodTarget; onClose: () => void; onClosed?: ((p: ModulePeriod) => void) | undefined }) {
  const toast = useToast();
  const close = useClosePeriod();
  // Mensagem da API quando o estágio de fechamento precisa ser atualizado junto.
  const [closingStage, setClosingStage] = useState<string | null>(null);

  const run = (confirmSetClosing: boolean) => {
    close.mutate(
      { id: target.periodId, version: target.version, confirm_set_closing: confirmSetClosing },
      {
        onSuccess: (p) => {
          toast({ title: `${target.moduleLabel} fechado`, description: `${target.companyName} — ${displayCompetence(target.competence)}.`, variant: "success" });
          onClosed?.(p);
          onClose();
        },
        onError: (err) => {
          if (isApiError(err) && err.code === CLOSING_STATUS_REQUIRED) setClosingStage(err.message);
        },
      },
    );
  };
  const error = isApiError(close.error) && close.error.code === CLOSING_STATUS_REQUIRED ? null : close.error;

  return (
    <>
      <DialogHeader
        title={`Fechar ${target.moduleLabel} de ${displayCompetence(target.competence)}`}
        description={`${target.companyName}. O fechamento vale só para este módulo; os demais continuam como estão. As obrigações do módulo ficam bloqueadas até a reabertura.`}
      />
      {closingStage ? (
        <p className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          {closingStage}
        </p>
      ) : null}
      <ErrorAlert error={error} />
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Cancelar
        </Button>
        {closingStage ? (
          <Button onClick={() => run(true)} disabled={close.isPending}>
            {close.isPending ? "Fechando…" : "Atualizar e fechar"}
          </Button>
        ) : (
          <Button onClick={() => run(false)} disabled={close.isPending}>
            {close.isPending ? "Fechando…" : "Fechar competência"}
          </Button>
        )}
      </DialogFooter>
    </>
  );
}
