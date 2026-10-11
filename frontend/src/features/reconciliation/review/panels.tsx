import { CheckCheck, FileQuestion, Link2, ReceiptText, RotateCw } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast-context";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate } from "@/lib/decimal";
import { formatDate } from "@/lib/format";
import {
  type ClientEntry,
  type DocumentIdentify,
  type EntryFieldsPatch,
  type ReconciliationDocumentDetail,
  useConfirmNoReceipt,
  useCorrectEntry,
  useIdentifyDocument,
  useManualMatch,
  useReconciliationDocument,
  useRetryDocument,
  useReviewEntry,
  useReviewLine,
} from "@/services/reconciliation";
import { Amount, CompanySelect, DocStatusBadge } from "../common";
import { ENTRY_STATUS, methodText, reasonText } from "../labels";
import { canRetry, type FieldOrigin, fieldErrorsOf } from "./model";
import { ClassificationSection, ResolverSection, UndoButton } from "./sections";

interface Mark {
  tipo?: string;
  motivo?: string;
  justificativa?: string;
}

/** As marcas da linha em texto (divergência, borda, desfeita). */
function Marks({ marks }: { marks: unknown[] }) {
  const list = marks.filter((m): m is Mark => typeof m === "object" && m !== null && "tipo" in m);
  if (list.length === 0) return null;
  return (
    <ul className="grid gap-1.5" aria-label="Marcas da linha">
      {list.map((m, i) => (
        <li key={i} className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">
          {reasonText(m.motivo ?? m.tipo)}
          {m.justificativa ? <span className="block text-xs">Motivo do desfazer: {m.justificativa}</span> : null}
        </li>
      ))}
    </ul>
  );
}

