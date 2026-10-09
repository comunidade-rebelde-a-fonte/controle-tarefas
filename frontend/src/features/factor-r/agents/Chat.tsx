import { ScrollText, Send } from "lucide-react";
import { type SubmitEvent, type ReactNode, useId, useState } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import type { FactorRAgentResponse } from "@/services/factorRAgents";

interface Message {
  id: number;
  author: "analista" | "agente";
  text: string;
  response?: FactorRAgentResponse;
}

/** Recusas do agente em texto: a mensagem da API quando é regra de domínio. */
function failure(err: unknown): string {
  if (isApiError(err) && (err.status === 409 || err.status === 422 || err.status === 404 || err.status === 400)) return err.message;
  return "O agente não conseguiu responder agora. Tente de novo.";
}

/**
 * Chat com um agente do Fator R [FR-13, FR-14] (origem: componente Chat). Cada resposta traz o
 * texto validado contra a decisão (ou o template), a decisão estruturada e o link do trace.
 */
export function Chat({
  title,
  description,
  placeholder,
  send,
  renderDecision,
  testId,
  pending,
}: {
  title: string;
  description?: string;
  placeholder: string;
  send: (message: string) => Promise<FactorRAgentResponse>;
  renderDecision?: (response: FactorRAgentResponse) => ReactNode;
  testId: string;
  pending: boolean;
}) {
  const [text, setText] = useState("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [error, setError] = useState<string | null>(null);
  const inputId = useId();

  async function submit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault();
    const question = text.trim();
    if (!question || pending) return;
    setError(null);
    setText("");
    setMessages((m) => [...m, { id: m.length, author: "analista", text: question }]);
    try {
      const response = await send(question);
      setMessages((m) => [...m, { id: m.length, author: "agente", text: response.texto, response }]);
    } catch (err) {
      setError(failure(err));
    }
  }

  return (
    <Card data-testid={testId}>
      <CardHeader title={title} description={description} />
      <CardBody className="grid gap-3">
        <div role="log" aria-label={`Conversa com o ${title}`} aria-live="polite" className="grid max-h-[32rem] gap-2 overflow-y-auto">
          {messages.length === 0 ? <p className="text-sm text-muted-foreground">Nenhuma pergunta ainda.</p> : null}
          {messages.map((m) => (
            <div
              key={m.id}
              data-autor={m.author}
              className={cn("rounded-md border p-2.5 text-sm", m.author === "analista" ? "ml-8 border-primary/20 bg-primary/5" : "mr-8 bg-muted/40")}
            >
              <p className="sr-only">{m.author === "analista" ? "Você:" : `${title}:`}</p>
              <p className="whitespace-pre-line">{m.text}</p>
              {m.response ? (
                <>
                  {renderDecision?.(m.response)}
                  <Link
                    to={`/factor-r/runs/${m.response.trace_id}`}
                    className="mt-1 inline-flex items-center gap-1 text-xs underline underline-offset-4"
                    data-testid="ver-trace"
                  >
                    <ScrollText className="size-3.5" aria-hidden /> ver trace
                  </Link>
                </>
              ) : null}
            </div>
          ))}
          {pending ? (
            <p role="status" className="text-sm text-muted-foreground">
              O agente está respondendo…
            </p>
          ) : null}
        </div>
        {error ? (
          <p role="alert" className="text-sm font-medium text-destructive">
            {error}
          </p>
        ) : null}
        <form onSubmit={(e) => void submit(e)} className="flex gap-2">
          <label htmlFor={inputId} className="sr-only">
            Pergunta ao {title}
          </label>
          <Input id={inputId} name="mensagem" maxLength={1000} value={text} onChange={(e) => setText(e.target.value)} placeholder={placeholder} autoComplete="off" />
          <Button type="submit" disabled={pending || text.trim() === ""}>
            <Send aria-hidden /> Enviar
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
