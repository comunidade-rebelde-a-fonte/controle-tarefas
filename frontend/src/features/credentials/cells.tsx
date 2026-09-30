import { CalendarX, CircleAlert, CircleCheck, Clock, FileText } from "lucide-react";
import { type KeyboardEvent, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/input";
import {
  controlOf,
  CREDENTIAL_TYPE_LABELS,
  type CredentialRow,
  type CredentialSituation,
  type CredentialType,
  RENEWAL_LABELS,
  type RenewalStatus,
} from "@/services/credentials";
import { editId, useCredentialsActions } from "./context";
import { useSaveCredential } from "./useSaveCredential";

/** Esc devolve o foco à célula da grade. */
function backToGrid(e: KeyboardEvent<HTMLElement>) {
  e.currentTarget.closest<HTMLElement>("[data-cell]")?.focus();
}

const SITUATION_VISUAL: Record<CredentialSituation["code"], { variant: "danger" | "warning" | "success" | "muted"; Icon: typeof Clock }> = {
  EXPIRED: { variant: "danger", Icon: CircleAlert },
  DUE_SOON: { variant: "warning", Icon: Clock },
  CURRENT: { variant: "success", Icon: CircleCheck },
  NO_DATE: { variant: "muted", Icon: CalendarX },
};

/** Situação de validade com texto e ícone (nunca só cor) [PRD §45.4]. */
export function SituationBadge({ situation }: { situation: CredentialSituation }) {
  const { variant, Icon } = SITUATION_VISUAL[situation.code];
  const days = situation.days_left;
  const title = days === null ? undefined : days < 0 ? `Venceu há ${String(-days)} dia(s)` : `Faltam ${String(days)} dia(s)`;
  return (
    <Badge variant={variant} title={title}>
      <Icon aria-hidden="true" />
      {situation.label}
    </Badge>
  );
}

export function SituationCell({ row, type }: { row: CredentialRow; type: CredentialType }) {
  return <SituationBadge situation={controlOf(row, type).situation} />;
}

/** Vencimento editável na célula; grava ao sair do campo ou com Enter. */
export function ExpiresCell({ row, type, columnId }: { row: CredentialRow; type: CredentialType; columnId: string }) {
  const ctl = controlOf(row, type);
  const { save, isPending } = useSaveCredential();
  const [draft, setDraft] = useState<string | null>(null);
  const commit = () => {
    if (draft === null) return;
    const next = draft || null;
    setDraft(null);
    if (next !== ctl.expires_at) save(row, type, { expires_at: next });
  };
  return (
    <input
      type="date"
      data-edit={editId(row.company.id, columnId)}
      tabIndex={-1}
      aria-label={`Vencimento — ${CREDENTIAL_TYPE_LABELS[type]} — ${row.company.legal_name}`}
      className="h-7 w-full rounded border border-transparent bg-transparent px-1 text-xs hover:border-input focus:border-input focus:outline-none"
      value={draft ?? ctl.expires_at ?? ""}
      disabled={isPending}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") commit();
        if (e.key === "Escape") {
          setDraft(null);
          backToGrid(e);
        }
      }}
    />
  );
}

const STAGES: RenewalStatus[] = ["CURRENT", "RENEWAL_REQUESTED", "AWAITING_CLIENT", "RENEWED"];

/** Etapa de renovação; "Renovado" abre o diálogo que pede a nova validade [PRD §19.4]. */
export function RenewalCell({ row, type, columnId }: { row: CredentialRow; type: CredentialType; columnId: string }) {
  const ctl = controlOf(row, type);
  const { save, isPending } = useSaveCredential();
  const { openRenewed } = useCredentialsActions();
  // Valor escolhido enquanto grava (a grade só é recarregada depois da resposta).
  const [pending, setPending] = useState<RenewalStatus | null>(null);
  return (
    <NativeSelect
      data-edit={editId(row.company.id, columnId)}
      tabIndex={-1}
      aria-label={`Etapa de renovação — ${CREDENTIAL_TYPE_LABELS[type]} — ${row.company.legal_name}`}
      className="h-7 w-full text-xs"
      value={pending ?? ctl.renewal_status}
      disabled={isPending}
      onChange={(e) => {
        const next = e.target.value as RenewalStatus;
        if (next === "RENEWED") openRenewed(row, type);
        else if (next !== ctl.renewal_status) {
          setPending(next);
          save(row, type, { renewal_status: next }, { onSettled: () => setPending(null) });
        }
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") backToGrid(e);
      }}
    >
      {STAGES.map((s) => (
        <option key={s} value={s}>
          {RENEWAL_LABELS[s]}
        </option>
      ))}
    </NativeSelect>
  );
}

export function NotesCell({ row }: { row: CredentialRow }) {
  const parts = [
    row.rfb_power_of_attorney.notes ? `Procuração: ${row.rfb_power_of_attorney.notes}` : null,
    row.digital_certificate.notes ? `Certificado: ${row.digital_certificate.notes}` : null,
  ].filter(Boolean);
  const text = parts.join(" · ");
  return (
    <span className="truncate text-xs text-muted-foreground" title={text || undefined}>
      {text || "—"}
    </span>
  );
}

export function DocumentsCell({ row, columnId }: { row: CredentialRow; columnId: string }) {
  const { openDocuments } = useCredentialsActions();
  const count = [row.rfb_power_of_attorney.file, row.digital_certificate.file].filter(Boolean).length;
  return (
    <Button
      variant="ghost"
      size="sm"
      data-edit={editId(row.company.id, columnId)}
      tabIndex={-1}
      aria-label={`Documentos e observações — ${row.company.legal_name} (${String(count)} de 2 documentos)`}
      onClick={() => openDocuments(row)}
      onKeyDown={(e) => {
        if (e.key === "Escape") backToGrid(e);
      }}
    >
      <FileText aria-hidden="true" />
      {count}/2
    </Button>
  );
}
