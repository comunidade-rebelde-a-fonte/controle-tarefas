import { useState } from "react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect, Textarea } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { moneyToInput, parseMoneyInput } from "@/lib/decimal";
import type { Company } from "@/services/companies";
import { type FactorRPackage, type FactorRSettings, useFactorRSettings, useUpdateFactorRSettings } from "@/services/factorR";
import { PACKAGE_LABELS, PACKAGES } from "./labels";

type Form = {
  cnae: string;
  atividade: string;
  quantidade_socios: string;
  contato: string;
  pacote_comercial: FactorRPackage | "";
  honorario_mensal: string;
  inicio_atividade: string;
  notas: string;
};

function toForm(s: FactorRSettings): Form {
  return {
    cnae: s.cnae ?? "",
    atividade: s.atividade ?? "",
    quantidade_socios: String(s.quantidade_socios),
    contato: s.contato ?? "",
    pacote_comercial: s.pacote_comercial ?? "",
    honorario_mensal: moneyToInput(s.honorario_mensal),
    inicio_atividade: s.inicio_atividade ?? "",
    notas: s.notas ?? "",
  };
}

function fieldErrors(err: unknown): Record<string, string> {
  if (!isApiError(err) || err.code !== "VALIDATION_ERROR") return {};
  const fields = err.details.fields;
  return fields && typeof fields === "object" ? (fields as Record<string, string>) : {};
}

function SettingsForm({ companyId, settings }: { companyId: string; settings: FactorRSettings }) {
  const toast = useToast();
  const update = useUpdateFactorRSettings(companyId);
  const [form, setForm] = useState<Form>(() => toForm(settings));
  const [localError, setLocalError] = useState<string | null>(null);
  const errors = fieldErrors(update.error);
  const set = <K extends keyof Form>(key: K, value: Form[K]) => setForm((f) => ({ ...f, [key]: value }));

  function submit() {
    setLocalError(null);
    const fee = parseMoneyInput(form.honorario_mensal);
    if (fee === null) {
      setLocalError("Honorário inválido. Use o formato 1.234,56.");
      return;
    }
    const partners = Number(form.quantidade_socios);
    update.mutate(
      {
        version: settings.version,
        cnae: form.cnae || null,
        atividade: form.atividade || null,
        quantidade_socios: Number.isInteger(partners) ? partners : -1,
        contato: form.contato || null,
        pacote_comercial: form.pacote_comercial || null,
        honorario_mensal: fee,
        inicio_atividade: form.inicio_atividade || null,
        notas: form.notas || null,
      },
      { onSuccess: () => toast({ title: "Configuração do Fator R salva", variant: "success" }) },
    );
  }

  return (
    <form
      className="grid gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Field label="CNAE" error={errors.cnae}>
          <Input value={form.cnae} onChange={(e) => set("cnae", e.target.value)} maxLength={20} />
        </Field>
        <Field label="Atividade" error={errors.atividade} className="md:col-span-2">
          <Input value={form.atividade} onChange={(e) => set("atividade", e.target.value)} maxLength={500} />
        </Field>
        <Field label="Início da atividade (mês)" error={errors.inicio_atividade} hint="Empresa com menos de 12 meses na janela fica com dados insuficientes.">
          <Input type="month" value={form.inicio_atividade} onChange={(e) => set("inicio_atividade", e.target.value)} />
        </Field>
        <Field label="Pacote comercial" error={errors.pacote_comercial}>
          <NativeSelect value={form.pacote_comercial} onChange={(e) => set("pacote_comercial", e.target.value as FactorRPackage | "")}>
            <option value="">Sem pacote</option>
            {PACKAGES.map((p) => (
              <option key={p} value={p}>
                {PACKAGE_LABELS[p]}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label="Honorário mensal do pacote (R$)" error={errors.honorario_mensal ?? localError ?? undefined}>
          <Input inputMode="decimal" placeholder="0,00" value={form.honorario_mensal} onChange={(e) => set("honorario_mensal", e.target.value)} />
        </Field>
        <Field label="Quantidade de sócios" error={errors.quantidade_socios}>
          <Input type="number" min={0} max={1000} value={form.quantidade_socios} onChange={(e) => set("quantidade_socios", e.target.value)} />
        </Field>
        <Field label="Contato" error={errors.contato}>
          <Input value={form.contato} onChange={(e) => set("contato", e.target.value)} maxLength={500} />
        </Field>
        <Field label="Notas do Fator R" error={errors.notas} className="md:col-span-4">
          <Textarea value={form.notas} onChange={(e) => set("notas", e.target.value)} maxLength={5000} />
        </Field>
      </div>
      <div className="flex items-center gap-3">
        <Button type="submit" disabled={update.isPending}>
          {update.isPending ? "Salvando…" : "Salvar configuração"}
        </Button>
      </div>
      {Object.keys(errors).length === 0 ? <ErrorAlert error={update.error} /> : null}
    </form>
  );
}

/**
 * Configuração do Fator R da empresa (company_factor_r_settings). "Sujeita ao Fator R" é o
 * indicador do cadastro, editado na aba Folha (D-017).
 */
export function SettingsCard({ company }: { company: Company }) {
  const settings = useFactorRSettings(company.id);
  const subject = company.factor_r === null ? "não informado" : company.factor_r ? "sim" : "não";
  return (
    <Card>
      <CardHeader
        title="Configuração do Fator R"
        description={
          <>
            Sujeita ao Fator R: <strong>{subject}</strong> (altere na{" "}
            <Link to={`/companies/${company.id}?tab=payroll`} className="underline">
              aba Folha
            </Link>
            ).
          </>
        }
      />
      <CardBody>
        <ErrorAlert error={settings.error} fallback="Não foi possível carregar a configuração." />
        {settings.isPending ? (
          <Skeleton className="h-48" />
        ) : settings.data ? (
          <SettingsForm key={settings.data.version} companyId={company.id} settings={settings.data} />
        ) : null}
      </CardBody>
    </Card>
  );
}
