import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { NativeSelect } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { PAYROLL_TYPE_LABELS, PAYROLL_TYPES, type PayrollType } from "@/lib/labels";
import { type Company, useUpdateCompany } from "@/services/companies";

/** Tipo de folha e fator R (só Simples) [PRD §17.1-§17.2]. Valem para as próximas competências. */
export function PayrollTab({ company }: { company: Company }) {
  const toast = useToast();
  const update = useUpdateCompany(company.id);
  const [payrollType, setPayrollType] = useState<PayrollType>(company.payroll_type);
  const [factorR, setFactorR] = useState(company.factor_r === null ? "" : String(company.factor_r));
  const isSimples = company.current_tax_regime === "SIMPLES_NACIONAL";
  const dirty = payrollType !== company.payroll_type || factorR !== (company.factor_r === null ? "" : String(company.factor_r));

  return (
    <Card>
      <CardHeader title="Folha" description="Característica cadastral copiada para o snapshot das próximas competências." />
      <CardBody className="grid gap-4">
        <div className="flex flex-wrap items-end gap-4">
          <Field label="Tipo de folha" className="w-56">
            <NativeSelect value={payrollType} onChange={(e) => setPayrollType(e.target.value as PayrollType)}>
              {PAYROLL_TYPES.map((p) => (
                <option key={p} value={p}>
                  {PAYROLL_TYPE_LABELS[p]}
                </option>
              ))}
            </NativeSelect>
          </Field>
          {isSimples ? (
            <Field label="Fator R" className="w-44" hint="Indicador do Simples Nacional, sem cálculo.">
              <NativeSelect value={factorR} onChange={(e) => setFactorR(e.target.value)}>
                <option value="">Não informado</option>
                <option value="true">Sim</option>
                <option value="false">Não</option>
              </NativeSelect>
            </Field>
          ) : null}
          <Button
            disabled={!dirty || update.isPending}
            onClick={() =>
              update.mutate(
                { version: company.version, payroll_type: payrollType, ...(isSimples ? { factor_r: factorR === "" ? null : factorR === "true" } : {}) },
                { onSuccess: () => toast({ title: "Folha atualizada", variant: "success" }) },
              )
            }
          >
            Salvar
          </Button>
        </div>
        <ErrorAlert error={update.error} />
      </CardBody>
    </Card>
  );
}
