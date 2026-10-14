import { Link } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { displayCompetence } from "@/lib/competence";
import { formatMoney } from "@/lib/decimal";
import { formatDateTime } from "@/lib/format";
import { usePgdasDocuments } from "@/services/factorRInbox";
import { DocumentStatusBadge } from "./DocumentStatusBadge";

/** Extratos PGDAS-D vinculados à empresa [08 §6], com o caminho para o documento. */
export function CompanyStatementsCard({ companyId }: { companyId: string }) {
  const docs = usePgdasDocuments({ companyId, limit: 24 });
  const items = docs.data?.items ?? [];
  return (
    <Card>
      <CardHeader
        title="Extratos PGDAS-D"
        description="Vinculados a esta empresa."
        actions={
          <Link to="/factor-r/inbox" className="text-xs underline underline-offset-4">
            Inbox
          </Link>
        }
      />
      <CardBody className="grid gap-2 text-sm">
        <ErrorAlert error={docs.error} fallback="Não foi possível carregar os extratos." />
        {docs.data && items.length === 0 ? <p className="text-muted-foreground">Nenhum extrato vinculado.</p> : null}
        <ul className="grid gap-1.5" aria-label="Extratos vinculados">
          {items.map((d) => (
            <li key={d.id} className="flex flex-wrap items-center gap-2">
              <Link to={`/factor-r/inbox/${d.id}`} className="font-medium hover:underline">
                PA {d.campos.pa ? displayCompetence(d.campos.pa) : "—"}
              </Link>
              <DocumentStatusBadge document={d} />
              <span className="text-muted-foreground">RPA {formatMoney(d.campos.rpa)}</span>
              <span className="text-xs text-muted-foreground">recebido em {formatDateTime(d.created_at)}</span>
            </li>
          ))}
        </ul>
      </CardBody>
    </Card>
  );
}
