import { Check, Copy } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Tooltip } from "@/components/ui/tooltip";
import { sanitizeCnpj } from "@/lib/cnpj";
import { cn } from "@/lib/utils";

interface CopyButtonProps {
  /** Valor exato colocado na área de transferência. */
  value: string;
  /** Rótulo acessível, ex.: "Copiar CNPJ". */
  label: string;
  /** Exibe o rótulo ao lado do ícone. */
  showLabel?: boolean;
  className?: string;
  /** -1 dentro da DataGrid (a célula é que recebe o foco). */
  tabIndex?: number;
}

/** Copia um valor para a área de transferência com feedback acessível ("Copiado"). */
export function CopyButton({ value, label, showLabel = false, className, tabIndex }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  const text = copied ? "Copiado" : label;
  return (
    <Tooltip content={text}>
      <Button
        variant="ghost"
        size={showLabel ? "sm" : "icon-sm"}
        className={cn("text-muted-foreground", className)}
        aria-label={text}
        tabIndex={tabIndex}
        onClick={(e) => {
          e.stopPropagation();
          void copy();
        }}
      >
        {copied ? <Check className="text-success" aria-hidden /> : <Copy aria-hidden />}
        {showLabel ? <span>{text}</span> : null}
        <span className="sr-only" aria-live="polite">
          {copied ? "Copiado" : ""}
        </span>
      </Button>
    </Tooltip>
  );
}

/** Copia o CNPJ somente com os 14 dígitos [RN-014, CA-003]. */
export function CopyCnpjButton({ cnpj, showLabel, tabIndex }: { cnpj: string; showLabel?: boolean; tabIndex?: number }) {
  return (
    <CopyButton
      value={sanitizeCnpj(cnpj)}
      label="Copiar CNPJ"
      {...(showLabel !== undefined ? { showLabel } : {})}
      {...(tabIndex !== undefined ? { tabIndex } : {})}
    />
  );
}
