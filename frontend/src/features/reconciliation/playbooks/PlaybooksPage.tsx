import { BookMarked } from "lucide-react";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { formatDateTime } from "@/lib/format";
import { type BankPlaybook, usePlaybooks } from "@/services/reconciliation";
import { RateBar } from "../common";

/** A receita da versão (os campos do playbook_banco congelado da origem). */
function Recipe({ p }: { p: BankPlaybook }) {
  const rows: [string, string][] = [
    ["Tabela começa na página", String(p.table_start_page)],
    ["Páginas ignoradas", p.ignored_pages.length > 0 ? p.ignored_pages.join(", ") : "nenhuma"],
    ["Formato da data", p.date_format],
    ["Separador decimal", p.decimal_separator],
    ["Sinal", p.sign_representation],
    ["Saldo", p.balance_location],
    ["Estratégia vencedora", p.winning_strategy],
    ["Validado em", p.validated_at ? formatDateTime(p.validated_at) : "—"],
    ["Criado em", formatDateTime(p.created_at)],
  ];
  return (
    <details className="text-xs">
      <summary className="inline-flex min-h-6 cursor-pointer items-center underline underline-offset-4">receita da v{p.version}</summary>
      <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="font-mono">{value}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}

/**
 * Playbooks por banco [REQ-022, CC-09]: as receitas de extração descobertas pelo agente, que só
 * persistem depois que o portão aprova (o /playbooks da origem). Somente leitura: a que falha na
 * 1ª tentativa é invalidada e a próxima extração redescobre.
 */
export function PlaybooksPage() {
  const playbooks = usePlaybooks();
  const items = playbooks.data?.items ?? [];
  return (
    <>
      <PageHeader
        title="Playbooks por banco"
        description="Receitas de extração descobertas pelo agente. Só persistem depois que o portão aprova; a que falha na 1ª tentativa é invalidada e redescoberta."
      />
      <div className="grid gap-4 p-4">
        <ErrorAlert error={playbooks.error} fallback="Não foi possível carregar os playbooks." />
        {playbooks.isPending ? <Skeleton className="h-48" /> : null}
        {playbooks.data && items.length === 0 ? (
          <EmptyState icon={BookMarked} title="Nenhum playbook ainda" description="O primeiro extrato aprovado de cada banco cria um." />
        ) : null}
        {items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Playbooks por banco">
              <thead>
                <tr>
                  <Th scope="col">Banco</Th>
                  <Th scope="col" className="text-right">
                    Versão
                  </Th>
                  <Th scope="col">Estado</Th>
                  <Th scope="col" className="text-right">
                    Sucessos
                  </Th>
                  <Th scope="col" className="text-right">
                    Falhas
                  </Th>
                  <Th scope="col">Taxa</Th>
                  <Th scope="col">Receita</Th>
                </tr>
              </thead>
              <tbody>
                {items.map((p) => (
                  <tr key={p.id} data-playbook={`${p.bank}-v${String(p.version)}`} className={p.valid ? undefined : "text-muted-foreground"}>
                    <Td className="whitespace-nowrap">{p.bank}</Td>
                    <Td className="text-right tabular-nums">v{p.version}</Td>
                    <Td>{p.valid ? <Badge variant="success">válido</Badge> : <Badge variant="muted">inválido</Badge>}</Td>
                    <Td className="text-right tabular-nums">{p.successes}</Td>
                    <Td className="text-right tabular-nums">{p.failures}</Td>
                    <Td>
                      <RateBar value={p.success_percent} />
                    </Td>
                    <Td>
                      <Recipe p={p} />
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </div>
        ) : null}
      </div>
    </>
  );
}
