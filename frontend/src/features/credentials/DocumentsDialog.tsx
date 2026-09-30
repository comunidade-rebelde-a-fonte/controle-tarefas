import { Trash2, Upload } from "lucide-react";
import { useId, useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Dialog, DialogContent, DialogHeader } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { formatDate, formatDateTime } from "@/lib/format";
import {
  controlOf,
  CREDENTIAL_TYPE_LABELS,
  CREDENTIAL_TYPES,
  type CredentialRow,
  type CredentialType,
  useCompanyCredentials,
  useDeleteCredentialFile,
  useUploadCredentialFile,
} from "@/services/credentials";
import { ExpiresCell, RenewalCell, SituationBadge } from "./cells";
import { useSaveCredential } from "./useSaveCredential";

/** Formatos aceitos por tipo (dica ao navegador; a validação real é da API) [PRD §19.1-§19.2]. */
const ACCEPT: Record<CredentialType, string> = {
  RFB_POWER_OF_ATTORNEY: ".pdf,.png,.jpg,.jpeg,application/pdf,image/png,image/jpeg",
  DIGITAL_CERTIFICATE: ".cer,.crt,.pem",
};

const HINT: Record<CredentialType, string> = {
  RFB_POWER_OF_ATTORNEY: "PDF, PNG ou JPEG, até 10 MB.",
  DIGITAL_CERTIFICATE: "Somente o certificado público (.cer, .crt ou .pem), até 10 MB. Arquivos .pfx/.p12 ou com chave privada são recusados.",
};

