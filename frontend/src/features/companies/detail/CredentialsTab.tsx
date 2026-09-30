import { useMemo, useState } from "react";
import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Skeleton } from "@/components/ui/skeleton";
import { CredentialsContext, type CredentialsActions } from "@/features/credentials/context";
import { CredentialSection } from "@/features/credentials/DocumentsDialog";
import { RenewedDialog, type RenewedTarget } from "@/features/credentials/RenewedDialog";
import { CREDENTIAL_TYPES, useCompanyCredentials } from "@/services/credentials";

/**
 * Aba 7 da ficha [PRD §27.3]: procuração e certificado da empresa, com os mesmos componentes
 * da tela Procurações e Certificados (T-M4-10). As alterações invalidam também a tela geral.
 */
export function CredentialsTab({ companyId }: { companyId: string }) {
  const query = useCompanyCredentials(companyId);
  const [renewed, setRenewed] = useState<RenewedTarget | null>(null);
  const actions = useMemo<CredentialsActions>(
    () => ({ openRenewed: (row, type) => setRenewed({ row, type }), openDocuments: () => undefined }),
    [setRenewed],
  );

  if (query.error) return <ErrorAlert error={query.error} fallback="Não foi possível carregar procurações e certificados." />;
  const row = query.data;
  if (!row) return <Skeleton className="h-64" />;

  return (
    <CredentialsContext.Provider value={actions}>
      <div className="grid gap-4">
        <div className="grid gap-4 lg:grid-cols-2">
          {CREDENTIAL_TYPES.map((type) => (
            <CredentialSection key={type} row={row} type={type} editable />
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          Visão de todas as empresas em{" "}
          <Link to="/credentials" className="underline underline-offset-2">
            Procurações e Certificados
          </Link>
          .
        </p>
      </div>
      <RenewedDialog target={renewed} onClose={() => setRenewed(null)} />
    </CredentialsContext.Provider>
  );
}
