import { ArrowLeft } from "lucide-react";
import { Link, useParams, useSearchParams } from "react-router";
import { CopyButton, CopyCnpjButton } from "@/components/CopyButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatCnpj } from "@/lib/cnpj";
import { TAX_REGIME_LABELS } from "@/lib/labels";
import { useCompany } from "@/services/companies";
import { CredentialsTab } from "./CredentialsTab";
import { GeneralTab } from "./GeneralTab";
import { ModulesTab } from "./ModulesTab";
import { OverridesTab } from "./OverridesTab";
import { PayrollTab } from "./PayrollTab";
import { RegimeTab } from "./RegimeTab";
import { ResponsiblesTab } from "./ResponsiblesTab";

const TABS = [
  ["general", "Dados gerais"],
  ["regime", "Regime tributário"],
  ["modules", "Módulos"],
  ["obligations", "Obrigações/aplicabilidade"],
  ["payroll", "Folha"],
  ["responsibles", "Responsáveis"],
  ["credentials", "Procurações/Certificados"],
  ["history", "Histórico de competências"],
] as const;

type TabId = (typeof TABS)[number][0];

function Placeholder({ text }: { text: string }) {
  return <p className="rounded-lg border border-dashed bg-card p-6 text-sm text-muted-foreground">{text}</p>;
}

/** Ficha da empresa com as 8 abas [PRD §27.3]. A aba ativa fica na URL. */
export function CompanyDetailPage() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const tab = (TABS.some(([t]) => t === params.get("tab")) ? params.get("tab") : "general") as TabId;
  const { data: company, isPending, error } = useCompany(id);

  if (error) {
    return (
      <div className="p-6">
        <ErrorAlert error={error} />
      </div>
    );
  }
  if (isPending) {
    return (
      <div className="space-y-3 p-6">
        <Skeleton className="h-10 w-96" />
        <Skeleton className="h-64" />
      </div>
    );
  }

  return (
    <>
      <header className="border-b bg-card px-6 pt-4 pb-3">
        <Link to="/companies" className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-3.5" aria-hidden /> Empresas
        </Link>
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <h1 className="text-lg font-semibold tracking-tight">{company.legal_name}</h1>
          {company.current_tax_regime ? <Badge>{TAX_REGIME_LABELS[company.current_tax_regime]}</Badge> : null}
          {company.active ? <Badge variant="success">Ativa</Badge> : <Badge variant="muted">Inativa</Badge>}
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-4 text-sm text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            Código <span className="font-mono text-foreground">{company.internal_code}</span>
            <CopyButton value={company.internal_code} label="Copiar código" showLabel />
          </span>
          <span className="inline-flex items-center gap-1">
            CNPJ <span className="font-mono text-foreground">{formatCnpj(company.cnpj)}</span>
            <CopyCnpjButton cnpj={company.cnpj} showLabel />
          </span>
          {company.trade_name ? <span>{company.trade_name}</span> : null}
        </div>
      </header>
      <Tabs
        value={tab}
        onValueChange={(v) =>
          setParams(
            (prev) => {
              const next = new URLSearchParams(prev);
              if (v === "general") next.delete("tab");
              else next.set("tab", v);
              return next;
            },
            { replace: true },
          )
        }
      >
        <TabsList aria-label="Seções da ficha">
          {TABS.map(([value, label]) => (
            <TabsTrigger key={value} value={value}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="general">
          <GeneralTab company={company} />
        </TabsContent>
        <TabsContent value="regime">
          <RegimeTab companyId={company.id} />
        </TabsContent>
        <TabsContent value="modules">
          <ModulesTab companyId={company.id} />
        </TabsContent>
        <TabsContent value="obligations">
          <OverridesTab companyId={company.id} />
        </TabsContent>
        <TabsContent value="payroll">
          <PayrollTab key={company.version} company={company} />
        </TabsContent>
        <TabsContent value="responsibles">
          <ResponsiblesTab key={company.version} company={company} />
        </TabsContent>
        <TabsContent value="credentials">
          <CredentialsTab companyId={company.id} />
        </TabsContent>
        <TabsContent value="history">
          <Placeholder text="Histórico de competências (estado aberto/fechado por módulo) — disponível a partir de T-M5-08." />
        </TabsContent>
      </Tabs>
    </>
  );
}
