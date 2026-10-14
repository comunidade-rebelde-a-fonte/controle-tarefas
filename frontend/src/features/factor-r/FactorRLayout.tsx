import { Activity, Inbox, MessagesSquare, Percent, type LucideIcon } from "lucide-react";
import { NavLink, Outlet, useSearchParams } from "react-router";
import { cn } from "@/lib/utils";
import { PA_PARAM } from "./usePA";

interface Section {
  to: string;
  label: string;
  icon: LucideIcon;
  end?: boolean;
  /** A seção usa o PA da URL: o link leva o PA escolhido. */
  keepsPA?: boolean;
}

/** Seções da área Fator R, como o menu da origem (carteira, inbox, agentes e observabilidade). */
const SECTIONS: readonly Section[] = [
  { to: "/factor-r", label: "Carteira", icon: Percent, end: true, keepsPA: true },
  { to: "/factor-r/inbox", label: "Inbox PGDAS-D", icon: Inbox },
  { to: "/factor-r/agents", label: "Agentes", icon: MessagesSquare, keepsPA: true },
  { to: "/factor-r/runs", label: "Observabilidade", icon: Activity },
];

/** Submenu da área Fator R [08 §6; D-025]. */
export function FactorRNav() {
  const [params] = useSearchParams();
  const pa = params.get(PA_PARAM);
  return (
    <nav aria-label="Seções do Fator R" className="flex gap-1 overflow-x-auto border-b bg-card px-4">
      {SECTIONS.map((s) => (
        <NavLink
          key={s.to}
          to={s.keepsPA && pa ? `${s.to}?${PA_PARAM}=${pa}` : s.to}
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

/** Área Fator R: submenu e a seção escolhida. */
export function FactorRLayout() {
  return (
    <>
      <FactorRNav />
      <Outlet />
    </>
  );
}
