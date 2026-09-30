import { ModuleScreen, regimeColumn } from "@/components/ModuleGrid";
import { dependenciesColumn } from "./columns";

const EXTRA_COLUMNS = [regimeColumn];
const TRAILING_COLUMNS = [dependenciesColumn];

/**
 * Visão DCTFWeb [PRD §16, §31]: MIT, REINF Retidos, REINF Lucros e eSocial são as mesmas
 * instâncias de Declarações e Folha. A conclusão da DCTFWeb com componentes pendentes passa pelo
 * alerta com exceção. Não há fechamento próprio (plan R1).
 */
export function DctfwebPage() {
  return (
    <ModuleScreen
      module="dctfweb"
      title="DCTFWeb"
      description="Componentes da DCTFWeb e dependências por competência. O fechamento é feito em Declarações."
      moduleLabel="DCTFWeb"
      extraColumns={EXTRA_COLUMNS}
      trailingColumns={TRAILING_COLUMNS}
      showProgress={false}
    />
  );
}
