import type { ComponentProps } from "react";
import { Input } from "@/components/ui/input";

/** Seletor de competência (mês/ano). Valor no formato de armazenamento `AAAA-MM`. */
export function CompetenceInput({ value, onChange, ...props }: Omit<ComponentProps<"input">, "onChange" | "value" | "type"> & { value: string; onChange: (v: string) => void }) {
  return <Input type="month" value={value} onChange={(e) => onChange(e.target.value)} className="w-44" {...props} />;
}
