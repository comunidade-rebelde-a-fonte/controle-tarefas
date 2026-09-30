import { createContext, useContext } from "react";

export type ToastVariant = "default" | "success" | "error";

export interface ToastOptions {
  title: string;
  description?: string;
  variant?: ToastVariant;
}

export type ToastFn = (options: ToastOptions) => void;

export const ToastContext = createContext<ToastFn | null>(null);

/** Dispara uma notificação. */
export function useToast(): ToastFn {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast fora do ToastProvider");
  return ctx;
}
