import { NavLink, Outlet } from "react-router";
import logo from "@/assets/brand/inovasala-by-ceath-branco.png";
import { cn } from "@/lib/utils";
import { ApiStatus } from "./ApiStatus";
import { AREA_GROUPS, AREAS } from "./areas";

/**
 * Casca da aplicação (TASK-029, D-043): o menu lateral com o logo InovaSala by Ceath e o nome do
 * sistema, as áreas na ordem aprovada e o item ativo em destaque (fundo neutral-800 e o acento
 * laranja do Design System).
 */
export function Layout() {
  return (
    <div className="flex h-full">
      <aside className="flex w-60 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground" aria-label="Menu principal">
        <div className="border-b border-sidebar-border px-5 pt-5 pb-4">
          <img src={logo} alt="InovaSala by Ceath" width={144} height={46} className="h-auto w-36" />
          <p className="mt-2 font-display text-sm font-light tracking-tight whitespace-nowrap text-white">Controle de Obrigações</p>
          <p className="text-[11px] text-sidebar-muted">Escritório contábil</p>
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
                          "relative flex items-center gap-2.5 truncate rounded-lg px-2.5 py-1.5 text-sm whitespace-nowrap transition-colors hover:bg-white/5 hover:text-white",
                          isActive && "bg-sidebar-accent font-medium text-white",
                        )
                      }
                    >
                      {({ isActive }) => (
                        <>
                          {isActive ? <span aria-hidden className="absolute top-1/2 left-0 h-4 w-0.5 -translate-y-1/2 rounded-full bg-primary" /> : null}
                          <area.icon className={cn("size-4 shrink-0", isActive && "text-orange-400")} aria-hidden />
                          {area.label}
                        </>
                      )}
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
