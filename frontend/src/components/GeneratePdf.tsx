import { FileDown } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * Atalho "Gerar PDF" das telas de gestão [PRD §36]: baixa o relatório com a competência e os
 * filtros atuais. Sem URL (competência ainda carregando), fica desabilitado.
 */
export function GeneratePdf({ href, label }: { href: string | null; label: string }) {
  if (!href) {
    return (
      <Button variant="outline" disabled>
        <FileDown aria-hidden /> Gerar PDF
      </Button>
    );
  }
  return (
    <Button variant="outline" asChild>
      <a href={href} download aria-label={`Gerar PDF — ${label}`} title={`Relatório em PDF: ${label}, com os filtros atuais`}>
        <FileDown aria-hidden /> Gerar PDF
      </a>
    </Button>
  );
}
