import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Table, Td, Th } from "@/components/ui/table";
import { formatPercent } from "@/lib/format";
import { MODULE_LABELS, type ModuleCode } from "@/lib/labels";
import type { Dashboard } from "@/services/dashboard";

/** Progresso por responsável e módulo (responsável do snapshot) [PRD §24.5]. */
export function ResponsibleTable({ rows }: { rows: Dashboard["responsibles"] }) {
  return (
    <Card>
      <CardHeader title="Progresso por responsável" description="Responsável de cada módulo no snapshot da competência." />
      <CardBody className="p-0">
        <Table aria-label="Progresso por responsável">
          <thead>
            <tr>
              <Th>Responsável</Th>
              <Th>Módulo</Th>
              <Th className="text-right">Aplicáveis</Th>
              <Th className="text-right">Concluídas</Th>
              <Th className="text-right">Pendentes</Th>
              <Th className="text-right">Atrasadas</Th>
              <Th className="text-right">Conclusão</Th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={`${r.user?.id ?? "none"}:${r.module}`}>
                <Td>{r.user?.name ?? <span className="text-muted-foreground">Sem responsável</span>}</Td>
                <Td>{r.module in MODULE_LABELS ? MODULE_LABELS[r.module as ModuleCode] : r.module}</Td>
                <Td className="text-right tabular-nums">{r.applicable}</Td>
                <Td className="text-right tabular-nums">{r.done}</Td>
                <Td className="text-right tabular-nums">{r.pending}</Td>
                <Td className="text-right tabular-nums">{r.overdue}</Td>
                <Td className="text-right tabular-nums">{formatPercent(r.final_completion)}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
      </CardBody>
    </Card>
  );
}
