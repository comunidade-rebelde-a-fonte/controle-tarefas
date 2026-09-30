import { Field } from "@/components/ui/field";
import { Textarea } from "@/components/ui/input";

/** Campo de motivo obrigatório (reabertura, exceção de dependência, vencimento manual). */
export function ReasonField({
  label = "Motivo",
  value,
  onChange,
  error,
  placeholder,
}: {
  label?: string;
  value: string;
  onChange: (value: string) => void;
  error?: string | undefined;
  placeholder?: string;
}) {
  return (
    <Field label={label} required error={error}>
      <Textarea rows={3} maxLength={1000} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
    </Field>
  );
}
