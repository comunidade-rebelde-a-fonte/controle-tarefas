import { Plus, X } from "lucide-react";
import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/toast-context";
import { useCredentialThresholds, useSetCredentialThresholds } from "@/services/settings";

/** Faixas de validade de procurações e certificados [PRD §19.3]. */
export function ThresholdsTab() {
  const { data } = useCredentialThresholds();
  if (!data) return <Skeleton className="h-32" />;
  return <ThresholdsForm key={data.days.join(",")} initial={data.days} />;
}

function ThresholdsForm({ initial }: { initial: number[] }) {
  const toast = useToast();
  const save = useSetCredentialThresholds();
  const [days, setDays] = useState(initial.map(String));
  const values = days.map(Number);
  const dirty = days.join(",") !== initial.join(",");

  return (
    <Card>
      <CardHeader
        title="Faixas de validade"
        description="Procurações e certificados são classificados em: Vencido, Vence em até N dias (para cada faixa), Vigente e Sem data cadastrada."
      />
      <CardBody className="grid gap-4">
        <div className="flex flex-wrap items-center gap-2">
          {days.map((d, i) => (
            <div key={i} className="flex items-center gap-1 rounded-md border bg-muted/50 py-1 pr-1 pl-2 text-sm">
              <span className="text-muted-foreground">até</span>
              <Input
                type="number"
                min={1}
                max={365}
                aria-label={`Faixa ${String(i + 1)} (dias)`}
                className="h-7 w-16"
                value={d}
                onChange={(e) => setDays((prev) => prev.map((x, j) => (j === i ? e.target.value : x)))}
              />
              <span className="text-muted-foreground">dias</span>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`Remover faixa ${String(i + 1)}`}
                disabled={days.length <= 1}
                onClick={() => setDays((prev) => prev.filter((_, j) => j !== i))}
              >
                <X aria-hidden />
              </Button>
            </div>
          ))}
          <Button variant="outline" size="sm" disabled={days.length >= 6} onClick={() => setDays((prev) => [...prev, ""])}>
            <Plus aria-hidden /> Faixa
          </Button>
        </div>
        <div>
          <Button
            disabled={!dirty || save.isPending || values.some((v) => !Number.isInteger(v))}
            onClick={() => save.mutate(values, { onSuccess: () => toast({ title: "Faixas atualizadas", variant: "success" }) })}
          >
            Salvar faixas
          </Button>
        </div>
        <ErrorAlert error={save.error} />
      </CardBody>
    </Card>
  );
}
