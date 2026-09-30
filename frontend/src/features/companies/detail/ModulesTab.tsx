import { useState } from "react";
import { CompetenceInput } from "@/components/CompetenceInput";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { displayCompetence } from "@/lib/competence";
import { MODULE_LABELS } from "@/lib/labels";
import { useCompanyModules, useSetCompanyModule } from "@/services/companies";

/** Ativação de módulos com vigência. Não reescreve competências já geradas [R6, RN-004]. */
export function ModulesTab({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { data, isPending } = useCompanyModules(companyId);
  const set = useSetCompanyModule(companyId);
  const [chosen, setFrom] = useState("");
  // Vigência padrão: próxima competência ainda não gerada (sugestão da API), até o usuário escolher outra.
  const from = chosen || data?.suggested_effective_from || "";

  if (isPending || !data) return <Skeleton className="h-40" />;

  return (
    <Card>
      <CardHeader
        title="Módulos atendidos"
        description={`Situação na competência ${displayCompetence(data.reference_competence)}. Alterações valem a partir da competência escolhida.`}
        actions={
          <Field label="Vigência das alterações" hint="Sugestão: próxima competência ainda não gerada.">
            <CompetenceInput value={from} onChange={setFrom} />
          </Field>
        }
      />
      <CardBody className="grid gap-3 p-0">
        <Table aria-label="Módulos">
          <thead>
            <tr>
              <Th>Módulo</Th>
              <Th>Situação</Th>
              <Th>Histórico de vigências</Th>
              <Th className="w-56">
                <span className="sr-only">Ação</span>
              </Th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((m) => (
              <tr key={m.module}>
                <Td className="font-medium">{MODULE_LABELS[m.module]}</Td>
                <Td>{m.enabled ? <Badge variant="success">Ativo</Badge> : <Badge variant="muted">Inativo</Badge>}</Td>
                <Td className="text-xs text-muted-foreground">
                  {m.timeline.length === 0
                    ? "Sem histórico"
                    : m.timeline
                        .map((p) => `${displayCompetence(p.effective_from)} – ${p.effective_to ? displayCompetence(p.effective_to) : "atual"}: ${p.enabled ? "ativo" : "inativo"}`)
                        .join(" · ")}
                </Td>
                <Td>
                  <Button
                    size="sm"
                    variant={m.enabled ? "outline" : "default"}
                    disabled={!from || set.isPending}
                    onClick={() =>
                      set.mutate(
                        { module: m.module, enabled: !m.enabled, effective_from: from },
                        {
                          onSuccess: () =>
                            toast({
                              title: `${MODULE_LABELS[m.module]} ${m.enabled ? "desativado" : "ativado"} a partir de ${displayCompetence(from)}`,
                              variant: "success",
                            }),
                        },
                      )
                    }
                  >
                    {m.enabled ? `Desativar a partir de ${displayCompetence(from)}` : `Ativar a partir de ${displayCompetence(from)}`}
                  </Button>
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
        <div className="px-4 pb-4">
          <ErrorAlert error={set.error} />
        </div>
      </CardBody>
    </Card>
  );
}
