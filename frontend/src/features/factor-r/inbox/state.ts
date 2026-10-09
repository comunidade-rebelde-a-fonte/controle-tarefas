import { Ban, CircleCheck, CircleX, Clock, FileCheck, FileInput, LoaderCircle, RotateCw, TriangleAlert, type LucideIcon } from "lucide-react";
import type { BadgeProps } from "@/components/ui/badge";
import type { PgdasDocument } from "@/services/factorRInbox";

type Variant = NonNullable<BadgeProps["variant"]>;

interface StateView {
  key: string;
  label: string;
  variant: Variant;
  Icon: LucideIcon;
}

/**
 * Estado visual do documento [08 §6]: recebido, processando (na fila, em execução, nova
 * tentativa ou falha do job), extraído, em revisão, vinculado e rejeitado. Sempre com ícone e
 * texto além da cor.
 */
export function documentState(d: Pick<PgdasDocument, "status" | "job">): StateView {
  switch (d.status) {
    case "received":
      switch (d.job?.status) {
        case "pending":
          return { key: "na_fila", label: "Na fila", variant: "muted", Icon: Clock };
        case "running":
          return { key: "processando", label: "Processando", variant: "default", Icon: LoaderCircle };
        case "retry":
          return { key: "nova_tentativa", label: "Nova tentativa", variant: "warning", Icon: RotateCw };
        case "dead":
          return { key: "falhou", label: "Falha no processamento", variant: "danger", Icon: CircleX };
        default:
          return { key: "recebido", label: "Recebido", variant: "muted", Icon: FileInput };
      }
    case "parsed":
      return { key: "extraido", label: "Extraído", variant: "default", Icon: FileCheck };
    case "needs_review":
      return { key: "revisao", label: "Em revisão", variant: "warning", Icon: TriangleAlert };
    case "linked":
      return { key: "vinculado", label: "Vinculado", variant: "success", Icon: CircleCheck };
    case "rejected":
      return { key: "rejeitado", label: "Rejeitado", variant: "muted", Icon: Ban };
  }
}
