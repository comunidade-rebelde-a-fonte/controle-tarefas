import { ChevronLeft, ChevronRight, CircleAlert, CircleCheck, CircleHelp, Info, TriangleAlert } from "lucide-react";
import { CompetenceInput } from "@/components/CompetenceInput";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { displayCompetence, nextCompetence, prevCompetence } from "@/lib/competence";
import type { FactorRLight } from "@/services/factorR";
import { DISCLAIMER, LIGHT_LABELS } from "./labels";
import { usePA } from "./usePA";

const LIGHT_STYLE = {
  vermelho: { variant: "danger", Icon: CircleAlert },
  amarelo: { variant: "warning", Icon: TriangleAlert },
  verde: { variant: "success", Icon: CircleCheck },
} as const;

/** Semáforo com ícone e texto, além da cor [AC-007]. */
export function LightBadge({ light }: { light: FactorRLight | null }) {
  if (!light) {
    return (
      <Badge variant="muted">
        <CircleHelp aria-hidden /> Dados insuficientes
      </Badge>
    );
  }
  const { variant, Icon } = LIGHT_STYLE[light];
  return (
    <Badge variant={variant}>
      <Icon aria-hidden /> {LIGHT_LABELS[light]}
    </Badge>
  );
}

/** Aviso de conformidade em todas as telas do Fator R [FR-19]. */
export function FactorRDisclaimer() {
  return (
    <p role="note" className="flex items-center gap-2 rounded-md border bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
      <Info className="size-4 shrink-0" aria-hidden />
      {DISCLAIMER}
    </p>
  );
}

/** `← PA anterior | seletor | PA seguinte →` e "PA corrente". Qualquer mês pode ser PA. */
export function PANav() {
  const { pa, current, setPA } = usePA();
  if (pa === null) return <Skeleton className="h-9 w-96" />;
  const prev = prevCompetence(pa);
  const next = nextCompetence(pa);
  return (
    <nav aria-label="Período de apuração" className="flex flex-wrap items-center gap-2">
      <Button variant="outline" size="sm" onClick={() => setPA(prev)} aria-label={`PA anterior: ${displayCompetence(prev)}`}>
        <ChevronLeft aria-hidden="true" />
        {displayCompetence(prev)}
      </Button>
      <CompetenceInput aria-label="Período de apuração (PA)" value={pa} onChange={setPA} />
      <Button variant="outline" size="sm" onClick={() => setPA(next)} aria-label={`PA seguinte: ${displayCompetence(next)}`}>
        {displayCompetence(next)}
        <ChevronRight aria-hidden="true" />
      </Button>
      <Button variant="ghost" size="sm" onClick={() => current && setPA(current)} disabled={current === null || pa === current}>
        PA corrente
      </Button>
    </nav>
  );
}
