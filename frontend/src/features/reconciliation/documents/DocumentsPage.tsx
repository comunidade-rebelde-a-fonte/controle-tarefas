import { FileUp, FolderOpen } from "lucide-react";
import { type DragEvent, useId, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { CompanyFilterChips } from "@/components/CompanyFilterChips";
import { CompetenceInput } from "@/components/CompetenceInput";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { FilterToggle } from "@/features/factor-r/common";
import { isApiError } from "@/lib/api";
import { companyFilterFrom } from "@/lib/companyFilter";
import { displayCompetence } from "@/lib/competence";
import { formatDate, formatDateTime } from "@/lib/format";
import { cn } from "@/lib/utils";
import {
  type ReconciliationDocumentStatus,
  type ReconciliationUpload,
  useReconciliationDocument,
  useReconciliationDocuments,
  useUploadDocument,
} from "@/services/reconciliation";
import { CompanySelect, DocStatusBadge, DocumentJob } from "../common";
import { DOC_KIND, DOC_STATUS_FILTERS, reasonText } from "../labels";

interface Sent {
  key: string;
  name: string;
  result?: ReconciliationUpload;
  error?: string;
}

/** Recusas do envio em texto. */
function uploadError(err: unknown): string {
  if (isApiError(err)) {
    if (err.status === 409) return "Este arquivo já foi enviado (o original é o mesmo).";
    if (err.status === 413) return "Arquivo acima de 20 MB.";
    if (err.status === 503) return "O armazenamento dos originais não está disponível. Nada foi gravado; tente de novo.";
    return err.message;
  }
  return "Não foi possível enviar o arquivo.";
}

/** Um envio desta sessão: acompanha o documento até o fim do processamento. */
function SentDocument({ sent }: { sent: Sent & { result: ReconciliationUpload } }) {
  const doc = useReconciliationDocument(sent.result.document_id);
  const d = doc.data;
  return (
    <li className="grid gap-2 rounded-md border bg-card p-3 text-sm" data-testid="resultado-envio">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{sent.name}</span>
        {d ? <DocStatusBadge status={d.status} /> : null}
        <Link to={`/reconciliation/documents/${sent.result.document_id}`} className="ml-auto text-xs underline underline-offset-4">
          ver documento
        </Link>
      </div>
      {d ? <DocumentJob document={d} /> : null}
    </li>
  );
}

/**
 * Envio de extratos e comprovantes [REQ-018, AC-013]: PDF, imagem, CSV ou TXT, um arquivo por
 * envio. O envio volta na hora (202) e o worker tria e extrai; aqui se acompanha cada documento.
 */
function UploadPanel() {
  const upload = useUploadDocument();
  const input = useRef<HTMLInputElement>(null);
  const inputId = useId();
  const [dragging, setDragging] = useState(false);
  const [companyId, setCompanyId] = useState("");
  const [competence, setCompetence] = useState("");
  const [sent, setSent] = useState<Sent[]>([]);
  const seq = useRef(0);

  async function send(files: FileList | null) {
    if (!files?.length) return;
    for (const file of Array.from(files)) {
      seq.current += 1;
      const key = String(seq.current);
      try {
        const result = await upload.mutateAsync({ file, companyId: companyId || undefined, competence: competence || undefined });
        setSent((s) => [{ key, name: file.name, result }, ...s]);
      } catch (err) {
        setSent((s) => [{ key, name: file.name, error: uploadError(err) }, ...s]);
      }
    }
    if (input.current) input.current.value = "";
  }

  function onDrop(e: DragEvent<HTMLDivElement>) {
    e.preventDefault();
    setDragging(false);
    void send(e.dataTransfer.files);
  }

  return (
    <section aria-label="Enviar documentos" className="grid gap-3">
      <div className="flex flex-wrap items-end gap-3">
        <label className="grid gap-1 text-xs font-medium text-muted-foreground">
          Empresa (opcional)
          <CompanySelect value={companyId} onChange={setCompanyId} allLabel="A triagem identifica" />
        </label>
        <label className="grid gap-1 text-xs font-medium text-muted-foreground">
          Competência (opcional)
          <CompetenceInput value={competence} onChange={setCompetence} />
        </label>
      </div>
      <div
        data-testid="dropzone"
        onDragOver={(e) => {
          e.preventDefault();
          setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={onDrop}
        className={cn(
          "grid justify-items-center gap-2 rounded-lg border-2 border-dashed bg-card px-6 py-6 text-center text-sm transition-colors",
          dragging ? "border-primary bg-primary/5" : "border-border",
        )}
      >
        <FileUp className="size-6 text-muted-foreground" aria-hidden />
        <p>Arraste aqui extratos e comprovantes (PDF, imagem, CSV ou TXT, até 20 MB) ou</p>
        <label htmlFor={inputId} className="sr-only">
          Extratos e comprovantes
        </label>
        <input
          ref={input}
          id={inputId}
          type="file"
          multiple
          accept=".pdf,.png,.jpg,.jpeg,.csv,.txt,application/pdf,image/png,image/jpeg,text/csv,text/plain"
          className="text-sm file:mr-2 file:rounded file:border file:bg-card file:px-2 file:py-1"
          onChange={(e) => void send(e.target.files)}
        />
        {upload.isPending ? (
          <p role="status" className="text-muted-foreground">
            Enviando…
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          O original fica guardado e nunca é apagado. A triagem diz se é extrato ou comprovante; o extrato só é importado se o saldo
          fechar exatamente.
        </p>
      </div>
      {sent.length > 0 ? (
        <ul aria-label="Envios desta sessão" className="grid gap-2">
          {sent.map((s) =>
            s.result ? (
              <SentDocument key={s.key} sent={{ ...s, result: s.result }} />
            ) : (
              <li key={s.key} role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
                <span className="font-medium">{s.name}</span>: {s.error}
              </li>
            ),
          )}
        </ul>
      ) : null}
    </section>
  );
}

function isStatus(v: string): v is ReconciliationDocumentStatus {
  return DOC_STATUS_FILTERS.some(([s]) => s === v);
}

// Os grupos de status do Dashboard (TASK-024, D-038): o que está em processamento e o que está em
// revisão de triagem ou extração.
const DOC_STATUS_GROUPS: [string, ReconciliationDocumentStatus[]][] = [
  ["Processando", ["pendente", "triado"]],
  ["Em revisão", ["revisao_triagem", "revisao_extracao"]],
];

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((v) => b.includes(v));

/**
 * Documentos da Conciliação (08 §7): status, tipo, banco, período, empresa e diagnóstico. O
 * status aceita mais de um na URL (os grupos do Dashboard), e os filtros de empresa do Dashboard
 * valem pelo snapshot de `filter_competence` (D-038).
 */
export function DocumentsPage() {
  const [params, setParams] = useSearchParams();
  const statuses = (params.get("status") ?? "").split(",").filter(isStatus);
  const companyId = params.get("company_id") ?? "";
  const docs = useReconciliationDocuments({
    status: statuses.length > 0 ? statuses.join(",") : undefined,
    companyId: companyId || undefined,
    filter: companyFilterFrom(params),
    filterCompetence: params.get("filter_competence") ?? undefined,
  });
  const data = docs.data;
  const counts = data?.count_by_status ?? {};
  const all = Object.values(counts).reduce((a, b) => a + b, 0);
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
  return (
    <>
      <PageHeader title="Documentos" description="Extratos e comprovantes enviados: triagem, extração, importação e o que ficou em revisão." />
      <div className="grid gap-4 p-4">
        <UploadPanel />
        <div className="flex flex-wrap items-end gap-3">
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Empresa
            <CompanySelect value={companyId} onChange={(id) => update({ company_id: id || null })} allLabel="Todas as empresas" />
          </label>
        </div>
        <CompanyFilterChips module="Contábil" snapshotKey="filter_competence" />
        <div role="group" aria-label="Filtrar por status" className="flex flex-wrap gap-2">
          <FilterToggle active={statuses.length === 0} onClick={() => update({ status: null })}>
            Todos ({all})
          </FilterToggle>
          {DOC_STATUS_GROUPS.map(([label, group]) => (
            <FilterToggle key={label} active={sameSet(statuses, group)} onClick={() => update({ status: group.join(",") })}>
              {label} ({group.reduce((n, st) => n + (counts[st] ?? 0), 0)})
            </FilterToggle>
          ))}
          {DOC_STATUS_FILTERS.map(([s, label]) => (
            <FilterToggle key={s} active={sameSet(statuses, [s])} onClick={() => update({ status: s })}>
              {label} ({counts[s] ?? 0})
            </FilterToggle>
          ))}
        </div>
        {data ? (
          <p role="status" className="text-sm text-muted-foreground">
            {data.total === 1 ? "1 documento" : `${data.total} documentos`}
          </p>
        ) : null}
        <ErrorAlert error={docs.error} fallback="Não foi possível carregar os documentos." />
        {docs.isPending ? <Skeleton className="h-64" /> : null}
        {data && data.items.length === 0 ? (
          <EmptyState icon={FolderOpen} title="Nenhum documento" description="Envie extratos e comprovantes acima." />
        ) : null}
        {data && data.items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Documentos da Conciliação">
              <thead>
                <tr>
                  <Th scope="col">Recebido em</Th>
                  <Th scope="col">Arquivo</Th>
                  <Th scope="col">Tipo</Th>
                  <Th scope="col">Banco</Th>
                  <Th scope="col">Período</Th>
                  <Th scope="col">Status</Th>
                  <Th scope="col">Diagnóstico</Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((d) => (
                  <tr key={d.id} data-documento={d.id}>
                    <Td className="whitespace-nowrap">
                      <Link to={`/reconciliation/documents/${d.id}`} className="inline-flex min-h-6 items-center font-medium hover:underline">
                        {formatDateTime(d.created_at)}
                      </Link>
                    </Td>
                    <Td className="max-w-64 truncate">{d.file_name}</Td>
                    <Td>{d.kind ? DOC_KIND[d.kind] : "—"}</Td>
                    <Td className="whitespace-nowrap">{d.bank ?? "—"}</Td>
                    <Td className="whitespace-nowrap tabular-nums">
                      {d.period_start ? `${formatDate(d.period_start)} a ${formatDate(d.period_end)}` : d.competence ? displayCompetence(d.competence) : "—"}
                    </Td>
                    <Td>
                      <DocStatusBadge status={d.status} />
                    </Td>
                    <Td className="max-w-72 text-xs text-muted-foreground">{reasonText((d.diagnosis as { motivo?: string } | null)?.motivo)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            {data.total > data.items.length ? (
              <p className="border-t px-3 py-2 text-xs text-muted-foreground">
                Mostrando os {data.items.length} mais recentes de {data.total}.
              </p>
            ) : null}
          </div>
        ) : null}
      </div>
    </>
  );
}
