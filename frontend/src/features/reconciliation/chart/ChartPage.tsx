import { Copy, ListTree, Pencil, Plus, Power, PowerOff, Upload } from "lucide-react";
import { type SubmitEvent, useId, useRef, useState } from "react";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ConfirmDialog, Dialog, DialogContent, DialogFooter, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input, Label } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { type ChartAccount, useChart, useChartActions } from "@/services/reconciliation";
import { CompanySelect, CsvImportError } from "../common";
import { useSearchUpdate } from "../model";
import { fieldErrorsOf } from "../review/model";

/** Inclui uma conta (M5.1): código sem espaço nem ponto e vírgula, e o nome. */
function NewAccountForm({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { create } = useChartActions(companyId);
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const errors = fieldErrorsOf(create.error);

  function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    create.mutate(
      { code: code.trim(), name: name.trim() },
      {
        onSuccess: (a) => {
          setCode("");
          setName("");
          toast({ title: `Conta ${a.code} incluída`, variant: "success" });
        },
      },
    );
  }

  return (
    <form onSubmit={submit} aria-label="Nova conta" className="grid gap-3">
      <h2 className="text-sm font-semibold">Nova conta</h2>
      <div className="flex flex-wrap items-start gap-3">
        <Field label="Código" required error={errors.code} className="w-40">
          <Input maxLength={32} value={code} onChange={(e) => setCode(e.target.value)} required />
        </Field>
        <Field label="Nome" required error={errors.name} className="min-w-72 flex-1">
          <Input maxLength={255} value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <Button type="submit" size="sm" className="mt-6" disabled={create.isPending}>
          <Plus aria-hidden /> Incluir
        </Button>
      </div>
      {errors.code || errors.name ? null : <ErrorAlert error={create.error} fallback="Não foi possível incluir a conta." />}
    </form>
  );
}

/** Importa o plano de um CSV (codigo;nome), tudo ou nada [CC-20]. */
function ImportChart({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { importCsv } = useChartActions(companyId);
  const input = useRef<HTMLInputElement>(null);
  const inputId = useId();
  const [file, setFile] = useState<File | null>(null);
  return (
    <div className="grid gap-2">
      <h2 className="text-sm font-semibold">Importar de um CSV</h2>
      <div className="flex flex-wrap items-end gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor={inputId}>Arquivo do plano (codigo;nome)</Label>
          <input
            ref={input}
            id={inputId}
            type="file"
            accept=".csv,text/csv"
            className="text-sm file:mr-2 file:rounded file:border file:bg-card file:px-2 file:py-1"
            onChange={(e) => {
              importCsv.reset();
              setFile(e.target.files?.[0] ?? null);
            }}
          />
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={!file || importCsv.isPending}
          onClick={() => {
            if (!file) return;
            importCsv.mutate(file, {
              onSuccess: (r) => {
                setFile(null);
                if (input.current) input.current.value = "";
                toast({ title: "Plano importado", description: `${String(r.inserted)} contas novas e ${String(r.updated)} atualizadas.`, variant: "success" });
              },
            });
          }}
        >
          <Upload aria-hidden /> Importar CSV
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        Uma conta por linha, separada por ponto e vírgula. Conta que já existe ganha o nome do arquivo e volta a ativa. Com uma linha
        inválida, nada é gravado.
      </p>
      <CsvImportError error={importCsv.error} fallback="Não foi possível importar o plano." />
    </div>
  );
}

/** Copia as contas ativas de outra empresa (o plano padrão); as que esta já tem ficam como estão. */
function CopyChart({ companyId }: { companyId: string }) {
  const toast = useToast();
  const { copy } = useChartActions(companyId);
  const [source, setSource] = useState("");
  const sourceId = useId();
  const valid = !!source && source !== companyId;
  return (
    <div className="grid gap-2">
      <h2 className="text-sm font-semibold">Copiar de outra empresa</h2>
      <div className="flex flex-wrap items-end gap-2">
        <div className="grid gap-1.5">
          <Label htmlFor={sourceId}>Empresa de origem</Label>
          <CompanySelect
            id={sourceId}
            value={source}
            onChange={(id) => {
              copy.reset();
              setSource(id);
            }}
          />
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={!valid || copy.isPending}
          onClick={() =>
            copy.mutate(source, {
              onSuccess: (r) =>
                toast({ title: "Plano copiado", description: `${String(r.inserted)} contas copiadas; ${String(r.skipped)} já existiam.`, variant: "success" }),
            })
          }
        >
          <Copy aria-hidden /> Copiar o plano
        </Button>
      </div>
      {source && source === companyId ? <p className="text-xs text-red-800">Escolha uma empresa diferente desta.</p> : null}
      <ErrorAlert error={copy.error} fallback="Não foi possível copiar o plano." />
    </div>
  );
}

/** Renomeia a conta; o código não muda. */
function RenameDialog({ companyId, account, onClose }: { companyId: string; account: ChartAccount; onClose: () => void }) {
  const toast = useToast();
  const { update } = useChartActions(companyId);
  const [name, setName] = useState(account.name);
  const errors = fieldErrorsOf(update.error);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent size="sm">
        <DialogHeader title={`Renomear a conta ${account.code}`} description="O código não muda; as classificações e regras continuam apontando para ela." />
        <form
          className="grid gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            update.mutate(
              { accountId: account.id, name: name.trim() },
              {
                onSuccess: () => {
                  toast({ title: "Conta renomeada", variant: "success" });
                  onClose();
                },
              },
            );
          }}
        >
          <Field label="Nome" required error={errors.name}>
            <Input maxLength={255} value={name} onChange={(e) => setName(e.target.value)} required />
          </Field>
          {errors.name ? null : <ErrorAlert error={update.error} fallback="Não foi possível renomear." />}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancelar
            </Button>
            <Button type="submit" disabled={update.isPending || !name.trim()}>
              Salvar
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Desativa ou reativa a conta; desativar pede confirmação (sai das classificações novas). */
function ToggleAccount({ companyId, account }: { companyId: string; account: ChartAccount }) {
  const toast = useToast();
  const { update } = useChartActions(companyId);
  const [confirm, setConfirm] = useState(false);
  const toggle = (active: boolean) =>
    update.mutate(
      { accountId: account.id, active },
      {
        onSuccess: () => {
          setConfirm(false);
          toast({ title: active ? "Conta reativada" : "Conta desativada", variant: "success" });
        },
      },
    );
  if (!account.active) {
    return (
      <Button size="sm" variant="outline" disabled={update.isPending} onClick={() => toggle(true)} aria-label={`Reativar a conta ${account.code}`}>
        <Power aria-hidden /> Reativar
      </Button>
    );
  }
  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setConfirm(true)} aria-label={`Desativar a conta ${account.code}`}>
        <PowerOff aria-hidden /> Desativar
      </Button>
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={`Desativar a conta ${account.code}?`}
        description="A conta sai das classificações novas, das regras que apontam para ela e do histórico. O que já foi classificado nela fica, e nada é apagado: dá para reativar depois."
        confirmLabel="Desativar"
        destructive
        pending={update.isPending}
        onConfirm={() => toggle(false)}
      >
        <ErrorAlert error={update.error} fallback="Não foi possível desativar." />
      </ConfirmDialog>
    </>
  );
}

