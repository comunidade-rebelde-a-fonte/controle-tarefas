import { Construction } from "lucide-react";
import { PageHeader } from "./PageHeader";

interface PlaceholderPageProps {
  title: string;
  description: string;
  availableFrom: string;
}

/** Página provisória de uma área ainda não implementada. */
export function PlaceholderPage({ title, description, availableFrom }: PlaceholderPageProps) {
  return (
    <>
      <PageHeader title={title} description={description} />
      <div className="p-6">
        <div className="flex items-center gap-3 rounded-lg border border-dashed bg-card p-6 text-sm text-muted-foreground">
          <Construction className="size-5 shrink-0" aria-hidden />
          <span>Em construção — disponível a partir de {availableFrom}.</span>
        </div>
      </div>
    </>
  );
}
