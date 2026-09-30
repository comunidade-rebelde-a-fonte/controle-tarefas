import { OctagonX, TriangleAlert } from "lucide-react";
import { useState } from "react";
import type { DependencyErrorEvent } from "@/components/StatusCell";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast-context";
import { pendingPrerequisites, StatusErrorCode, type StatusChangeResult, useChangeStatus } from "@/services/obligations";
import { ReasonField } from "./ReasonField";

/**
 * Pré-requisitos pendentes ao concluir uma obrigação [PRD §16.4, §18.2]:
 *   - HARD (422 DEPENDENCY_BLOCKED, Contábil): informa as etapas anteriores; não há exceção;
 *   - SOFT (409 DEPENDENCY_WARNING, DCTFWeb): alerta forte e permite concluir com exceção,
 *     com motivo obrigatório que fica no histórico [RN-012, CA-011].
 */
export function DependencyWarningDialog({
  event,
  onClose,
  onResolved,
}: {
  event: DependencyErrorEvent | null;
  onClose: () => void;
  onResolved?: (result: StatusChangeResult) => void;
}) {
  return (
    <Dialog open={!!event} onOpenChange={(open) => !open && onClose()}>
      <DialogContent size="sm">{event ? <DependencyBody event={event} onClose={onClose} onResolved={onResolved} /> : null}</DialogContent>
    </Dialog>
  );
}

function DependencyBody({
  event,
  onClose,
  onResolved,
}: {
  event: DependencyErrorEvent;
  onClose: () => void;
  onResolved?: ((r: StatusChangeResult) => void) | undefined;
}) {
  const toast = useToast();
  const change = useChangeStatus();
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string>();
  const soft = event.error.code === StatusErrorCode.DependencyWarning;
  const pending = pendingPrerequisites(event.error);
  const typeName = event.instance.obligation_type.name;

  const confirm = () => {
    const text = reason.trim();
    if (!text) {
      setError("Informe o motivo da exceção.");
      return;
    }
    change.mutate(
      { id: event.instance.id, status_code: event.statusCode, version: event.instance.version, confirm_exception: true, exception_reason: text },
      {
        onSuccess: (res) => {
          toast({ title: `${typeName} concluída com exceção`, description: "O motivo foi registrado no histórico da obrigação.", variant: "success" });
          onResolved?.(res);
          onClose();
        },
      },
    );
  };

  const Icon = soft ? TriangleAlert : OctagonX;
  return (
    <>
      <DialogHeader
        title={soft ? "Pré-requisitos pendentes" : "Etapa anterior pendente"}
        description={soft ? `Concluir ${typeName} mesmo com pré-requisitos pendentes exige confirmação e motivo.` : `${typeName} não pode ser concluída fora de ordem.`}
      />
      <div
        className={soft ? "rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950" : "rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900"}
        role="alert"
      >
        <p className="flex items-start gap-2 font-medium">
          <Icon className="mt-0.5 size-4 shrink-0" aria-hidden />
          {event.error.message}
        </p>
        {pending.length > 0 ? (
          <ul className="mt-2 list-disc pl-10" aria-label="Pré-requisitos pendentes">
            {pending.map((p) => (
              <li key={p.code}>{p.name}</li>
            ))}
          </ul>
        ) : null}
      </div>
      {soft ? (
        <ReasonField
          label="Motivo da exceção"
          value={reason}
          onChange={(v) => {
            setReason(v);
            setError(undefined);
          }}
          error={error}
          placeholder="Ex.: eSocial transmitido pelo cliente em outro sistema"
        />
      ) : null}
      <ErrorAlert error={change.error} />
      <DialogFooter>
        {soft ? (
          <>
            <Button variant="outline" onClick={onClose}>
              Cancelar
            </Button>
            <Button onClick={confirm} disabled={change.isPending}>
              {change.isPending ? "Salvando…" : "Concluir com exceção"}
            </Button>
          </>
        ) : (
          <Button onClick={onClose}>Entendi</Button>
        )}
      </DialogFooter>
    </>
  );
}
