import { CircleAlert, CircleCheck, Info, X } from "lucide-react";
import { Toast as ToastPrimitive } from "radix-ui";
import { useCallback, useMemo, useState, type ReactNode } from "react";
import { cn } from "@/lib/utils";

import { ToastContext, type ToastFn, type ToastOptions } from "./toast-context";

interface ToastItem extends ToastOptions {
  id: number;
}

let nextId = 1;

const icons = { default: Info, success: CircleCheck, error: CircleAlert };

/** Provedor de notificações. Mensagens usam texto + ícone (não só cor) [PRD §45.4]. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const toast = useCallback<ToastFn>((options) => {
    setItems((prev) => [...prev, { id: nextId++, ...options }]);
  }, []);
  const remove = (id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id));
  };
  const value = useMemo(() => toast, [toast]);
  return (
    <ToastContext.Provider value={value}>
      <ToastPrimitive.Provider swipeDirection="right" duration={5000}>
        {children}
        {items.map((t) => {
          const Icon = icons[t.variant ?? "default"];
          return (
            <ToastPrimitive.Root
              key={t.id}
              type={t.variant === "error" ? "foreground" : "background"}
              onOpenChange={(open) => {
                if (!open) remove(t.id);
              }}
              className={cn(
                "flex items-start gap-3 rounded-xl border border-white/10 bg-popover p-3 pr-8 shadow-xl",
                t.variant === "error" && "border-destructive/40",
                t.variant === "success" && "border-success/40",
              )}
            >
              <Icon
                className={cn(
                  "mt-0.5 size-4 shrink-0",
                  t.variant === "error" ? "text-destructive" : t.variant === "success" ? "text-success" : "text-primary",
                )}
                aria-hidden
              />
              <div className="grid gap-0.5">
                <ToastPrimitive.Title className="text-sm font-semibold">{t.title}</ToastPrimitive.Title>
                {t.description ? <ToastPrimitive.Description className="text-sm text-muted-foreground">{t.description}</ToastPrimitive.Description> : null}
              </div>
              <ToastPrimitive.Close className="absolute top-2 right-2 rounded p-1 text-muted-foreground hover:bg-accent" aria-label="Fechar">
                <X className="size-3.5" aria-hidden />
              </ToastPrimitive.Close>
            </ToastPrimitive.Root>
          );
        })}
        <ToastPrimitive.Viewport className="fixed right-4 bottom-4 z-[100] flex w-96 max-w-[calc(100vw-2rem)] flex-col gap-2 outline-none" />
      </ToastPrimitive.Provider>
    </ToastContext.Provider>
  );
}
