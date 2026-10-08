import {
  ArrowLeftRight,
  Building2,
  Calculator,
  FileBarChart,
  FileText,
  History,
  KeyRound,
  LayoutDashboard,
  Layers,
  Percent,
  Receipt,
  Settings,
  Users,
  type LucideIcon,
} from "lucide-react";

/** Áreas principais: as 12 do PRD §6 e o Fator R da consolidação (08 §2, TASK-014a). */
export interface Area {
  path: string;
  label: string;
  icon: LucideIcon;
  group: "gestao" | "operacao" | "dados";
}

export const AREAS: readonly Area[] = [
  { path: "/dashboard", label: "Dashboard", icon: LayoutDashboard, group: "gestao" },
  { path: "/companies", label: "Empresas", icon: Building2, group: "gestao" },
  { path: "/fiscal", label: "Fiscal", icon: Receipt, group: "operacao" },
  { path: "/declarations", label: "Declarações", icon: FileText, group: "operacao" },
  { path: "/accounting", label: "Contábil", icon: Calculator, group: "operacao" },
  { path: "/payroll", label: "Folha", icon: Users, group: "operacao" },
  { path: "/dctfweb", label: "DCTFWeb", icon: Layers, group: "operacao" },
  { path: "/factor-r", label: "Fator R", icon: Percent, group: "operacao" },
  { path: "/credentials", label: "Procurações e Certificados", icon: KeyRound, group: "operacao" },
  { path: "/backlog", label: "Backlog", icon: History, group: "gestao" },
  { path: "/settings", label: "Cadastros e Configurações", icon: Settings, group: "dados" },
  { path: "/imports", label: "Importação/Exportação", icon: ArrowLeftRight, group: "dados" },
  { path: "/reports", label: "Relatórios", icon: FileBarChart, group: "dados" },
];

export const AREA_GROUPS: readonly { id: Area["group"]; label: string }[] = [
  { id: "gestao", label: "Gestão" },
  { id: "operacao", label: "Operação mensal" },
  { id: "dados", label: "Dados" },
];
