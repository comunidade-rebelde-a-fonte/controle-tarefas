import { useState } from "react";
import { CompetenceInput } from "@/components/CompetenceInput";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { displayCompetence } from "@/lib/competence";
import { formatDateTime } from "@/lib/format";
import { TAX_REGIME_LABELS, TAX_REGIMES, type TaxRegime } from "@/lib/labels";
import { useChangeTaxRegime, useTaxRegimes } from "@/services/companies";

/** Linha do tempo de regimes e inclusão de novo regime com vigência [UC-04]. */
export function RegimeTab({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { data, isPending } = useTaxRegimes(companyId);
  const change = useChangeTaxRegime(companyId);
  const [regime, setRegime] = useState<TaxRegime | "">("");
  const [from, setFrom] = useState("");

  const submit = () => {
    if (!regime || !from) return;
    change.mutate(
      { tax_regime: regime, effective_from: from },
      {
        onSuccess: () => {
          toast({
            title: "Regime incluído",
            description: `${TAX_REGIME_LABELS[regime]} a partir de ${displayCompetence(from)}. A vigência anterior foi encerrada no mês anterior.`,
            variant: "success",
          });
          setRegime("");
          setFrom("");
        },
      },
    );
  };

  return (
    <div className="grid gap-4">
      <Card>
        <CardHeader title="Linha do tempo" description="O regime usado em cada competência fica preservado no snapshot da competência." />
        <CardBody className="p-0">
          {isPending ? (
            <Skeleton className="m-4 h-20" />
          ) : (
            <Table aria-label="Linha do tempo de regimes">
              <thead>
                <tr>
                  <Th>Regime</Th>
                  <Th>Vigência inicial</Th>
                  <Th>Vigência final</Th>
                  <Th>Registrado por</Th>
                  <Th>Registrado em</Th>
                </tr>
              </thead>
              <tbody>
                {data?.items.map((p) => (
                  <tr key={p.id}>
                    <Td>
                      <span className="font-medium">{TAX_REGIME_LABELS[p.tax_regime]}</span>{" "}
                      <Badge variant="muted">{p.regime_group === "SIMPLES" ? "Simples" : "Normal"}</Badge>
                    </Td>
                    <Td className="tabular-nums">{displayCompetence(p.effective_from)}</Td>
                    <Td className="tabular-nums">{p.effective_to ? displayCompetence(p.effective_to) : <Badge variant="success">Vigente</Badge>}</Td>
                    <Td>{p.created_by.name}</Td>
                    <Td className="text-xs tabular-nums text-muted-foreground">{formatDateTime(p.created_at)}</Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </CardBody>
      </Card>

      <Card>
        <CardHeader title="Incluir novo regime" description="A vigência anterior é encerrada automaticamente no mês anterior à nova competência inicial." />
        <CardBody className="grid gap-3">
          <div className="flex flex-wrap items-end gap-3">
            <Field label="Novo regime" className="w-56">
              <NativeSelect value={regime} onChange={(e) => setRegime(e.target.value as TaxRegime | "")}>
                <option value="">Selecione…</option>
                {TAX_REGIMES.map((r) => (
                  <option key={r} value={r}>
                    {TAX_REGIME_LABELS[r]}
                  </option>
                ))}
              </NativeSelect>
            </Field>
            <Field label="Competência inicial">
              <CompetenceInput value={from} onChange={setFrom} />
            </Field>
            <Button onClick={submit} disabled={!regime || !from || change.isPending}>
              {change.isPending ? "Salvando…" : "Incluir regime"}
            </Button>
          </div>
          <ErrorAlert error={change.error} />
        </CardBody>
      </Card>
    </div>
  );
}
