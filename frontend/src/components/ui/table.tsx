import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

export function Table({ className, ...props }: ComponentProps<"table">) {
  return <table className={cn("w-full caption-bottom border-separate border-spacing-0 text-sm", className)} {...props} />;
}

export function Th({ className, ...props }: ComponentProps<"th">) {
  return (
    <th
      className={cn(
        "sticky top-0 z-10 h-9 border-b bg-muted px-2.5 text-left align-middle text-xs font-medium tracking-wide text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}

export function Td({ className, ...props }: ComponentProps<"td">) {
  return <td className={cn("border-b px-2.5 py-2 align-middle", className)} {...props} />;
}
