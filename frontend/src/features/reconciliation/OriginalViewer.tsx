import { useQuery } from "@tanstack/react-query";
import { ExternalLink, FileX } from "lucide-react";
import { useState } from "react";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { documentFileUrl, documentPageUrl, useDocumentPages } from "@/services/reconciliation";

/** Marcação de um campo lido: a página (a partir de 1) e o retângulo [x0, y0, x1, y1] no DPI do render. */
export interface Highlight {
  page: number;
  bbox: number[];
  label: string;
}

const FIRST_PAGES = 5;

/** Uma página (ou imagem) com as marcações por cima, em proporção ao tamanho real da imagem. */
function PageImage({ src, alt, boxes }: { src: string; alt: string; boxes: Highlight[] }) {
  const [size, setSize] = useState<{ w: number; h: number } | null>(null);
  return (
    <div className="relative overflow-hidden rounded border bg-white">
      <img
        src={src}
        alt={alt}
        loading="lazy"
        className="block h-auto w-full"
        onLoad={(e) => setSize({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
      />
      {size
        ? boxes.map((b, i) => {
            const [x0 = 0, y0 = 0, x1 = 0, y1 = 0] = b.bbox;
            return (
              <div
                key={i}
                title={b.label}
                aria-hidden
                className="pointer-events-none absolute rounded-sm border-2 border-amber-500 bg-amber-300/20"
                style={{
                  left: `${(x0 / size.w) * 100}%`,
                  top: `${(y0 / size.h) * 100}%`,
                  width: `${((x1 - x0) / size.w) * 100}%`,
                  height: `${((y1 - y0) / size.h) * 100}%`,
                }}
              />
            );
          })
        : null}
    </div>
  );
}

/** Original em texto (CSV, TXT): mostrado como está, com rolagem acessível pelo teclado. */
function TextOriginal({ id }: { id: string }) {
  const text = useQuery({
    queryKey: ["reconciliation", "file-text", id],
    queryFn: async ({ signal }) => {
      const res = await fetch(documentFileUrl(id), { signal });
      if (!res.ok) throw new Error("Não foi possível abrir o original.");
      return res.text();
    },
    staleTime: Infinity,
  });
  if (text.isPending) return <Skeleton className="h-64" />;
  if (text.error) return <ErrorAlert error={text.error} fallback="Não foi possível abrir o original." />;
  return (
    <pre tabIndex={0} aria-label="Conteúdo do original" className="max-h-[32rem] overflow-auto rounded border bg-muted/40 p-3 text-xs whitespace-pre">
      {text.data}
    </pre>
  );
}

/**
 * O original ao lado dos dados (08 §7): as páginas do PDF como imagem, no DPI do extrator, com a
 * marcação de cada campo lido (decisão do usuário, D-032); a imagem como está; o texto como texto.
 */
export function OriginalViewer({ documentId, highlights = [] }: { documentId: string | null | undefined; highlights?: Highlight[] }) {
  const pages = useDocumentPages(documentId);
  const [all, setAll] = useState(false);
  if (!documentId) {
    return <EmptyState icon={FileX} title="Sem original" description="Lançamento inserido à mão: não há documento para mostrar." />;
  }
  if (pages.isPending) return <Skeleton className="h-96" />;
  const open = (
    <a href={documentFileUrl(documentId)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs underline underline-offset-4">
      <ExternalLink className="size-3.5" aria-hidden /> Abrir o original
    </a>
  );
  if (pages.isError) {
    return (
      <div className="grid gap-2">
        <ErrorAlert error={pages.error} fallback="Não foi possível mostrar o original." />
        {open}
      </div>
    );
  }
  const info = pages.data;
  let body;
  if (info.pages > 0) {
    const shown = all ? info.pages : Math.min(info.pages, FIRST_PAGES);
    body = (
      <>
        {Array.from({ length: shown }, (_, i) => i + 1).map((n) => (
          <PageImage key={n} src={documentPageUrl(documentId, n)} alt={`Página ${n} de ${info.pages} do original`} boxes={highlights.filter((h) => h.page === n)} />
        ))}
        {shown < info.pages ? (
          <Button variant="outline" size="sm" className="w-fit" onClick={() => setAll(true)}>
            Mostrar as {info.pages} páginas
          </Button>
        ) : null}
      </>
    );
  } else if (info.content_type.startsWith("image/")) {
    body = <PageImage src={documentFileUrl(documentId)} alt="Imagem do original" boxes={highlights.filter((h) => h.page <= 1)} />;
  } else {
    body = <TextOriginal id={documentId} />;
  }
  return (
    <div className="grid content-start gap-2" data-testid="original">
      {body}
      {open}
    </div>
  );
}
