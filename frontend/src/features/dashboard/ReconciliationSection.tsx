import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { displayCompetence } from "@/lib/competence";
import type { Dashboard } from "@/services/dashboard";
import { StatTile } from "./StatTile";

/**
 * A seção Conciliação do Dashboard (PRD §8, 08 §3; TASK-024, D-038), com os filtros do Dashboard
 * (o responsável é o do Contábil). Os documentos contam todas as competências; a fila e as
 * empresas são as da competência (decisões do usuário). Cada número abre a lista que o explica.
 */
export function ReconciliationSection({ section: s, competence }: { section: Dashboard["reconciliation"]; competence: string }) {
  const shown = displayCompetence(competence);
  return (
    <Card aria-label="Conciliação">
      <CardHeader title="Conciliação" description={`Documentos de todas as competências; fila de revisão e empresas de ${shown}.`} />
      <CardBody className="grid gap-4">
        <div role="group" aria-label="Documentos da Conciliação" className="flex flex-wrap gap-3">
          <StatTile label="Processando" value={String(s.processing.value)} metric={s.processing} hint="todas as competências" />
          <StatTile
            label="Em revisão de triagem ou extração"
            value={String(s.extraction_review.value)}
            metric={s.extraction_review}
            hint="todas as competências"
            tone={s.extraction_review.value > 0 ? "warning" : undefined}
          />
          <StatTile
            label="Parados por período fechado"
            value={String(s.closed_period.value)}
            metric={s.closed_period}
            hint="todas as competências"
            tone={s.closed_period.value > 0 ? "warning" : undefined}
          />
        </div>
        <div role="group" aria-label={`Fila de revisão de ${shown}`} className="flex flex-wrap gap-3">
          <StatTile label="Na fila de revisão" value={String(s.queue.value)} metric={s.queue} />
          <StatTile label="Divergências" value={String(s.divergent.value)} metric={s.divergent} tone={s.divergent.value > 0 ? "danger" : undefined} />
          <StatTile label="Extrato sem comprovante" value={String(s.without_receipt.value)} metric={s.without_receipt} />
          <StatTile label="Comprovante sem extrato" value={String(s.without_statement.value)} metric={s.without_statement} />
        </div>
        <div role="group" aria-label={`Empresas de ${shown}`} className="flex flex-wrap gap-3">
          <StatTile label="Prontas para concluir" value={String(s.ready.value)} metric={s.ready} tone={s.ready.value > 0 ? "success" : undefined} />
          <StatTile
            label="Contábil bloqueado pela Conciliação"
            value={String(s.blocks_accounting.value)}
            metric={s.blocks_accounting}
            hint="Contábil aberto e a Conciliação não concluída"
            tone={s.blocks_accounting.value > 0 ? "warning" : undefined}
          />
        </div>
      </CardBody>
    </Card>
  );
}
