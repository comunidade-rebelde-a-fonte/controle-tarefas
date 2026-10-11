import { ListPlus, ReceiptText, Upload } from "lucide-react";
import { type SubmitEvent, useId, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { CompetenceInput } from "@/components/CompetenceInput";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field } from "@/components/ui/field";
import { Input, Label, NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { FilterToggle } from "@/features/factor-r/common";
import { formatDate } from "@/lib/format";
import { useCompanies } from "@/services/companies";
import { type ReconciliationEntryStatus, useCreateEntry, useEntries, useImportEntries } from "@/services/reconciliation";
import { Amount, CompanySelect, CsvImportError, RunReconciliation } from "../common";
import { ENTRY_STATUS } from "../labels";
import { useSearchUpdate } from "../model";
import { fieldErrorsOf } from "../review/model";

const PAGE = 100;

const STATUS_FILTERS = Object.entries(ENTRY_STATUS) as [ReconciliationEntryStatus, string][];

function isStatus(v: string | null): v is ReconciliationEntryStatus {
  return STATUS_FILTERS.some(([s]) => s === v);
}

interface Draft {
  date: string;
  amount: string;
  direction: "C" | "D";
  counterparty: string;
  counterpartyDoc: string;
  documentRef: string;
}

const EMPTY: Draft = { date: "", amount: "", direction: "D", counterparty: "", counterpartyDoc: "", documentRef: "" };

/** Os campos do formulário que recebem o erro do 400 (os demais vão para o alerta). */
const FORM_FIELDS = ["date", "amount", "direction", "counterparty", "counterparty_doc", "document_ref"];

/** Lançamento manual (CC-12): conta a pagar ou a receber informada pelo cliente. */
function NewEntryForm({ companyId }: { companyId: string }) {
  const toast = useToast();
  const create = useCreateEntry();
  const [draft, setDraft] = useState<Draft>(EMPTY);
  const errors = fieldErrorsOf(create.error);
  const set = (patch: Partial<Draft>) => setDraft((d) => ({ ...d, ...patch }));

  function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    create.mutate(
      {
        company_id: companyId,
        date: draft.date,
        amount: draft.amount.trim(),
        direction: draft.direction,
        counterparty: draft.counterparty.trim() || null,
        counterparty_doc: draft.counterpartyDoc.trim() || null,
        document_ref: draft.documentRef.trim() || null,
      },
      {
        onSuccess: () => {
          // Mantém a data e o sentido: o cliente costuma mandar vários do mesmo dia.
          setDraft({ ...EMPTY, date: draft.date, direction: draft.direction });
          toast({ title: "Lançamento incluído", variant: "success" });
        },
      },
    );
  }

  return (
    <form onSubmit={submit} aria-label="Novo lançamento" className="grid gap-3">
      <h2 className="text-sm font-semibold">Novo lançamento</h2>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Field label="Data" required error={errors.date}>
          <Input type="date" value={draft.date} onChange={(e) => set({ date: e.target.value })} required />
        </Field>
        <Field label="Valor" required error={errors.amount} hint="Sem separador de milhar: 1234,56">
          <Input inputMode="decimal" placeholder="0,00" value={draft.amount} onChange={(e) => set({ amount: e.target.value })} required />
        </Field>
        <Field label="Sentido" required error={errors.direction}>
          <NativeSelect value={draft.direction} onChange={(e) => set({ direction: e.target.value === "C" ? "C" : "D" })}>
            <option value="D">D · saída (a pagar)</option>
            <option value="C">C · entrada (a receber)</option>
          </NativeSelect>
        </Field>
        <Field label="Contraparte" error={errors.counterparty}>
          <Input placeholder="Fornecedor ou cliente" maxLength={500} value={draft.counterparty} onChange={(e) => set({ counterparty: e.target.value })} />
        </Field>
        <Field label="CPF/CNPJ da contraparte" error={errors.counterparty_doc}>
          <Input inputMode="numeric" value={draft.counterpartyDoc} onChange={(e) => set({ counterpartyDoc: e.target.value })} />
        </Field>
        <Field label="Documento" error={errors.document_ref} hint="Nota fiscal, ID da transação, nosso número…">
          <Input maxLength={500} value={draft.documentRef} onChange={(e) => set({ documentRef: e.target.value })} />
        </Field>
      </div>
      {FORM_FIELDS.some((k) => errors[k]) ? null : <ErrorAlert error={create.error} fallback="Não foi possível incluir o lançamento." />}
      <Button type="submit" size="sm" className="w-fit" disabled={create.isPending}>
        <ListPlus aria-hidden /> Incluir
      </Button>
    </form>
  );
}

