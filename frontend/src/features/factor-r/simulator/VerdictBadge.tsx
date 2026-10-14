import { CircleCheck, CircleHelp, OctagonX, Wrench } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { FactorRVerdict } from "@/services/factorRAgents";

const VERDICTS = {
  ja_na_meta: { label: "Já na meta", variant: "success", Icon: CircleCheck },
  corrigir: { label: "Corrigir", variant: "warning", Icon: Wrench },
  nao_forcar: { label: "Não forçar", variant: "danger", Icon: OctagonX },
} as const;

/** Veredito do simulador (origem: VEREDITO), com ícone e texto; nulo = dados insuficientes. */
export function VerdictBadge({ verdict }: { verdict: FactorRVerdict | null | undefined }) {
  if (!verdict) {
    return (
      <Badge variant="muted" data-veredito="dados_insuficientes">
        <CircleHelp aria-hidden /> Dados insuficientes
      </Badge>
    );
  }
  const { label, variant, Icon } = VERDICTS[verdict];
  return (
    <Badge variant={variant} data-veredito={verdict}>
      <Icon aria-hidden /> {label}
    </Badge>
  );
}