/** A linha do extrato na fila: a conciliação (com desfazer e classificação) ou os candidatos. */
export function LinePanel({ id }: { id: string }) {
  const toast = useToast();
  const detail = useReviewLine(id);
  const match = useManualMatch();
  const noReceipt = useConfirmNoReceipt();
  const [chosen, setChosen] = useState<string[]>([]);
  const [confirm, setConfirm] = useState(false);
  if (detail.isPending) return <Skeleton className="h-64" />;
  if (detail.isError) return <ErrorAlert error={detail.error} fallback="Não foi possível abrir a linha." />;
  const { line, reconciliation: rec, candidates } = detail.data;
  const toggle = (entry: string) => setChosen((c) => (c.includes(entry) ? c.filter((x) => x !== entry) : [...c, entry]));
  return (
    <div className="grid content-start gap-4">
      <div className="grid gap-1">
        <p className="text-xs text-muted-foreground">
          {line.company_code} · {line.company_name} · {line.bank_account} · {displayCompetence(line.competence)}
        </p>
        <p className="flex flex-wrap items-center gap-2 text-base font-medium">
          {formatDate(line.date)} <Amount value={line.amount} direction={line.direction} />
        </p>
        <p className="text-sm">{line.description}</p>
      </div>
      <Marks marks={line.marks} />
      {rec ? (
        <>
          <section aria-label="Conciliação" className="grid gap-2 rounded-md border p-3 text-sm">
            <p className="flex flex-wrap items-center gap-2">
              <Badge variant="success">
                <CheckCheck aria-hidden /> Conciliada
              </Badge>
              {methodText(rec.method)}
              <span className="ml-auto">
                <UndoButton reconciliationId={rec.id} />
              </span>
            </p>
            {rec.entries.length > 0 ? (
              <ul className="grid gap-1">
                {rec.entries.map((e) => (
                  <li key={e.id} className="flex flex-wrap items-center gap-2">
                    {formatDate(e.date)} <Amount value={e.amount} direction={e.direction} /> {e.counterparty ?? "—"}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-muted-foreground">Sem comprovante.</p>
            )}
          </section>
          <ClassificationSection
            lineId={line.id}
            companyId={line.company_id}
            accountCode={line.account_code}
            origin={line.classification_origin}
            pending={line.classification_pending}
          />
        </>
      ) : (
        <>
          <section aria-label="Candidatos" className="grid gap-2">
            <h3 className="text-sm font-medium">Comprovantes candidatos</h3>
            {candidates.length === 0 ? (
              <p className="text-sm text-muted-foreground">Nenhum comprovante livre de mesmo valor e sentido em até 3 dias.</p>
            ) : (
              <ul className="grid gap-1.5">
                {candidates.map((c) => (
                  <li key={c.id}>
                    <label className="flex cursor-pointer flex-wrap items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm has-checked:border-primary has-checked:bg-primary/5">
                      <input type="checkbox" checked={chosen.includes(c.id)} onChange={() => toggle(c.id)} className="size-4 accent-primary" />
                      {formatDate(c.date)} <Amount value={c.amount} direction={c.direction} />
                      <span className="font-medium">{c.counterparty ?? "—"}</span>
                      <span className="text-xs text-muted-foreground">
                        {c.days_apart === null ? "sem data" : `${c.days_apart} ${c.days_apart === 1 ? "dia" : "dias"}`}
                      </span>
                      {c.annotated ? <Badge variant="warning">anotado pelo matching</Badge> : null}
                      {c.status === "parcial" ? <Badge variant="warning">parcial</Badge> : null}
                    </label>
                  </li>
                ))}
              </ul>
            )}
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                disabled={chosen.length === 0 || match.isPending}
                onClick={() =>
                  match.mutate(
                    { lineId: line.id, entryIds: chosen },
                    {
                      onSuccess: (r) => {
                        setChosen([]);
                        toast({
                          title: "Linha conciliada",
                          description: r.difference === "0.00" ? undefined : `Diferença entre a linha e os comprovantes: ${formatMoney(r.difference)}.`,
                          variant: "success",
                        });
                      },
                    },
                  )
                }
              >
                <Link2 aria-hidden /> Conciliar {chosen.length > 1 ? `os ${chosen.length} selecionados` : "o selecionado"}
              </Button>
              <Button size="sm" variant="outline" onClick={() => setConfirm(true)}>
                <ReceiptText aria-hidden /> Não há comprovante
              </Button>
            </div>
            <ErrorAlert error={match.error ?? noReceipt.error} fallback="Não foi possível conciliar." />
          </section>
          <ResolverSection lineId={line.id} />
          <ConfirmDialog
            open={confirm}
            onOpenChange={setConfirm}
            title="Confirmar sem comprovante"
            description="A linha fica conciliada sem comprovante (por exemplo, tarifa do banco) e passa pela classificação."
            confirmLabel="Confirmar"
            pending={noReceipt.isPending}
            onConfirm={() =>
              noReceipt.mutate(line.id, {
                onSuccess: () => {
                  setConfirm(false);
                  toast({ title: "Linha confirmada sem comprovante", variant: "success" });
                },
              })
            }
          />
        </>
      )}
    </div>
  );
}

const ENTRY_FIELDS: { key: keyof EntryFieldsPatch; origin: string; label: string }[] = [
  { key: "date", origin: "data", label: "Data" },
  { key: "amount", origin: "valor", label: "Valor" },
  { key: "direction", origin: "sentido", label: "Sentido" },
  { key: "counterparty", origin: "contraparte", label: "Contraparte" },
  { key: "counterparty_doc", origin: "contraparte_doc", label: "CPF/CNPJ da contraparte" },
  { key: "document_ref", origin: "documento", label: "Documento" },
];

function currentValue(entry: ClientEntry, key: keyof EntryFieldsPatch): string {
  const v = entry[key];
  return typeof v === "string" ? v : "";
}

/** Os campos lidos, com a confiança e a origem, e a correção campo a campo (REQ-023). */
export function EntryFields({ entry, editable }: { entry: ClientEntry; editable: boolean }) {
  const toast = useToast();
  const correct = useCorrectEntry(entry.id);
  const [values, setValues] = useState<Record<string, string>>(() =>
    Object.fromEntries(ENTRY_FIELDS.map((f) => [f.key, currentValue(entry, f.key)])),
  );
  const origin = entry.field_origin as Record<string, FieldOrigin>;
  const changed = ENTRY_FIELDS.filter((f) => (values[f.key] ?? "") !== currentValue(entry, f.key));
  const fieldErrors = fieldErrorsOf(correct.error);
  return (
    <form
      className="grid gap-3"
      aria-label="Campos do comprovante"
      onSubmit={(e) => {
        e.preventDefault();
        // O sentido vai como veio do select (C ou D); a API recusa o resto.
        const body = Object.fromEntries(changed.map((f) => [f.key, values[f.key] ?? ""])) as EntryFieldsPatch;
        correct.mutate(body, {
          onSuccess: (r) =>
            toast({
              title: "Comprovante corrigido",
              description: r.round ? `A competência rodou de novo: ${r.round.new_matches} conciliação(ões) nova(s).` : undefined,
              variant: "success",
            }),
        });
      }}
    >
      <p className="flex flex-wrap items-center gap-2 text-sm">
        <Badge variant={entry.status === "parcial" ? "warning" : "muted"}>{ENTRY_STATUS[entry.status]}</Badge>
        {entry.duplicate_flagged ? <Badge variant="warning">possível duplicata</Badge> : null}
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        {ENTRY_FIELDS.map((f) => {
          const conf = entry.field_confidence[f.origin];
          const o = origin[f.origin];
          const hint = o?.origem === "humano" ? "Corrigido à mão" : conf ? `Confiança ${formatRate(conf, 0)}${o?.trecho ? ` · “${o.trecho}”` : ""}` : undefined;
          return (
            <Field key={f.key} label={f.label} hint={hint} error={fieldErrors[f.key]}>
              {f.key === "direction" ? (
                <NativeSelect disabled={!editable} value={values[f.key] ?? ""} onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}>
                  <option value="">—</option>
                  <option value="C">C (entrada)</option>
                  <option value="D">D (saída)</option>
                </NativeSelect>
              ) : (
                <Input
                  type={f.key === "date" ? "date" : "text"}
                  inputMode={f.key === "amount" ? "decimal" : undefined}
                  disabled={!editable}
                  value={values[f.key] ?? ""}
                  onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))}
                />
              )}
            </Field>
          );
        })}
      </div>
      {editable ? (
        <>
          <ErrorAlert error={correct.error} fallback="Não foi possível corrigir." />
          <Button type="submit" size="sm" className="w-fit" disabled={changed.length === 0 || correct.isPending}>
            Salvar correção
          </Button>
        </>
      ) : null}
    </form>
  );
}

