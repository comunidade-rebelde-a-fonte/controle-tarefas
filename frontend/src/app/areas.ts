import {
  ArrowLeftRight,
  Building2,
  Calculator,
  FileBarChart,
  FileText,
  Files,
  History,
  KeyRound,
  LayoutDashboard,
  Layers,
  Percent,
  Receipt,
  Scale,
  Settings,
  Users,
  type LucideIcon,
} from "lucide-react";

/**
 * Áreas principais, na ordem aprovada da navegação consolidada (08 §2, REQ-050; TASK-029, D-043):
 * as 14 áreas e, no fim, a Importação/Exportação do Controle (PRD §6), num item próprio (decisão
 * do usuário). Os rótulos são os do PRD do Controle.
 */
export interface Area {
  path: string;
  label: string;
  icon: LucideIcon;
  group: "visao" | "operacao" | "modulos" | "acompanhamento" | "sistema";
}

export const AREAS: readonly Area[] = [
  { path: "/dashboard", label: "Dashboard", icon: LayoutDashboard, group: "visao" },
  { path: "/companies", label: "Empresas", icon: Building2, group: "visao" },
  { path: "/fiscal", label: "Fiscal", icon: Receipt, group: "operacao" },
  { path: "/declarations", label: "Declarações", icon: FileText, group: "operacao" },
  { path: "/accounting", label: "Contábil", icon: Calculator, group: "operacao" },
  { path: "/payroll", label: "Folha", icon: Users, group: "operacao" },
  { path: "/dctfweb", label: "DCTFWeb", icon: Layers, group: "operacao" },
  { path: "/factor-r", label: "Fator R", icon: Percent, group: "modulos" },
  { path: "/reconciliation", label: "Conciliação", icon: Scale, group: "modulos" },
  { path: "/credentials", label: "Procurações e Certificados", icon: KeyRound, group: "acompanhamento" },
  { path: "/backlog", label: "Backlog", icon: History, group: "acompanhamento" },
  { path: "/documents", label: "Documentos", icon: Files, group: "acompanhamento" },
  { path: "/reports", label: "Relatórios", icon: FileBarChart, group: "acompanhamento" },
  { path: "/settings", label: "Cadastros e Configurações", icon: Settings, group: "sistema" },
  { path: "/imports", label: "Importação/Exportação", icon: ArrowLeftRight, group: "sistema" },
];

export const AREA_GROUPS: readonly { id: Area["group"]; label: string }[] = [
  { id: "visao", label: "Visão geral" },
  { id: "operacao", label: "Operação mensal" },
  { id: "modulos", label: "Fator R e Conciliação" },
  { id: "acompanhamento", label: "Acompanhamento" },
  { id: "sistema", label: "Sistema" },
];
