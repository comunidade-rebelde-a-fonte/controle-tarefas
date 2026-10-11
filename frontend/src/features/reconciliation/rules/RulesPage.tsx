import { CircleHelp, Play, PowerOff, Plus, Wand2 } from "lucide-react";
import { type SubmitEvent, useState } from "react";
import { Link } from "react-router";
import { CompetenceInput } from "@/components/CompetenceInput";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { FilterToggle } from "@/features/factor-r/common";
import { isCompetenceKey } from "@/lib/competence";
import { formatDateTime } from "@/lib/format";
import { type ClassificationRule, type ClassificationRuleType, useChart, useRuleActions, useRules } from "@/services/reconciliation";
import { CompanySelect } from "../common";
import { classificationRunText, RULE_TYPES } from "../labels";
import { useSearchUpdate } from "../model";
import { fieldErrorsOf } from "../review/model";

const TYPES = Object.entries(RULE_TYPES) as [ClassificationRuleType, string][];

/** O que o padrão de cada tipo de regra compara (a regra_classificacao da origem). */
const PATTERN_HINT: Record<ClassificationRuleType, string> = {
  cnpj: "Só os dígitos do CPF ou CNPJ da contraparte (11 ou 14); a pontuação é ignorada.",
  contraparte: "O nome inteiro da contraparte, comparado sem diferenciar maiúsculas, acentos nem espaços repetidos.",
  regex_descritivo: "Expressão regular buscada no descritivo do extrato, sem diferenciar maiúsculas (ex.: TARIFA|PACOTE).",
};

function isType(v: string): v is ClassificationRuleType {
  return TYPES.some(([t]) => t === v);
}

