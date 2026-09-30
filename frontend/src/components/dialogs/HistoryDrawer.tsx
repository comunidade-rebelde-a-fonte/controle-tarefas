import { ErrorAlert } from "@/components/ErrorAlert";
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDate, formatDateTime } from "@/lib/format";
import { CHANGE_SOURCE_LABELS, HISTORY_FIELD_LABELS } from "@/lib/labels";
import { type HistoryEntry, useInstanceHistory } from "@/services/obligations";

function valueText(entry: HistoryEntry, which: "old" | "new"): string {
  const value = which === "old" ? entry.old_value : entry.new_value;
  const label = which === "old" ? entry.old_label : entry.new_label;
  if (value === null) return "—";
  switch (entry.field) {
    case "STATUS":
      return label ?? value;
    case "DUE_DATE":
    case "TRANSMISSION_DATE":
      return formatDate(value) || value;
    case "APPLICABLE":
      return value === "true" ? "Aplicável" : "Não aplicável";
    default:
      return value;
  }
}

/** Histórico de alterações da obrigação, mais recente primeiro [PRD §23]. */
export function HistoryDrawer({
  instanceId,
  title,
  open,
  onOpenChange,
}: {
  instanceId: string | null;
  title: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const history = useInstanceHistory(instanceId, open);
  const items = history.data?.items ?? [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-0 right-0 left-auto h-screen max-h-screen w-full max-w-md translate-x-0 translate-y-0 content-start rounded-none">
        <DialogHeader title="Histórico" description={title} />
        {history.isPending ? (
          <Skeleton className="h-24" />
        ) : history.isError ? (
          <ErrorAlert error={history.error} />
        ) : items.length === 0 ? (
          <p className="text-sm text-muted-foreground">Nenhuma alteração registrada.</p>
        ) : (
          <ol className="grid gap-0" aria-label="Alterações">
            {items.map((e) => (
              <li key={e.id} className="grid gap-0.5 border-b py-2.5 text-sm last:border-b-0">
                <div className="flex justify-between gap-2 text-xs text-muted-foreground">
                  <span className="tabular-nums">{formatDateTime(e.created_at)}</span>
                  <span>
                    {e.user.name} · {CHANGE_SOURCE_LABELS[e.change_source]}
                  </span>
                </div>
                <div>
                  <span className="font-medium">{HISTORY_FIELD_LABELS[e.field]}:</span> {valueText(e, "old")} → {valueText(e, "new")}
                </div>
                {e.override_reason ? <div className="text-xs text-muted-foreground">Motivo: {e.override_reason}</div> : null}
              </li>
            ))}
          </ol>
        )}
      </DialogContent>
    </Dialog>
  );
}
