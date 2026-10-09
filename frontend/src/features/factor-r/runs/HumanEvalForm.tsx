import { type SubmitEvent, useId, useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Textarea } from "@/components/ui/input";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { type AIHumanEvalRating, useEvaluateAIRun } from "@/services/aiRuns";
import { RATING_LABELS, RATINGS } from "./labels";

const COMMENT_REQUIRED = "Comentário é obrigatório quando a nota é erro.";

/**
 * Nota humana da corrida [FR-17, AC-031]: acerto, parcial ou erro, com comentário obrigatório
 * em erro (conferido também pela API e pelo banco). Vai ao Langfuse como o score human_eval.
 */
export function HumanEvalForm({ traceId }: { traceId: string }) {
  const toast = useToast();
  const evaluate = useEvaluateAIRun(traceId);
  const [rating, setRating] = useState<AIHumanEvalRating>("acerto");
  const [comment, setComment] = useState("");
  const [error, setError] = useState<string | null>(null);
  const groupId = useId();

  function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(null);
    const text = comment.trim();
    if (rating === "erro" && text === "") {
      setError(COMMENT_REQUIRED);
      return;
    }
    evaluate.mutate(
      { rating, comment: text === "" ? null : text },
      {
        onSuccess: () => {
          setComment("");
          toast({ title: "Nota registrada", variant: "success" });
        },
        onError: (err) => {
          if (isApiError(err) && err.code === "EVAL_COMMENT_REQUIRED") setError(COMMENT_REQUIRED);
        },
      },
    );
  }

  return (
    <Card>
      <CardHeader title="Nota humana" description="A sua avaliação da corrida. Conta no acerto humano do painel." />
      <CardBody>
        <form onSubmit={submit} className="grid gap-3" aria-label="Registrar nota humana">
          <div role="radiogroup" aria-labelledby={groupId} className="flex flex-wrap gap-4">
            <span id={groupId} className="sr-only">
              Nota
            </span>
            {RATINGS.map((r) => (
              <label key={r} className="inline-flex items-center gap-2 text-sm">
                <input type="radio" name="nota" value={r} checked={rating === r} onChange={() => setRating(r)} className="size-4 accent-primary" />
                {RATING_LABELS[r]}
              </label>
            ))}
          </div>
          <Field label={rating === "erro" ? "Comentário (obrigatório na nota erro)" : "Comentário"} hint="Até 2.000 caracteres. CNPJ e CPF são mascarados no envio ao Langfuse.">
            <Textarea name="comentario" rows={3} maxLength={2000} value={comment} onChange={(e) => setComment(e.target.value)} />
          </Field>
          {error ? (
            <p role="alert" className="text-sm font-medium text-destructive">
              {error}
            </p>
          ) : null}
          {evaluate.error && !error ? <ErrorAlert error={evaluate.error} fallback="Não foi possível registrar a nota." /> : null}
          <Button type="submit" className="w-fit" disabled={evaluate.isPending}>
            Registrar nota
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
