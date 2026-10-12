import { FileText, FolderOpen, KeyRound } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { fichaHref } from "./fichaHref";

function Shortcut({ icon, title, description, children }: { icon: ReactNode; title: string; description: string; children: ReactNode }) {
  return (
    <Card aria-label={title}>
      <CardHeader
        title={
          <span className="flex items-center gap-2">
            {icon}
            {title}
          </span>
        }
        description={description}
      />
      <CardBody className="flex flex-wrap gap-3 text-sm">{children}</CardBody>
    </Card>
  );
}

const linkClass = "inline-flex min-h-6 items-center underline underline-offset-4";

/**
 * Os documentos da empresa em cada módulo (TASK-025, D-039; decisão do usuário): atalhos já
 * filtrados pela empresa, cada um no seu armazenamento. A lista consolidada vem com a área
 * Documentos.
 */
export function DocumentsTab({ companyId }: { companyId: string }) {
  const [params] = useSearchParams();
  const tab = (t: string) => fichaHref(companyId, params, t);
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Shortcut
        icon={<FileText className="size-4" aria-hidden />}
        title="Extratos do PGDAS-D"
        description="Os extratos do Simples Nacional lidos pelo Fator R, com o mês que cada um lançou."
      >
        <Link to={tab("factor-r")} className={linkClass}>
          Ver os extratos da empresa
        </Link>
        <Link to="/factor-r/inbox" className={linkClass}>
          Enviar pelo inbox do PGDAS-D
        </Link>
      </Shortcut>
      <Shortcut
        icon={<FolderOpen className="size-4" aria-hidden />}
        title="Documentos da Conciliação"
        description="Extratos bancários e comprovantes, com a triagem, a leitura e o que ficou em revisão."
      >
        <Link to={`/reconciliation/documents?${new URLSearchParams({ company_id: companyId }).toString()}`} className={linkClass}>
          Ver os documentos da empresa
        </Link>
      </Shortcut>
      <Shortcut
        icon={<KeyRound className="size-4" aria-hidden />}
        title="Procurações e certificados"
        description="Os arquivos e as validades da procuração RFB e do certificado digital."
      >
        <Link to={tab("credentials")} className={linkClass}>
          Ver procurações e certificados
        </Link>
      </Shortcut>
    </div>
  );
}
