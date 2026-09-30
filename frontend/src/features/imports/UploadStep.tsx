import { Download, FileUp } from "lucide-react";
import { useId, useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { type ImportPreview, IMPORT_TEMPLATE_URL, useImportPreview } from "@/services/imports";

/** Passos 1 e 2: baixar o modelo e enviar a planilha para a prévia [PRD §35.3]. */
export function UploadStep({ onPreview }: { onPreview: (p: ImportPreview) => void }) {
  const [file, setFile] = useState<File | null>(null);
  const preview = useImportPreview();
  const id = useId();

  return (
    <div className="grid max-w-2xl gap-6">
      <section className="grid gap-2 rounded-lg border bg-card p-4">
        <h2 className="text-sm font-semibold">1. Baixe o modelo</h2>
        <p className="text-sm text-muted-foreground">
          Uma linha por empresa. O modelo traz as colunas, as listas de valores aceitos e uma aba de instruções.
        </p>
        <a
          href={IMPORT_TEMPLATE_URL}
          download
          className="inline-flex h-9 w-fit items-center gap-1.5 rounded-md border border-input bg-card px-3.5 text-sm font-medium hover:bg-accent"
        >
          <Download className="size-4" aria-hidden="true" /> Baixar modelo (.xlsx)
        </a>
      </section>

      <form
        className="grid gap-3 rounded-lg border bg-card p-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (file) preview.mutate(file, { onSuccess: onPreview });
        }}
      >
        <h2 className="text-sm font-semibold">2. Envie a planilha preenchida</h2>
        <label htmlFor={id} className="text-sm">
          Planilha (.xlsx ou .csv, até 10 MB)
        </label>
        <input
          id={id}
          type="file"
          accept=".xlsx,.csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"
          className="text-sm file:mr-2 file:rounded file:border file:bg-card file:px-2 file:py-1"
          onChange={(e) => {
            setFile(e.target.files?.[0] ?? null);
            preview.reset();
          }}
        />
        <p className="text-xs text-muted-foreground">Nada é gravado agora: a próxima etapa mostra a prévia com os erros por linha.</p>
        <ErrorAlert error={preview.error} fallback="Não foi possível ler a planilha." />
        <Button type="submit" className="w-fit" disabled={!file || preview.isPending}>
          <FileUp aria-hidden="true" /> {preview.isPending ? "Validando…" : "Validar planilha"}
        </Button>
      </form>
    </div>
  );
}
