import { CalendarClock, History } from "lucide-react";
import { type ReactNode, useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useToast } from "@/components/ui/toast-context";
import { formatDate } from "@/lib/format";
import { type InstanceDetails, type ObligationInstance, type StatusChangeResult, useUpdateDetails } from "@/services/obligations";

export interface CellDetailsPopoverProps {
  instance: ObligationInstance;
  /** Do catálogo (coluna): quais campos a obrigação registra. */
  allowsTransmissionDate: boolean;
  allowsProtocol: boolean;
  /** Módulo fechado: somente leitura [plan R9]. */
  locked?: boolean;
  onChanged?: (result: StatusChangeResult) => void;
  onEditDueDate?: () => void;
  onShowHistory?: () => void;
  /** Botão que abre o popover. */
  children: ReactNode;
}

/** Detalhes da célula: vencimento, transmissão e protocolo [PRD §21.3, §29]. */
export function CellDetailsPopover({ children, ...props }: CellDetailsPopoverProps) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent aria-label={`Detalhes de ${props.instance.obligation_type.name}`}>
        {open ? <DetailsBody {...props} onDone={() => setOpen(false)} /> : null}
      </PopoverContent>
    </Popover>
  );
}

function DetailsBody({
  instance,
  allowsTransmissionDate,
  allowsProtocol,
  locked = false,
  onChanged,
  onEditDueDate,
  onShowHistory,
  onDone,
}: Omit<CellDetailsPopoverProps, "children"> & { onDone: () => void }) {
  const toast = useToast();
  const update = useUpdateDetails();
  const [transmission, setTransmission] = useState(instance.transmission_date ?? "");
  const [protocol, setProtocol] = useState(instance.protocol ?? "");
  const editable = !locked && instance.applicable && (allowsTransmissionDate || allowsProtocol);

  const save = () => {
    const body: InstanceDetails = { version: instance.version };
    if (allowsTransmissionDate && transmission !== (instance.transmission_date ?? "")) body.transmission_date = transmission || null;
    if (allowsProtocol && protocol.trim() !== (instance.protocol ?? "")) body.protocol = protocol.trim() || null;
    if (body.transmission_date === undefined && body.protocol === undefined) {
      onDone();
      return;
    }
    update.mutate(
      { id: instance.id, ...body },
      {
        onSuccess: (res) => {
          toast({ title: "Detalhes salvos", description: instance.obligation_type.name, variant: "success" });
          onChanged?.(res);
          onDone();
        },
      },
    );
  };

  const effective = formatDate(instance.due_date_effective) || "—";
  return (
    <div className="grid gap-3">
      <div className="font-medium">{instance.obligation_type.name}</div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
        <dt className="text-muted-foreground">Vencimento</dt>
        <dd className="tabular-nums">
          {effective}
          {instance.due_overridden ? <span className="text-muted-foreground"> (calculado: {formatDate(instance.due_date_original) || "—"})</span> : null}
        </dd>
        {instance.due_overridden && instance.due_date_override_reason ? (
          <>
            <dt className="text-muted-foreground">Motivo</dt>
            <dd>{instance.due_date_override_reason}</dd>
          </>
        ) : null}
        {!editable && allowsTransmissionDate ? (
          <>
            <dt className="text-muted-foreground">Transmissão</dt>
            <dd className="tabular-nums">{formatDate(instance.transmission_date) || "—"}</dd>
          </>
        ) : null}
        {!editable && allowsProtocol ? (
          <>
            <dt className="text-muted-foreground">Protocolo</dt>
            <dd className="break-all">{instance.protocol ?? "—"}</dd>
          </>
        ) : null}
      </dl>
      {editable ? (
        <div className="grid gap-2">
          {allowsTransmissionDate ? (
            <Field label="Data de transmissão">
              <Input type="date" value={transmission} onChange={(e) => setTransmission(e.target.value)} />
            </Field>
          ) : null}
          {allowsProtocol ? (
            <Field label="Protocolo">
              <Input maxLength={120} value={protocol} onChange={(e) => setProtocol(e.target.value)} />
            </Field>
          ) : null}
          <ErrorAlert error={update.error} />
          <Button size="sm" onClick={save} disabled={update.isPending}>
            {update.isPending ? "Salvando…" : "Salvar"}
          </Button>
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2 border-t pt-2">
        {onEditDueDate && !locked && instance.applicable ? (
          <Button size="sm" variant="outline" onClick={() => { onDone(); onEditDueDate(); }}>
            <CalendarClock aria-hidden /> Alterar vencimento
          </Button>
        ) : null}
        {onShowHistory ? (
          <Button size="sm" variant="outline" onClick={() => { onDone(); onShowHistory(); }}>
            <History aria-hidden /> Ver histórico
          </Button>
        ) : null}
      </div>
    </div>
  );
}
