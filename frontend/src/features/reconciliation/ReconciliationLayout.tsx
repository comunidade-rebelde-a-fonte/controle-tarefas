import { ClipboardList, FolderOpen, type LucideIcon } from "lucide-react";
import { NavLink, Outlet } from "react-router";
import { cn } from "@/lib/utils";

interface Section {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
}

/** Seções da área Conciliação (TASK-021a: fila e documentos; as demais chegam na 021b e na 021c). */
const SECTIONS: readonly Section[] = [
  { to: "/reconciliation", label: "Fila de revisão", icon: ClipboardList, end: true },
  { to: "/reconciliation/documents", label: "Documentos", icon: FolderOpen },
];

/** Submenu da área Conciliação [08 §7]. */
export function ReconciliationNav() {
  return (
    <nav aria-label="Seções da Conciliação" className="flex gap-1 overflow-x-auto border-b bg-card px-4">
      {SECTIONS.map((s) => (
        <NavLink
          key={s.to}
          to={s.to}
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
