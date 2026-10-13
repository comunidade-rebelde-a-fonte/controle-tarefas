import { BookOpen, Download, FileText, Search } from "lucide-react";
import { type SubmitEvent, type ReactNode, useState } from "react";
import { Link } from "react-router";
import { CompetenceInput } from "@/components/CompetenceInput";
import { EmptyState } from "@/components/EmptyState";
import { ErrorAlert } from "@/components/ErrorAlert";
import { PageHeader } from "@/components/PageHeader";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { buttonVariants } from "@/components/ui/button-variants";
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { compareCompetence, displayCompetence, isCompetenceKey } from "@/lib/competence";
import { formatMoney, formatRate, isPositive } from "@/lib/decimal";
import { formatDate, formatDateTime } from "@/lib/format";
import { useLatestCompetence } from "@/services/competencies";
import { type LedgerLine, ledgerExportUrl, useLedger, useReconciliationDetail } from "@/services/reconciliation";
import { Amount, CompanySelect } from "../common";
import { classificationText, methodText, RULE_TYPES } from "../labels";
import { useSearchUpdate } from "../model";

/** Um elo do rastro da linha do razão. */
function Step({ n, title, children }: { n: number; title: string; children: ReactNode }) {
  return (
    <li className="grid gap-1.5 rounded-md border p-3 text-sm">
      <h3 className="flex items-center gap-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
        <span className="inline-flex size-5 items-center justify-center rounded-full bg-muted text-[11px] text-foreground" aria-hidden>
          {n}
        </span>
        {title}
      </h3>
      {children}
    </li>
  );
}

const linkClass = "inline-flex min-h-6 items-center gap-1 font-medium underline underline-offset-4";

/**
 * O drill-down da linha do razão [AC-021]: classificação → conciliação → transação do extrato →
 * documentos originais, cada elo com o caminho para a tela de origem.
 */
