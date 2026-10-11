import { BookOpen, ClipboardList, FolderOpen, ListTree, type LucideIcon, ReceiptText, Wand2 } from "lucide-react";
import { NavLink, Outlet, useSearchParams } from "react-router";
import { cn } from "@/lib/utils";

interface Section {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
}

/** Seções da área Conciliação (TASK-021a e 021b; playbooks e métricas chegam na 021c). */
const SECTIONS: readonly Section[] = [
  { to: "/reconciliation", label: "Fila de revisão", icon: ClipboardList, end: true },
  { to: "/reconciliation/documents", label: "Documentos", icon: FolderOpen },
  { to: "/reconciliation/entries", label: "Lançamentos", icon: ReceiptText },
  { to: "/reconciliation/ledger", label: "Razão", icon: BookOpen },
  { to: "/reconciliation/chart", label: "Plano de contas", icon: ListTree },
  { to: "/reconciliation/rules", label: "Regras", icon: Wand2 },
];

/** Submenu da área Conciliação [08 §7]; a empresa escolhida acompanha a troca de seção. */
export function ReconciliationNav() {
  const [params] = useSearchParams();
  const company = params.get("company_id");
  const search = company ? `?${new URLSearchParams({ company_id: company }).toString()}` : "";
  return (
    <nav aria-label="Seções da Conciliação" className="flex gap-1 overflow-x-auto border-b bg-card px-4">
      {SECTIONS.map((s) => (
        <NavLink
          key={s.to}
          to={{ pathname: s.to, search }}
          end={s.end ?? false}
          className={({ isActive }) =>
            cn(
              "inline-flex h-10 items-center gap-1.5 border-b-2 px-3 text-sm whitespace-nowrap transition-colors focus-visible:outline-2 focus-visible:outline-ring",
              isActive ? "border-primary font-medium text-foreground" : "border-transparent text-muted-foreground hover:text-foreground",
            )
          }
        >
          <s.icon className="size-4" aria-hidden />
          {s.label}
        </NavLink>
      ))}
    </nav>
  );
}

/** Área Conciliação: submenu e a seção escolhida. */
export function ReconciliationLayout() {
  return (
    <>
      <ReconciliationNav />
      <Outlet />
    </>
  );
}
