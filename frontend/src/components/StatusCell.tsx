import { useQueryClient } from "@tanstack/react-query";
import {
  CalendarClock,
  ChevronDown,
  Circle,
  CircleAlert,
  CircleCheck,
  CircleDot,
  CircleMinus,
  CircleSlash,
  History,
  LoaderCircle,
  Lock,
  type LucideIcon,
  TriangleAlert,
} from "lucide-react";
import { useId, useState } from "react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip } from "@/components/ui/tooltip";
import { useToast } from "@/components/ui/toast-context";
import { type ApiError, api, isApiError } from "@/lib/api";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import {
  isDependencyError,
  isVersionConflict,
  type ObligationInstance,
  obligationKeys,
  type StatusChangeResult,
  type StatusDefinition,
  StatusErrorCode,
  useChangeStatus,
} from "@/services/obligations";

export interface DependencyErrorEvent {
  /** 409 DEPENDENCY_WARNING (SOFT) ou 422 DEPENDENCY_BLOCKED (HARD), com os pré-requisitos em `details`. */
  error: ApiError;
  instance: ObligationInstance;
  /** Status que o usuário tentou aplicar (para repetir com `confirm_exception`). */
  statusCode: string;
}

export interface StatusCellProps {
  instance: ObligationInstance;
  /** Status do conjunto do tipo, na ordem visual. */
  statuses: StatusDefinition[];
  /** Contexto para o nome acessível (ex.: "ICMS — Alfa Comércio"). */
  label?: string;
  /** Período do módulo fechado: edição bloqueada até reabrir [plan R9]. */
  locked?: boolean;
  /** -1 dentro da DataGrid (a célula é que recebe o foco). */
  tabIndex?: number;
  /** Menu controlado (ex.: aberto pelo Enter da DataGrid). */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  /** Alteração confirmada pela API (para a tela atualizar a linha e o agregado). */
  onChanged?: (result: StatusChangeResult) => void;
  /** Dependência pendente: tratada pelos modais de T-M3-05. Sem handler, vira aviso. */
  onDependencyError?: (event: DependencyErrorEvent) => void;
}

const NOT_APPLICABLE_REASON = "Não se aplica a esta empresa nesta competência.";
const LOCKED_REASON = "Módulo fechado nesta competência. Reabra o módulo para alterar.";

interface StatusVisual {
  Icon: LucideIcon;
  tone: string;
}

/** Ícone e tom pelo significado do status (a cor nunca é o único sinal: sempre há texto). */
function visualOf(code: string, def: StatusDefinition | undefined, applicable: boolean): StatusVisual {
  if (!applicable || code === "NOT_APPLICABLE") return { Icon: CircleMinus, tone: "text-muted-foreground" };
  if (code === "NO_MOVEMENT") return { Icon: CircleSlash, tone: "text-muted-foreground" };
  if (!def) return { Icon: Circle, tone: "text-muted-foreground" };
  if (def.is_blocking) return { Icon: TriangleAlert, tone: "text-amber-700" };
  if (def.is_final) return { Icon: CircleCheck, tone: "text-emerald-700" };
  if (def.counts_as_started) return { Icon: CircleDot, tone: "text-sky-700" };
  return { Icon: Circle, tone: "text-muted-foreground" };
}

/**
 * Célula de status das grades [PRD §26.2, §28.3, §45.4]: dropdown com os status do conjunto,
 * indicadores de atraso, prazo alterado e histórico (ícone + aria-label), atualização otimista
 * com rollback [§49.3] e recarga em conflito de versão [§49.4]. A regra (atraso, dependências,
 * período fechado) é sempre da API; a célula só exibe e revalida.
 */
