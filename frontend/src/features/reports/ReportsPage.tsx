import { FileDown } from "lucide-react";
import type { ReactNode } from "react";
import { CompetenceNav } from "@/components/CompetenceNav";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { NativeSelect } from "@/components/ui/input";
import { useCompetence } from "@/lib/useCompetence";
import { useUrlParams } from "@/lib/useUrlParams";
import { DashboardFilters } from "@/features/dashboard/DashboardFilters";
import { dashboardQuery } from "@/features/dashboard/query";
import { type ReportName, type ReportQuery, reportUrl } from "@/services/reports";

type Option = { value: string; label: string };

const BACKLOG_MODULES: Option[] = [
  { value: "", label: "Todos os módulos" },
  { value: "FISCAL", label: "Fiscal" },
  { value: "DECLARATIONS", label: "Declarações" },
  { value: "PAYROLL", label: "Folha" },
  { value: "ACCOUNTING", label: "Contábil" },
];

const PENDING_STATES: Option[] = [
  { value: "", label: "Todas em aberto" },
  { value: "pending", label: "Pendentes" },
  { value: "not_started", label: "Não iniciadas" },
  { value: "in_progress", label: "Em andamento" },
  { value: "overdue", label: "Atrasadas" },
];

const GRID_MODULES: Option[] = [
  { value: "fiscal", label: "Fiscal" },
  { value: "declarations", label: "Declarações" },
  { value: "payroll", label: "Folha" },
  { value: "accounting", label: "Contábil" },
  { value: "dctfweb", label: "DCTFWeb" },
];

const FISCAL_GROUPS: Option[] = [
  { value: "", label: "Normais e Simples" },
  { value: "normal", label: "Normais" },
  { value: "simples", label: "Simples Nacional" },
];

const CREDENTIAL_FILTERS: Option[] = [
  { value: "", label: "Vencidos ou a vencer" },
  { value: "expired", label: "Vencidos" },
  { value: "due_30", label: "Vencem em até 30 dias" },
  { value: "awaiting_client", label: "Aguardando cliente" },
  { value: "no_date", label: "Sem data de vencimento" },
];

const CREDENTIAL_TYPES: Option[] = [
  { value: "", label: "Procurações e certificados" },
  { value: "RFB_POWER_OF_ATTORNEY", label: "Procurações RFB" },
  { value: "DIGITAL_CERTIFICATE", label: "Certificados digitais" },
];

/** Um parâmetro próprio do relatório, guardado na URL como `{relatório}.{nome}`. */
interface Param {
  name: string;
  label: string;
  options: Option[];
}

interface ReportDef {
  name: ReportName;
  title: string;
  description: string;
  /** Quais filtros comuns (competência e filtros do topo) o relatório usa. */
  uses: string;
  params: Param[];
  /** Monta a query a partir da competência, dos filtros comuns e dos parâmetros próprios. */
  query: (common: ReportQuery, own: (name: string) => string) => ReportQuery;
}

