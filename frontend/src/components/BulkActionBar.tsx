import { ListChecks, TriangleAlert, X } from "lucide-react";
import { useState } from "react";
import { ReasonField } from "@/components/dialogs/ReasonField";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Label, NativeSelect } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import type { Schemas } from "@/lib/api";
import { type BulkStatusChange, type BulkStatusResult, useBulkStatus } from "@/services/obligations";

export type GridColumn = Schemas["GridColumn"];

export interface BulkActionBarProps {
  competence: string;
  /** Colunas da grade (obrigações e seus status). */
  columns: GridColumn[];
  /** Empresas selecionadas na grade. */
  selectedCompanyIds: string[];
  onClearSelection: () => void;
  onDone?: (result: BulkStatusResult) => void;
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

/**
 * Ações em massa [PRD §26.3, CA-016]: com empresas selecionadas, escolhe a obrigação e o status,
 * mostra a prévia calculada pela API (dry_run: atualizadas e puladas com motivo) e confirma.
 * A regra de cada item (período fechado, dependências, aplicabilidade) é sempre da API.
 */
export function BulkActionBar({ competence, columns, selectedCompanyIds, onClearSelection, onDone }: BulkActionBarProps) {
  const [typeId, setTypeId] = useState("");
  const [statusCode, setStatusCode] = useState("");
  const [preview, setPreview] = useState<BulkStatusResult | null>(null);
  const bulk = useBulkStatus();
  const column = columns.find((c) => c.obligation_type.id === typeId);
  const count = selectedCompanyIds.length;
  if (count === 0) return null;

  const body = (): BulkStatusChange => ({
    target: { competence, obligation_type_id: typeId, company_ids: selectedCompanyIds },
    status_code: statusCode,
  });

  const runPreview = () => {
    bulk.mutate({ ...body(), dry_run: true }, { onSuccess: setPreview });
  };

  return (
    <div role="region" aria-label="Ações em massa" className="flex flex-wrap items-end gap-3 rounded-md border bg-accent/40 px-3 py-2">
      <div className="flex items-center gap-2 self-center text-sm font-medium">
        <ListChecks aria-hidden className="size-4" />
        {plural(count, "empresa selecionada", "empresas selecionadas")}
      </div>
      <div className="grid gap-1">
        <Label htmlFor="bulk-type">Obrigação</Label>
        <NativeSelect
          id="bulk-type"
          className="w-48"
          value={typeId}
          onChange={(e) => {
            setTypeId(e.target.value);
            setStatusCode("");
          }}
        >
          <option value="">Selecione…</option>
          {columns.map((c) => (
            <option key={c.obligation_type.id} value={c.obligation_type.id}>
              {c.obligation_type.name}
            </option>
          ))}
        </NativeSelect>
      </div>
      <div className="grid gap-1">
        <Label htmlFor="bulk-status">Status</Label>
        <NativeSelect id="bulk-status" className="w-48" value={statusCode} disabled={!column} onChange={(e) => setStatusCode(e.target.value)}>
          <option value="">Selecione…</option>
          {column?.statuses.map((s) => (
            <option key={s.code} value={s.code}>
              {s.label}
            </option>
          ))}
        </NativeSelect>
      </div>
      <Button onClick={runPreview} disabled={!column || !statusCode || bulk.isPending}>
        {bulk.isPending && !preview ? "Calculando…" : "Pré-visualizar"}
      </Button>
      <Button variant="ghost" onClick={onClearSelection}>
        <X aria-hidden /> Limpar seleção
      </Button>
      {!preview ? <ErrorAlert error={bulk.error} /> : null}
      {preview && column ? (
        <ConfirmBulk
          preview={preview}
          columnName={column.obligation_type.name}
          statusLabel={column.statuses.find((s) => s.code === statusCode)?.label ?? statusCode}
          body={body()}
          onCancel={() => {
            setPreview(null);
            bulk.reset();
          }}
          onDone={(res) => {
            setPreview(null);
            setTypeId("");
            setStatusCode("");
            onClearSelection();
            onDone?.(res);
          }}
        />
      ) : null}
    </div>
  );
}

function ConfirmBulk({
  preview,
  columnName,
  statusLabel,
  body,
  onCancel,
  onDone,
}: {
  preview: BulkStatusResult;
  columnName: string;
  statusLabel: string;
  body: BulkStatusChange;
  onCancel: () => void;
  onDone: (res: BulkStatusResult) => void;
}) {
  const toast = useToast();
  const apply = useBulkStatus();
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string>();
  const soft = preview.soft_warnings > 0;

  const confirm = () => {
    const text = reason.trim();
    if (soft && !text) {
      setError("Informe o motivo da exceção.");
      return;
    }
    apply.mutate(soft ? { ...body, confirm_exception: true, exception_reason: text } : body, {
      onSuccess: (res) => {
        toast({
          title: plural(res.updated, "obrigação atualizada", "obrigações atualizadas"),
          description: res.skipped.length > 0 ? `${plural(res.skipped.length, "item pulado", "itens pulados")}; veja o motivo na prévia.` : `${columnName} → ${statusLabel}.`,
          variant: "success",
        });
        onDone(res);
      },
    });
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onCancel()}>
      <DialogContent size="md">
        <DialogHeader title="Confirmar alteração em massa" description="Cada obrigação recebe um registro no histórico, com origem Em massa." />
        <p className="text-sm font-medium">
          {plural(preview.updated, "empresa", "empresas")} → {columnName} → {statusLabel}
        </p>
        {preview.skipped.length > 0 ? (
          <div className="grid gap-1 text-sm">
            <p className="text-muted-foreground">{plural(preview.skipped.length, "empresa será pulada", "empresas serão puladas")}:</p>
            <ul aria-label="Empresas puladas" className="max-h-48 list-disc overflow-y-auto pl-5">
              {preview.skipped.map((s) => (
                <li key={s.id}>
                  <span className="font-medium">{s.company_name}</span> — {s.message}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
        {soft ? (
          <>
            <p className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning" role="alert">
              <TriangleAlert aria-hidden className="mt-0.5 size-4 shrink-0" />
              {plural(preview.soft_warnings, "obrigação tem", "obrigações têm")} pré-requisitos pendentes. Para concluir, registre o motivo da exceção.
            </p>
            <ReasonField
              label="Motivo da exceção"
              value={reason}
              onChange={(v) => {
                setReason(v);
                setError(undefined);
              }}
              error={error}
            />
          </>
        ) : null}
        <ErrorAlert error={apply.error} />
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            Cancelar
          </Button>
          <Button onClick={confirm} disabled={apply.isPending || preview.updated === 0}>
            {apply.isPending ? "Aplicando…" : "Confirmar"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
