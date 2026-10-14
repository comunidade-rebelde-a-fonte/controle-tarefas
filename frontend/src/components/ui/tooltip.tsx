import { Tooltip as TooltipPrimitive } from "radix-ui";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";

export const TooltipProvider = TooltipPrimitive.Provider;

export function TooltipContent({ className, sideOffset = 4, ...props }: ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        sideOffset={sideOffset}
        className={cn("z-50 max-w-xs rounded-lg border border-white/10 bg-popover px-2.5 py-1.5 text-xs text-popover-foreground shadow-md", className)}
        {...props}
      />
    </TooltipPrimitive.Portal>
  );
}

/** Tooltip simples: envolve o gatilho e mostra o conteúdo ao passar o mouse ou focar. */
export function Tooltip({ content, children, side }: { content: ReactNode; children: ReactNode; side?: "top" | "right" | "bottom" | "left" }) {
  return (
    <TooltipPrimitive.Root delayDuration={300}>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipContent {...(side ? { side } : {})}>{content}</TooltipContent>
    </TooltipPrimitive.Root>
  );
}
