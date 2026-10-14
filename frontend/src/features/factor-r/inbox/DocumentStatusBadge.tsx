import { Badge } from "@/components/ui/badge";
import type { PgdasDocument } from "@/services/factorRInbox";
import { documentState } from "./state";

/** Estado do documento do inbox, com ícone e texto além da cor. */
export function DocumentStatusBadge({ document }: { document: Pick<PgdasDocument, "status" | "job"> }) {
  const { key, label, variant, Icon } = documentState(document);
  return (
    <Badge variant={variant} data-estado={key}>
      <Icon aria-hidden className={key === "processando" ? "animate-spin motion-reduce:animate-none" : undefined} /> {label}
    </Badge>
  );
}
