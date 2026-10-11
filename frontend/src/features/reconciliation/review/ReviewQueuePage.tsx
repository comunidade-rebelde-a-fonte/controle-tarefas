import { ClipboardCheck, Keyboard, X } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useSearchParams } from "react-router";
import { CompetenceInput } from "@/components/CompetenceInput";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { FilterToggle } from "@/features/factor-r/common";
import { cn } from "@/lib/utils";
import { formatDate } from "@/lib/format";
import { type ReviewBucket, type ReviewQueueItem, useRecentReconciliations, useReviewEntry, useReviewLine, useReviewQueue } from "@/services/reconciliation";
import { Amount, CompanySelect, RunReconciliation } from "../common";
import { BUCKETS, bucketText, methodText, reasonText } from "../labels";
import { OriginalViewer } from "../OriginalViewer";
import { entryHighlights } from "./model";
import { DocumentPanel, EntryPanel, LinePanel } from "./panels";
import { UndoButton } from "./sections";

const KINDS = ["linha", "comprovante", "documento"] as const;
type Kind = (typeof KINDS)[number];

function isBucket(v: string | null): v is ReviewBucket {
  return BUCKETS.some(([b]) => b === v);
}

function isKind(v: string | null): v is Kind {
  return KINDS.some((k) => k === v);
}

/** O original do item à esquerda: o extrato da linha, o comprovante ou o próprio documento. */
function ItemOriginal({ kind, id }: { kind: Kind; id: string }) {
  const line = useReviewLine(kind === "linha" ? id : null);
  const entry = useReviewEntry(kind === "comprovante" ? id : null);
  if (kind === "documento") return <OriginalViewer documentId={id} />;
  if (kind === "linha") return line.data ? <OriginalViewer documentId={line.data.line.statement_document_id} /> : <Skeleton className="h-96" />;
  return entry.data ? (
    <OriginalViewer documentId={entry.data.entry.document_id} highlights={entryHighlights(entry.data.entry)} />
  ) : (
    <Skeleton className="h-96" />
  );
}

/** O item escolhido: o original à esquerda e os dados, os candidatos e as ações à direita (08 §7). */
function ReviewPanel({ kind, id, onClose, onOpen }: { kind: Kind; id: string; onClose: () => void; onOpen: (kind: Kind, id: string) => void }) {
  return (
    <section aria-label="Item em revisão" className="grid gap-3 rounded-lg border bg-card p-4">
      <div className="flex items-center gap-2">
        <h2 className="text-sm font-semibold">{{ linha: "Linha do extrato", comprovante: "Comprovante", documento: "Documento em revisão" }[kind]}</h2>
        <Button variant="ghost" size="sm" className="ml-auto" onClick={onClose} aria-label="Fechar o item">
          <X aria-hidden />
        </Button>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <ItemOriginal kind={kind} id={id} />
        {kind === "linha" ? <LinePanel key={id} id={id} /> : null}
        {kind === "comprovante" ? <EntryPanel key={id} id={id} onOpenLine={(line) => onOpen("linha", line)} /> : null}
        {kind === "documento" ? <DocumentPanel key={id} id={id} /> : null}
      </div>
    </section>
  );
}

