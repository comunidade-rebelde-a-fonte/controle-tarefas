import { zodResolver } from "@hookform/resolvers/zod";
import { CircleAlert } from "lucide-react";
import { useState } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { Link, useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect, Textarea } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { formatCnpj, maskCnpjInput } from "@/lib/cnpj";
import { applyApiFieldErrors } from "@/lib/formErrors";
import { MODULE_LABELS, MODULES, PAYROLL_TYPE_LABELS, PAYROLL_TYPES, TAX_REGIME_LABELS, TAX_REGIMES, UFS } from "@/lib/labels";
import { type Company, useCreateCompany, useUpdateCompany } from "@/services/companies";
import { toCreatePayload, toUpdatePayload } from "./payload";
import { useUsers } from "@/services/users";
import { companyFormSchema, emptyCompanyForm, type CompanyFormValues } from "./schemas";

const FORM_FIELDS = Object.keys(emptyCompanyForm);

interface Conflict {
  message: string;
  company: { id: string; internal_code: string; legal_name: string } | null;
}

function toValues(c: Company): CompanyFormValues {
  return {
    internal_code: c.internal_code,
    legal_name: c.legal_name,
    trade_name: c.trade_name ?? "",
    cnpj: formatCnpj(c.cnpj),
    tax_regime: c.current_tax_regime ?? "SIMPLES_NACIONAL",
    service_start_date: c.service_start_date,
    service_end_date: c.service_end_date ?? "",
    city: c.city ?? "",
    state: c.state ?? "",
    state_registration: c.state_registration ?? "",
    municipal_registration: c.municipal_registration ?? "",
    payroll_type: c.payroll_type,
    factor_r: c.factor_r === null ? "" : c.factor_r ? "true" : "false",
    fiscal_responsible_user_id: c.responsibles.fiscal?.id ?? "",
    accounting_responsible_user_id: c.responsibles.accounting?.id ?? "",
    payroll_responsible_user_id: c.responsibles.payroll?.id ?? "",
    declarations_responsible_user_id: c.responsibles.declarations?.id ?? "",
    notes: c.notes ?? "",
    modules: c.active_modules,
  };
}

interface CompanyFormProps {
  company?: Company | undefined;
  onSubmit: (values: CompanyFormValues) => Promise<void>;
  onCancel: () => void;
  conflict: Conflict | null;
  submitting: boolean;
}

