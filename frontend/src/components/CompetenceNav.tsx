import { ChevronLeft, ChevronRight, History } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { compareCompetence, displayCompetence, nextCompetence, prevCompetence } from "@/lib/competence";
import { useCompetence } from "@/lib/useCompetence";
import { CompetenceInput } from "./CompetenceInput";

/**
 * Navegação de competência das telas mensais [PRD §10.7]:
 * `← MM/AAAA | seletor | MM/AAAA →` e "Mais recente". O estado fica na URL.
 * Não avança além da competência mais recente gerada (não há dados depois dela).
 */
export function CompetenceNav() {
  const { competence, latest, setCompetence, isLoading } = useCompetence();

  if (isLoading || competence === null) {
    return <Skeleton className="h-9 w-96" />;
  }

  const prev = prevCompetence(competence);
  const next = nextCompetence(competence);
  // Enquanto a mais recente não chega, não há como saber se a próxima existe.
  const atLatest = latest === null || compareCompetence(competence, latest) >= 0;

  return (
    <nav aria-label="Competência" className="flex items-center gap-2">
      <Button variant="outline" size="sm" onClick={() => setCompetence(prev)} aria-label={`Competência anterior: ${displayCompetence(prev)}`}>
        <ChevronLeft aria-hidden="true" />
        {displayCompetence(prev)}
      </Button>
      <CompetenceInput
        aria-label="Selecionar competência"
        value={competence}
        {...(latest ? { max: latest } : {})}
        onChange={(v) => {
          if (latest === null || compareCompetence(v, latest) <= 0) setCompetence(v);
        }}
      />
      <Button
        variant="outline"
        size="sm"
        onClick={() => setCompetence(next)}
        disabled={atLatest}
        aria-label={`Próxima competência: ${displayCompetence(next)}`}
        title={atLatest ? "Não há competência gerada depois desta" : undefined}
      >
        {displayCompetence(next)}
        <ChevronRight aria-hidden="true" />
      </Button>
      <Button variant="ghost" size="sm" onClick={() => latest && setCompetence(latest)} disabled={latest === null || competence === latest}>
        <History aria-hidden="true" />
        Mais recente
      </Button>
    </nav>
  );
}
