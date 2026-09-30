import { useState } from "react";
import { CompetenceInput } from "@/components/CompetenceInput";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import type { Schemas } from "@/lib/api";
import { displayCompetence } from "@/lib/competence";
import { MODULE_LABELS, TAX_REGIME_LABELS } from "@/lib/labels";
import { useObligationOverrides, useSetObligationOverride } from "@/services/companies";

type Applicability = Schemas["Applicability"];

const APPLICABILITY_LABELS: Record<Applicability, string> = {
  DEFAULT: "Padrão",
  APPLICABLE: "Aplicável",
  NOT_APPLICABLE: "Não se aplica",
};

const MODULE_ORDER = ["FISCAL", "DECLARATIONS", "PAYROLL", "ACCOUNTING"] as const;

function ResultBadge({ value }: { value: boolean | null }) {
  if (value === null) return <Badge variant="muted">Não gerado no regime</Badge>;
  return value ? <Badge variant="success">Aplicável</Badge> : <Badge variant="warning">Não se aplica</Badge>;
}

/** Aplicabilidade por tipo: Padrão / Aplicável / Não se aplica com vigência [PRD §9, RN-015]. */
export function OverridesTab({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { data, isPending } = useObligationOverrides(companyId);
  const set = useSetObligationOverride(companyId);
  const [chosen, setFrom] = useState("");
  // Vigência padrão: próxima competência ainda não gerada (sugestão da API), até o usuário escolher outra.
  const from = chosen || data?.suggested_effective_from || "";

  if (isPending || !data) return <Skeleton className="h-60" />;

  return (
    <Card>
      <CardHeader
        title="Obrigações e aplicabilidade"
        description={`Regra padrão pelo regime ${data.tax_regime ? TAX_REGIME_LABELS[data.tax_regime] : "—"} em ${displayCompetence(data.reference_competence)}. A dispensa continua nas competências seguintes até nova alteração.`}
        actions={
          <Field label="Vigência das alterações" hint="Sugestão: próxima competência ainda não gerada.">
            <CompetenceInput value={from} onChange={setFrom} />
          </Field>
        }
      />
      <CardBody className="grid gap-4">
        <ErrorAlert error={set.error} />
        {MODULE_ORDER.map((module) => {
          const items = data.items.filter((i) => i.obligation_type.primary_module === module);
          if (items.length === 0) return null;
          return (
            <div key={module}>
              <h3 className="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">{MODULE_LABELS[module]}</h3>
              <div className="rounded-md border">
                <Table aria-label={`Aplicabilidade — ${MODULE_LABELS[module]}`}>
                  <thead>
                    <tr>
                      <Th>Obrigação</Th>
                      <Th>Regra padrão</Th>
                      <Th>Configuração da empresa</Th>
                      <Th>Resultado</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((i) => (
                      <tr key={i.obligation_type.id}>
                        <Td className="font-medium">{i.obligation_type.name}</Td>
                        <Td>
                          <ResultBadge value={i.default_applicable} />
                        </Td>
                        <Td>
                          <NativeSelect
                            aria-label={`Configuração de ${i.obligation_type.name}`}
                            className="w-44"
                            value={i.applicability}
                            disabled={!from || set.isPending || i.default_applicable === null}
                            onChange={(e) => {
                              const applicability = e.target.value as Applicability;
                              set.mutate(
                                { obligationTypeId: i.obligation_type.id, applicability, effective_from: from },
                                {
                                  onSuccess: () =>
                                    toast({
                                      title: `${i.obligation_type.name}: ${APPLICABILITY_LABELS[applicability]} a partir de ${displayCompetence(from)}`,
                                      variant: "success",
                                    }),
                                },
                              );
                            }}
                          >
                            {(Object.keys(APPLICABILITY_LABELS) as Applicability[]).map((a) => (
                              <option key={a} value={a}>
                                {APPLICABILITY_LABELS[a]}
                              </option>
                            ))}
                          </NativeSelect>
                        </Td>
                        <Td>
                          <ResultBadge value={i.effective_applicable} />
                        </Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              </div>
            </div>
          );
        })}
      </CardBody>
    </Card>
  );
}
