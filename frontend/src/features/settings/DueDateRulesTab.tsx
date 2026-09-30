import { Pencil, Plus } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { type DueDateRule, useDueDateRules, useSaveDueDateRule } from "@/services/catalog";
import { DueDateRuleForm, type DueDateRuleValues } from "./DueDateRuleForm";
import { describeRule } from "./labels";

/** Regras de vencimento. Alterações valem para as próximas gerações [RN-004]. */
export function DueDateRulesTab() {
  const toast = useToast();
  const { data, isPending } = useDueDateRules();
  const save = useSaveDueDateRule();
  const [editing, setEditing] = useState<DueDateRule | "new" | null>(null);

  const submit = async (values: DueDateRuleValues) => {
    const rule = editing === "new" ? undefined : editing ?? undefined;
    try {
      await save.mutateAsync({ ...values, notes: values.notes.trim() || null, ...(rule ? { id: rule.id, version: rule.version } : {}) });
      toast({ title: rule ? "Regra atualizada" : "Regra criada", description: "Vale para as próximas competências geradas.", variant: "success" });
      setEditing(null);
    } catch (err) {
      if (!(isApiError(err) && err.code === "VALIDATION_ERROR")) {
        toast({ title: "Não foi possível salvar a regra", description: isApiError(err) ? err.message : undefined, variant: "error" });
      }
      throw err;
    }
  };

  return (
    <Card>
      <CardHeader
        title="Regras de vencimento"
        description="Parâmetros do motor de vencimento. Regras marcadas “A VALIDAR” são provisórias até a confirmação do escritório."
        actions={
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus aria-hidden /> Nova regra
          </Button>
        }
      />
      <CardBody className="p-0">
        {isPending ? (
          <Skeleton className="m-4 h-32" />
        ) : (
          <Table aria-label="Regras de vencimento">
            <thead>
              <tr>
                <Th>Nome</Th>
                <Th>Regra</Th>
                <Th>Observações</Th>
                <Th className="w-12">
                  <span className="sr-only">Editar</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {data?.items.map((r) => (
                <tr key={r.id}>
                  <Td className="font-medium">{r.name}</Td>
                  <Td>{describeRule(r)}</Td>
                  <Td>{r.notes?.includes("A VALIDAR") ? <Badge variant="warning">{r.notes}</Badge> : (r.notes ?? "—")}</Td>
                  <Td>
                    <Button variant="ghost" size="icon-sm" aria-label={`Editar ${r.name}`} onClick={() => setEditing(r)}>
                      <Pencil aria-hidden />
                    </Button>
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </CardBody>
      <Dialog open={editing !== null} onOpenChange={(o) => !o && setEditing(null)}>
        <DialogContent>
          <DialogHeader title={editing === "new" ? "Nova regra de vencimento" : "Editar regra de vencimento"} />
          {editing !== null ? (
            <DueDateRuleForm
              key={editing === "new" ? "new" : editing.id}
              rule={editing === "new" ? undefined : editing}
              onSubmit={submit}
              onCancel={() => setEditing(null)}
              submitting={save.isPending}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </Card>
  );
}
