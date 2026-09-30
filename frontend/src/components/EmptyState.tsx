import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

/** Estado vazio com orientação e ações [PRD §49]. */
export function EmptyState({ icon: Icon, title, description, children }: { icon: LucideIcon; title: string; description: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center rounded-lg border border-dashed bg-card px-6 py-12 text-center">
      <div className="mb-3 rounded-full bg-primary/10 p-3 text-primary">
        <Icon className="size-6" aria-hidden />
      </div>
      <h2 className="text-base font-semibold">{title}</h2>
      <p className="mt-1 max-w-lg text-sm text-muted-foreground">{description}</p>
      {children ? <div className="mt-5 flex flex-wrap justify-center gap-2">{children}</div> : null}
    </div>
  );
}