/** Formulário de cadastro/edição [PRD §7.1]. Regime e módulos iniciais só no cadastro. */
export function CompanyForm({ company, onSubmit, onCancel, conflict, submitting }: CompanyFormProps) {
  const editing = !!company;
  const users = useUsers(true);
  const form = useForm<CompanyFormValues>({
    resolver: zodResolver(companyFormSchema),
    defaultValues: company ? toValues(company) : emptyCompanyForm,
  });
  const { register, handleSubmit, formState, control, setError } = form;
  const errors = formState.errors;
  const regime = useWatch({ control, name: "tax_regime" });
  const isSimples = editing ? company.current_tax_regime === "SIMPLES_NACIONAL" : regime === "SIMPLES_NACIONAL";

  const submit = handleSubmit(async (values) => {
    try {
      await onSubmit(values);
    } catch (err) {
      applyApiFieldErrors(err, setError, FORM_FIELDS);
    }
  });

  const userOptions = (users.data?.items ?? []).map((u) => (
    <option key={u.id} value={u.id}>
      {u.name}
    </option>
  ));

  return (
    <form onSubmit={(e) => void submit(e)} noValidate className="grid gap-5" aria-label={editing ? "Editar empresa" : "Cadastrar empresa"}>
      {conflict ? (
        <div className="flex gap-2 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-900" role="alert">
          <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          <div>
            <p>{conflict.message}</p>
            {conflict.company ? (
              <Link to={`/companies/${conflict.company.id}`} className="font-medium underline underline-offset-4">
                Abrir ficha de {conflict.company.internal_code} — {conflict.company.legal_name}
              </Link>
            ) : null}
          </div>
        </div>
      ) : null}

      <fieldset className="grid grid-cols-2 items-start gap-4">
        <legend className="mb-2 text-sm font-semibold">Identificação</legend>
        <Field label="Código interno" required error={errors.internal_code?.message}>
          <Input {...register("internal_code")} autoComplete="off" />
        </Field>
        <Field label="CNPJ" required error={errors.cnpj?.message}>
          {(aria) => (
            <Controller
              control={control}
              name="cnpj"
              render={({ field }) => (
                <Input
                  {...field}
                  {...aria}
                  inputMode="numeric"
                  placeholder="00.000.000/0000-00"
                  onChange={(e) => field.onChange(maskCnpjInput(e.target.value))}
                />
              )}
            />
          )}
        </Field>
        <Field label="Razão social" required error={errors.legal_name?.message} className="col-span-2">
          <Input {...register("legal_name")} />
        </Field>
        <Field label="Nome fantasia" error={errors.trade_name?.message} className="col-span-2">
          <Input {...register("trade_name")} />
        </Field>
      </fieldset>

      <fieldset className="grid grid-cols-3 items-start gap-4">
        <legend className="mb-2 text-sm font-semibold">Tributação e atendimento</legend>
        {editing ? (
          <Field label="Regime vigente" hint="Altere pela aba Regime tributário da ficha.">
            <Input value={company.current_tax_regime ? TAX_REGIME_LABELS[company.current_tax_regime] : "—"} readOnly disabled />
          </Field>
        ) : (
          <Field label="Regime inicial" required error={errors.tax_regime?.message} hint="Vale a partir da competência de início.">
            <NativeSelect {...register("tax_regime")}>
              {TAX_REGIMES.map((r) => (
                <option key={r} value={r}>
                  {TAX_REGIME_LABELS[r]}
                </option>
              ))}
            </NativeSelect>
          </Field>
        )}
        <Field label="Início do atendimento" required error={errors.service_start_date?.message}>
          <Input type="date" {...register("service_start_date")} />
        </Field>
        <Field label="Fim do atendimento" error={errors.service_end_date?.message}>
          <Input type="date" {...register("service_end_date")} />
        </Field>
        <Field label="Tipo de folha" required error={errors.payroll_type?.message}>
          <NativeSelect {...register("payroll_type")}>
            {PAYROLL_TYPES.map((p) => (
              <option key={p} value={p}>
                {PAYROLL_TYPE_LABELS[p]}
              </option>
            ))}
          </NativeSelect>
        </Field>
        {isSimples ? (
          <Field label="Fator R" hint="Indicador do Simples Nacional.">
            <NativeSelect {...register("factor_r")}>
              <option value="">Não informado</option>
              <option value="true">Sim</option>
              <option value="false">Não</option>
            </NativeSelect>
          </Field>
        ) : null}
      </fieldset>

      <fieldset className="grid grid-cols-4 items-start gap-4">
        <legend className="mb-2 text-sm font-semibold">Localização e inscrições</legend>
        <Field label="Município" error={errors.city?.message} className="col-span-2">
          <Input {...register("city")} />
        </Field>
        <Field label="UF" error={errors.state?.message}>
          <NativeSelect {...register("state")}>
            <option value="">—</option>
            {UFS.map((uf) => (
              <option key={uf} value={uf}>
                {uf}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <div />
        <Field label="Inscrição estadual" error={errors.state_registration?.message} className="col-span-2">
          <Input {...register("state_registration")} />
        </Field>
        <Field label="Inscrição municipal" error={errors.municipal_registration?.message} className="col-span-2">
          <Input {...register("municipal_registration")} />
        </Field>
      </fieldset>

      <fieldset className="grid grid-cols-2 items-start gap-4">
        <legend className="mb-2 text-sm font-semibold">Responsáveis</legend>
        {(
          [
            ["fiscal_responsible_user_id", "Fiscal"],
            ["accounting_responsible_user_id", "Contábil"],
            ["payroll_responsible_user_id", "Folha"],
            ["declarations_responsible_user_id", "Declarações"],
          ] as const
        ).map(([name, label]) => (
          <Field key={name} label={`Responsável ${label}`} error={errors[name]?.message}>
            <NativeSelect {...register(name)}>
              <option value="">Sem responsável</option>
              {userOptions}
            </NativeSelect>
          </Field>
        ))}
      </fieldset>

      {!editing ? (
        <fieldset>
          <legend className="mb-2 text-sm font-semibold">Módulos atendidos</legend>
          <Controller
            control={control}
            name="modules"
            render={({ field }) => (
              <div className="flex flex-wrap gap-4">
                {MODULES.map((m) => (
                  <label key={m} className="inline-flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      className="size-4 accent-primary"
                      checked={field.value.includes(m)}
                      onChange={(e) =>
                        field.onChange(e.target.checked ? [...field.value, m] : field.value.filter((x) => x !== m))
                      }
                    />
                    {MODULE_LABELS[m]}
                  </label>
                ))}
              </div>
            )}
          />
        </fieldset>
      ) : null}

      <Field label="Observações cadastrais" error={errors.notes?.message}>
        <Textarea rows={3} {...register("notes")} />
      </Field>

      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>
          Cancelar
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? "Salvando…" : editing ? "Salvar alterações" : "Cadastrar empresa"}
        </Button>
      </DialogFooter>
    </form>
  );
}

function conflictFrom(err: unknown): Conflict | null {
  if (!isApiError(err) || (err.code !== "CNPJ_CONFLICT" && err.code !== "CODE_CONFLICT")) return null;
  const c = err.details.conflicting_company as Conflict["company"] | undefined;
  return { message: err.message, company: c ?? null };
}

/** Diálogo de cadastro/edição de empresa. */
export function CompanyFormDialog({
  open,
  onOpenChange,
  company,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  company?: Company | undefined;
}) {
  const toast = useToast();
  const navigate = useNavigate();
  const create = useCreateCompany();
  const update = useUpdateCompany(company?.id ?? "");
  const [conflict, setConflict] = useState<Conflict | null>(null);

  const onSubmit = async (values: CompanyFormValues) => {
    setConflict(null);
    try {
      if (company) {
        await update.mutateAsync(toUpdatePayload(values, company.version));
        toast({ title: "Empresa atualizada", variant: "success" });
        onOpenChange(false);
      } else {
        const created = await create.mutateAsync(toCreatePayload(values));
        toast({ title: "Empresa cadastrada", description: "Configure regime, módulos e aplicabilidade na ficha.", variant: "success" });
        onOpenChange(false);
        void navigate(`/companies/${created.id}`);
      }
    } catch (err) {
      const c = conflictFrom(err);
      if (c) {
        setConflict(c);
        return;
      }
      if (isApiError(err) && err.code === "VERSION_CONFLICT") {
        toast({ title: "Registro alterado por outra operação", description: "Os dados foram recarregados. Revise e salve novamente.", variant: "error" });
        onOpenChange(false);
        return;
      }
      if (!(isApiError(err) && err.code === "VALIDATION_ERROR")) {
        toast({ title: "Não foi possível salvar", description: isApiError(err) ? err.message : undefined, variant: "error" });
      }
      throw err;
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setConflict(null);
        onOpenChange(o);
      }}
    >
      <DialogContent size="lg">
        <DialogHeader
          title={company ? `Editar ${company.internal_code} — ${company.legal_name}` : "Nova empresa"}
          description={company ? "Alterações cadastrais valem para as próximas competências." : "Campos com * são obrigatórios."}
        />
        {open ? (
          <CompanyForm
            key={company?.id ?? "new"}
            company={company}
            onSubmit={onSubmit}
            onCancel={() => onOpenChange(false)}
            conflict={conflict}
            submitting={create.isPending || update.isPending}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
