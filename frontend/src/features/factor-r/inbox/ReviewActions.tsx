import { Ban, Link2, Search } from "lucide-react";
import { useId, useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input, Textarea } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { formatCnpj } from "@/lib/cnpj";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { useCompanies } from "@/services/companies";
import { type PgdasDocument, useLinkPgdas, useRejectPgdas } from "@/services/factorRInbox";

/** Vínculo manual de um documento em revisão [FR-11]: numa corrida nova do parser_pgdas. */
function LinkCard({ document }: { document: PgdasDocument }) {
  const toast = useToast();
  const link = useLinkPgdas(document.id);
  const [term, setTerm] = useState(document.campos.cnpj ?? "");
  const [companyId, setCompanyId] = useState("");
  const q = useDebouncedValue(term.trim());
  const companies = useCompanies({ q: q || undefined, active: true });
  const options = (companies.data?.items ?? []).slice(0, 8);
  const groupId = useId();
  return (
    <Card>
      <CardHeader
        title="Vincular a uma empresa"
        description="A receita do PA é lançada só se a competência estiver vazia; os meses anteriores, só se conferirem com o RBT12 e a FS12. Nada é sobrescrito."
      />
      <CardBody className="grid gap-3">
        <Field label="Buscar empresa (nome, código ou CNPJ)">
          <Input type="search" value={term} onChange={(e) => setTerm(e.target.value)} />
        </Field>
        <div role="radiogroup" aria-labelledby={groupId} className="grid gap-1">
          <p id={groupId} className="text-xs font-medium text-muted-foreground">
            {companies.isFetching ? "Buscando…" : options.length === 0 ? "Nenhuma empresa ativa encontrada." : "Empresas ativas encontradas"}
          </p>
          {options.map((c) => (
            <label key={c.id} className="flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm has-checked:border-primary has-checked:bg-primary/5">
              <input type="radio" name="empresa-vinculo" value={c.id} checked={companyId === c.id} onChange={() => setCompanyId(c.id)} className="size-4 accent-primary" />
              <span className="font-medium">{c.legal_name}</span>
              <span className="text-xs text-muted-foreground tabular-nums">
                {c.internal_code} · {formatCnpj(c.cnpj)}
              </span>
            </label>
          ))}
        </div>
        <ErrorAlert error={link.error} fallback="Não foi possível vincular." />
        <Button
          className="w-fit"
          disabled={!companyId || link.isPending}
          onClick={() =>
            link.mutate(companyId, {
              onSuccess: (doc) => toast({ title: "Extrato vinculado", description: doc.texto_agente ?? undefined, variant: "success" }),
            })
          }
        >
          <Link2 aria-hidden /> Vincular
        </Button>
      </CardBody>
    </Card>
  );
}

/** Rejeição do documento com motivo [FR-11], numa corrida nova (gatilho rejeicao). */
function RejectCard({ document }: { document: PgdasDocument }) {
  const toast = useToast();
  const reject = useRejectPgdas(document.id);
  const [reason, setReason] = useState("");
  const valid = reason.trim().length >= 3;
  return (
    <Card>
      <CardHeader title="Rejeitar" description="O documento sai da revisão e nada é lançado. O original continua guardado." />
      <CardBody className="grid gap-3">
        <Field label="Motivo da rejeição" hint="De 3 a 500 caracteres.">
          <Textarea rows={3} maxLength={500} value={reason} onChange={(e) => setReason(e.target.value)} />
        </Field>
        <ErrorAlert error={reject.error} fallback="Não foi possível rejeitar." />
        <Button
          variant="destructive"
          className="w-fit"
          disabled={!valid || reject.isPending}
          onClick={() => reject.mutate(reason.trim(), { onSuccess: () => toast({ title: "Extrato rejeitado", variant: "success" }) })}
        >
          <Ban aria-hidden /> Rejeitar
        </Button>
      </CardBody>
    </Card>
  );
}

/** Decisões do analista sobre o documento: vincular (em revisão) e rejeitar. */
export function ReviewActions({ document }: { document: PgdasDocument }) {
  const canLink = document.status === "needs_review";
  const canReject = canLink || document.status === "parsed" || (document.status === "received" && document.job?.status === "dead");
  if (!canLink && !canReject) return null;
  return (
    <section aria-label="Decisão do analista" className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {canLink ? <LinkCard document={document} /> : null}
      {canReject ? <RejectCard document={document} /> : null}
      {!canLink ? (
        <p className="flex items-center gap-2 text-xs text-muted-foreground lg:col-span-2">
          <Search className="size-4" aria-hidden /> O vínculo manual fica disponível quando o documento está em revisão.
        </p>
      ) : null}
    </section>
  );
}