export function StatusCell({
  instance,
  statuses,
  label,
  locked = false,
  tabIndex,
  open,
  onOpenChange,
  onChanged,
  onDependencyError,
}: StatusCellProps) {
  const toast = useToast();
  const qc = useQueryClient();
  const change = useChangeStatus();
  const reasonId = useId();
  // Resposta mais nova que a prop (a tela pode demorar a repassar a linha atualizada).
  const [confirmed, setConfirmed] = useState<ObligationInstance | null>(null);
  // Status otimista, válido só enquanto a versão-base for a exibida.
  const [pending, setPending] = useState<{ code: string; baseVersion: number } | null>(null);

  const current = confirmed && confirmed.version > instance.version ? confirmed : instance;
  const optimistic = pending?.baseVersion === current.version ? pending.code : null;
  const code = optimistic ?? current.status_code;
  const def = statuses.find((s) => s.code === code);
  const statusLabel = !current.applicable ? "Não se aplica" : (def?.label ?? current.status_label);
  const { Icon, tone } = visualOf(code, def, current.applicable);
  const name = label ? `${label}: ${statusLabel}` : statusLabel;
  const disabledReason = !current.applicable ? NOT_APPLICABLE_REASON : locked ? LOCKED_REASON : null;

  const reload = async (id: string) => {
    void qc.invalidateQueries({ queryKey: obligationKeys.all });
    try {
      const fresh = await qc.query({
        queryKey: obligationKeys.instance(id),
        queryFn: ({ signal }) => api.get("/obligation-instances/{id}", { path: { id }, signal }),
        staleTime: 0,
      });
      setConfirmed(fresh);
    } catch {
      // A grade recarregada (invalidação acima) repassa a linha atualizada.
    }
  };

  const select = (next: string) => {
    if (next === current.status_code || change.isPending) return;
    const base = current;
    setPending({ code: next, baseVersion: base.version });
    change.mutate(
      { id: base.id, status_code: next, version: base.version },
      {
        onSuccess: (res) => {
          setConfirmed(res.instance);
          setPending(null);
          onChanged?.(res);
        },
        onError: (err) => {
          setPending(null); // rollback
          if (isVersionConflict(err)) {
            void reload(base.id);
            toast({
              title: "Obrigação alterada em outro lugar",
              description: "Os dados foram recarregados. Confira o status e tente de novo.",
              variant: "error",
            });
            return;
          }
          if (isDependencyError(err) && onDependencyError) {
            onDependencyError({ error: err, instance: base, statusCode: next });
            return;
          }
          if (isApiError(err) && err.code === StatusErrorCode.PeriodClosed) void reload(base.id);
          toast({
            title: "Não foi possível alterar o status",
            description: isApiError(err) ? err.message : "Falha de comunicação com o servidor.",
            variant: "error",
          });
        },
      },
    );
  };

  const content = (
    <>
      {optimistic ? <LoaderCircle aria-hidden="true" className="animate-spin" /> : <Icon aria-hidden="true" className={tone} />}
      <span className="truncate">{statusLabel}</span>
    </>
  );
  const buttonClass =
    "inline-flex h-7 min-w-0 flex-1 items-center gap-1.5 rounded-md px-1.5 text-left text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-3.5 [&_svg]:shrink-0";

  return (
    <div className="flex min-w-0 items-center gap-1">
      {disabledReason ? (
        <>
          <Tooltip content={disabledReason}>
            <button
              type="button"
              aria-disabled="true"
              aria-label={name}
              aria-describedby={reasonId}
              tabIndex={tabIndex}
              className={cn(buttonClass, "cursor-not-allowed text-muted-foreground")}
            >
              {content}
              {locked && current.applicable && <Lock aria-hidden="true" />}
            </button>
          </Tooltip>
          <span id={reasonId} className="sr-only">
            {disabledReason}
          </span>
        </>
      ) : (
        <DropdownMenu open={open} onOpenChange={onOpenChange}>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={name}
              aria-busy={optimistic ? true : undefined}
              tabIndex={tabIndex}
              className={cn(buttonClass, "hover:bg-accent", current.is_overdue && "font-medium")}
            >
              {content}
              <ChevronDown aria-hidden="true" className="ml-auto opacity-50" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuRadioGroup value={code} onValueChange={select}>
              {statuses.map((s) => {
                const v = visualOf(s.code, s, true);
                return (
                  <DropdownMenuRadioItem key={s.code} value={s.code}>
                    <v.Icon aria-hidden="true" className={v.tone} />
                    {s.label}
                  </DropdownMenuRadioItem>
                );
              })}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      <Indicators instance={current} />
    </div>
  );
}

/** Indicadores da célula [PRD §28.3]: ícone com aria-label e tooltip, nunca só cor. */
function Indicators({ instance }: { instance: ObligationInstance }) {
  const items: { key: string; Icon: LucideIcon; text: string; tone: string }[] = [];
  if (instance.is_overdue) {
    const due = formatDate(instance.due_date_effective);
    items.push({ key: "overdue", Icon: CircleAlert, text: due ? `Atrasada: venceu em ${due}` : "Atrasada", tone: "text-destructive" });
  }
  if (instance.due_overridden) {
    const reason = instance.due_date_override_reason;
    items.push({ key: "due", Icon: CalendarClock, text: reason ? `Prazo alterado manualmente: ${reason}` : "Prazo alterado manualmente", tone: "text-sky-700" });
  }
  if (instance.has_history) {
    items.push({ key: "history", Icon: History, text: "Possui histórico de alterações", tone: "text-muted-foreground" });
  }
  if (items.length === 0) return null;
  return (
    <span className="flex shrink-0 items-center gap-0.5 [&_svg]:size-3.5">
      {items.map(({ key, Icon, text, tone }) => (
        <Tooltip key={key} content={text}>
          <span role="img" aria-label={text} className={cn("inline-flex", tone)}>
            <Icon aria-hidden="true" />
          </span>
        </Tooltip>
      ))}
    </span>
  );
}
