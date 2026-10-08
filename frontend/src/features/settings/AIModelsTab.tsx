import { Bot, KeyRound, Pencil, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { formatDateTime } from "@/lib/format";
import { type AIModelAlias, useAIModels } from "@/services/aiModels";
import { AIModelForm } from "./AIModelForm";
import { CREDENTIALS_NOTE, splitModel } from "./labels";

const MODULE_LABELS: Record<AIModelAlias["module"], string> = { conciliacao: "Conciliação", fator_r: "Fator R" };

function StatusBadge({ item }: { item: AIModelAlias }) {
  if (!item.enabled) return <Badge variant="muted">Desligado</Badge>;
  if (item.source === "none") return <Badge variant="warning">Sem modelo</Badge>;
  return <Badge variant="success">Ativo</Badge>;
}

function ModelCell({ item }: { item: AIModelAlias }) {
  const parts = splitModel(item.effective_model);
  if (!parts) return <span className="text-muted-foreground">—</span>;
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="font-mono text-xs">{parts[1]}</span>
      {item.source === "settings" ? <Badge variant="default">escolhido aqui</Badge> : <Badge variant="outline">padrão do ambiente</Badge>}
    </div>
  );
}

/** Parâmetros gravados, em texto curto ("temperature 0,2 · max_tokens 4.000"). */
function paramsText(p: AIModelAlias["parameters"]): string {
  const parts: string[] = [];
  if (p.temperature !== undefined) parts.push(`temperature ${p.temperature.toLocaleString("pt-BR")}`);
  if (p.max_tokens !== undefined) parts.push(`max_tokens ${p.max_tokens.toLocaleString("pt-BR")}`);
  return parts.join(" · ");
}

/**
 * Configurações → IA [REQ-040, 08 §9, F-010]: modelo e parâmetros de cada alias. A troca vale
 * para as próximas execuções; a execução em andamento mantém o modelo do início. Sem campo de
 * credencial: as chaves ficam só no ambiente do LiteLLM [REQ-047].
 */
export function AIModelsTab() {
  const { data, error, isLoading } = useAIModels();
  const [editing, setEditing] = useState<string | null>(null);
  const current = data?.items.find((i) => i.alias === editing);

  return (
    <Card>
      <CardHeader
        title="Modelos de IA"
        description="Cada agente usa um alias. Troque o modelo ou os parâmetros permitidos: a mudança vale para as próximas execuções, sem reiniciar nada."
      />
      <CardBody className="grid gap-3">
        <p role="note" className="flex items-center gap-2 rounded-md border bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
          <KeyRound className="size-3.5 shrink-0" aria-hidden />
          {CREDENTIALS_NOTE}
        </p>
        {data && !data.gateway_configured ? (
          <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" data-testid="sem-gateway">
            <TriangleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            <p>Gateway de IA não configurado (LITELLM_BASE_URL): dá para ligar e desligar agentes, mas não para validar nem trocar o modelo.</p>
          </div>
        ) : null}
        <ErrorAlert error={error} fallback="Não foi possível carregar a configuração de IA." />
        {isLoading ? <Skeleton className="h-64" /> : null}
        {data && data.items.length === 0 ? (
          <EmptyState icon={Bot} title="Nenhum alias de IA cadastrado" description="Os aliases vêm da migração do banco; rode make migrate." />
        ) : null}
        {data && data.items.length > 0 ? (
          <div className="overflow-x-auto rounded-md border">
            <Table aria-label="Modelos de IA por agente">
              <thead>
                <tr>
                  <Th>Agente</Th>
                  <Th>Provedor</Th>
                  <Th>Modelo</Th>
                  <Th>Status</Th>
                  <Th>Versão</Th>
                  <Th>Última alteração</Th>
                  <Th className="w-24">
                    <span className="sr-only">Ação</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((item) => (
                  <tr key={item.alias} data-testid={`alias-${item.alias}`}>
                    <Td>
                      <div className="font-mono text-xs font-semibold">{item.alias}</div>
                      <div className="text-xs text-muted-foreground">
                        {MODULE_LABELS[item.module]} · {item.purpose}
                      </div>
                      {paramsText(item.parameters) ? <div className="mt-0.5 text-xs text-muted-foreground">{paramsText(item.parameters)}</div> : null}
                    </Td>
                    <Td>{splitModel(item.effective_model)?.[0] ?? "—"}</Td>
                    <Td>
                      <ModelCell item={item} />
                    </Td>
                    <Td>
                      <StatusBadge item={item} />
                    </Td>
                    <Td className="tabular-nums">v{item.version}</Td>
                    <Td className="text-xs whitespace-nowrap">{formatDateTime(item.updated_at)}</Td>
                    <Td>
                      <Button variant="outline" size="sm" aria-label={`Editar ${item.alias}`} onClick={() => setEditing(item.alias)}>
                        <Pencil aria-hidden /> Editar
                      </Button>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        ) : null}
      </CardBody>
      <Dialog open={!!current} onOpenChange={(open) => (open ? null : setEditing(null))}>
        {current && data ? (
          <DialogContent size="lg">
            {/* A chave é só o alias: num 409 a lista recarrega com a versão nova e o formulário
                mantém o que foi digitado e a mensagem do conflito. */}
            <AIModelForm
              key={current.alias}
              item={current}
              gatewayConfigured={data.gateway_configured}
              onDone={() => setEditing(null)}
            />
          </DialogContent>
        ) : null}
      </Dialog>
    </Card>
  );
}
