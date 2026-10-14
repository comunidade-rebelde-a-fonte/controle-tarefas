import type { Company } from "@/services/companies";
import { type FactorRVerdict, useConsultAgent } from "@/services/factorRAgents";
import { VerdictBadge } from "../simulator/VerdictBadge";
import { Chat } from "./Chat";

const VERDICTS: readonly string[] = ["ja_na_meta", "corrigir", "nao_forcar"];

/** Consultor na ficha [FR-13]: perguntas sobre esta empresa (situação, simulação, explicação). */
export function CompanyConsultant({ company }: { company: Company }) {
  const consult = useConsultAgent(company.id);
  return (
    <Chat
      title={`Consultor · ${company.legal_name}`}
      description="O consultor só classifica o pedido e redige a resposta; os números vêm do motor. Uma simulação feita aqui entra no histórico do simulador."
      placeholder="Pergunte sobre esta empresa (situação, simulação, explicação)"
      testId="chat-consultor-ficha"
      send={(m) => consult.mutateAsync(m)}
      pending={consult.isPending}
      renderDecision={(r) => {
        const verdict = r.decisao.veredito;
        return typeof verdict === "string" && VERDICTS.includes(verdict) ? (
          <div className="mt-1">
            <VerdictBadge verdict={verdict as FactorRVerdict} />
          </div>
        ) : null;
      }}
    />
  );
}
