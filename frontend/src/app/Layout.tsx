import { NavLink, Outlet } from "react-router";
import { cn } from "@/lib/utils";
import { ApiStatus } from "./ApiStatus";
import { AREA_GROUPS, AREAS } from "./areas";

export function Layout() {
  return (
    <div className="flex h-full">
      <aside className="flex w-64 shrink-0 flex-col bg-sidebar text-sidebar-foreground" aria-label="Menu principal">
        <div className="flex h-14 items-center gap-2 border-b border-sidebar-border px-5">
          <img src="/favicon.svg" alt="" className="size-7" />
          <div className="leading-tight">
            <p className="text-sm font-semibold">Controle de Obrigações</p>
            <p className="text-[11px] text-sidebar-muted">Escritório contábil</p>
          </div>
        </div>
        <nav className="flex-1 overflow-y-auto px-3 py-4">
          {AREA_GROUPS.map((group) => (
            <div key={group.id} className="mb-4">
              <p className="px-2 pb-1.5 text-[11px] font-medium tracking-wide text-sidebar-muted uppercase">{group.label}</p>
              <ul className="space-y-0.5">
                {AREAS.filter((a) => a.group === group.id).map((area) => (
                  <li key={area.path}>
                    <NavLink
                      to={area.path}
                      className={({ isActive }) =>
                        cn(
                          "flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-sidebar-accent",
                          isActive && "bg-sidebar-accent font-medium text-white",
                        )
                      }
                    >
                      <area.icon className="size-4 shrink-0" aria-hidden />
                      {area.label}
                    </NavLink>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </nav>
        <div className="border-t border-sidebar-border px-5 py-3">
          <ApiStatus />
          <p className="mt-1 text-[11px] text-sidebar-muted">Usuário: Administrador</p>
        </div>
      </aside>
      <main className="min-w-0 flex-1 overflow-auto">
        <Outlet />
      </main>
    </div>
  );
}
