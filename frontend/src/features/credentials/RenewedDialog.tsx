import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { formatDate } from "@/lib/format";
import { controlOf, CREDENTIAL_TYPE_LABELS, type CredentialRow, type CredentialType } from "@/services/credentials";
import { useSaveCredential } from "./useSaveCredential";

export interface RenewedTarget {
  row: CredentialRow;
  type: CredentialType;
}

/**
 * "Renovado" exige a nova data de validade; ao confirmar, o item volta a Vigente
 * [PRD §19.4, UC-05]. A validação final é da API (422 RENEWAL_DATE_REQUIRED).
 */
export function RenewedDialog({ target, onClose }: { target: RenewedTarget | null; onClose: () => void }) {
  return (
    <Dialog open={target !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent size="sm">{target && <RenewedForm key={`${target.row.company.id}:${target.type}`} target={target} onClose={onClose} />}</DialogContent>
    </Dialog>
  );
}

function RenewedForm({ target, onClose }: { target: RenewedTarget; onClose: () => void }) {
  const { row, type } = target;
  const current = controlOf(row, type).expires_at;
  const [date, setDate] = useState("");
  const [error, setError] = useState<string | undefined>();
  const { save, isPending } = useSaveCredential();
  const toast = useToast();

  const submit = () => {
    if (!date) {
      setError("Informe a nova data de validade.");
      return;
    }
    if (date === current) {
      setError("A nova data de validade deve ser diferente da atual.");
      return;
    }
    save(
      row,
      type,
      { renewal_status: "RENEWED", expires_at: date },
      {
        onDone: (c) => {
          toast({ title: "Renovação registrada", description: `Vigente até ${formatDate(c.expires_at)}.`, variant: "success" });
          onClose();
        },
        onFail: setError,
      },
    );
  };

  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <DialogHeader
        title={`Renovar ${CREDENTIAL_TYPE_LABELS[type].toLowerCase()}`}
        description={`${row.company.legal_name}. Validade atual: ${current ? formatDate(current) : "sem data"}. Ao confirmar, o item volta a Vigente.`}
      />
      <Field label="Nova data de validade" required error={error}>
        <Input
          type="date"
          value={date}
          onChange={(e) => {
            setDate(e.target.value);
            setError(undefined);
          }}
          autoFocus
        />
      </Field>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Cancelar
        </Button>
        <Button type="submit" disabled={isPending}>
          Confirmar renovação
        </Button>
      </DialogFooter>
    </form>
  );
}
