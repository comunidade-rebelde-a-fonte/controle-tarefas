import { ArrowLeft, Pencil, PowerOff } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { CompanySelect } from "@/components/CompanySelect";
import { CompetenceNav } from "@/components/CompetenceNav";
import { CopyButton, CopyCnpjButton } from "@/components/CopyButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { FactorRTab } from "@/features/factor-r/FactorRTab";
import { ReconciliationTab } from "@/features/reconciliation/company/ReconciliationTab";
import { formatCnpj } from "@/lib/cnpj";
import { TAX_REGIME_LABELS } from "@/lib/labels";
import { useCompetence } from "@/lib/useCompetence";
import { type Company, useCompany } from "@/services/companies";
import { DeactivateCompanyDialog, EditCompanyDialog } from "../CompanyActions";
import { CredentialsTab } from "./CredentialsTab";
import { DocumentsTab } from "./DocumentsTab";
import { GeneralTab } from "./GeneralTab";
import { HistoryTab } from "./HistoryTab";
import { ModulesTab } from "./ModulesTab";
import { ObligationsTab } from "./ObligationsTab";
import { OverridesTab } from "./OverridesTab";
import { OverviewTab } from "./OverviewTab";
import { PayrollTab } from "./PayrollTab";
import { RegimeTab } from "./RegimeTab";
import { ResponsiblesTab } from "./ResponsiblesTab";

interface Section {
  id: string;
  label: string;
  /** Subabas: a primeira abre ao escolher a seção. */
  subs?: readonly (readonly [string, string])[];
}

/**
 * As seções da ficha (08 §5, PRD §9; TASK-025, D-039, decisão do usuário): as telas de antes
 * viraram subabas de Dados gerais e de Obrigações. A aba fica na URL (`?tab=`) pelo id da subaba,
 * e os links antigos (`?tab=regime`, `?tab=payroll` etc.) abrem a subaba certa.
 */
const SECTIONS: readonly Section[] = [
  { id: "overview", label: "Visão geral" },
  {
    id: "data",
    label: "Dados gerais",
    subs: [
      ["general", "Cadastro"],
      ["regime", "Regime tributário"],
      ["modules", "Módulos"],
      ["responsibles", "Responsáveis"],
      ["payroll", "Folha"],
    ],
  },
  {
    id: "obligations-section",
    label: "Obrigações",
    subs: [
      ["obligations", "Da competência"],
      ["applicability", "Aplicabilidade"],
    ],
  },
  { id: "factor-r", label: "Fator R" },
  { id: "reconciliation", label: "Conciliação" },
  { id: "documents", label: "Documentos" },
  { id: "credentials", label: "Procurações/Certificados" },
  { id: "history", label: "Histórico de competências" },
];

const LEAVES = SECTIONS.flatMap((s) => (s.subs ? s.subs.map(([id]) => id) : [s.id]));

function sectionOf(leaf: string): Section {
  return SECTIONS.find((s) => s.id === leaf || s.subs?.some(([id]) => id === leaf)) ?? (SECTIONS[0] as Section);
}

function firstLeaf(s: Section): string {
  return s.subs?.[0]?.[0] ?? s.id;
}

/** O cabeçalho único: identidade, competência da ficha, troca de empresa e ações rápidas. */
function Header({ company }: { company: Company }) {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [editing, setEditing] = useState(false);
  const [deactivating, setDeactivating] = useState(false);
  return (
    <header className="border-b bg-card px-6 pt-4 pb-3">
      <Link to="/companies" className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
        <ArrowLeft className="size-3.5" aria-hidden /> Empresas
      </Link>
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <h1 className="text-lg font-semibold tracking-tight">{company.legal_name}</h1>
        {company.current_tax_regime ? <Badge>{TAX_REGIME_LABELS[company.current_tax_regime]}</Badge> : null}
        {company.active ? <Badge variant="success">Ativa</Badge> : <Badge variant="muted">Inativa</Badge>}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <Button size="sm" variant="outline" onClick={() => setEditing(true)}>
            <Pencil aria-hidden /> Editar
          </Button>
          {company.active ? (
            <Button size="sm" variant="outline" onClick={() => setDeactivating(true)}>
              <PowerOff aria-hidden /> Inativar
            </Button>
          ) : null}
        </div>
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
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <CompetenceNav unbounded />
        <label className="ml-auto flex items-center gap-2 text-xs font-medium text-muted-foreground">
          Trocar de empresa
          <CompanySelect
            value={company.id}
            className="w-72"
            onChange={(id) => {
              // A aba e a competência ficam; tudo passa a ser da empresa nova.
              if (id && id !== company.id) void navigate({ pathname: `/companies/${id}`, search: params.toString() });
            }}
          />
        </label>
      </div>
      {editing ? <EditCompanyDialog companyId={company.id} onClose={() => setEditing(false)} /> : null}
      {deactivating ? <DeactivateCompanyDialog company={company} onClose={() => setDeactivating(false)} /> : null}
    </header>
  );
}

