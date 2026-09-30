import { ModuleScreen, regimeColumn } from "@/components/ModuleGrid";
import type { GridColumn } from "@/services/grids";

const EXTRA_COLUMNS = [regimeColumn];

/** Etapas numeradas na ordem da sequência do catálogo (1 → 6) [PRD §18.2, §32]. */
const stepHeader = (column: GridColumn, index: number) => `${index + 1}. ${column.obligation_type.name}`;

/**
 * Tela Contábil [PRD §18, §32]: colunas na ordem da sequência; quando uma etapa anterior está
 * pendente, o "OK" fica desabilitado com a mensagem da API (§18.3). O backend revalida e
 * devolve 422 se a sequência for violada.
 */
export function AccountingPage() {
  return (
    <ModuleScreen
      module="accounting"
      title="Contábil"
      description="Etapas contábeis em sequência por competência."
      moduleLabel="Contábil"
      extraColumns={EXTRA_COLUMNS}
      columnHeader={stepHeader}
      readyToCloseText="Todas as etapas concluídas — sugerido encerrar o Contábil desta competência."
    />
  );
}