/**
 * Plano de contas da empresa (plano_contas da origem; CC-20, M5.1): incluir, renomear,
 * desativar e reativar, importar do CSV e copiar de outra empresa. Nada é apagado.
 */
export function ChartPage() {
  const [params, update] = useSearchUpdate();
  const companyId = params.get("company_id") ?? "";
  const chart = useChart(companyId || undefined);
  const [renaming, setRenaming] = useState<ChartAccount | null>(null);
  const items = chart.data?.items ?? [];
  const active = items.filter((a) => a.active).length;
  return (
    <>
      <PageHeader
        title="Plano de contas"
        description="As contas em que a classificação lança as linhas conciliadas. Desativar tira a conta das classificações novas; nada é apagado."
      />
      <div className="grid gap-4 p-4">
        <label className="grid w-fit gap-1 text-xs font-medium text-muted-foreground">
          Empresa
          <CompanySelect value={companyId} onChange={(id) => update({ company_id: id || null })} />
        </label>
        {!companyId ? <p className="text-sm text-muted-foreground">Escolha a empresa para ver e editar o plano de contas.</p> : null}
        {companyId ? (
          <section aria-label="Incluir contas" className="grid gap-4 rounded-lg border bg-card p-4">
            <NewAccountForm key={companyId} companyId={companyId} />
            <div className="grid gap-4 border-t pt-4 lg:grid-cols-2">
              <ImportChart key={`csv-${companyId}`} companyId={companyId} />
              <CopyChart key={`copy-${companyId}`} companyId={companyId} />
            </div>
          </section>
        ) : null}
        <ErrorAlert error={chart.error} fallback="Não foi possível carregar o plano de contas." />
        {companyId && chart.isPending ? <Skeleton className="h-64" /> : null}
        {chart.data && items.length === 0 ? (
          <EmptyState icon={ListTree} title="Plano vazio" description="Inclua as contas, importe o CSV ou copie o plano de outra empresa." />
        ) : null}
        {items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Contas do plano">
              <thead>
                <tr>
                  <Th scope="col">Código</Th>
                  <Th scope="col">Nome</Th>
                  <Th scope="col">Situação</Th>
                  <Th scope="col">
                    <span className="sr-only">Ações</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {items.map((a) => (
                  <tr key={a.id} data-conta={a.code} className={a.active ? undefined : "text-muted-foreground"}>
                    <Td className="font-mono text-xs whitespace-nowrap">{a.code}</Td>
                    <Td>{a.name}</Td>
                    <Td>
                      <Badge variant={a.active ? "success" : "muted"}>{a.active ? "Ativa" : "Desativada"}</Badge>
                    </Td>
                    <Td className="whitespace-nowrap">
                      <span className="flex justify-end gap-2">
                        <Button size="sm" variant="ghost" onClick={() => setRenaming(a)} aria-label={`Renomear a conta ${a.code}`}>
                          <Pencil aria-hidden /> Renomear
                        </Button>
                        <ToggleAccount companyId={companyId} account={a} />
                      </span>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <p className="border-t px-3 py-2 text-xs text-muted-foreground">
              {items.length} contas, {active} ativas.
            </p>
          </div>
        ) : null}
      </div>
      {renaming ? <RenameDialog key={renaming.id} companyId={companyId} account={renaming} onClose={() => setRenaming(null)} /> : null}
    </>
  );
}