function Trail({ line, onClose }: { line: LedgerLine | null; onClose: () => void }) {
  const detail = useReconciliationDetail(line?.reconciliation_id ?? null);
  const d = detail.data;
  const c = d?.classification;
  const st = d?.statement_transaction;
  return (
    <Dialog
      open={!!line}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent size="lg">
        <DialogHeader
          title="Origem da linha do razão"
          description="Da classificação ao documento original: a conta, a conciliação, a linha do extrato e os comprovantes."
        />
        <ErrorAlert error={detail.error} fallback="Não foi possível carregar a origem da linha." />
        {detail.isPending ? <Skeleton className="h-64" /> : null}
        {d && c && st ? (
          <ol className="grid gap-3" aria-label="Rastro da linha do razão">
            <Step n={1} title="Classificação">
              <p>
                <span className="font-medium">Conta:</span> {c.account_code ? `${c.account_code}${c.account_name ? ` · ${c.account_name}` : ""}` : "—"}{" "}
                <Badge variant={!c.origin || c.pending ? "warning" : "success"}>{classificationText(c.origin, c.pending)}</Badge>
              </p>
              {c.rule ? (
                <p className="text-xs text-muted-foreground">
                  Pela regra {RULE_TYPES[c.rule.rule_type]} <code className="rounded bg-muted px-1">{c.rule.pattern}</code>
                </p>
              ) : null}
              {c.history_line_id ? (
                <p className="text-xs text-muted-foreground">
                  Pelo histórico:{" "}
                  <Link to={`/reconciliation?item=${c.history_line_id}&kind=linha`} className={linkClass}>
                    a linha já classificada que serviu de base
                  </Link>
                </p>
              ) : null}
              {c.classified_at ? <p className="text-xs text-muted-foreground">Em {formatDateTime(c.classified_at)}</p> : null}
            </Step>
            <Step n={2} title="Conciliação">
              <p>
                {methodText(d.method)} <Badge variant={d.origin === "auto" ? "muted" : "default"}>{d.origin === "auto" ? "automática" : "manual"}</Badge>
              </p>
              <p className="text-xs text-muted-foreground">
                Em {formatDateTime(d.created_at)}
                {d.score ? ` · pontuação ${formatRate(d.score, 0)}` : ""}
                {d.proposal_id ? " · a partir da hipótese do resolvedor" : ""}
              </p>
            </Step>
            <Step n={3} title="Transação do extrato">
              <p className="flex flex-wrap items-center gap-2">
                <span className="tabular-nums">{formatDate(st.date)}</span>
                <Amount value={st.amount} direction={st.direction} />
                <span className="break-all">{st.description}</span>
              </p>
              <p className="text-xs text-muted-foreground">
                {st.bank_account} · competência {displayCompetence(st.competence)}
              </p>
              <Link to={`/reconciliation?item=${st.id}&kind=linha`} className={`${linkClass} w-fit text-xs`}>
                Abrir a linha (desfazer, reclassificar)
              </Link>
            </Step>
            <Step n={4} title="Documentos originais">
              <ul className="grid gap-1.5">
                <li>
                  <Link to={`/reconciliation/documents/${st.statement_document_id}`} className={linkClass}>
                    <FileText className="size-3.5" aria-hidden /> Extrato bancário
                  </Link>
                </li>
                {d.entries.map((e) => (
                  <li key={e.id} className="flex flex-wrap items-center gap-2">
                    {e.document_id ? (
                      <Link to={`/reconciliation/documents/${e.document_id}`} className={linkClass}>
                        <FileText className="size-3.5" aria-hidden /> Comprovante
                      </Link>
                    ) : (
                      <span className="font-medium">Lançamento manual (sem original)</span>
                    )}
                    <span className="text-xs text-muted-foreground">
                      {e.date ? formatDate(e.date) : "—"} · {e.amount ? formatMoney(e.amount) : "—"} · {e.counterparty ?? "sem contraparte"}
                    </span>
                  </li>
                ))}
              </ul>
            </Step>
          </ol>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

/**
 * Razão [REQ-028, AC-021, AC-022]: projeção sobre as conciliações ativas (nenhuma linha é
 * escrita à mão), com filtros, totais, a exportação em CSV e em XLSX e o drill-down de cada linha.
 */
export function LedgerPage() {
  const [params, update] = useSearchUpdate();
  const latest = useLatestCompetence();
  const current = latest.data?.current ?? "";
  const companyId = params.get("company_id") ?? "";
  const fromParam = params.get("from");
  const toParam = params.get("to");
  const from = isCompetenceKey(fromParam) ? fromParam : current;
  const to = isCompetenceKey(toParam) ? toParam : current;
  const account = params.get("account") ?? "";
  const [accountDraft, setAccountDraft] = useState(account);
  const [open, setOpen] = useState<LedgerLine | null>(null);
  const inverted = !!from && !!to && compareCompetence(from, to) > 0;
  const ready = !!companyId && isCompetenceKey(from) && isCompetenceKey(to) && !inverted;
  const filters = { companyId, from, to, account: account || undefined };
  const ledger = useLedger(filters, ready);
  const data = ledger.data;

  function applyAccount(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    update({ account: accountDraft.trim() || null });
  }

  return (
    <>
      <PageHeader
        title="Razão"
        description="Projeção sobre o que está conciliado: nenhuma linha é escrita à mão. Reclassificar ou desfazer muda o razão porque muda a origem."
        actions={
          ready ? (
            <div className="flex gap-2">
              <a href={ledgerExportUrl(filters, "csv")} download className={buttonVariants({ variant: "outline", size: "sm" })}>
                <Download aria-hidden /> Exportar CSV
              </a>
              <a href={ledgerExportUrl(filters, "xlsx")} download className={buttonVariants({ variant: "outline", size: "sm" })}>
                <Download aria-hidden /> Exportar XLSX
              </a>
            </div>
          ) : null
        }
      />
      <div className="grid gap-4 p-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Empresa
            <CompanySelect value={companyId} onChange={(id) => update({ company_id: id || null })} />
          </label>
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            De
            <CompetenceInput value={from} onChange={(v) => update({ from: v || null })} />
          </label>
          <label className="grid gap-1 text-xs font-medium text-muted-foreground">
            Até
            <CompetenceInput value={to} onChange={(v) => update({ to: v || null })} />
          </label>
          <form onSubmit={applyAccount} className="flex items-end gap-2">
            <label className="grid gap-1 text-xs font-medium text-muted-foreground">
              Conta bancária ou contábil
              <Input value={accountDraft} onChange={(e) => setAccountDraft(e.target.value)} placeholder="Banco, número ou código" className="w-56" />
            </label>
            <Button type="submit" size="sm" variant="outline">
              <Search aria-hidden /> Filtrar
            </Button>
          </form>
        </div>
        {inverted ? (
          <p role="alert" className="text-sm text-destructive">
            A competência inicial é depois da final.
          </p>
        ) : null}
        {!companyId ? <p className="text-sm text-muted-foreground">Escolha a empresa para ver o razão.</p> : null}
        <ErrorAlert error={ledger.error} fallback="Não foi possível carregar o razão." />
        {ready && ledger.isPending ? <Skeleton className="h-64" /> : null}
        {data && data.items.length === 0 ? (
          <EmptyState icon={BookOpen} title="Razão vazio" description="Nada conciliado ainda para este filtro." />
        ) : null}
        {data && data.items.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border bg-card">
            <Table aria-label="Linhas do razão">
              <thead>
                <tr>
                  <Th scope="col">Data</Th>
                  <Th scope="col">Conta bancária</Th>
                  <Th scope="col">Conta contábil</Th>
                  <Th scope="col">Histórico</Th>
                  <Th scope="col">Contraparte</Th>
                  <Th scope="col" className="text-right">
                    Débito
                  </Th>
                  <Th scope="col" className="text-right">
                    Crédito
                  </Th>
                  <Th scope="col">
                    <span className="sr-only">Origem</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((l) => (
                  <tr key={l.reconciliation_id} data-conciliacao={l.reconciliation_id}>
                    <Td className="whitespace-nowrap tabular-nums">{formatDate(l.date)}</Td>
                    <Td className="text-xs whitespace-nowrap">{l.bank_account}</Td>
                    <Td className="max-w-56 text-xs">
                      {l.account_code ? (
                        <span className="block truncate">
                          {l.account_code}
                          {l.account_name ? ` · ${l.account_name}` : ""}
                        </span>
                      ) : null}
                      <span className="text-muted-foreground">{classificationText(l.classification_origin, l.classification_pending)}</span>
                    </Td>
                    <Td className="max-w-80 truncate">{l.history}</Td>
                    <Td className="max-w-48 truncate text-xs">{l.counterparty ?? "—"}</Td>
                    <Td className="text-right whitespace-nowrap tabular-nums">{isPositive(l.debit) ? formatMoney(l.debit) : ""}</Td>
                    <Td className="text-right whitespace-nowrap tabular-nums">{isPositive(l.credit) ? formatMoney(l.credit) : ""}</Td>
                    <Td>
                      <Button size="sm" variant="ghost" onClick={() => setOpen(l)} aria-label={`Origem da linha de ${formatDate(l.date)}: ${l.history}`}>
                        Origem
                      </Button>
                    </Td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="border-t font-medium">
                  <Td colSpan={5}>Total do período</Td>
                  <Td className="text-right whitespace-nowrap tabular-nums">{formatMoney(data.total_debit)}</Td>
                  <Td className="text-right whitespace-nowrap tabular-nums">{formatMoney(data.total_credit)}</Td>
                  <Td />
                </tr>
              </tfoot>
            </Table>
          </div>
        ) : null}
      </div>
      <Trail line={open} onClose={() => setOpen(null)} />
    </>
  );
}
