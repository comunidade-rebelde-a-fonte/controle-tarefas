import { CircleCheck, CircleX } from "lucide-react";
import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import {
  AI_PROVIDERS,
  type AIModelAlias,
  type AIModelCheck,
  type AIModelValidateRequest,
  type AIModelValidation,
  type AIProvider,
  useUpdateAIModel,
  useValidateAIModel,
} from "@/services/aiModels";
import { CREDENTIALS_NOTE, splitModel } from "./labels";

type Form = {
  useDefault: boolean;
  provider: AIProvider;
  model: string;
  temperature: string;
  maxTokens: string;
  enabled: boolean;
};

function toForm(item: AIModelAlias): Form {
  const effective = splitModel(item.effective_model);
  const provider = item.provider ?? effective?.[0];
  return {
    useDefault: item.source !== "settings",
    provider: AI_PROVIDERS.find((p) => p === provider) ?? "openai",
    model: item.model ?? effective?.[1] ?? "",
    temperature: item.parameters.temperature?.toString() ?? "",
    maxTokens: item.parameters.max_tokens?.toString() ?? "",
    enabled: item.enabled,
  };
}

/** Monta o corpo da API; parâmetro vazio = padrão do provedor. */
function toRequest(f: Form): { request: AIModelValidateRequest } | { error: string } {
  const parameters: AIModelValidateRequest["parameters"] = {};
  const temperature = f.temperature.trim().replace(",", ".");
  if (temperature) {
    const t = Number(temperature);
    if (!Number.isFinite(t)) return { error: "Temperature inválida: use um número de 0 a 2." };
    parameters.temperature = t;
  }
  const maxTokens = f.maxTokens.trim().replace(/\./g, "");
  if (maxTokens) {
    const n = Number(maxTokens);
    if (!Number.isInteger(n)) return { error: "max_tokens inválido: use um número inteiro." };
    parameters.max_tokens = n;
  }
  return {
    request: f.useDefault
      ? { provider: null, model: null, parameters }
      : { provider: f.provider, model: f.model.trim(), parameters },
  };
}

function fieldErrors(err: unknown): Record<string, string> {
  if (!isApiError(err) || err.code !== "VALIDATION_ERROR") return {};
  const fields = err.details.fields;
  return fields && typeof fields === "object" ? (fields as Record<string, string>) : {};
}

/** Conferências reprovadas no salvar (422 AI_MODEL_INVALID): as mesmas da validação. */
function rejectedChecks(err: unknown): AIModelCheck[] | null {
  if (!isApiError(err) || err.code !== "AI_MODEL_INVALID") return null;
  const checks = err.details.checks;
  return Array.isArray(checks) ? (checks as AIModelCheck[]) : null;
}

function ChecksPanel({ title, ok, checks, footer }: { title: string; ok: boolean; checks: AIModelCheck[]; footer?: string | undefined }) {
  return (
    <div
      role="status"
      data-testid="resultado-validacao"
      className={ok ? "rounded-md border border-success/30 bg-success/10 p-3 text-sm text-success" : "rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive"}
    >
      <p className="font-medium">{title}</p>
      <ul className="mt-2 grid gap-1">
        {checks.map((c) => (
          <li key={c.code} className="flex items-start gap-2">
            {c.ok ? (
              <CircleCheck className="mt-0.5 size-4 shrink-0 text-success" aria-label="ok" />
            ) : (
              <CircleX className="mt-0.5 size-4 shrink-0 text-destructive" aria-label="reprovado" />
            )}
            <span>{c.message}</span>
          </li>
        ))}
      </ul>
      {footer ? <p className="mt-2 text-xs text-neutral-300">{footer}</p> : null}
    </div>
  );
}

function capabilitiesText(v: AIModelValidation): string | undefined {
  const c = v.capabilities;
  if (!c) return undefined;
  const params = c.parameters.length ? c.parameters.join(", ") : "nenhum dos permitidos";
  const max = c.max_output_tokens ? ` · saída máxima de ${c.max_output_tokens.toLocaleString("pt-BR")} tokens` : "";
  return `Parâmetros aceitos pelo modelo: ${params}${max}.`;
}

