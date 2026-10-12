import { ArrowLeft, Download, ScrollText } from "lucide-react";
import { Link, useParams } from "react-router";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { buttonVariants } from "@/components/ui/button-variants";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { formatCnpj } from "@/lib/cnpj";
import { displayCompetence } from "@/lib/competence";
import { formatMoney, formatRate } from "@/lib/decimal";
import { formatDateTime } from "@/lib/format";
import { useCompany } from "@/services/companies";
import { type PgdasDocument, pgdasFileUrl, usePgdasDocument } from "@/services/factorRInbox";
import { FactorRDisclaimer } from "../common";
import { CreateFromStatement } from "./CreateFromStatement";
import { DocumentStatusBadge } from "./DocumentStatusBadge";
import { JobProgress } from "./JobProgress";
import { PGDAS_FIELDS, reasonText, warningText } from "./labels";
import { ReviewActions } from "./ReviewActions";

function fieldValue(value: string | null, kind: (typeof PGDAS_FIELDS)[number][2]): string {
  if (value === null) return "não encontrado";
  switch (kind) {
    case "reais":
      return formatMoney(value);
    case "percentual":
      return formatRate(value);
    case "cnpj":
      return formatCnpj(value);
    case "texto":
      return value;
  }
}

/** Campos lidos com a confiança de cada um (1, 0 ou 0,3 com dígito verificador inválido). */
function FieldsTable({ d }: { d: PgdasDocument }) {
  return (
    <Card>
      <CardHeader title="Campos lidos do extrato" description={`Parser ${d.parser_version ?? "—"} · confiança do documento ${formatRate(d.confianca)}`} />
      <div className="overflow-x-auto">
        <Table aria-label="Campos lidos do extrato" data-testid="campos-extraidos">
          <thead>
            <tr>
              <Th scope="col">Campo</Th>
              <Th scope="col">Valor lido</Th>
              <Th scope="col" className="text-right">
                Confiança do campo
              </Th>
            </tr>
          </thead>
          <tbody>
            {PGDAS_FIELDS.map(([key, label, kind]) => {
              const value = d.campos[key];
              const conf = d.confianca_campos[key];
              return (
                <tr key={key} data-campo={key}>
                  <Th scope="row" className="static bg-card font-medium text-foreground">
                    {label}
                  </Th>
                  <Td className={value === null ? "text-muted-foreground" : "tabular-nums"}>
                    {key === "pa" && value !== null ? displayCompetence(value) : fieldValue(value, kind)}
                  </Td>
                  <Td className="text-right tabular-nums">{conf !== undefined ? formatRate(conf, 0) : "—"}</Td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      </div>
      {d.motivos.length > 0 ? (
        <CardBody className="border-t">
          <p className="text-xs font-medium">Avisos do parser</p>
          <ul className="mt-1 list-disc pl-5 text-xs text-muted-foreground">
            {d.motivos.map((m) => (
              <li key={m}>{warningText(m)}</li>
            ))}
          </ul>
        </CardBody>
      ) : null}
    </Card>
  );
}

/** Receita e folha mês a mês dos 12 meses anteriores ao PA, como declaradas (só gravadas se conferem). */
function PriorSeries({ d }: { d: PgdasDocument }) {
  const s = d.series_anteriores;
  const months = Array.from(new Set([...Object.keys(s.receitas), ...Object.keys(s.folhas)])).sort();
  if (months.length === 0) return null;
  return (
    <Card>
      <CardHeader
        title="Meses anteriores declarados"
        description={`Receitas ${s.receitas_conferem ? "conferem" : "não conferem"} com o RBT12; folhas ${s.folhas_conferem ? "conferem" : "não conferem"} com a FS12. Só o que confere é lançado, em competência vazia.`}
      />
      <div className="overflow-x-auto">
        <Table aria-label="Meses anteriores declarados">
          <thead>
            <tr>
              <Th scope="col">Competência</Th>
              <Th scope="col" className="text-right">
                Receita
              </Th>
              <Th scope="col" className="text-right">
                Folha declarada
              </Th>
            </tr>
          </thead>
          <tbody>
            {months.map((m) => (
              <tr key={m}>
                <Th scope="row" className="static bg-card font-medium text-foreground">
                  {displayCompetence(m)}
                </Th>
                <Td className="text-right tabular-nums">{formatMoney(s.receitas[m])}</Td>
                <Td className="text-right tabular-nums">{formatMoney(s.folhas[m])}</Td>
              </tr>
            ))}
          </tbody>
        </Table>
      </div>
    </Card>
  );
}

function LinkedCompany({ companyId, pa }: { companyId: string; pa: string | null }) {
  const company = useCompany(companyId);
  const query = pa ? `?tab=factor-r&competence=${pa}` : "?tab=factor-r";
  return (
    <p className="text-sm" data-testid="empresa-vinculada">
      Vinculado a{" "}
      <Link to={`/companies/${companyId}${query}`} className="font-medium underline underline-offset-4">
        {company.data ? `${company.data.internal_code} — ${company.data.legal_name}` : "empresa"}
      </Link>
    </p>
  );
}

/**
 * Documento do inbox [FR-09 a FR-11]: o que o parser leu, o andamento do processamento, o texto
 * do agente e as decisões do analista (vincular, rejeitar ou cadastrar pelo extrato).
 */
export function PgdasDocumentPage() {
  const { id = "" } = useParams();
  const doc = usePgdasDocument(id);
  const d = doc.data;
  return (
    <>
      <PageHeader
        title="Extrato PGDAS-D"
        description={d ? `${d.nome_original ?? "arquivo"} · recebido em ${formatDateTime(d.created_at)}` : undefined}
        actions={
          d ? (
            <>
              <a href={pgdasFileUrl(d.id)} download className={buttonVariants({ variant: "outline", size: "sm" })}>
                <Download aria-hidden /> Baixar original
              </a>
              {d.trace_id ? (
                <Link to={`/factor-r/runs/${d.trace_id}`} className={buttonVariants({ variant: "outline", size: "sm" })}>
                  <ScrollText aria-hidden /> Ver trace
                </Link>
              ) : null}
            </>
          ) : null
        }
      />
      <div className="grid grid-cols-1 gap-4 p-4">
        <Link to="/factor-r/inbox" className="inline-flex w-fit items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-3.5" aria-hidden /> Inbox PGDAS-D
        </Link>
        <ErrorAlert error={doc.error} fallback="Documento não encontrado." />
        {doc.isPending ? <Skeleton className="h-64" /> : null}
        {d ? (
          <>
            <div className="grid gap-1.5">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <DocumentStatusBadge document={d} />
                {d.motivo ? (
                  <span data-testid="motivo">
                    {d.status === "rejected" ? "Motivo da rejeição" : "Motivo"}: {reasonText(d.motivo)}
                  </span>
                ) : null}
              </div>
              {d.identificacao.nome_empresarial ? (
                <p className="text-sm" data-testid="nome-empresarial">
                  Nome empresarial no extrato: {d.identificacao.nome_empresarial}
                </p>
              ) : null}
              {d.company_id ? <LinkedCompany companyId={d.company_id} pa={d.campos.pa} /> : null}
            </div>
            <JobProgress document={d} />
            {d.texto_agente ? (
              <p className="rounded-md border bg-muted/40 p-3 text-sm whitespace-pre-line" data-testid="mensagem-agente">
                {d.texto_agente}
              </p>
            ) : null}
            <CreateFromStatement document={d} />
            <div className="grid grid-cols-1 items-start gap-4 xl:grid-cols-2">
              <FieldsTable d={d} />
              <PriorSeries d={d} />
            </div>
            <ReviewActions document={d} />
          </>
        ) : null}
        <FactorRDisclaimer />
      </div>
    </>
  );
}
