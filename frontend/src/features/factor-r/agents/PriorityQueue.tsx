import { Link } from "react-router";
import { Table, Td, Th } from "@/components/ui/table";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate } from "@/lib/decimal";
import type { FactorRLight } from "@/services/factorR";
import type { FactorRAgentResponse } from "@/services/factorRAgents";
import { LightBadge } from "../common";

interface QueueItem {
  posicao: number;
  company_id: string;
  empresa: string;
  semaforo: FactorRLight;
  fator_r: string | null;
  economia_12m: string | null;
  acao: string;
}

/** Fila do priorizador [FR-14]: vermelhos e amarelos por economia, com os valores do motor. */
export function PriorityQueue({ response }: { response: FactorRAgentResponse }) {
  const fila = (response.decisao.fila ?? []) as QueueItem[];
  const pa = typeof response.decisao.pa === "string" ? response.decisao.pa : null;
  if (fila.length === 0) return null;
  return (
    <div className="mt-2 overflow-x-auto rounded-md border bg-card">
      <Table aria-label={`Fila do priorizador${pa ? ` — PA ${displayCompetence(pa)}` : ""}`} data-testid="fila-priorizador">
        <thead>
          <tr>
            <Th scope="col">#</Th>
            <Th scope="col">Empresa</Th>
            <Th scope="col">Semáforo</Th>
            <Th scope="col" className="text-right">
              Fator R
            </Th>
            <Th scope="col" className="text-right">
              Economia 12 meses
            </Th>
            <Th scope="col">Ação</Th>
          </tr>
        </thead>
        <tbody>
          {fila.map((item) => (
            <tr key={item.company_id}>
              <Td className="tabular-nums">{item.posicao}</Td>
              <Td>
                <Link to={`/companies/${item.company_id}?tab=factor-r${pa ? `&competence=${pa}` : ""}`} className="font-medium hover:underline">
                  {item.empresa}
                </Link>
              </Td>
              <Td>
                <LightBadge light={item.semaforo} />
              </Td>
              <Td className="text-right tabular-nums">{formatRate(item.fator_r)}</Td>
              <Td className="text-right tabular-nums">{formatMoney(item.economia_12m)}</Td>
              <Td className="whitespace-nowrap">{item.acao}</Td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
}
