import { Bot, Check, CircleHelp, Lightbulb, Undo2, X } from "lucide-react";
import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { NativeSelect, Textarea } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { formatRate } from "@/lib/decimal";
import {
  type ResolverProposal,
  type RuleProposal,
  useChartAccounts,
  useClassificationActions,
  useProposalActions,
  useProposals,
  useSuggestions,
  useUndoReconciliation,
} from "@/services/reconciliation";
import { classificationText, RULE_TYPES } from "../labels";

/** Desfazer a conciliação (automática ou manual) com o motivo obrigatório (D-032). */
export function UndoButton({ reconciliationId, label = "Desfazer" }: { reconciliationId: string; label?: string }) {
  const toast = useToast();
  const undo = useUndoReconciliation();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        <Undo2 aria-hidden /> {label}
      </Button>
      <DialogContent size="sm">
        <DialogHeader
          title="Desfazer a conciliação"
          description="Nada é apagado: fica quem desfez, quando e por quê. Os comprovantes ficam livres, e a linha volta para a fila sem voltar sozinha ao automático."
        />
        <Field label="Motivo" required hint="Até 500 caracteres.">
          <Textarea rows={3} maxLength={500} value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <ErrorAlert error={undo.error} fallback="Não foi possível desfazer." />
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancelar
          </Button>
          <Button
            variant="destructive"
            disabled={!reason.trim() || undo.isPending}
            onClick={() =>
              undo.mutate(
                { id: reconciliationId, reason: reason.trim() },
                {
                  onSuccess: () => {
                    setOpen(false);
                    setReason("");
                    toast({ title: "Conciliação desfeita", variant: "success" });
                  },
                },
              )
            }
          >
            Desfazer
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Uma hipótese do resolvedor, apresentada como sugestão (08 §7), com os pares e a confiança. */
function Proposal({ p, lineId }: { p: ResolverProposal; lineId: string }) {
  const toast = useToast();
  const actions = useProposalActions(lineId);
  const pending = p.status === "pendente";
  return (
    <li className="grid gap-2 rounded-md border bg-amber-50/40 p-3 text-sm" data-testid="hipotese">
      <p className="flex items-center gap-1.5 text-xs font-medium text-amber-900">
        <Lightbulb className="size-3.5" aria-hidden /> Sugestão do resolvedor — não é fato; quem decide é o contador.
        <Badge variant={pending ? "warning" : p.status === "aceita" ? "success" : "muted"} className="ml-auto">
          {p.status}
        </Badge>
      </p>
      <p>{p.hypothesis.hipotese}</p>
      <p className="text-xs text-muted-foreground">{p.hypothesis.justificativa}</p>
      <p className="text-xs text-muted-foreground">
        {p.hypothesis.pares_propostos.length} {p.hypothesis.pares_propostos.length === 1 ? "par proposto" : "pares propostos"} · confiança{" "}
        {formatRate(p.hypothesis.confianca, 0)}
        {p.hypothesis.pede_humano ? " · pede revisão humana" : ""}
      </p>
      {pending ? (
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            disabled={actions.accept.isPending || p.hypothesis.pares_propostos.length === 0}
            onClick={() => actions.accept.mutate(p.id, { onSuccess: () => toast({ title: "Hipótese aceita e conciliada", variant: "success" }) })}
          >
            <Check aria-hidden /> Aceitar e conciliar
          </Button>
          <Button size="sm" variant="outline" disabled={actions.reject.isPending} onClick={() => actions.reject.mutate(p.id)}>
            <X aria-hidden /> Rejeitar
          </Button>
        </div>
      ) : null}
      <ErrorAlert error={actions.accept.error ?? actions.reject.error} fallback="Não foi possível decidir a hipótese." />
    </li>
  );
}

/** O resolvedor (TASK-019): pedir a hipótese e decidir. Nada é conciliado sem o aceite. */
export function ResolverSection({ lineId }: { lineId: string }) {
  const actions = useProposalActions(lineId);
  const [waiting, setWaiting] = useState<number | null>(null);
  const proposals = useProposals(lineId, waiting !== null);
  const items = proposals.data?.items ?? [];
  if (waiting !== null && items.length > waiting) setWaiting(null);
  return (
    <section aria-label="Resolvedor" className="grid gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={actions.request.isPending || waiting !== null}
          onClick={() => actions.request.mutate(undefined, { onSuccess: () => setWaiting(items.length) })}
        >
          <Bot aria-hidden /> Pedir hipótese ao resolvedor
        </Button>
        {waiting !== null ? (
          <span role="status" className="text-xs text-muted-foreground">
            Pedido na fila: o resolvedor escreve a hipótese no worker.
          </span>
        ) : null}
      </div>
      <ErrorAlert error={actions.request.error} fallback="Não foi possível pedir a hipótese." />
      {items.length > 0 ? (
        <ul className="grid gap-2" aria-label="Hipóteses do resolvedor">
          {items.map((p) => (
            <Proposal key={p.id} p={p} lineId={lineId} />
          ))}
        </ul>
      ) : null}
    </section>
  );
}

/** A regra proposta depois da correção ou do aceite (M5.5): só vale confirmada. */
function ProposedRule({ rule, onConfirm, pending }: { rule: RuleProposal; onConfirm: () => void; pending: boolean }) {
  return (
    <div className="grid gap-2 rounded-md border border-dashed p-3 text-sm" data-testid="regra-proposta">
      <p>
        <span className="font-medium">Regra proposta:</span> {RULE_TYPES[rule.rule_type]} <code className="rounded bg-muted px-1">{rule.pattern}</code> → conta{" "}
        {rule.account_code}
      </p>
      <p className="text-xs text-muted-foreground">Confirmada, ela classifica sozinha as próximas linhas iguais; nada já classificado muda.</p>
      <Button size="sm" variant="outline" className="w-fit" disabled={pending} onClick={onConfirm}>
        <Check aria-hidden /> Confirmar regra
      </Button>
    </div>
  );
}

/**
 * A classificação da linha conciliada (TASK-020): a conta e a origem; a correção do contador, que
 * propõe a regra; e a sugestão do agente, que só vale aceita.
 */
export function ClassificationSection({
  lineId,
  companyId,
  accountCode,
  origin,
  pending,
}: {
  lineId: string;
  companyId: string;
  accountCode: string | null;
  origin: string | null;
  pending: boolean;
}) {
  const toast = useToast();
  const chart = useChartAccounts(companyId);
  const actions = useClassificationActions(lineId, companyId);
  const [account, setAccount] = useState(accountCode ?? "");
  const [rule, setRule] = useState<RuleProposal | null>(null);
  const [waiting, setWaiting] = useState<number | null>(null);
  const suggestions = useSuggestions(lineId, waiting !== null);
  const items = suggestions.data?.items ?? [];
  if (waiting !== null && items.length > waiting) setWaiting(null);
  const accounts = chart.data?.items ?? [];
  const name = (code: string | null) => accounts.find((a) => a.code === code)?.name;
  const awaiting = !origin || pending;
  return (
    <section aria-label="Classificação" className="grid gap-3 rounded-md border p-3">
      <p className="text-sm">
        <span className="font-medium">Conta:</span> {accountCode ? `${accountCode}${name(accountCode) ? ` · ${name(accountCode)}` : ""}` : "—"}{" "}
        <Badge variant={awaiting ? "warning" : "success"}>{classificationText(origin, pending)}</Badge>
      </p>
      {accounts.length === 0 && !chart.isPending ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <CircleHelp className="size-3.5" aria-hidden /> A empresa não tem plano de contas: cadastre o plano para classificar.
        </p>
      ) : (
        <div className="flex flex-wrap items-end gap-2">
          <Field label="Classificar na conta" className="min-w-64">
            <NativeSelect value={account} onChange={(e) => setAccount(e.target.value)}>
              <option value="">Escolha a conta</option>
              {accounts.map((a) => (
                <option key={a.id} value={a.code}>
                  {a.code} · {a.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Button
            size="sm"
            disabled={!account || actions.correct.isPending}
            onClick={() =>
              actions.correct.mutate(account, {
                onSuccess: (r) => {
                  setRule(r.proposed_rule);
                  toast({ title: "Linha classificada", variant: "success" });
                },
              })
            }
          >
            Classificar
          </Button>
          {awaiting ? (
            <Button
              size="sm"
              variant="outline"
              disabled={actions.request.isPending || waiting !== null}
              onClick={() => actions.request.mutate(undefined, { onSuccess: () => setWaiting(items.length) })}
            >
              <Bot aria-hidden /> Pedir sugestão ao agente
            </Button>
          ) : null}
        </div>
      )}
      {waiting !== null ? (
        <p role="status" className="text-xs text-muted-foreground">
          Pedido na fila: o agente sugere a conta no worker.
        </p>
      ) : null}
      <ErrorAlert error={actions.correct.error ?? actions.request.error ?? actions.confirmRule.error} fallback="Não foi possível classificar." />
      {rule ? (
        <ProposedRule
          rule={rule}
          pending={actions.confirmRule.isPending}
          onConfirm={() =>
            actions.confirmRule.mutate(rule, {
              onSuccess: () => {
                setRule(null);
                toast({ title: "Regra confirmada", variant: "success" });
              },
            })
          }
        />
      ) : null}
      {items.length > 0 ? (
        <ul className="grid gap-2" aria-label="Sugestões do agente">
          {items.map((s) => (
            <li key={s.id} className="grid gap-1.5 rounded-md border bg-amber-50/40 p-3 text-sm" data-testid="sugestao">
              <p className="flex items-center gap-1.5 text-xs font-medium text-amber-900">
                <Lightbulb className="size-3.5" aria-hidden /> Sugestão do agente — só vale aceita.
                <Badge variant={s.status === "pendente" ? "warning" : s.status === "aceita" ? "success" : "muted"} className="ml-auto">
                  {s.status}
                </Badge>
              </p>
              <p>
                Conta: {s.account_code ? `${s.account_code}${name(s.account_code) ? ` · ${name(s.account_code)}` : ""}` : "nenhuma do plano"} · confiança{" "}
                {formatRate(s.suggestion.confianca, 0)}
              </p>
              <p className="text-xs text-muted-foreground">{s.suggestion.justificativa}</p>
              {s.status === "pendente" ? (
                <div className="flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    disabled={!s.account_code || actions.accept.isPending}
                    onClick={() =>
                      actions.accept.mutate(s.id, {
                        onSuccess: (r) => {
                          setRule(r.proposed_rule);
                          toast({ title: "Sugestão aceita", variant: "success" });
                        },
                      })
                    }
                  >
                    <Check aria-hidden /> Aceitar
                  </Button>
                  <Button size="sm" variant="outline" disabled={actions.reject.isPending} onClick={() => actions.reject.mutate(s.id)}>
                    <X aria-hidden /> Rejeitar
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
      <ErrorAlert error={actions.accept.error ?? actions.reject.error} fallback="Não foi possível decidir a sugestão." />
    </section>
  );
}
