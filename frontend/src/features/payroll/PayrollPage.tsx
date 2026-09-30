import { ModuleScreen } from "@/components/ModuleGrid";
import { PAYROLL_EXTRA_COLUMNS } from "./columns";

/**
 * Tela Folha [PRD §17, §30]: Tipo de Folha e Fator R do snapshot da competência (Fator R só no
 * Simples) e os controles do catálogo. O eSocial é a mesma instância exibida na DCTFWeb.
 */
export function PayrollPage() {
  return (
    <ModuleScreen
      module="payroll"
      title="Folha"
      description="Folha, eSocial, FGTS e INSS/IRRF por competência."
      moduleLabel="Folha"
      extraColumns={PAYROLL_EXTRA_COLUMNS}
      readyToCloseText="Todos os controles concluídos — sugerido encerrar a Folha desta competência."
    />
  );
}
