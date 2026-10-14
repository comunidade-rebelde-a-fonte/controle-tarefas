import { CircleAlert, CircleCheck, CircleDashed, CloudCheck, CloudOff, CloudUpload, TriangleAlert, UserCheck } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { AIHumanEvalRating, AIRunStatus, AISyncStatus } from "@/services/aiRuns";
import { RATING_LABELS, RUN_STATUS_LABELS, SYNC_LABELS } from "./labels";

const RUN_STATUS = {
  ok: { variant: "success", Icon: CircleCheck },
  needs_review: { variant: "warning", Icon: TriangleAlert },
  error: { variant: "danger", Icon: CircleAlert },
} as const;

/** Resultado da corrida (ok, em revisão, erro), com ícone e texto. */
export function RunStatusBadge({ status }: { status: AIRunStatus }) {
  const { variant, Icon } = RUN_STATUS[status];
  return (
    <Badge variant={variant} data-status={status}>
      <Icon aria-hidden /> {RUN_STATUS_LABELS[status]}
    </Badge>
  );
}

const SYNC = {
  pending: { variant: "muted", Icon: CloudUpload },
  ok: { variant: "success", Icon: CloudCheck },
  failed: { variant: "danger", Icon: CloudOff },
} as const;

/** Sincronização com o Langfuse (a trilha local vale sempre). */
export function SyncBadge({ status }: { status: AISyncStatus }) {
  const { variant, Icon } = SYNC[status];
  return (
    <Badge variant={variant}>
      <Icon aria-hidden /> {SYNC_LABELS[status]}
    </Badge>
  );
}

const RATING = {
  acerto: { variant: "success", Icon: CircleCheck },
  parcial: { variant: "warning", Icon: TriangleAlert },
  erro: { variant: "danger", Icon: CircleAlert },
} as const;

/** Nota humana registrada. */
export function RatingBadge({ rating }: { rating: AIHumanEvalRating }) {
  const { variant, Icon } = RATING[rating];
  return (
    <Badge variant={variant}>
      <Icon aria-hidden /> {RATING_LABELS[rating]}
    </Badge>
  );
}

/** A corrida já tem nota humana? */
export function HumanEvalBadge({ evaluated }: { evaluated: boolean }) {
  return evaluated ? (
    <Badge variant="success">
      <UserCheck aria-hidden /> avaliada
    </Badge>
  ) : (
    <Badge variant="muted">
      <CircleDashed aria-hidden /> sem nota
    </Badge>
  );
}
