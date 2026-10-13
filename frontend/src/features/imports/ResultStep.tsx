import { CircleCheck } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import type { ImportResult } from "@/services/imports";

/** Passo final: resumo da importação. */
export function ResultStep({ result, onRestart }: { result: ImportResult; onRestart: () => void }) {
  return (
    <div className="grid max-w-2xl gap-4">
      <div role="status" className="flex items-start gap-2 rounded-lg border border-success/30 bg-success/10 p-4 text-sm text-success">
        <CircleCheck className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <div>
          <p className="font-medium">Importação concluída</p>
          <p>
            {result.created} {result.created === 1 ? "empresa cadastrada" : "empresas cadastradas"}, {result.updated}{" "}
            {result.updated === 1 ? "atualizada" : "atualizadas"}. {result.skipped_invalid} com erro e {result.skipped_existing} existente(s) sem
            confirmação ficaram de fora.
          </p>
        </div>
      </div>
      {result.companies.length > 0 && (
        <ul className="grid gap-1 text-sm" aria-label="Empresas importadas">
          {result.companies.map((c) => (
            <li key={c.company_id}>
              <Link to={`/companies/${c.company_id}`} className="underline underline-offset-2">
                {c.internal_code} — {c.legal_name}
              </Link>{" "}
              <span className="text-xs text-muted-foreground">({c.action === "CREATED" ? "cadastrada" : "atualizada"}, linha {c.line})</span>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2">
        <Button variant="outline" onClick={onRestart}>
          Nova importação
        </Button>
        <Button asChild>
          <Link to="/companies">Ver empresas</Link>
        </Button>
      </div>
    </div>
  );
}
