import { FileUp } from "lucide-react";
import { type DragEvent, useId, useRef, useState } from "react";
import { Link } from "react-router";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { type PgdasUpload, usePgdasDocument, useUploadPgdas } from "@/services/factorRInbox";
import { CreateFromStatement } from "./CreateFromStatement";
import { DocumentStatusBadge } from "./DocumentStatusBadge";
import { JobProgress } from "./JobProgress";

interface Sent {
  key: string;
  name: string;
  result?: PgdasUpload;
  error?: string;
}

/** Recusas do envio em texto (origem: mensagemUpload). */
function uploadError(err: unknown): string {
  if (isApiError(err)) {
    if (err.status === 415) return "Formato não aceito. Envie o extrato em PDF ou TXT.";
    if (err.status === 413) return "Arquivo acima do tamanho máximo (10 MB).";
    return err.message;
  }
  return "Não foi possível enviar o arquivo.";
}

/** Um envio desta sessão: acompanha o documento até o fim do processamento. */
function SentDocument({ sent }: { sent: Sent & { result: PgdasUpload } }) {
  const doc = usePgdasDocument(sent.result.document_id);
  const d = doc.data;
  return (
    <li className="grid gap-2 rounded-md border bg-card p-3 text-sm" data-testid="resultado-upload">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{sent.name}</span>
        {d ? <DocumentStatusBadge document={d} /> : null}
        {sent.result.duplicate ? <span className="text-xs text-muted-foreground">Este arquivo já tinha sido enviado.</span> : null}
        <Link to={`/factor-r/inbox/${sent.result.document_id}`} className="ml-auto text-xs underline underline-offset-4">
          ver documento
        </Link>
      </div>
      {d ? <JobProgress document={d} /> : null}
      {d?.texto_agente ? <p className="whitespace-pre-line">{d.texto_agente}</p> : null}
      {d ? <CreateFromStatement document={d} /> : null}
    </li>
  );
}

/**
 * Envio de extratos PGDAS-D [FR-08]: PDF com texto ou TXT, validados pelo conteúdo na API. O
 * envio volta na hora (202) e o worker lê o extrato; aqui se acompanha cada documento.
 */
export function UploadPanel() {
  const upload = useUploadPgdas();
  const input = useRef<HTMLInputElement>(null);
  const inputId = useId();
  const [dragging, setDragging] = useState(false);
  const [sent, setSent] = useState<Sent[]>([]);
  const seq = useRef(0);

  async function send(files: FileList | null) {
    if (!files?.length) return;
    for (const file of Array.from(files)) {
      seq.current += 1;
      const key = String(seq.current);
      try {
        const result = await upload.mutateAsync(file);
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
    <section aria-label="Enviar extratos" className="grid gap-3">
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
        <p>Arraste aqui o extrato do PGDAS-D (PDF com texto ou TXT, até 10 MB) ou</p>
        <label htmlFor={inputId} className="sr-only">
          Extratos do PGDAS-D (PDF ou TXT)
        </label>
        <input
          ref={input}
          id={inputId}
          type="file"
          name="arquivo"
          multiple
          accept=".pdf,.txt,application/pdf,text/plain"
          className="text-sm file:mr-2 file:rounded file:border file:bg-card file:px-2 file:py-1"
          onChange={(e) => void send(e.target.files)}
        />
        {upload.isPending ? (
          <p role="status" className="text-muted-foreground">
            Enviando…
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">
          O CNPJ do extrato é procurado no cadastro de empresas. Com o CNPJ cadastrado e leitura confiável, o extrato é vinculado e a
          receita do PA é lançada só se a competência estiver vazia; o resto fica em revisão.
        </p>
      </div>
      {sent.length > 0 ? (
        <ul aria-label="Envios desta sessão" className="grid gap-2">
          {sent.map((s) =>
            s.result ? (
              <SentDocument key={s.key} sent={{ ...s, result: s.result }} />
            ) : (
              <li key={s.key} role="alert" className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900">
                <span className="font-medium">{s.name}</span>: {s.error}
              </li>
            ),
          )}
        </ul>
      ) : null}
    </section>
  );
}
