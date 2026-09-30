import { ModuleScreen, regimeColumn } from "@/components/ModuleGrid";

const EXTRA_COLUMNS = [regimeColumn];

/**
 * Tela Declarações [PRD §15, §29]: colunas dinâmicas do catálogo; o popover da célula mostra
 * vencimento, data de transmissão e protocolo (editáveis quando o tipo registra) e o histórico.
 */
export function DeclarationsPage() {
  return (
    <ModuleScreen
      module="declarations"
      title="Declarações"
      description="Declarações do catálogo por competência, com transmissão e protocolo."
      moduleLabel="Declarações"
      extraColumns={EXTRA_COLUMNS}
      readyToCloseText="Todas as declarações concluídas — sugerido encerrar as Declarações desta competência."
    />
  );
}