/** O comprovante na fila: os campos (com a correção) e as linhas candidatas. */
export function EntryPanel({ id, onOpenLine }: { id: string; onOpenLine: (lineId: string) => void }) {
  const toast = useToast();
  const detail = useReviewEntry(id);
  const match = useManualMatch();
  if (detail.isPending) return <Skeleton className="h-64" />;
  if (detail.isError) return <ErrorAlert error={detail.error} fallback="Não foi possível abrir o comprovante." />;
  const { entry, candidates, statement_transaction_id: lineId } = detail.data;
  return (
    <div className="grid content-start gap-4">
      <EntryFields key={entry.updated_at} entry={entry} editable={!lineId} />
      {lineId ? (
        <p className="text-sm">
          Conciliado.{" "}
          <button type="button" className="underline underline-offset-4" onClick={() => onOpenLine(lineId)}>
            Ver a linha do extrato
          </button>
        </p>
      ) : (
        <section aria-label="Linhas candidatas" className="grid gap-2">
          <h3 className="text-sm font-medium">Linhas do extrato candidatas</h3>
          {candidates.length === 0 ? (
            <p className="text-sm text-muted-foreground">Nenhuma linha livre de mesmo valor e sentido em até 3 dias.</p>
          ) : (
            <ul className="grid gap-1.5">
              {candidates.map((c) => (
                <li key={c.id} className="flex flex-wrap items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm">
                  {formatDate(c.date)} <Amount value={c.amount} direction={c.direction} />
                  <span className="max-w-72 truncate">{c.description}</span>
                  <span className="text-xs text-muted-foreground">{c.bank_account}</span>
                  <Button
                    size="sm"
                    className="ml-auto"
                    disabled={match.isPending}
                    onClick={() =>
                      match.mutate({ lineId: c.id, entryIds: [entry.id] }, { onSuccess: () => toast({ title: "Comprovante conciliado", variant: "success" }) })
                    }
                  >
                    <Link2 aria-hidden /> Conciliar com esta linha
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <ErrorAlert error={match.error} fallback="Não foi possível conciliar." />
        </section>
      )}
    </div>
  );
}

/** "Que documento é esse?": o tipo, a empresa e, no extrato, os dados da conta e o período. */
export function IdentifyForm({ document }: { document: ReconciliationDocumentDetail }) {
  const toast = useToast();
  const identify = useIdentifyDocument(document.id);
  const [form, setForm] = useState<DocumentIdentify>({
    kind: document.kind === "comprovante" ? "comprovante" : "extrato",
    company_id: document.company_id ?? "",
    bank: document.bank,
    branch: document.branch,
    account: document.account,
    period_start: document.period_start,
    period_end: document.period_end,
  });
  const set = (patch: Partial<DocumentIdentify>) => setForm((f) => ({ ...f, ...patch }));
  const fieldErrors = fieldErrorsOf(identify.error);
  return (
    <form
      aria-label="Que documento é esse?"
      className="grid gap-3 rounded-md border p-3"
      onSubmit={(e) => {
        e.preventDefault();
        identify.mutate(form, { onSuccess: () => toast({ title: "Documento de volta à extração", variant: "success" }) });
      }}
    >
      <h3 className="flex items-center gap-1.5 text-sm font-medium">
        <FileQuestion className="size-4" aria-hidden /> Que documento é esse?
      </h3>
      <fieldset className="flex flex-wrap gap-3 text-sm">
        <legend className="sr-only">Tipo do documento</legend>
        {(["extrato", "comprovante"] as const).map((k) => (
          <label key={k} className="inline-flex items-center gap-1.5">
            <input type="radio" name="tipo-documento" value={k} checked={form.kind === k} onChange={() => set({ kind: k })} className="size-4 accent-primary" />
            {k === "extrato" ? "Extrato bancário" : "Comprovante"}
          </label>
        ))}
      </fieldset>
      <Field label="Empresa" required error={fieldErrors.company_id}>
        {(p) => <CompanySelect value={form.company_id} onChange={(id) => set({ company_id: id })} {...p} />}
      </Field>
      {form.kind === "extrato" ? (
        <div className="grid gap-3 sm:grid-cols-3">
          <Field label="Banco" error={fieldErrors.bank}>
            <Input value={form.bank ?? ""} onChange={(e) => set({ bank: e.target.value })} />
          </Field>
          <Field label="Agência" error={fieldErrors.branch}>
            <Input value={form.branch ?? ""} onChange={(e) => set({ branch: e.target.value })} />
          </Field>
          <Field label="Conta" error={fieldErrors.account}>
            <Input value={form.account ?? ""} onChange={(e) => set({ account: e.target.value })} />
          </Field>
          <Field label="Início do período" error={fieldErrors.period_start}>
            <Input type="date" value={form.period_start ?? ""} onChange={(e) => set({ period_start: e.target.value || null })} />
          </Field>
          <Field label="Fim do período" error={fieldErrors.period_end}>
            <Input type="date" value={form.period_end ?? ""} onChange={(e) => set({ period_end: e.target.value || null })} />
          </Field>
        </div>
      ) : null}
      <ErrorAlert error={identify.error} fallback="Não foi possível identificar o documento." />
      <Button type="submit" size="sm" className="w-fit" disabled={!form.company_id || identify.isPending}>
        Enviar para a extração
      </Button>
    </form>
  );
}

/**
 * O documento em revisão na fila: o motivo, "que documento é esse?" e o reprocessamento. Na tela
 * do próprio documento, sem o cabeçalho (showHeader).
 */
export function DocumentPanel({ id, showHeader = true }: { id: string; showHeader?: boolean }) {
  const toast = useToast();
  const doc = useReconciliationDocument(id);
  const retry = useRetryDocument(id);
  if (doc.isPending) return <Skeleton className="h-64" />;
  if (doc.isError) return <ErrorAlert error={doc.error} fallback="Não foi possível abrir o documento." />;
  const d = doc.data;
  const reason = (d.diagnosis as { motivo?: string } | null)?.motivo;
  const identifiable = d.status === "revisao_triagem" || (d.status === "revisao_extracao" && !d.entry);
  return (
    <div className="grid content-start gap-4">
      {showHeader ? (
        <div className="grid gap-1">
          <p className="flex flex-wrap items-center gap-2 font-medium">
            {d.file_name} <DocStatusBadge status={d.status} />
          </p>
          {reason ? <p className="text-sm text-amber-900">{reasonText(reason)}</p> : null}
          <Link to={`/reconciliation/documents/${d.id}`} className="w-fit text-xs underline underline-offset-4">
            Abrir o documento
          </Link>
        </div>
      ) : null}
      {d.entry ? <EntryFields key={d.entry.updated_at} entry={d.entry} editable /> : null}
      {identifiable ? <IdentifyForm document={d} /> : null}
      {canRetry(d) ? (
        <div className="grid gap-2">
          <Button
            size="sm"
            variant="outline"
            className="w-fit"
            disabled={retry.isPending}
            onClick={() => retry.mutate(undefined, { onSuccess: () => toast({ title: "Documento de volta à fila", variant: "success" }) })}
          >
            <RotateCw aria-hidden /> Reprocessar
          </Button>
          <ErrorAlert error={retry.error} fallback="Não foi possível reprocessar." />
        </div>
      ) : null}
    </div>
  );
}
