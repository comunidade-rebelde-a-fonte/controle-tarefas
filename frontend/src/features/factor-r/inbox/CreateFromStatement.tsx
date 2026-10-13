import { Building2, Link2 } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";
import { Field } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { CompanyForm } from "@/features/companies/CompanyForm";
import { type Conflict, conflictFrom } from "@/features/companies/conflict";
import { toCreatePayload } from "@/features/companies/payload";
import type { CompanyFormValues } from "@/features/companies/schemas";
import { isApiError } from "@/lib/api";
import { type PgdasDocument, type PgdasSuggestion, useCreateCompanyFromPgdas, useLinkPgdas } from "@/services/factorRInbox";

function defaultsFrom(s: PgdasSuggestion): Partial<CompanyFormValues> {
  return {
    legal_name: s.nome_empresarial ?? "",
    cnpj: s.cnpj_formatado,
    // Extrato do PGDAS-D: a empresa é do Simples Nacional; a sujeição ao Fator R vem da atividade.
    tax_regime: "SIMPLES_NACIONAL",
    factor_r: s.sujeita_fator_r === null ? "" : s.sujeita_fator_r ? "true" : "false",
  };
}

function CreateDialog({
  open,
  onOpenChange,
  document,
  suggestion,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  document: PgdasDocument;
  suggestion: PgdasSuggestion;
}) {
  const toast = useToast();
  const create = useCreateCompanyFromPgdas(document.id);
  const link = useLinkPgdas(document.id);
  const [conflict, setConflict] = useState<Conflict | null>(null);
  const [activityStart, setActivityStart] = useState(suggestion.inicio_atividade ?? "");

  const onSubmit = async (values: CompanyFormValues) => {
    setConflict(null);
    try {
      const result = await create.mutateAsync({ empresa: toCreatePayload(values), inicio_atividade: activityStart || null });
      toast({ title: "Empresa cadastrada e extrato vinculado", description: result.empresa.legal_name, variant: "success" });
      onOpenChange(false);
    } catch (err) {
      const c = conflictFrom(err);
      if (c) {
        setConflict(c);
        return;
      }
      if (!(isApiError(err) && err.code === "VALIDATION_ERROR")) {
        toast({ title: "Não foi possível cadastrar", description: isApiError(err) ? err.message : undefined, variant: "error" });
      }
      throw err;
    }
  };

  const linkExisting = (company: NonNullable<Conflict["company"]>) =>
    link.mutate(company.id, {
      onSuccess: () => {
        toast({ title: "Extrato vinculado", description: company.legal_name, variant: "success" });
        onOpenChange(false);
      },
      onError: (err) => toast({ title: "Não foi possível vincular", description: isApiError(err) ? err.message : undefined, variant: "error" }),
    });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setConflict(null);
        onOpenChange(o);
      }}
    >
      <DialogContent size="lg">
        <DialogHeader
          title="Cadastrar a empresa do extrato"
          description="Os dados lidos do extrato já vêm preenchidos. Complete o cadastro: a empresa entra no cadastro único e o extrato é vinculado na mesma operação."
        />
        {open ? (
          <CompanyForm
            defaults={defaultsFrom(suggestion)}
            cnpjReadOnly
            submitLabel="Confirmar cadastro e vínculo"
            onSubmit={onSubmit}
            onCancel={() => onOpenChange(false)}
            conflict={conflict}
            submitting={create.isPending}
            conflictAction={
              conflict?.company ? (
                <Button size="sm" variant="outline" onClick={() => conflict.company && linkExisting(conflict.company)} disabled={link.isPending}>
                  <Link2 aria-hidden /> Vincular o extrato a esta empresa
                </Button>
              ) : null
            }
            extra={
              <Field label="Início da atividade (Fator R)" hint="Data de abertura no CNPJ, lida do extrato. Define os meses válidos da janela.">
                <Input type="month" value={activityStart} onChange={(e) => setActivityStart(e.target.value)} className="w-44" />
              </Field>
            }
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

/**
 * CNPJ do extrato fora do cadastro [FR-11]: cadastra a empresa pelo cadastro canônico e vincula
 * o documento. Nada é gravado sem o analista confirmar o formulário.
 */
export function CreateFromStatement({ document }: { document: PgdasDocument }) {
  const [open, setOpen] = useState(false);
  const suggestion = document.sugestao_cadastro;
  if (!suggestion || document.status !== "needs_review") return null;
  return (
    <div data-testid="cadastro-pelo-extrato" className="grid gap-2 rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
      <p>
        <strong>Esse CNPJ não está no cadastro de empresas</strong> ({suggestion.cnpj_formatado}).
        {suggestion.nome_empresarial ? ` O extrato indica: ${suggestion.nome_empresarial}.` : null}
      </p>
      <p className="text-xs">Nada é gravado sem a sua confirmação.</p>
      <Button size="sm" className="w-fit" onClick={() => setOpen(true)}>
        <Building2 aria-hidden /> Cadastrar e vincular
      </Button>
      <CreateDialog open={open} onOpenChange={setOpen} document={document} suggestion={suggestion} />
    </div>
  );
}
