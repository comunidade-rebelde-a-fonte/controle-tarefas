import { Pencil } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { formatCnpj } from "@/lib/cnpj";
import { formatDate } from "@/lib/format";
import { PAYROLL_TYPE_LABELS, TAX_REGIME_LABELS } from "@/lib/labels";
import { cn } from "@/lib/utils";
import type { Company } from "@/services/companies";
import { CompanyFormDialog } from "../CompanyForm";

function Item({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  // Cada par dt/dd fica numa div filha direta do dl (estrutura válida de lista de definição).
  return (
    <div className={cn("grid gap-0.5", className)}>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm">{children ?? "—"}</dd>
    </div>
  );
}

export function GeneralTab({ company: c }: { company: Company }) {
  const [editing, setEditing] = useState(false);
  return (
    <Card>
      <CardHeader
        title="Dados gerais"
        actions={
          <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
            <Pencil aria-hidden /> Editar dados
          </Button>
        }
      />
      <CardBody>
        <dl className="grid grid-cols-4 gap-x-6 gap-y-4">
          <Item label="Código interno">{c.internal_code}</Item>
          <Item label="CNPJ">{formatCnpj(c.cnpj)}</Item>
          <Item label="Razão social">{c.legal_name}</Item>
          <Item label="Nome fantasia">{c.trade_name}</Item>
          <Item label="Regime vigente">{c.current_tax_regime ? TAX_REGIME_LABELS[c.current_tax_regime] : null}</Item>
          <Item label="Tipo de folha">{PAYROLL_TYPE_LABELS[c.payroll_type]}</Item>
          <Item label="Início do atendimento">{formatDate(c.service_start_date)}</Item>
          <Item label="Fim do atendimento">{c.service_end_date ? formatDate(c.service_end_date) : null}</Item>
          <Item label="Município/UF">{[c.city, c.state].filter(Boolean).join("/") || null}</Item>
          <Item label="Inscrição estadual">{c.state_registration}</Item>
          <Item label="Inscrição municipal">{c.municipal_registration}</Item>
          <Item label="Situação">{c.active ? "Ativa" : "Inativa"}</Item>
          <Item label="Observações cadastrais" className="col-span-4">
            {c.notes ? <span className="whitespace-pre-line">{c.notes}</span> : null}
          </Item>
        </dl>
      </CardBody>
      <CompanyFormDialog open={editing} onOpenChange={setEditing} company={c} />
    </Card>
  );
}
