import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

/**
 * Selos do Design System InovaSala (TASK-029): o destaque laranja ("trending"), a tag neutra e os
 * tons translúcidos dos estados, sempre com texto (nunca só cor).
 */
const badgeVariants = cva("inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium whitespace-nowrap [&_svg]:size-3", {
  variants: {
    variant: {
      default: "border-primary/20 bg-primary/10 text-orange-400",
      secondary: "border-white/10 bg-white/5 text-neutral-300",
      outline: "border-white/10 text-foreground",
      success: "border-success/20 bg-success/10 text-success",
      warning: "border-warning/20 bg-warning/10 text-warning",
      danger: "border-destructive/20 bg-destructive/10 text-destructive",
      muted: "border-white/5 bg-white/5 text-muted-foreground",
    },
  },
  defaultVariants: { variant: "default" },
});

export interface BadgeProps extends ComponentProps<"span">, VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}