/** As últimas conciliações, para desfazer (a lista "recentes" da origem). */
function Recent({ companyId }: { companyId: string | undefined }) {
  const recent = useRecentReconciliations(companyId);
  const items = recent.data?.items ?? [];
  if (items.length === 0) return null;
  return (
    <section aria-label="Conciliações recentes" className="grid gap-2">
      <h2 className="text-sm font-semibold">Conciliações recentes</h2>
      <ul className="grid gap-1.5">
        {items.map((r) => (
          <li key={r.id} className="flex flex-wrap items-center gap-2 rounded-md border bg-card px-3 py-1.5 text-sm">
            <span className="text-xs text-muted-foreground">{r.company_code}</span>
            {formatDate(r.date)} <Amount value={r.amount} direction={r.direction} />
            <span className="max-w-80 truncate">{r.description}</span>
            <Badge variant={r.origin === "auto" ? "muted" : "default"}>{methodText(r.method)}</Badge>
            <span className="ml-auto">
              <UndoButton reconciliationId={r.id} />
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function isTyping(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName));
}

/**
 * Fila de revisão da Conciliação — a tela prioritária (08 §7): os baldes da origem com a
 * contagem, filtros por empresa e competência, o item com o original ao lado e as ações. Teclado:
 * J e K passam de item, Esc fecha.
 */
export function ReviewQueuePage() {
  const [params, setParams] = useSearchParams();
  const companyId = params.get("company_id") ?? "";
  const competence = params.get("competence") ?? "";
  const rawBucket = params.get("bucket");
  const bucket = isBucket(rawBucket) ? rawBucket : undefined;
  const item = params.get("item");
  const rawKind = params.get("kind");
  const kind = isKind(rawKind) ? rawKind : null;
  const queue = useReviewQueue({ companyId: companyId || undefined, competence: competence || undefined, bucket });
  const items = useMemo(() => queue.data?.items ?? [], [queue.data]);
  const counts = queue.data?.counts ?? {};

  const update = (patch: Record<string, string | null>) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        for (const [k, v] of Object.entries(patch)) {
          if (v) next.set(k, v);
          else next.delete(k);
        }
        return next;
      },
      { replace: true },
    );
  const open = (k: Kind, id: string) => update({ item: id, kind: k });

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (isTyping(e.target) || e.ctrlKey || e.metaKey || e.altKey) return;
      if (e.key === "Escape" && item) {
        update({ item: null, kind: null });
        return;
      }
      if (e.key !== "j" && e.key !== "k") return;
      if (items.length === 0) return;
      const at = items.findIndex((i) => i.id === item);
      const next = e.key === "j" ? Math.min(at + 1, items.length - 1) : Math.max(at - 1, 0);
      const it = items[at === -1 ? 0 : next];
      if (it) open(it.kind, it.id);
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  return (
    <>
      <PageHeader
        title="Fila de revisão"
        description="O que o automático não resolveu: documentos e comprovantes a revisar, divergências, linhas sem comprovante, comprovantes sem extrato e classificações pendentes."
      />
      <div className="grid gap-4 p-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Empresa
            <CompanySelect value={companyId} onChange={(id) => update({ company_id: id || null, item: null, kind: null })} allLabel="Todas as empresas" />
          </label>
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Competência
            <CompetenceInput value={competence} onChange={(v) => update({ competence: v || null, item: null, kind: null })} />
          </label>
          <p className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground">
            <Keyboard className="size-3.5" aria-hidden /> J e K passam de item; Esc fecha.
          </p>
        </div>
        {companyId && competence ? <RunReconciliation companyId={companyId} competence={competence} /> : null}
        <div role="group" aria-label="Filtrar por balde" className="flex flex-wrap gap-2">
          <FilterToggle active={!bucket} onClick={() => update({ bucket: null })}>
            Todos ({counts.total ?? 0})
          </FilterToggle>
          {BUCKETS.map(([b, label]) => (
            <FilterToggle key={b} active={bucket === b} onClick={() => update({ bucket: b })}>
              {label} ({counts[b] ?? 0})
            </FilterToggle>
          ))}
        </div>
        {item && kind ? <ReviewPanel kind={kind} id={item} onClose={() => update({ item: null, kind: null })} onOpen={open} /> : null}
        <ErrorAlert error={queue.error} fallback="Não foi possível carregar a fila." />
        {queue.isPending ? <Skeleton className="h-64" /> : null}
        {queue.data && items.length === 0 ? (
          <EmptyState icon={ClipboardCheck} title="Nada a revisar aqui" description="Com os filtros escolhidos, a fila está vazia." />
        ) : null}
        {items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Itens da fila">
              <thead>
                <tr>
                  <Th scope="col">Data</Th>
                  <Th scope="col">Empresa</Th>
                  <Th scope="col">Descrição</Th>
                  <Th scope="col" className="text-right">
                    Valor
                  </Th>
                  <Th scope="col">Balde</Th>
                  <Th scope="col">Motivo</Th>
                </tr>
              </thead>
              <tbody>
                {items.map((it: ReviewQueueItem) => (
                  <tr key={`${it.kind}-${it.id}`} className={cn(it.id === item && "bg-primary/5")} data-item={it.id}>
                    <Td className="whitespace-nowrap tabular-nums">{it.date ? formatDate(it.date) : "—"}</Td>
                    <Td className="text-xs whitespace-nowrap">{it.company_code ?? "—"}</Td>
                    <Td className="max-w-96">
                      <button
                        type="button"
                        className="min-h-6 truncate text-left font-medium hover:underline"
                        onClick={() => open(it.kind, it.id)}
                        aria-current={it.id === item ? "true" : undefined}
                      >
                        {it.summary || "—"}
                      </button>
                    </Td>
                    <Td className="text-right">{it.amount ? <Amount value={it.amount} direction={it.direction} /> : "—"}</Td>
                    <Td className="text-xs whitespace-nowrap">{bucketText(it.bucket)}</Td>
                    <Td className="max-w-80 text-xs text-muted-foreground">{reasonText(it.reason)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        ) : null}
        <Recent companyId={companyId || undefined} />
      </div>
    </>
  );
}