function formatSize(bytes: number | null): string {
  if (bytes === null) return "";
  if (bytes < 1024) return `${String(bytes)} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

/** Documentos vigentes e observações de uma empresa (procuração e certificado). */
export function DocumentsDialog({ companyId, onClose }: { companyId: string | null; onClose: () => void }) {
  return (
    <Dialog open={companyId !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent size="lg">{companyId && <CompanyDocuments companyId={companyId} />}</DialogContent>
    </Dialog>
  );
}

function CompanyDocuments({ companyId }: { companyId: string }) {
  const query = useCompanyCredentials(companyId);
  const row = query.data;
  if (query.error) return <ErrorAlert error={query.error} fallback="Não foi possível carregar os documentos." />;
  if (!row) {
    return (
      <>
        <DialogHeader title="Documentos" />
        <Skeleton className="h-48" />
      </>
    );
  }
  return (
    <>
      <DialogHeader title={`Documentos — ${row.company.legal_name}`} description="Um documento vigente por tipo; um novo envio substitui o anterior." />
      <div className="grid gap-4 md:grid-cols-2">
        {CREDENTIAL_TYPES.map((type) => (
          <CredentialSection key={`${row.company.id}:${type}`} row={row} type={type} />
        ))}
      </div>
    </>
  );
}

/**
 * Procuração ou certificado de uma empresa: situação, documento vigente e observação. Com
 * `editable`, também edita vencimento e etapa (ficha da empresa, fora da grade).
 */
export function CredentialSection({ row, type, editable = false }: { row: CredentialRow; type: CredentialType; editable?: boolean }) {
  const ctl = controlOf(row, type);
  const upload = useUploadCredentialFile();
  const remove = useDeleteCredentialFile();
  const { save, isPending: savingNotes } = useSaveCredential();
  const toast = useToast();
  const [error, setError] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [notes, setNotes] = useState(ctl.notes ?? "");
  const id = useId();
  const label = CREDENTIAL_TYPE_LABELS[type];

  const onFile = (file: File | undefined) => {
    if (!file) return;
    setError(null);
    upload.mutate(
      { companyId: row.company.id, type, file },
      {
        onSuccess: () => toast({ title: "Documento enviado", description: `${label}: ${file.name}`, variant: "success" }),
        onError: (err) => setError(isApiError(err) ? err.message : "Falha de comunicação com o servidor."),
      },
    );
  };

  return (
    <section aria-labelledby={`${id}-title`} className="grid content-start gap-3 rounded-md border p-4">
      <div className="flex items-center justify-between gap-2">
        <h3 id={`${id}-title`} className="text-sm font-semibold">
          {label}
        </h3>
        <SituationBadge situation={ctl.situation} />
      </div>
      {editable ? (
        <div className="grid grid-cols-2 gap-3 text-xs">
          <div className="grid gap-1">
            <span className="font-medium">Vencimento</span>
            <div className="rounded-md border">
              <ExpiresCell row={row} type={type} columnId={`${type}_expires`} tabIndex={0} />
            </div>
          </div>
          <div className="grid gap-1">
            <span className="font-medium">Etapa de renovação</span>
            <RenewalCell row={row} type={type} columnId={`${type}_renewal`} tabIndex={0} />
          </div>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">Validade: {ctl.expires_at ? formatDate(ctl.expires_at) : "sem data cadastrada"}</p>
      )}

      <div className="grid gap-2 text-sm">
        {ctl.file ? (
          <p>
            <span className="font-medium">{ctl.file.name}</span>{" "}
            <span className="text-xs text-muted-foreground">
              {formatSize(ctl.file.size_bytes)} · enviado em {formatDateTime(ctl.file.uploaded_at)}
            </span>
          </p>
        ) : (
          <p className="text-muted-foreground">Nenhum documento enviado.</p>
        )}
        <label className="grid gap-1 text-xs">
          <span className="font-medium">
            <Upload aria-hidden="true" className="mr-1 inline size-3.5" />
            {ctl.file ? `Substituir documento — ${label}` : `Enviar documento — ${label}`}
          </span>
          <input
            type="file"
            accept={ACCEPT[type]}
            disabled={upload.isPending}
            className="text-xs file:mr-2 file:rounded file:border file:bg-card file:px-2 file:py-1"
            aria-describedby={`${id}-hint`}
            onChange={(e) => {
              onFile(e.target.files?.[0]);
              e.target.value = "";
            }}
          />
        </label>
        <p id={`${id}-hint`} className="text-xs text-muted-foreground">
          {HINT[type]}
        </p>
        {error && (
          <p role="alert" className="rounded-md border border-destructive/40 bg-red-50 px-3 py-2 text-sm text-red-800">
            {error}
          </p>
        )}
        {ctl.file &&
          (confirmRemove ? (
            <div className="flex gap-2">
              <Button
                variant="destructive"
                size="sm"
                disabled={remove.isPending}
                onClick={() =>
                  remove.mutate(
                    { companyId: row.company.id, type },
                    {
                      onSuccess: () => {
                        setConfirmRemove(false);
                        toast({ title: "Documento removido", variant: "success" });
                      },
                      onError: (err) => setError(isApiError(err) ? err.message : "Falha de comunicação com o servidor."),
                    },
                  )
                }
              >
                Confirmar remoção
              </Button>
              <Button variant="outline" size="sm" onClick={() => setConfirmRemove(false)}>
                Cancelar
              </Button>
            </div>
          ) : (
            <Button variant="outline" size="sm" className="justify-self-start" onClick={() => setConfirmRemove(true)}>
              <Trash2 aria-hidden="true" />
              Remover documento
            </Button>
          ))}
      </div>

      <form
        className="grid gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          save(row, type, { notes: notes.trim() || null }, { onDone: () => toast({ title: "Observação salva", variant: "success" }) });
        }}
      >
        <label className="grid gap-1 text-xs font-medium">
          Observação — {label}
          <Textarea value={notes} maxLength={2000} rows={3} onChange={(e) => setNotes(e.target.value)} />
        </label>
        <Button type="submit" size="sm" variant="outline" className="justify-self-start" disabled={savingNotes || notes === (ctl.notes ?? "")}>
          Salvar observação
        </Button>
      </form>
    </section>
  );
}
