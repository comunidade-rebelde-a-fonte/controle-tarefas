import { useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, MessageSquarePlus } from "lucide-react";
import { type KeyboardEvent, useRef, useState } from "react";
import { Input } from "@/components/ui/input";
import { Tooltip } from "@/components/ui/tooltip";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { isVersionConflict, obligationKeys } from "@/services/obligations";
import { type ModulePeriod, useUpdateNote } from "@/services/periods";

export interface NoteCellProps {
  periodId: string;
  /** Versão do período (a observação usa o controle de versão do período). */
  version: number;
  note: string | null;
  /** Contexto para o nome acessível (ex.: "Alfa Comércio"). */
  label?: string;
  /** -1 dentro da DataGrid. */
  tabIndex?: number;
  onSaved?: (period: ModulePeriod) => void;
}

/**
 * Observação geral da empresa no módulo e competência [PRD §22, RN-016]: exibição truncada com
 * tooltip e edição inline (Enter ou sair do campo salva, Esc cancela), otimista com rollback.
 */
export function NoteCell({ periodId, version, note, label, tabIndex, onSaved }: NoteCellProps) {
  const toast = useToast();
  const qc = useQueryClient();
  const update = useUpdateNote();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [confirmed, setConfirmed] = useState<ModulePeriod | null>(null);
  const [optimistic, setOptimistic] = useState<{ note: string | null; baseVersion: number } | null>(null);
  const cancelled = useRef(false);

  const current = confirmed && confirmed.version > version ? { note: confirmed.general_note, version: confirmed.version } : { note, version };
  const pending = optimistic?.baseVersion === current.version;
  const shown = pending ? optimistic.note : current.note;
  const name = label ? `Observação de ${label}` : "Observação";

  const start = () => {
    if (update.isPending) return;
    cancelled.current = false;
    setDraft(shown ?? "");
    setEditing(true);
  };

  const save = () => {
    if (cancelled.current) return;
    setEditing(false);
    const next = draft.trim() || null;
    if (next === (shown ?? null)) return;
    const base = current;
    setOptimistic({ note: next, baseVersion: base.version });
    update.mutate(
      { id: periodId, version: base.version, general_note: next },
      {
        onSuccess: (p) => {
          setConfirmed(p);
          setOptimistic(null);
          onSaved?.(p);
        },
        onError: (err) => {
          setOptimistic(null); // rollback
          if (isVersionConflict(err)) void qc.invalidateQueries({ queryKey: obligationKeys.all });
          toast({
            title: "Não foi possível salvar a observação",
            description: isVersionConflict(err)
              ? "A linha foi alterada em outro lugar e será recarregada. Tente de novo."
              : isApiError(err)
                ? err.message
                : "Falha de comunicação com o servidor.",
            variant: "error",
          });
        },
      },
    );
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      save();
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      cancelled.current = true;
      setEditing(false);
    }
  };

  if (editing) {
    return (
      <Input
        autoFocus
        aria-label={name}
        maxLength={2000}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onKeyDown}
        onBlur={save}
        className="h-7 text-xs"
      />
    );
  }

  const button = (
    <button
      type="button"
      tabIndex={tabIndex}
      aria-label={shown ? `${name}: ${shown}` : `${name}: adicionar`}
      aria-busy={pending ? true : undefined}
      onClick={start}
      onKeyDown={(e) => {
        if (e.key === "F2") start();
      }}
      className={cn(
        "flex h-7 w-full min-w-0 items-center gap-1.5 rounded-md px-1.5 text-left text-xs outline-none hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring",
        !shown && "text-muted-foreground",
      )}
    >
      {pending ? <LoaderCircle aria-hidden className="size-3.5 shrink-0 animate-spin" /> : null}
      {shown ? (
        <span className="truncate">{shown}</span>
      ) : (
        <>
          <MessageSquarePlus aria-hidden className="size-3.5 shrink-0" />
          <span className="truncate">Adicionar</span>
        </>
      )}
    </button>
  );
  return shown ? <Tooltip content={<span className="block max-w-xs whitespace-pre-wrap">{shown}</span>}>{button}</Tooltip> : button;
}
