import { useState } from "react";
import { ConfirmDialog } from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { type Company, useCompany, useDeactivateCompany } from "@/services/companies";
import { CompanyFormDialog } from "./CompanyForm";

/** Edição: carrega a ficha completa antes de abrir o formulário. */
export function EditCompanyDialog({ companyId, onClose }: { companyId: string; onClose: () => void }) {
  const { data } = useCompany(companyId);
  if (!data) return null;
  return <CompanyFormDialog open onOpenChange={(o) => !o && onClose()} company={data} />;
}

/** Inativação com confirmação. Não há exclusão física [RN-008]. */
export function DeactivateCompanyDialog({ company, onClose }: { company: Pick<Company, "id" | "legal_name" | "internal_code" | "version">; onClose: () => void }) {
  const toast = useToast();
  const deactivate = useDeactivateCompany();
  const [open, setOpen] = useState(true);
  const close = () => {
    setOpen(false);
    onClose();
  };
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={(o) => !o && close()}
      title="Inativar empresa?"
      description={`${company.internal_code} — ${company.legal_name} deixará de receber novas obrigações. O histórico é preservado e a empresa pode ser reativada.`}
      confirmLabel="Inativar"
      destructive
      pending={deactivate.isPending}
      onConfirm={() => {
        deactivate.mutate(
          { id: company.id, version: company.version },
          {
            onSuccess: () => {
              toast({ title: "Empresa inativada", variant: "success" });
              close();
            },
            onError: (err) => {
              toast({
                title: "Não foi possível inativar",
                description: isApiError(err) ? err.message : undefined,
                variant: "error",
              });
              close();
            },
          },
        );
      }}
    />
  );
}