/** Confirma uma regra nova (M5.2): vale para as próximas classificações, nunca troca o confirmado. */
function NewRuleForm({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { create } = useRuleActions(companyId);
  const chart = useChart(companyId);
  const accounts = (chart.data?.items ?? []).filter((a) => a.active);
  const [type, setType] = useState<ClassificationRuleType>("contraparte");
  const [pattern, setPattern] = useState("");
  const [account, setAccount] = useState("");
  const errors = fieldErrorsOf(create.error);

  function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    create.mutate(
      { rule_type: type, pattern: pattern.trim(), account_code: account },
      {
        onSuccess: (r) => {
          setPattern("");
          const replaced = r.replaced_rule_ids.length;
          toast({
            title: "Regra confirmada",
            description: replaced > 0 ? `Substitui ${String(replaced)} ${replaced === 1 ? "regra antiga" : "regras antigas"} com o mesmo padrão.` : undefined,
            variant: "success",
          });
        },
      },
    );
  }

  if (chart.data && accounts.length === 0) {
    return (
      <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
        <CircleHelp className="size-4" aria-hidden /> A empresa não tem conta ativa no plano.{" "}
        <Link to={`/reconciliation/chart?company_id=${companyId}`} className="font-medium text-foreground underline underline-offset-4">
          Cadastre o plano de contas
        </Link>{" "}
        para criar regras.
      </p>
    );
  }

  return (
    <form onSubmit={submit} aria-label="Nova regra" className="grid gap-3">
      <h2 className="text-sm font-semibold">Nova regra</h2>
      <div className="grid gap-3 md:grid-cols-[12rem_1fr_16rem]">
        <Field label="Tipo" required error={errors.rule_type}>
          <NativeSelect
            value={type}
            onChange={(e) => {
              if (isType(e.target.value)) setType(e.target.value);
            }}
          >
            {TYPES.map(([t, label]) => (
              <option key={t} value={t}>
                {label}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label="Padrão" required error={errors.pattern} hint={PATTERN_HINT[type]}>
          <Input maxLength={512} value={pattern} onChange={(e) => setPattern(e.target.value)} required className="font-mono" />
        </Field>
        <Field label="Conta" required error={errors.account_code}>
          <NativeSelect value={account} onChange={(e) => setAccount(e.target.value)} required>
            <option value="">Escolha a conta</option>
            {accounts.map((a) => (
              <option key={a.id} value={a.code}>
                {a.code} · {a.name}
              </option>
            ))}
          </NativeSelect>
        </Field>
      </div>
      {errors.rule_type || errors.pattern || errors.account_code ? null : <ErrorAlert error={create.error} fallback="Não foi possível confirmar a regra." />}
      <Button type="submit" size="sm" className="w-fit" disabled={create.isPending || !pattern.trim() || !account}>
        <Plus aria-hidden /> Confirmar regra
      </Button>
    </form>
  );
}

/** Aplica as regras e o histórico às linhas conciliadas da competência que esperam classificação. */
function ApplyRules({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { classify } = useRuleActions(companyId);
  const [competence, setCompetence] = useState("");
  return (
    <div className="grid gap-2 border-t pt-4">
      <h2 className="text-sm font-semibold">Aplicar às linhas pendentes</h2>
      <p className="text-xs text-muted-foreground">
        Classifica por regra e, sem regra, pelo histórico as linhas conciliadas da competência que ainda não têm conta. O que o contador
        já confirmou não muda.
      </p>
      <div className="flex flex-wrap items-end gap-2">
        <label className="grid gap-1 text-xs font-medium text-muted-foreground">
          Competência
          <CompetenceInput value={competence} onChange={setCompetence} />
        </label>
        <Button
          size="sm"
          variant="outline"
          disabled={!isCompetenceKey(competence) || classify.isPending}
          onClick={() =>
            classify.mutate(competence, {
              onSuccess: (s) => toast({ title: "Regras aplicadas", description: `Linhas: ${classificationRunText(s)}.`, variant: "success" }),
            })
          }
        >
          <Play aria-hidden /> Aplicar
        </Button>
      </div>
      <ErrorAlert error={classify.error} fallback="Não foi possível aplicar as regras." />
    </div>
  );
}

/** Desativa a regra (nunca apaga), com confirmação. */
function DeactivateRule({ companyId, rule }: { companyId: string; rule: ClassificationRule }) {
  const toast = useToast();
  const { deactivate } = useRuleActions(companyId);
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)} aria-label={`Desativar a regra ${rule.pattern}`}>
        <PowerOff aria-hidden /> Desativar
      </Button>
      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title="Desativar a regra?"
        description="Ela deixa de classificar as próximas linhas. O que ela já classificou fica como está, e a regra continua na lista, desativada."
        confirmLabel="Desativar"
        destructive
        pending={deactivate.isPending}
        onConfirm={() =>
          deactivate.mutate(rule.id, {
            onSuccess: () => {
              setOpen(false);
              toast({ title: "Regra desativada", variant: "success" });
            },
          })
        }
      >
        <ErrorAlert error={deactivate.error} fallback="Não foi possível desativar a regra." />
      </ConfirmDialog>
    </>
  );
}

/**
 * Regras de classificação (regra_classificacao da origem; CC-21, M5.2, M5.5): da mais nova para
 * a mais antiga, que é a ordem em que valem; confirmar, desativar e aplicar às pendentes.
 */
export function RulesPage() {
  const [params, update] = useSearchUpdate();
  const companyId = params.get("company_id") ?? "";
  const onlyActive = params.get("active") === "true";
  const rules = useRules(companyId || undefined);
  const chart = useChart(companyId || undefined);
  const names = new Map((chart.data?.items ?? []).map((a) => [a.code, a.name]));
  const accountText = (code: string) => {
    const name = names.get(code);
    return name ? `${code} · ${name}` : code;
  };
  const all = rules.data?.items ?? [];
  const items = onlyActive ? all.filter((r) => r.active) : all;
  return (
    <>
      <PageHeader
        title="Regras de classificação"
        description="A regra mais nova vale. Confirmar uma regra com o mesmo tipo e padrão de outra ativa desativa a antiga; nenhuma regra é apagada."
      />
      <div className="grid gap-4 p-4">
        <label className="grid w-fit gap-1 text-xs font-medium text-muted-foreground">
          Empresa
          <CompanySelect value={companyId} onChange={(id) => update({ company_id: id || null })} />
        </label>
        {!companyId ? <p className="text-sm text-muted-foreground">Escolha a empresa para ver e criar regras.</p> : null}
        {companyId ? (
          <section aria-label="Criar e aplicar regras" className="grid gap-4 rounded-lg border bg-card p-4">
            <NewRuleForm key={companyId} companyId={companyId} />
            <ApplyRules key={`apply-${companyId}`} companyId={companyId} />
          </section>
        ) : null}
        {companyId ? (
          <div role="group" aria-label="Filtrar regras" className="flex flex-wrap gap-2">
            <FilterToggle active={!onlyActive} onClick={() => update({ active: null })}>
              Todas ({all.length})
            </FilterToggle>
            <FilterToggle active={onlyActive} onClick={() => update({ active: "true" })}>
              Só as ativas ({all.filter((r) => r.active).length})
            </FilterToggle>
          </div>
        ) : null}
        <ErrorAlert error={rules.error} fallback="Não foi possível carregar as regras." />
        {companyId && rules.isPending ? <Skeleton className="h-48" /> : null}
        {rules.data && items.length === 0 ? (
          <EmptyState
            icon={Wand2}
            title="Nenhuma regra"
            description="Crie a regra acima ou confirme a regra proposta depois de corrigir uma classificação na fila."
          />
        ) : null}
        {items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Regras da empresa">
              <thead>
                <tr>
                  <Th scope="col">Criada em</Th>
                  <Th scope="col">Tipo</Th>
                  <Th scope="col">Padrão</Th>
                  <Th scope="col">Conta</Th>
                  <Th scope="col">Situação</Th>
                  <Th scope="col">
                    <span className="sr-only">Ações</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {items.map((r) => (
                  <tr key={r.id} data-regra={r.id} className={r.active ? undefined : "text-muted-foreground"}>
                    <Td className="text-xs whitespace-nowrap tabular-nums">{formatDateTime(r.created_at)}</Td>
                    <Td className="text-xs whitespace-nowrap">{RULE_TYPES[r.rule_type]}</Td>
                    <Td className="max-w-80">
                      <code className="rounded bg-muted px-1 text-xs break-all">{r.pattern}</code>
                      {r.source_statement_transaction_id ? (
                        <Link
                          to={`/reconciliation?item=${r.source_statement_transaction_id}&kind=linha`}
                          className="mt-0.5 block w-fit text-xs underline underline-offset-4"
                        >
                          da correção de uma linha
                        </Link>
                      ) : null}
                    </Td>
                    <Td className="text-xs">{accountText(r.account_code)}</Td>
                    <Td className="whitespace-nowrap">
                      {r.active ? (
                        <Badge variant="success">Ativa</Badge>
                      ) : (
                        <span className="grid gap-0.5">
                          <Badge variant="muted" className="w-fit">
                            Desativada
                          </Badge>
                          {r.deactivated_at ? <span className="text-xs">em {formatDateTime(r.deactivated_at)}</span> : null}
                        </span>
                      )}
                    </Td>
                    <Td className="text-right whitespace-nowrap">{r.active ? <DeactivateRule companyId={companyId} rule={r} /> : null}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        ) : null}
      </div>
    </>
  );
}
