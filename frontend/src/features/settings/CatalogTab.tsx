import { Pencil, Plus, X } from "lucide-react";
import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import type { Schemas } from "@/lib/api";
import { TAX_REGIME_LABELS, TAX_REGIMES, type TaxRegime } from "@/lib/labels";
import {
  type ObligationType,
  useCreateObligationType,
  useDueDateRules,
  useObligationTypes,
  useStatusSets,
  useUpdateObligationType,
} from "@/services/catalog";
import { STATUS_SET_LABELS } from "./labels";

type DisplayModule = Schemas["DisplayModule"];
type ObligationModule = Schemas["ObligationModule"];

const DISPLAY_LABELS: Record<DisplayModule, string> = {
  FISCAL: "Fiscal",
  DECLARATIONS: "Declarações",
  ACCOUNTING: "Contábil",
  PAYROLL: "Folha",
  DCTFWEB: "DCTFWeb",
};
const DISPLAY_MODULES = Object.keys(DISPLAY_LABELS) as DisplayModule[];
const OBLIGATION_MODULES: ObligationModule[] = ["FISCAL", "DECLARATIONS", "ACCOUNTING", "PAYROLL"];
const REGIME_SHORT: Record<TaxRegime, string> = {
  SIMPLES_NACIONAL: "SN",
  LUCRO_PRESUMIDO: "LP",
  LUCRO_REAL: "LR",
  LUCRO_ARBITRADO: "LA",
  IMUNE_ISENTA: "II",
};