/** CSV do cliente (csv_lancamentos da origem): data;valor;sentido;contraparte, tudo ou nada. */
function ImportEntries({ companyId }: { companyId: string }) {
  const toast = useToast();
  const imp = useImportEntries();
  const input = useRef<HTMLInputElement>(null);
  const inputId = useId();
  const [file, setFile] = useState<File | null>(null);
  return (
    <div className="grid gap-2 border-t pt-4">
      <div className="flex flex-wrap items-end gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor={inputId}>Ou importe o CSV do cliente</Label>
          <input
            ref={input}
            id={inputId}
            type="file"
            accept=".csv,text/csv"
            className="text-sm file:mr-2 file:rounded file:border file:bg-card file:px-2 file:py-1"
            onChange={(e) => {
              imp.reset();
              setFile(e.target.files?.[0] ?? null);
            }}
          />
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={!file || imp.isPending}
          onClick={() => {
            if (!file) return;
            imp.mutate(
              { file, companyId },
              {
                onSuccess: (r) => {
                  setFile(null);
                  if (input.current) input.current.value = "";
                  toast({ title: `${String(r.inserted)} ${r.inserted === 1 ? "lançamento importado" : "lançamentos importados"}`, variant: "success" });
                },
              },
            );
          }}
        >
          <Upload aria-hidden /> Importar CSV
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        Uma linha por lançamento, separada por ponto e vírgula: data (AAAA-MM-DD);valor;sentido (C ou D);contraparte. Com uma linha
        inválida, nada é gravado e cada erro aparece aqui.
      </p>
      <CsvImportError error={imp.error} fallback="Não foi possível importar o CSV." />
    </div>
  );
}

/**
 * Lançamentos do cliente [REQ-023, CC-12]: os manuais (formulário e CSV) e os lidos dos
 * comprovantes, para casar com o extrato; e a rodada da conciliação da competência (o "Rodar
 * matching" da origem, aqui por empresa e competência).
 */
