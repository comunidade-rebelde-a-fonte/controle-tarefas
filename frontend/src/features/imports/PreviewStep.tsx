import { CircleAlert, CircleCheck, CirclePlus, RefreshCw } from "lucide-react";
import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, Td, Th } from "@/components/ui/table";
import { formatCnpj } from "@/lib/cnpj";
import { formatDateTime } from "@/lib/format";
import { cn } from "@/lib/utils";
import { type ImportPreview, type ImportResult, type ImportRow, type ImportRowStatus, useImportCommit } from "@/services/imports";

const STATUS_ORDER: ImportRowStatus[] = ["NEW", "EXISTING", "INVALID_CNPJ", "DUPLICATE_CODE", "MISSING_REQUIRED", "INVALID_VALUE"];

const STATUS_LABEL: Record<ImportRowStatus, string> = {
  NEW: "Nova empresa",
  EXISTING: "Empresa existente",
  INVALID_CNPJ: "CNPJ inválido",
  DUPLICATE_CODE: "Código duplicado",
  MISSING_REQUIRED: "Campo obrigatório ausente",
  INVALID_VALUE: "Valor inválido",
};

function StatusBadge({ status, label }: { status: ImportRowStatus; label: string }) {
  if (status === "NEW")
    return (
      <Badge variant="success">
        <CirclePlus aria-hidden="true" /> {label}
      </Badge>
    );
  if (status === "EXISTING")
    return (
      <Badge variant="default">
        <RefreshCw aria-hidden="true" /> {label}
      </Badge>
    );
  return (
    <Badge variant="danger">
      <CircleAlert aria-hidden="true" /> {label}
    </Badge>
  );
}

/**
 * Passos 3 e 4: prévia com a classificação e os erros por linha; o usuário confirma quais
 * empresas existentes atualizar [PRD §35.5]. Linhas com erro nunca são importadas [CA-018].
 */
export function PreviewStep({ preview, onBack, onDone }: { preview: ImportPreview; onBack: () => void; onDone: (r: ImportResult) => void }) {
  const [filter, setFilter] = useState<ImportRowStatus | "">("");
  const [confirmed, setConfirmed] = useState<Set<number>>(new Set());
  const commit = useImportCommit();
  const rows = filter ? preview.rows.filter((r) => r.status === filter) : preview.rows;
  const toCreate = preview.summary.by_status.NEW ?? 0;
  const total = toCreate + confirmed.size;
  const invalid = preview.summary.total - preview.summary.importable;

  const toggle = (row: ImportRow) =>
    setConfirmed((prev) => {
      const next = new Set(prev);
      if (next.has(row.line)) next.delete(row.line);
      else next.add(row.line);
      return next;
    });

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm">
          <span className="font-medium">{preview.file_name}</span> · {preview.summary.total} {preview.summary.total === 1 ? "linha" : "linhas"}
          {invalid > 0 ? ` · ${String(invalid)} com erro (não serão importadas)` : ""}
        </p>
        <p className="ml-auto text-xs text-muted-foreground">Prévia válida até {formatDateTime(preview.expires_at)}.</p>
      </div>
      {preview.unknown_headers.length > 0 && (
        <p className="text-xs text-muted-foreground">Colunas ignoradas: {preview.unknown_headers.join(", ")}.</p>
      )}

      <div role="group" aria-label="Filtrar por classificação" className="flex flex-wrap gap-1.5">
        <button
          type="button"
          aria-pressed={filter === ""}
          onClick={() => setFilter("")}
          className={cn("rounded-full border px-3 py-1 text-xs font-medium", filter === "" ? "border-primary bg-primary text-primary-foreground" : "bg-card")}
        >
          Todas ({preview.summary.total})
        </button>
        {STATUS_ORDER.filter((s) => (preview.summary.by_status[s] ?? 0) > 0).map((s) => (
          <button
            key={s}
            type="button"
            aria-pressed={filter === s}
            onClick={() => setFilter(s)}
            className={cn("rounded-full border px-3 py-1 text-xs font-medium", filter === s ? "border-primary bg-primary text-primary-foreground" : "bg-card")}
          >
            {STATUS_LABEL[s]} ({preview.summary.by_status[s]})
          </button>
        ))}
      </div>

      <div className="max-h-[calc(100vh-22rem)] overflow-auto rounded-lg border bg-card">
        <Table aria-label="Prévia da importação">
          <thead>
            <tr>
              <Th className="w-16">Linha</Th>
              <Th className="w-24">Código</Th>
              <Th>Razão social</Th>
              <Th className="w-44">CNPJ</Th>
              <Th className="w-48">Classificação</Th>
              <Th>Erros</Th>
              <Th className="w-40">Atualizar cadastro?</Th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.line} className={r.errors.length > 0 ? "bg-red-50/40" : undefined}>
                <Td className="tabular-nums">{r.line}</Td>
                <Td>{r.internal_code || "—"}</Td>
                <Td>{r.legal_name || "—"}</Td>
                <Td className="tabular-nums">{r.cnpj ? formatCnpj(r.cnpj) : "—"}</Td>
                <Td>
                  <StatusBadge status={r.status} label={r.status_label} />
                </Td>
                <Td>
                  {r.errors.length > 0 ? (
                    <ul className="grid gap-0.5 text-xs text-red-800">
                      {r.errors.map((e, i) => (
                        <li key={i}>
                          <span className="font-medium">{e.column || e.field}:</span> {e.message}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <span className="text-xs text-muted-foreground">—</span>
                  )}
                </Td>
                <Td>
                  {r.status === "EXISTING" ? (
                    <label className="flex items-center gap-1.5 text-xs">
                      <input
                        type="checkbox"
                        className="size-4 accent-primary"
                        checked={confirmed.has(r.line)}
                        onChange={() => toggle(r)}
                        aria-label={`Atualizar o cadastro de ${r.legal_name} (linha ${String(r.line)})`}
                      />
                      Atualizar
                    </label>
                  ) : null}
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
      </div>

      <p className="text-xs text-muted-foreground">
        Empresas existentes só são atualizadas se marcadas; campos vazios mantêm o valor atual. Regime, módulos e situação de empresas
        existentes não mudam pela importação.
      </p>
      <ErrorAlert error={commit.error} fallback="Não foi possível concluir a importação." />
      <div className="flex gap-2">
        <Button variant="outline" onClick={onBack} disabled={commit.isPending}>
          Enviar outra planilha
        </Button>
        <Button
          disabled={total === 0 || commit.isPending}
          onClick={() => commit.mutate({ token: preview.token, update_existing_rows: [...confirmed] }, { onSuccess: onDone })}
        >
          <CircleCheck aria-hidden="true" />
          {commit.isPending ? "Importando…" : `Importar ${String(total)} ${total === 1 ? "empresa" : "empresas"}`}
        </Button>
      </div>
    </div>
  );
}