/** Catálogo de obrigações: novos tipos entram por cadastro, sem migration [PRD §15.2, §20]. */
export function CatalogTab() {
  const { data, isPending } = useObligationTypes();
  const rules = useDueDateRules();
  const [editing, setEditing] = useState<ObligationType | "new" | null>(null);
  const ruleName = new Map((rules.data?.items ?? []).map((r) => [r.id, r.name]));

  return (
    <Card>
      <CardHeader
        title="Catálogo de obrigações"
        description="Colunas das telas, regimes em que cada obrigação é gerada, regra de vencimento e dependências. Alterações valem para as próximas competências."
        actions={
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus aria-hidden /> Novo tipo
          </Button>
        }
      />
      <CardBody className="grid gap-5">
        {isPending ? <Skeleton className="h-60" /> : null}
        {OBLIGATION_MODULES.map((module) => {
          const items = (data?.items ?? []).filter((t) => t.primary_module === module);
          if (items.length === 0) return null;
          return (
            <div key={module}>
              <h3 className="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">{DISPLAY_LABELS[module]}</h3>
              <div className="overflow-x-auto rounded-md border">
                <Table aria-label={`Catálogo — ${DISPLAY_LABELS[module]}`} className="[&_td]:whitespace-nowrap">
                  <thead>
                    <tr>
                      <Th>Obrigação</Th>
                      <Th>Status</Th>
                      <Th>Vencimento</Th>
                      <Th>Telas</Th>
                      <Th>Regimes (gera / fora do progresso)</Th>
                      <Th>Dependências</Th>
                      <Th>Situação</Th>
                      <Th className="w-12">
                        <span className="sr-only">Editar</span>
                      </Th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((t) => (
                      <tr key={t.id}>
                        <Td>
                          <p className="font-medium">{t.name}</p>
                          <p className="font-mono text-[11px] text-muted-foreground">{t.code}</p>
                        </Td>
                        <Td className="text-xs">{STATUS_SET_LABELS[t.status_set_code] ?? t.status_set_code}</Td>
                        <Td className="max-w-56 truncate text-xs" title={t.due_date_rule_id ? ruleName.get(t.due_date_rule_id) : undefined}>
                          {t.due_date_rule_id ? (ruleName.get(t.due_date_rule_id) ?? "—") : "Sem regra"}
                        </Td>
                        <Td>
                          <div className="flex gap-0.5">
                            {t.display_contexts.map((c) => (
                              <Badge key={c.module_code} variant="secondary">
                                {DISPLAY_LABELS[c.module_code]}
                              </Badge>
                            ))}
                          </div>
                        </Td>
                        <Td>
                          <div className="flex gap-0.5">
                            {t.regime_rules.map((r) => (
                              <Badge
                                key={r.tax_regime}
                                variant={r.counts_in_progress === false ? "warning" : "outline"}
                                aria-label={`${TAX_REGIME_LABELS[r.tax_regime]}${r.counts_in_progress === false ? " (fora do progresso)" : ""}`}
                                title={`${TAX_REGIME_LABELS[r.tax_regime]}${r.counts_in_progress === false ? " — fora do progresso" : ""}`}
                              >
                                {REGIME_SHORT[r.tax_regime]}
                                {r.counts_in_progress === false ? "*" : ""}
                              </Badge>
                            ))}
                          </div>
                        </Td>
                        <Td className="text-xs">
                          {t.dependencies.length === 0
                            ? "—"
                            : t.dependencies.map((d) => `${d.prerequisite_code ?? ""} (${d.dependency_mode === "HARD" ? "bloqueia" : "alerta"})`).join(", ")}
                        </Td>
                        <Td>{t.active ? <Badge variant="success">Ativo</Badge> : <Badge variant="muted">Inativo</Badge>}</Td>
                        <Td>
                          <Button variant="ghost" size="icon-sm" aria-label={`Editar ${t.name}`} onClick={() => setEditing(t)}>
                            <Pencil aria-hidden />
                          </Button>
                        </Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              </div>
            </div>
          );
        })}
        <p className="text-xs text-muted-foreground">* Gera a obrigação no regime, mas fora do cálculo de progresso (ex.: ICMS no Simples, coluna Fechamento).</p>
      </CardBody>
      <Dialog open={editing !== null} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent size="lg">
          <DialogHeader
            title={editing === "new" ? "Novo tipo de obrigação" : `Editar ${editing?.name ?? ""}`}
            description="Alterações valem para as próximas competências geradas; competências existentes não são reescritas."
          />
          {editing !== null ? (
            <ObligationTypeForm
              key={editing === "new" ? "new" : editing.id}
              type={editing === "new" ? undefined : editing}
              allTypes={data?.items ?? []}
              onDone={() => setEditing(null)}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </Card>
  );
}

interface RegimeState {
  enabled: boolean;
  counts: boolean;
}

function ObligationTypeForm({ type, allTypes, onDone }: { type?: ObligationType | undefined; allTypes: ObligationType[]; onDone: () => void }) {
  const toast = useToast();
  const rules = useDueDateRules();
  const statusSets = useStatusSets();
  const create = useCreateObligationType();
  const update = useUpdateObligationType();
  const [code, setCode] = useState("");
  const [primaryModule, setPrimaryModule] = useState<ObligationModule>(type?.primary_module ?? "DECLARATIONS");
  const [statusSet, setStatusSet] = useState(type?.status_set_code ?? "DECLARATION");
  const [name, setName] = useState(type?.name ?? "");
  const [active, setActive] = useState(type?.active ?? true);
  const [sortOrder, setSortOrder] = useState(String(type?.sort_order ?? 500));
  const [ruleId, setRuleId] = useState(type?.due_date_rule_id ?? "");
  const [transmission, setTransmission] = useState(type?.allows_transmission_date ?? false);
  const [protocol, setProtocol] = useState(type?.allows_protocol ?? false);
  const [noPayroll, setNoPayroll] = useState<"NONE" | "NO_MOVEMENT" | "NOT_APPLICABLE">(type?.status_when_no_payroll ?? "NONE");
  const [contexts, setContexts] = useState<DisplayModule[]>(type ? type.display_contexts.map((c) => c.module_code) : ["DECLARATIONS"]);
  const [regimes, setRegimes] = useState<Record<TaxRegime, RegimeState>>(() =>
    Object.fromEntries(
      TAX_REGIMES.map((r) => {
        const rule = type?.regime_rules.find((x) => x.tax_regime === r);
        return [r, { enabled: !!rule, counts: rule?.counts_in_progress ?? true }];
      }),
    ) as Record<TaxRegime, RegimeState>,
  );
  const [deps, setDeps] = useState<{ id: string; mode: "HARD" | "SOFT" }[]>(
    type?.dependencies.map((d) => ({ id: d.prerequisite_obligation_type_id, mode: d.dependency_mode })) ?? [],
  );
  const [newDep, setNewDep] = useState("");
  const error = create.error ?? update.error;

  const sort = Number(sortOrder) || 0;
  const payload = {
    name: name.trim(),
    active,
    sort_order: sort,
    allows_transmission_date: transmission,
    allows_protocol: protocol,
    display_contexts: contexts.map((m) => ({ module_code: m, sort_order: type?.display_contexts.find((c) => c.module_code === m)?.sort_order ?? sort })),
    regime_rules: TAX_REGIMES.filter((r) => regimes[r].enabled).map((r) => ({ tax_regime: r, default_applicable: true, counts_in_progress: regimes[r].counts })),
    dependencies: deps.map((d) => ({ prerequisite_obligation_type_id: d.id, dependency_mode: d.mode })),
  };

  const submit = () => {
    const onSuccess = () => {
      toast({ title: type ? "Tipo atualizado" : "Tipo criado", description: "Vale para as próximas competências geradas.", variant: "success" });
      onDone();
    };
    if (type) {
      update.mutate(
        {
          id: type.id,
          version: type.version,
          ...payload,
          ...(ruleId ? { due_date_rule_id: ruleId } : { clear_due_date_rule: true }),
          status_when_no_payroll: noPayroll,
        },
        { onSuccess },
      );
    } else {
      create.mutate(
        {
          code: code.trim().toUpperCase(),
          primary_module: primaryModule,
          status_set_code: statusSet,
          ...payload,
          due_date_rule_id: ruleId || null,
          status_when_no_payroll: noPayroll === "NONE" ? null : noPayroll,
        },
        { onSuccess },
      );
    }
  };

  const typeName = new Map(allTypes.map((t) => [t.id, t.name]));

  return (
    <div className="grid gap-4">
      {!type ? (
        <div className="grid grid-cols-3 items-start gap-4">
          <Field label="Código" required hint="Maiúsculas, dígitos e _ (ex.: NOVA_DECLARACAO).">
            <Input value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} />
          </Field>
          <Field label="Módulo principal" required>
            <NativeSelect value={primaryModule} onChange={(e) => setPrimaryModule(e.target.value as ObligationModule)}>
              {OBLIGATION_MODULES.map((m) => (
                <option key={m} value={m}>
                  {DISPLAY_LABELS[m]}
                </option>
              ))}
            </NativeSelect>
          </Field>
          <Field label="Conjunto de status" required>
            <NativeSelect value={statusSet} onChange={(e) => setStatusSet(e.target.value)}>
              {(statusSets.data?.items ?? []).map((s) => (
                <option key={s.code} value={s.code}>
                  {STATUS_SET_LABELS[s.code] ?? s.name}
                </option>
              ))}
            </NativeSelect>
          </Field>
        </div>
      ) : null}
      <div className="grid grid-cols-3 items-start gap-4">
        <Field label="Nome" required className="col-span-2">
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Ordem de exibição">
          <Input type="number" value={sortOrder} onChange={(e) => setSortOrder(e.target.value)} />
        </Field>
        <Field label="Regra de vencimento" className="col-span-2">
          <NativeSelect value={ruleId} onChange={(e) => setRuleId(e.target.value)}>
            <option value="">Sem regra</option>
            {(rules.data?.items ?? []).map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label="Empresa “Sem folha”" hint="Status aplicado na geração.">
          <NativeSelect value={noPayroll} onChange={(e) => setNoPayroll(e.target.value as typeof noPayroll)}>
            <option value="NONE">Sem regra especial</option>
            <option value="NO_MOVEMENT">Sem movimento</option>
            <option value="NOT_APPLICABLE">Não se aplica</option>
          </NativeSelect>
        </Field>
      </div>
      <div className="flex flex-wrap gap-5 text-sm">
        {(
          [
            ["Ativo", active, setActive],
            ["Permite data de transmissão", transmission, setTransmission],
            ["Permite protocolo", protocol, setProtocol],
          ] as const
        ).map(([label, value, setter]) => (
          <label key={label} className="inline-flex items-center gap-2">
            <input type="checkbox" className="size-4 accent-primary" checked={value} onChange={(e) => setter(e.target.checked)} />
            {label}
          </label>
        ))}
      </div>
      <fieldset>
        <legend className="mb-2 text-sm font-semibold">Telas em que aparece</legend>
        <div className="flex flex-wrap gap-4 text-sm">
          {DISPLAY_MODULES.map((m) => (
            <label key={m} className="inline-flex items-center gap-2">
              <input
                type="checkbox"
                className="size-4 accent-primary"
                checked={contexts.includes(m)}
                onChange={(e) => setContexts((prev) => (e.target.checked ? [...prev, m] : prev.filter((x) => x !== m)))}
              />
              {DISPLAY_LABELS[m]}
            </label>
          ))}
        </div>
      </fieldset>
      <fieldset>
        <legend className="mb-2 text-sm font-semibold">Regimes</legend>
        <div className="rounded-md border">
          <Table aria-label="Regras por regime">
            <thead>
              <tr>
                <Th>Regime</Th>
                <Th>Gera a obrigação</Th>
                <Th>Conta no progresso</Th>
              </tr>
            </thead>
            <tbody>
              {TAX_REGIMES.map((r) => (
                <tr key={r}>
                  <Td>{TAX_REGIME_LABELS[r]}</Td>
                  <Td>
                    <input
                      type="checkbox"
                      aria-label={`Gera em ${TAX_REGIME_LABELS[r]}`}
                      className="size-4 accent-primary"
                      checked={regimes[r].enabled}
                      onChange={(e) => setRegimes((prev) => ({ ...prev, [r]: { ...prev[r], enabled: e.target.checked } }))}
                    />
                  </Td>
                  <Td>
                    <input
                      type="checkbox"
                      aria-label={`Conta no progresso em ${TAX_REGIME_LABELS[r]}`}
                      className="size-4 accent-primary"
                      disabled={!regimes[r].enabled}
                      checked={regimes[r].counts}
                      onChange={(e) => setRegimes((prev) => ({ ...prev, [r]: { ...prev[r], counts: e.target.checked } }))}
                    />
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </div>
      </fieldset>
      <fieldset>
        <legend className="mb-2 text-sm font-semibold">Dependências</legend>
        <div className="grid gap-2">
          {deps.map((d) => (
            <div key={d.id} className="flex items-center gap-2 text-sm">
              <span className="w-64 truncate">{typeName.get(d.id) ?? d.id}</span>
              <NativeSelect
                aria-label={`Modo da dependência ${typeName.get(d.id) ?? ""}`}
                className="w-56"
                value={d.mode}
                onChange={(e) => setDeps((prev) => prev.map((x) => (x.id === d.id ? { ...x, mode: e.target.value as "HARD" | "SOFT" } : x)))}
              >
                <option value="HARD">Bloqueia (HARD)</option>
                <option value="SOFT">Alerta com exceção (SOFT)</option>
              </NativeSelect>
              <Button variant="ghost" size="icon-sm" aria-label="Remover dependência" onClick={() => setDeps((prev) => prev.filter((x) => x.id !== d.id))}>
                <X aria-hidden />
              </Button>
            </div>
          ))}
          <div className="flex items-center gap-2">
            <NativeSelect aria-label="Novo pré-requisito" className="w-64" value={newDep} onChange={(e) => setNewDep(e.target.value)}>
              <option value="">Adicionar pré-requisito…</option>
              {allTypes
                .filter((t) => t.id !== type?.id && !deps.some((d) => d.id === t.id))
                .map((t) => (
                  <option key={t.id} value={t.id}>
                    {t.name}
                  </option>
                ))}
            </NativeSelect>
            <Button
              variant="outline"
              size="sm"
              disabled={!newDep}
              onClick={() => {
                setDeps((prev) => [...prev, { id: newDep, mode: "SOFT" }]);
                setNewDep("");
              }}
            >
              Adicionar
            </Button>
          </div>
        </div>
      </fieldset>
      <ErrorAlert error={error} />
      <DialogFooter>
        <Button variant="outline" onClick={onDone}>
          Cancelar
        </Button>
        <Button onClick={submit} disabled={!name.trim() || (!type && !code.trim()) || create.isPending || update.isPending}>
          {create.isPending || update.isPending ? "Salvando…" : "Salvar"}
        </Button>
      </DialogFooter>
    </div>
  );
}
