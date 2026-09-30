import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { NativeSelect } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { type Company, useUpdateCompany } from "@/services/companies";
import { useUsers } from "@/services/users";

const SECTORS = [
  ["fiscal", "fiscal_responsible_user_id", "Fiscal"],
  ["accounting", "accounting_responsible_user_id", "Contábil"],
  ["payroll", "payroll_responsible_user_id", "Folha"],
  ["declarations", "declarations_responsible_user_id", "Declarações"],
] as const;

type Sector = (typeof SECTORS)[number][0];

/** Responsáveis por setor [PRD §34.4]. */
export function ResponsiblesTab({ company }: { company: Company }) {
  const toast = useToast();
  const users = useUsers(true);
  const update = useUpdateCompany(company.id);
  const initial = Object.fromEntries(SECTORS.map(([s]) => [s, company.responsibles[s]?.id ?? ""])) as Record<Sector, string>;
  const [values, setValues] = useState(initial);
  const dirty = SECTORS.some(([s]) => values[s] !== initial[s]);

  return (
    <Card>
      <CardHeader title="Responsáveis" description="Quem conclui uma obrigação fica registrado no histórico, independentemente do responsável cadastrado." />
      <CardBody className="grid gap-4">
        <div className="grid grid-cols-4 gap-4">
          {SECTORS.map(([sector, , label]) => (
            <Field key={sector} label={label}>
              <NativeSelect value={values[sector]} onChange={(e) => setValues((v) => ({ ...v, [sector]: e.target.value }))}>
                <option value="">Sem responsável</option>
                {(users.data?.items ?? []).map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}
                  </option>
                ))}
              </NativeSelect>
            </Field>
          ))}
        </div>
        <div>
          <Button
            disabled={!dirty || update.isPending}
            onClick={() =>
              update.mutate(
                Object.assign(
                  { version: company.version },
                  ...SECTORS.map(([sector, field]) => ({ [field]: values[sector] === "" ? null : values[sector] })),
                ) as Parameters<typeof update.mutate>[0],
                { onSuccess: () => toast({ title: "Responsáveis atualizados", variant: "success" }) },
              )
            }
          >
            Salvar responsáveis
          </Button>
        </div>
        <ErrorAlert error={update.error} />
      </CardBody>
    </Card>
  );
}