/**
 * Ficha da empresa como hub (TASK-025, D-039): o cabeçalho único com a competência da ficha e a
 * troca de empresa, e as seções Visão geral, Dados gerais, Obrigações, Fator R, Conciliação,
 * Documentos, Procurações/Certificados e Histórico, todas da mesma empresa (REQ-002, REQ-036).
 * O conteúdo é refeito ao trocar de empresa: nenhuma aba guarda estado da empresa anterior.
 */
export function CompanyDetailPage() {
  const { id = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const { competence } = useCompetence();
  const { data: company, isPending, error } = useCompany(id);

  // Links de antes levavam o PA do Fator R em `?pa=`: na ficha, ele é a competência.
  const legacyPA = params.get("pa");
  useEffect(() => {
    if (legacyPA === null) return;
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (!next.get("competence")) next.set("competence", legacyPA);
        next.delete("pa");
        return next;
      },
      { replace: true },
    );
  }, [legacyPA, setParams]);

  const rawTab = params.get("tab");
  const leaf = rawTab && LEAVES.includes(rawTab) ? rawTab : "overview";
  const section = sectionOf(leaf);
  const setLeaf = (v: string) =>
    setParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (v === "overview") next.delete("tab");
        else next.set("tab", v);
        // Os filtros de uma aba não passam para a outra.
        next.delete("state");
        next.delete("status");
        return next;
      },
      { replace: true },
    );

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

  const subTabs = (s: Section, render: (leaf: string) => ReactNode) => (
    <Tabs value={leaf} onValueChange={setLeaf}>
      <TabsList aria-label={s.label}>
        {(s.subs ?? []).map(([value, label]) => (
          <TabsTrigger key={value} value={value}>
            {label}
          </TabsTrigger>
        ))}
      </TabsList>
      {(s.subs ?? []).map(([value]) => (
        <TabsContent key={value} value={value}>
          {render(value)}
        </TabsContent>
      ))}
    </Tabs>
  );

  return (
    <>
      <Header company={company} />
      {/* key: trocar de empresa refaz todas as abas (identidade única, TEST-025). */}
      <Tabs key={company.id} value={section.id} onValueChange={(v) => setLeaf(firstLeaf(sectionOf(v)))}>
        <TabsList aria-label="Seções da ficha">
          {SECTIONS.map((s) => (
            <TabsTrigger key={s.id} value={s.id}>
              {s.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="overview">
          <OverviewTab companyId={company.id} competence={competence} />
        </TabsContent>
        <TabsContent value="data">
          {subTabs(SECTIONS[1] as Section, (l) => {
            switch (l) {
              case "general":
                return <GeneralTab company={company} />;
              case "regime":
                return <RegimeTab companyId={company.id} />;
              case "modules":
                return <ModulesTab companyId={company.id} />;
              case "responsibles":
                return <ResponsiblesTab key={company.version} company={company} />;
              default:
                return <PayrollTab key={company.version} company={company} />;
            }
          })}
        </TabsContent>
        <TabsContent value="obligations-section">
          {subTabs(SECTIONS[2] as Section, (l) =>
            l === "obligations" ? <ObligationsTab companyId={company.id} competence={competence} /> : <OverridesTab companyId={company.id} />,
          )}
        </TabsContent>
        <TabsContent value="factor-r">
          <FactorRTab company={company} pa={competence} />
        </TabsContent>
        <TabsContent value="reconciliation">
          <ReconciliationTab company={company} competence={competence} />
        </TabsContent>
        <TabsContent value="documents">
          <DocumentsTab companyId={company.id} />
        </TabsContent>
        <TabsContent value="credentials">
          <CredentialsTab companyId={company.id} />
        </TabsContent>
        <TabsContent value="history">
          <HistoryTab companyId={company.id} internalCode={company.internal_code} />
        </TabsContent>
      </Tabs>
    </>
  );
}
