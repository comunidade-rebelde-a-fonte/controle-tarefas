import { useId, type ReactElement, type ReactNode } from "react";
import { cloneElement, isValidElement } from "react";
import { cn } from "@/lib/utils";
import { Label } from "./input";

export interface FieldControlProps {
  id: string;
  "aria-invalid": boolean | undefined;
  "aria-describedby": string | undefined;
}

interface FieldProps {
  label: string;
  error?: string | undefined;
  hint?: ReactNode;
  required?: boolean;
  className?: string;
  /** Controle do campo, ou função que recebe id/aria (útil com Controller). */
  children: ReactElement<Partial<FieldControlProps>> | ((props: FieldControlProps) => ReactNode);
}

/** Campo de formulário acessível: label associado, erro anunciado e aria-invalid. */
export function Field({ label, error, hint, required, className, children }: FieldProps) {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const describedBy = [error ? errorId : null, hint ? hintId : null].filter(Boolean).join(" ") || undefined;
  const controlProps: FieldControlProps = { id, "aria-invalid": error ? true : undefined, "aria-describedby": describedBy };
  const control =
    typeof children === "function" ? children(controlProps) : isValidElement(children) ? cloneElement(children, controlProps) : children;
  return (
    <div className={cn("grid gap-1.5", className)}>
      <Label htmlFor={id}>
        {label}
        {required ? <span className="text-destructive"> *</span> : null}
      </Label>
      {control}
      {hint ? (
        <p id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} className="text-xs font-medium text-destructive" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
