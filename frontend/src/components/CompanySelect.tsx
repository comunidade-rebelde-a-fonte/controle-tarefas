import { NativeSelect } from "@/components/ui/input";
import { useCompanies } from "@/services/companies";

/** Empresa ativa do cadastro canônico (todas, ou a escolhida). */
export function CompanySelect({
  value,
  onChange,
  allLabel,
  ...props
}: {
  value: string;
  onChange: (id: string) => void;
  allLabel?: string;
  id?: string;
  "aria-invalid"?: boolean | undefined;
  "aria-describedby"?: string | undefined;
  "aria-label"?: string;
  className?: string;
}) {
  const companies = useCompanies({ active: true });
  return (
    <NativeSelect value={value} onChange={(e) => onChange(e.target.value)} {...props}>
      <option value="">{allLabel ?? "Escolha a empresa"}</option>
      {(companies.data?.items ?? []).map((c) => (
        <option key={c.id} value={c.id}>
          {c.internal_code} · {c.legal_name}
        </option>
      ))}
    </NativeSelect>
  );
}
