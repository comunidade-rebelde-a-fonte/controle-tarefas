import { useState } from "react";
import { PageHeader } from "@/components/PageHeader";
import { cn } from "@/lib/utils";
import type { ImportPreview, ImportResult } from "@/services/imports";
import { PreviewStep } from "./PreviewStep";
import { ResultStep } from "./ResultStep";
import { UploadStep } from "./UploadStep";

type Step = { kind: "upload" } | { kind: "preview"; preview: ImportPreview } | { kind: "result"; result: ImportResult };

const STEPS = [
  { kind: "upload", label: "Modelo e envio" },
  { kind: "preview", label: "Prévia e confirmação" },
  { kind: "result", label: "Resumo" },
] as const;

/**
 * Importação de empresas por planilha [PRD §35.3]: baixar modelo → enviar → prévia com erros
 * por linha → confirmar (existentes marcadas) → resumo. A exportação é feita em cada grade.
 */
export function ImportsPage() {
  const [step, setStep] = useState<Step>({ kind: "upload" });
  const current = STEPS.findIndex((s) => s.kind === step.kind);

  return (
    <>
      <PageHeader title="Importação/Exportação" description="Importe a carteira de empresas a partir de uma planilha .xlsx ou .csv." />
      <div className="grid gap-5 p-6">
        <ol aria-label="Etapas da importação" className="flex flex-wrap items-center gap-2 text-sm">
          {STEPS.map((s, i) => (
            <li
              key={s.kind}
              aria-current={i === current ? "step" : undefined}
              className={cn(
                "flex items-center gap-1.5 rounded-full border px-3 py-1",
                i === current ? "border-primary bg-primary text-primary-foreground" : i < current ? "text-foreground" : "text-muted-foreground",
              )}
            >
              <span className="tabular-nums">{i + 1}.</span> {s.label}
            </li>
          ))}
        </ol>
        {step.kind === "upload" && <UploadStep onPreview={(preview) => setStep({ kind: "preview", preview })} />}
        {step.kind === "preview" && (
          <PreviewStep preview={step.preview} onBack={() => setStep({ kind: "upload" })} onDone={(result) => setStep({ kind: "result", result })} />
        )}
        {step.kind === "result" && <ResultStep result={step.result} onRestart={() => setStep({ kind: "upload" })} />}
      </div>
    </>
  );
}
