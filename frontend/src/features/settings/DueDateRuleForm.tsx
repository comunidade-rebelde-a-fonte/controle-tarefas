import { zodResolver } from "@hookform/resolvers/zod";
import { useForm, useWatch } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { DialogFooter } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect, Textarea } from "@/components/ui/input";
import { applyApiFieldErrors } from "@/lib/formErrors";
import type { DueDateRule } from "@/services/catalog";
import { ADJUSTMENT_LABELS, DAY_TYPE_LABELS, describeRule } from "./labels";

const schema = z.object({
  name: z.string().trim().min(1, "Informe o nome da regra.").max(120),
  month_offset: z.coerce.number<string>().int("Use um número inteiro.").min(0, "Mínimo 0.").max(12, "Máximo 12."),
  base_day: z.coerce.number<string>().int("Use um número inteiro.").min(1, "Entre 1 e 31.").max(31, "Entre 1 e 31."),
  day_type: z.enum(["CALENDAR", "BUSINESS"]),
  non_business_day_adjustment: z.enum(["PREVIOUS", "NEXT", "NONE"]),
  notes: z.string().max(500),
});

type FormInput = z.input<typeof schema>;
export type DueDateRuleValues = z.output<typeof schema>;

const FIELDS = ["name", "month_offset", "base_day", "day_type", "non_business_day_adjustment", "notes"];

/** Formulário de regra de vencimento [PRD §21.1]. O cálculo das datas é feito pela API. */
export function DueDateRuleForm({
  rule,
  onSubmit,
  onCancel,
  submitting,
}: {
  rule?: DueDateRule | undefined;
  onSubmit: (values: DueDateRuleValues) => Promise<void>;
  onCancel: () => void;
  submitting: boolean;
}) {
  const form = useForm<FormInput, unknown, DueDateRuleValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: rule?.name ?? "",
      month_offset: String(rule?.month_offset ?? 1),
      base_day: String(rule?.base_day ?? 20),
      day_type: rule?.day_type ?? "CALENDAR",
      non_business_day_adjustment: rule?.non_business_day_adjustment ?? "PREVIOUS",
      notes: rule?.notes ?? "",
    },
  });
  const { register, handleSubmit, formState, control, setError } = form;
  const errors = formState.errors;
  const watched = useWatch({ control });
  const preview = describeRule({
    month_offset: Number(watched.month_offset),
    base_day: Number(watched.base_day),
    day_type: watched.day_type ?? "CALENDAR",
    non_business_day_adjustment: watched.non_business_day_adjustment ?? "PREVIOUS",
  });

  const submit = handleSubmit(async (values) => {
    try {
      await onSubmit(values);
    } catch (err) {
      applyApiFieldErrors(err, setError, FIELDS);
    }
  });

  return (
    <form onSubmit={(e) => void submit(e)} noValidate className="grid gap-4" aria-label="Regra de vencimento">
      <Field label="Nome" required error={errors.name?.message}>
        <Input {...register("name")} />
      </Field>
      <div className="grid grid-cols-2 items-start gap-4">
        <Field label="Meses após a competência" required error={errors.month_offset?.message} hint="0 = no próprio mês da competência.">
          <Input type="number" min={0} max={12} {...register("month_offset")} />
        </Field>
        <Field label="Dia-base" required error={errors.base_day?.message} hint="Dia N (corrido, limitado ao fim do mês) ou N-ésimo dia útil.">
          <Input type="number" min={1} max={31} {...register("base_day")} />
        </Field>
        <Field label="Tipo de dia" required error={errors.day_type?.message}>
          <NativeSelect {...register("day_type")}>
            {Object.entries(DAY_TYPE_LABELS).map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label="Se cair em dia não útil" required error={errors.non_business_day_adjustment?.message}>
          <NativeSelect {...register("non_business_day_adjustment")}>
            {Object.entries(ADJUSTMENT_LABELS).map(([v, l]) => (
              <option key={v} value={v}>
                {l}
              </option>
            ))}
          </NativeSelect>
        </Field>
      </div>
      <p className="rounded-md bg-muted px-3 py-2 text-sm" aria-live="polite">
        <span className="text-muted-foreground">Resumo: </span>
        {preview}
      </p>
      <Field label="Observações" error={errors.notes?.message}>
        <Textarea rows={2} {...register("notes")} />
      </Field>
      <DialogFooter>
        <Button variant="outline" onClick={onCancel}>
          Cancelar
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? "Salvando…" : "Salvar regra"}
        </Button>
      </DialogFooter>
    </form>
  );
}