/** Edição de um alias: modelo (ou padrão do ambiente), parâmetros permitidos e ligado/desligado. */
export function AIModelForm({ item, gatewayConfigured, onDone }: { item: AIModelAlias; gatewayConfigured: boolean; onDone: () => void }) {
  const toast = useToast();
  const update = useUpdateAIModel();
  const validate = useValidateAIModel();
  const [form, setForm] = useState<Form>(() => toForm(item));
  const [localError, setLocalError] = useState<string | null>(null);
  const errors = { ...fieldErrors(update.error), ...fieldErrors(validate.error) };
  const rejected = rejectedChecks(update.error);
  const needs = [item.requirements.structured_output ? "saída estruturada (JSON Schema)" : null, item.requirements.vision ? "visão" : null].filter(Boolean);

  const set = <K extends keyof Form>(key: K, value: Form[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
    // Resultado de validação vale só para o formulário validado.
    validate.reset();
    update.reset();
    setLocalError(null);
  };

  function build(): AIModelValidateRequest | null {
    const out = toRequest(form);
    if ("error" in out) {
      setLocalError(out.error);
      return null;
    }
    setLocalError(null);
    return out.request;
  }

  function onValidate() {
    const request = build();
    if (request) validate.mutate({ alias: item.alias, body: request });
  }

  function onSave() {
    const request = build();
    if (!request) return;
    // O servidor valida de novo ao gravar; o resultado anterior sairia duplicado no diálogo.
    validate.reset();
    update.mutate(
      { alias: item.alias, body: { ...request, enabled: form.enabled, version: item.version } },
      {
        onSuccess: (saved) => {
          toast({ title: `Configuração de ${saved.alias} salva (versão ${String(saved.version)})`, description: "Vale para as próximas execuções.", variant: "success" });
          onDone();
        },
      },
    );
  }

  const otherError = update.error && !rejected && Object.keys(fieldErrors(update.error)).length === 0 ? update.error : null;
  const validateError = validate.error && Object.keys(fieldErrors(validate.error)).length === 0 ? validate.error : null;

  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        onSave();
      }}
    >
      <DialogHeader title={`Configurar ${item.alias}`} description={item.purpose} />
      {needs.length ? <p className="text-xs text-muted-foreground">Este agente exige {needs.join(" e ")}.</p> : null}

      <fieldset className="grid gap-3">
        <legend className="mb-1 text-sm font-semibold">Modelo</legend>
        <label className="inline-flex items-center gap-2 text-sm">
          <input type="radio" name="origem" className="size-4 accent-primary" checked={form.useDefault} onChange={() => set("useDefault", true)} />
          Padrão do ambiente{" "}
          <span className="font-mono text-xs text-muted-foreground">{item.default_model ?? "(não definido)"}</span>
        </label>
        <label className="inline-flex items-center gap-2 text-sm">
          <input type="radio" name="origem" className="size-4 accent-primary" checked={!form.useDefault} onChange={() => set("useDefault", false)} />
          Escolher o modelo
        </label>
        {!form.useDefault ? (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Field label="Provedor" error={errors.provider}>
              <NativeSelect value={form.provider} onChange={(e) => set("provider", e.target.value as AIProvider)}>
                {AI_PROVIDERS.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
              </NativeSelect>
            </Field>
            <Field label="Modelo" error={errors.model} hint="Nome no provedor, sem o prefixo (ex.: gpt-6-luna)." className="sm:col-span-2">
              <Input value={form.model} onChange={(e) => set("model", e.target.value)} placeholder="gpt-6-luna" autoComplete="off" spellCheck={false} />
            </Field>
          </div>
        ) : null}
      </fieldset>

      <fieldset className="grid gap-3">
        <legend className="mb-1 text-sm font-semibold">Parâmetros permitidos</legend>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <Field label="Temperature" error={errors["parameters.temperature"]} hint="De 0 a 2. Vazio = padrão do provedor.">
            <Input inputMode="decimal" value={form.temperature} onChange={(e) => set("temperature", e.target.value)} placeholder="padrão" />
          </Field>
          <Field label="max_tokens" error={errors["parameters.max_tokens"]} hint="Limite da resposta. Vazio = padrão do provedor.">
            <Input inputMode="numeric" value={form.maxTokens} onChange={(e) => set("maxTokens", e.target.value)} placeholder="padrão" />
          </Field>
        </div>
      </fieldset>

      <label className="inline-flex items-center gap-2 text-sm">
        <input type="checkbox" className="size-4 accent-primary" checked={form.enabled} onChange={(e) => set("enabled", e.target.checked)} />
        Agente ligado
      </label>
      {!form.enabled ? <p className="-mt-2 text-xs text-muted-foreground">Desligado, as execuções deste agente falham até religar.</p> : null}

      <p className="text-xs text-muted-foreground">{CREDENTIALS_NOTE}</p>

      {localError ? (
        <p className="text-xs font-medium text-destructive" role="alert">
          {localError}
        </p>
      ) : null}
      {validate.data ? (
        <ChecksPanel
          title={validate.data.ok ? `Configuração válida: ${validate.data.model ?? ""}` : "Configuração reprovada"}
          ok={validate.data.ok}
          checks={validate.data.checks}
          footer={capabilitiesText(validate.data)}
        />
      ) : null}
      {rejected ? <ChecksPanel title="Não foi gravado: o modelo não atende ao agente" ok={false} checks={rejected} /> : null}
      <ErrorAlert error={validateError} />
      <ErrorAlert error={otherError} />

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onDone}>
          Cancelar
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!gatewayConfigured || validate.isPending}
          title={gatewayConfigured ? undefined : "Gateway de IA não configurado"}
          onClick={onValidate}
        >
          {validate.isPending ? "Validando…" : "Validar configuração"}
        </Button>
        <Button type="submit" disabled={update.isPending}>
          {update.isPending ? "Salvando…" : "Salvar"}
        </Button>
      </DialogFooter>
    </form>
  );
}