export function EntriesPage() {
  const [params, update] = useSearchUpdate();
  const companyId = params.get("company_id") ?? "";
  const rawStatus = params.get("status");
  const status = isStatus(rawStatus) ? rawStatus : undefined;
  const from = params.get("from") ?? "";
  const to = params.get("to") ?? "";
  const competence = params.get("competence") ?? "";
  const page = Math.max(0, Math.trunc(Number(params.get("page") ?? "0")) || 0);
  const entries = useEntries({
    companyId: companyId || undefined,
    status,
    dateFrom: from || undefined,
    dateTo: to || undefined,
    offset: page * PAGE,
  });
  const companies = useCompanies({ active: true });
  const codes = useMemo(() => new Map((companies.data?.items ?? []).map((c) => [c.id, c.internal_code])), [companies.data]);
  const data = entries.data;
  const first = page * PAGE + 1;
  const last = page * PAGE + (data?.items.length ?? 0);

  return (
    <>
      <PageHeader
        title="Lançamentos"
        description="Contas a pagar e a receber informadas pelo cliente (formulário ou CSV) e as lidas dos comprovantes, para casar com o extrato."
      />
      <div className="grid gap-4 p-4">
        <label className="grid w-fit gap-1 text-xs font-medium text-muted-foreground">
          Empresa
          <CompanySelect value={companyId} onChange={(id) => update({ company_id: id || null, page: null })} allLabel="Todas as empresas" />
        </label>
        {companyId ? (
          <>
            <section aria-label="Incluir lançamentos" className="grid gap-4 rounded-lg border bg-card p-4">
              <NewEntryForm key={companyId} companyId={companyId} />
              <ImportEntries key={`csv-${companyId}`} companyId={companyId} />
            </section>
            <section aria-label="Conciliar a competência" className="grid gap-2 rounded-lg border bg-card p-4">
              <h2 className="text-sm font-semibold">Conciliar a competência</h2>
              <p className="text-xs text-muted-foreground">
                Casa as linhas do extrato com os lançamentos e comprovantes (passes 1 a 4) e classifica por regra e histórico. O que não
                for resolvido sem ambiguidade vai para a fila de revisão; rodar de novo não duplica nada.
              </p>
              <div className="flex flex-wrap items-start gap-3">
                <label className="grid gap-1 text-xs font-medium text-muted-foreground">
                  Competência
                  <CompetenceInput value={competence} onChange={(v) => update({ competence: v || null })} />
                </label>
                <div className="pt-5">
                  <RunReconciliation companyId={companyId} competence={competence} showQueueLink />
                </div>
              </div>
            </section>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">Escolha a empresa para incluir lançamentos, importar o CSV do cliente ou rodar a conciliação.</p>
        )}
        <div className="flex flex-wrap items-end gap-3">
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            De
            <Input type="date" value={from} onChange={(e) => update({ from: e.target.value || null, page: null })} />
          </label>
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Até
            <Input type="date" value={to} onChange={(e) => update({ to: e.target.value || null, page: null })} />
          </label>
        </div>
        <div role="group" aria-label="Filtrar por status" className="flex flex-wrap gap-2">
          <FilterToggle active={!status} onClick={() => update({ status: null, page: null })}>
            Todos
          </FilterToggle>
          {STATUS_FILTERS.map(([s, label]) => (
            <FilterToggle key={s} active={status === s} onClick={() => update({ status: s, page: null })}>
              {label}
            </FilterToggle>
          ))}
        </div>
        <ErrorAlert error={entries.error} fallback="Não foi possível carregar os lançamentos." />
        {entries.isPending ? <Skeleton className="h-64" /> : null}
        {data && data.items.length === 0 ? (
          <EmptyState icon={ReceiptText} title="Nenhum lançamento" description="Inclua um lançamento acima ou importe o CSV do cliente." />
        ) : null}
        {data && data.items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Lançamentos do cliente">
              <thead>
                <tr>
                  <Th scope="col">Data</Th>
                  <Th scope="col">Empresa</Th>
                  <Th scope="col" className="text-right">
                    Valor
                  </Th>
                  <Th scope="col">Contraparte</Th>
                  <Th scope="col">Documento</Th>
                  <Th scope="col">Origem</Th>
                  <Th scope="col">Status</Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((e) => (
                  <tr key={e.id} data-lancamento={e.id}>
                    <Td className="whitespace-nowrap tabular-nums">{e.date ? formatDate(e.date) : "—"}</Td>
                    <Td className="text-xs whitespace-nowrap">{codes.get(e.company_id) ?? "—"}</Td>
                    <Td className="text-right">{e.amount ? <Amount value={e.amount} direction={e.direction} /> : "—"}</Td>
                    <Td className="max-w-72">
                      <span className="block truncate">{e.counterparty ?? "—"}</span>
                      {e.counterparty_doc ? <span className="block text-xs text-muted-foreground tabular-nums">{e.counterparty_doc}</span> : null}
                    </Td>
                    <Td className="max-w-48 truncate text-xs">{e.document_ref ?? "—"}</Td>
                    <Td className="text-xs whitespace-nowrap">
                      {e.document_id ? (
                        <Link to={`/reconciliation/documents/${e.document_id}`} className="inline-flex min-h-6 items-center underline underline-offset-4">
                          Comprovante
                        </Link>
                      ) : (
                        "Formulário ou CSV"
                      )}
                    </Td>
                    <Td className="whitespace-nowrap">
                      <Badge variant={e.status === "parcial" ? "warning" : "muted"}>{ENTRY_STATUS[e.status]}</Badge>
                      {e.status === "parcial" ? (
                        <Link
                          to={`/reconciliation?item=${e.id}&kind=comprovante`}
                          className="ml-2 inline-flex min-h-6 items-center text-xs underline underline-offset-4"
                        >
                          revisar
                        </Link>
                      ) : null}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <div className="flex flex-wrap items-center gap-2 border-t px-3 py-2 text-xs text-muted-foreground">
              <span>
                {first}–{last} de {data.total}
              </span>
              <span className="ml-auto flex gap-2">
                <Button size="sm" variant="outline" disabled={page === 0} onClick={() => update({ page: page > 1 ? String(page - 1) : null })}>
                  Anterior
                </Button>
                <Button size="sm" variant="outline" disabled={last >= data.total} onClick={() => update({ page: String(page + 1) })}>
                  Próxima
                </Button>
              </span>
            </div>
          </div>
        ) : null}
      </div>
    </>
  );
}
