import type { ReactNode } from "react";

interface PageHeaderProps {
  title: string;
  description?: string;
  actions?: ReactNode;
}

/** Cabeçalho padrão das páginas. */
export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <header className="flex items-start justify-between gap-4 border-b bg-card px-6 py-4">
      <div>
        <h1 className="font-display text-2xl font-light tracking-tight text-white">{title}</h1>
        {description ? <p className="mt-0.5 text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
    </header>
  );
}
