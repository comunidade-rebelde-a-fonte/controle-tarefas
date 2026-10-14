import { PageHeader } from "@/components/PageHeader";
import { useConsultAgent, usePrioritizeAgent } from "@/services/factorRAgents";
import { FactorRDisclaimer, PANav } from "../common";
import { usePA } from "../usePA";
import { Chat } from "./Chat";
import { PriorityQueue } from "./PriorityQueue";

/**
 * Agentes do Fator R [REQ-014, FR-13, FR-14]: o priorizador monta a fila da carteira do PA e o
 * consultor responde sobre a empresa citada. Os agentes só classificam o pedido e redigem o
 * texto; todo número vem do motor, e cada resposta tem trace.
 */
export function AgentsPage() {
  const { pa } = usePA();
  const prioritize = usePrioritizeAgent(pa);
  const consult = useConsultAgent();
  return (
    <>
      <PageHeader
        title="Agentes do Fator R"
        description="Os agentes só classificam o pedido e redigem a resposta. Todo cálculo vem do motor do sistema, e cada resposta tem trace."
        actions={<PANav />}
      />
      <div className="grid grid-cols-1 gap-4 p-4">
        <div className="grid grid-cols-1 items-start gap-4 xl:grid-cols-2">
          <Chat
            title="Priorizador"
            description="Fila da carteira do PA escolhido: Anexo V e entre 28% e a meta, por economia de DAS."
            placeholder="O que priorizar esta semana?"
            testId="chat-priorizador"
            send={(m) => prioritize.mutateAsync(m)}
            pending={prioritize.isPending}
            renderDecision={(r) => <PriorityQueue response={r} />}
          />
          <Chat
            title="Consultor"
            description="Situação, simulação ou explicação de uma empresa: cite o nome ou o CNPJ."
            placeholder="Como está a empresa X? Simule a correção da empresa Y."
            testId="chat-consultor"
            send={(m) => consult.mutateAsync(m)}
            pending={consult.isPending}
          />
        </div>
        <FactorRDisclaimer />
      </div>
    </>
  );
}