const REPORTS: ReportDef[] = [
  {
    name: "general",
    title: "Situação geral da competência",
    description: "Indicadores do dashboard e, por empresa, o progresso e o estado de cada módulo, com obrigações em aberto e atrasadas.",
    uses: "Competência, agrupamento, regime e responsável.",
    params: [],
    query: (common) => common,
  },
  {
    name: "pending",
    title: "Pendências por módulo",
    description: "Obrigações ainda não finalizadas na competência, agrupadas por módulo.",
    uses: "Competência, agrupamento, regime e responsável.",
    params: [
      { name: "module", label: "Módulo", options: BACKLOG_MODULES },
      { name: "state", label: "Classe", options: PENDING_STATES },
    ],
    query: (common, own) => ({ ...common, module: own("module"), state: (own("state") || undefined) as ReportQuery["state"] }),
  },
  {
    name: "overdue",
    title: "Obrigações atrasadas",
    description: "Obrigações vencidas e não finalizadas da competência, das mais atrasadas para as menos.",
    uses: "Competência, agrupamento, regime e responsável.",
    params: [{ name: "module", label: "Módulo", options: BACKLOG_MODULES }],
    query: (common, own) => ({ ...common, module: own("module") }),
  },
  {
    name: "backlog",
    title: "Backlog",
    description: "O que ficou em aberto em competências anteriores à selecionada, inclusive módulos ainda não fechados.",
    uses: "Competência de referência, agrupamento, regime e responsável.",
    params: [{ name: "module", label: "Módulo", options: BACKLOG_MODULES }],
    query: (common, own) => ({ ...common, module: own("module") }),
  },
  {
    name: "credentials",
    title: "Procurações e certificados a vencer",
    description: "Documentos vencidos ou dentro da faixa de alerta configurada, dos mais urgentes para os menos.",
    uses: "Não usa os filtros do topo; a competência só aparece no cabeçalho.",
    params: [
      { name: "filter", label: "Situação", options: CREDENTIAL_FILTERS },
      { name: "type", label: "Documento", options: CREDENTIAL_TYPES },
    ],
    query: (common, own) => ({
      competence: common.competence,
      filter: own("filter"),
      type: (own("type") || undefined) as ReportQuery["type"],
    }),
  },
  {
    name: "module",
    title: "Controle completo de um módulo",
    description: "Uma linha por empresa e obrigação, com status, vencimento, atraso e o estado do módulo.",
    uses: "Competência, regime e responsável (no Fiscal, o agrupamento escolhe Normais ou Simples).",
    params: [
      { name: "module", label: "Módulo", options: GRID_MODULES },
      { name: "group", label: "Agrupamento do Fiscal", options: FISCAL_GROUPS },
    ],
    query: (common, own) => {
      const module = own("module") || "fiscal";
      const fromGroup = common.regime_group === "NORMAL" ? "normal" : common.regime_group === "SIMPLES" ? "simples" : undefined;
      const group = module === "fiscal" ? ((own("group") || fromGroup) as ReportQuery["group"]) : undefined;
      return { competence: common.competence, module, group, tax_regime: common.tax_regime, responsible_id: common.responsible_id };
    },
  },
];

function ReportCard({ def, common }: { def: ReportDef; common: ReportQuery | null }) {
  const { get, set } = useUrlParams();
  const key = (name: string) => `${def.name}.${name}`;
  const own = (name: string) => get(key(name));
  const href = common ? reportUrl(def.name, def.query(common, own)) : null;
  const visible = def.params.filter((p) => !(def.name === "module" && p.name === "group" && (own("module") || "fiscal") !== "fiscal"));

  let action: ReactNode = (
    <Button size="sm" disabled>
      <FileDown aria-hidden /> Baixar PDF
    </Button>
  );
  if (href) {
    action = (
      <Button size="sm" asChild>
        <a href={href} download aria-label={`Baixar PDF — ${def.title}`}>
          <FileDown aria-hidden /> Baixar PDF
        </a>
      </Button>
    );
  }

  return (
    <Card aria-labelledby={`report-${def.name}`}>
      <CardHeader title={<span id={`report-${def.name}`}>{def.title}</span>} description={def.description} actions={action} />
      <CardBody className="grid gap-3">
        {visible.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {visible.map((p) => (
              <div key={p.name} className="grid gap-1">
                <label htmlFor={`report-${def.name}-${p.name}`} className="text-xs font-medium">
                  {p.label}
                </label>
                <NativeSelect
                  id={`report-${def.name}-${p.name}`}
                  className="w-56"
                  value={own(p.name)}
                  onChange={(e) => set({ [key(p.name)]: e.target.value })}
                >
                  {p.options.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </NativeSelect>
              </div>
            ))}
          </div>
        ) : null}
        <p className="text-xs text-muted-foreground">{def.uses}</p>
      </CardBody>
    </Card>
  );
}

/**
 * Relatórios [PRD §36]: os 6 relatórios PDF com a competência e os filtros atuais (na URL) e
 * os parâmetros de cada um. O PDF é gerado pela API com as mesmas consultas das telas.
 */
export function ReportsPage() {
  const { competence, error } = useCompetence();
  const { params } = useUrlParams();
  const common: ReportQuery | null = competence ? dashboardQuery(competence, params) : null;
  return (
    <>
      <PageHeader title="Relatórios" description="Relatórios em PDF da competência, com os filtros escolhidos." actions={<CompetenceNav />} />
      <div className="grid gap-4 p-4">
        <DashboardFilters />
        <ErrorAlert error={error} fallback="Não foi possível carregar a competência." />
        <div className="grid grid-cols-2 gap-4">
          {REPORTS.map((def) => (
            <ReportCard key={def.name} def={def} common={common} />
          ))}
        </div>
      </div>
    </>
  );
}
