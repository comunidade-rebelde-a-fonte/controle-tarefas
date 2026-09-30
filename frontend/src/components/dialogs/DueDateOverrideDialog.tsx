import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { formatDate } from "@/lib/format";
import { type ObligationInstance, type StatusChangeResult, useUpdateDetails } from "@/services/obligations";
import { ReasonField } from "./ReasonField";

/** Altera o vencimento efetivo da obrigação; o motivo é obrigatório e vai para o histórico [PRD §21.3]. */
export function DueDateOverrideDialog({
  instance,
  label,
  onClose,
  onSaved,
}: {
  instance: ObligationInstance | null;
  /** Contexto exibido (ex.: "ICMS — Alfa Comércio"). */
  label?: string;
  onClose: () => void;
  onSaved?: (result: StatusChangeResult) => void;
}) {
  return (
    <Dialog open={!!instance} onOpenChange={(open) => !open && onClose()}>
      <DialogContent size="sm">{instance ? <DueDateForm instance={instance} label={label} onClose={onClose} onSaved={onSaved} /> : null}</DialogContent>
    </Dialog>
  );
}

function DueDateForm({
  instance,
  label,
  onClose,
  onSaved,
}: {
  instance: ObligationInstance;
  label?: string | undefined;
  onClose: () => void;
  onSaved?: ((r: StatusChangeResult) => void) | undefined;
}) {
  const toast = useToast();
  const update = useUpdateDetails();
  const [date, setDate] = useState(instance.due_date_effective ?? "");
  const [reason, setReason] = useState("");
  const [errors, setErrors] = useState<{ date?: string; reason?: string }>({});

  const submit = () => {
    const text = reason.trim();
    const next: typeof errors = {};
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) next.date = "Informe a nova data de vencimento.";
    if (!text) next.reason = "Informe o motivo da alteração do vencimento.";
    setErrors(next);
    if (next.date || next.reason) return;
    update.mutate(
      { id: instance.id, version: instance.version, due_date_effective: date, due_date_override_reason: text },
      {
        onSuccess: (res) => {
          toast({ title: "Vencimento alterado", description: `Novo vencimento: ${formatDate(date)}.`, variant: "success" });
          onSaved?.(res);
          onClose();
        },
      },
    );
  };

  return (
    <>
      <DialogHeader
        title="Alterar vencimento"
        description={`${label ?? instance.obligation_type.name}. Vencimento calculado: ${formatDate(instance.due_date_original) || "—"}.`}
      />
      <Field label="Novo vencimento" required error={errors.date}>
        <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
      </Field>
      <ReasonField label="Motivo da alteração" value={reason} onChange={setReason} error={errors.reason} />
      <ErrorAlert error={update.error} />
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Cancelar
        </Button>
        <Button onClick={submit} disabled={update.isPending}>
          {update.isPending ? "Salvando…" : "Salvar vencimento"}
        </Button>
      </DialogFooter>
    </>
  );
}
