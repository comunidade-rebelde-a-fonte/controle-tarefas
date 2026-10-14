import { Landmark, Plus } from "lucide-react";
import { type SubmitEvent, useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { formatDate } from "@/lib/format";
import { type BankAccountCreate, useBankAccounts, useCreateBankAccount } from "@/services/reconciliation";
import { fieldErrorsOf } from "../review/model";

const EMPTY: BankAccountCreate = { bank: "", branch: "", account: "" };

/**
 * Contas bancárias da empresa (a tela "Clientes e contas" da origem, CC-11): a conta cadastrada
 * decide a empresa dos próximos extratos dela. Banco, agência e conta precisam ser os que a
 * leitura do extrato grava: por isso as contas já lidas aparecem para cadastrar com um clique.
 */
export function BankAccounts({ companyId }: { companyId: string }) {
  const toast = useToast();
  const accounts = useBankAccounts(companyId);
  const create = useCreateBankAccount(companyId);
  const [draft, setDraft] = useState<BankAccountCreate>(EMPTY);
  const [fromForm, setFromForm] = useState(false);
  const errors = fromForm ? fieldErrorsOf(create.error) : {};
  const data = accounts.data;

  function register(body: BankAccountCreate, form: boolean) {
    setFromForm(form);
    create.mutate(body, {
      onSuccess: (a) => {
        if (form) setDraft(EMPTY);
        toast({ title: `Conta ${a.label} cadastrada`, variant: "success" });
      },
    });
  }

  function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    register({ bank: draft.bank.trim(), branch: draft.branch.trim(), account: draft.account.trim() }, true);
  }

  return (
    <Card aria-label="Contas bancárias">
      <CardHeader
        title="Contas bancárias"
        description="A conta cadastrada decide a empresa dos próximos extratos dela; o extrato dela enviado para outra empresa vai para a revisão."
      />
      <CardBody className="grid gap-4">
        <ErrorAlert error={accounts.error} fallback="Não foi possível carregar as contas." />
        {accounts.isPending ? <Skeleton className="h-24" /> : null}
        {data && data.items.length === 0 ? <p className="text-sm text-muted-foreground">Nenhuma conta cadastrada.</p> : null}
        {data && data.items.length > 0 ? (
          <Table aria-label="Contas cadastradas">
            <thead>
              <tr>
                <Th scope="col">Conta</Th>
                <Th scope="col" className="text-right">
                  Extratos
                </Th>
                <Th scope="col">Último período</Th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((a) => (
                <tr key={a.id}>
                  <Td className="font-mono text-xs">{a.label}</Td>
                  <Td className="text-right tabular-nums">{a.statements}</Td>
                  <Td className="tabular-nums">{a.last_period_end ? formatDate(a.last_period_end) : "—"}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        ) : null}
        {data && data.read.length > 0 ? (
          <section aria-label="Contas lidas nos extratos" className="grid gap-2">
            <h3 className="text-xs font-semibold text-muted-foreground">Lidas nos extratos da empresa, sem cadastro</h3>
            <ul className="grid gap-1.5">
              {data.read.map((r) => (
                <li key={r.label} className="flex flex-wrap items-center gap-2 rounded-md border px-3 py-1.5 text-sm">
                  <Landmark className="size-4 text-muted-foreground" aria-hidden />
                  <span className="font-mono text-xs">{r.label}</span>
                  <span className="text-xs text-muted-foreground">
                    {r.statements} {r.statements === 1 ? "extrato" : "extratos"}, até {formatDate(r.last_period_end)}
                  </span>
                  <Button
                    size="sm"
                    variant="outline"
                    className="ml-auto"
                    disabled={create.isPending}
                    onClick={() => register({ bank: r.bank, branch: r.branch, account: r.account }, false)}
                    aria-label={`Cadastrar a conta ${r.label}`}
                  >
                    <Plus aria-hidden /> Cadastrar
                  </Button>
                </li>
              ))}
            </ul>
          </section>
        ) : null}
        <form onSubmit={submit} aria-label="Nova conta bancária" className="grid gap-3 border-t pt-4">
          <h3 className="text-xs font-semibold text-muted-foreground">Cadastrar à mão</h3>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="Banco" required error={errors.bank} hint="Como a leitura grava (ex.: beta)">
              <Input maxLength={64} value={draft.bank} onChange={(e) => setDraft((d) => ({ ...d, bank: e.target.value }))} required />
            </Field>
            <Field label="Agência" required error={errors.branch}>
              <Input maxLength={16} value={draft.branch} onChange={(e) => setDraft((d) => ({ ...d, branch: e.target.value }))} required />
            </Field>
            <Field label="Conta" required error={errors.account}>
              <Input maxLength={32} value={draft.account} onChange={(e) => setDraft((d) => ({ ...d, account: e.target.value }))} required />
            </Field>
          </div>
          <Button type="submit" size="sm" variant="outline" className="w-fit" disabled={create.isPending}>
            <Plus aria-hidden /> Cadastrar conta
          </Button>
        </form>
        {errors.bank || errors.branch || errors.account ? null : <ErrorAlert error={create.error} fallback="Não foi possível cadastrar a conta." />}
      </CardBody>
    </Card>
  );
}
