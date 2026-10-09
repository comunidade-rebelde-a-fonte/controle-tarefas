# 11 — Observabilidade e Auditoria

## 1. Objetivo

Preservar e integrar as duas formas de observabilidade existentes:

1. auditoria local no banco do produto;
2. Langfuse self-hosted para corridas/evals de IA.

Nenhuma substitui a outra.

## 2. Logs da aplicação

Continuar com logs estruturados (`slog` no backend Go):
- request id;
- job id;
- trace id;
- módulo;
- erro categorizado;
- duração.

Não logar:
- API keys;
- conteúdo integral de extratos/comprovantes;
- senha;
- dados sensíveis desnecessários.

## 3. Corridas de IA

Todo efeito persistente derivado de agente precisa de `trace_id` local.

Campos mínimos:
- agente;
- gatilho;
- company_id quando aplicável;
- PA/competência quando aplicável;
- alias;
- provider/modelo resolvido;
- prompt version/hash;
- status `ok|error|needs_review`;
- confiança quando fizer sentido;
- latência;
- tokens/custo quando provider/gateway fornecer;
- decisão estruturada sanitizada;
- status de sync Langfuse.

## 4. Langfuse

Preservar convenção de spans do Fator R sempre que aplicável:
- `plan`
- `parse` ou `tool`
- `decide`
- `render`

Para Conciliador, extração multi-tentativa pode ter spans por tentativa/ferramenta, sem perder `extracao_log` local.

Tags/metadata:
- agente;
- gatilho;
- company_id;
- competence/PA;
- parser version;
- política relevante;
- alias/modelo.

Não enviar texto integral de PDF ao Langfuse por padrão.

## 5. Sincronização resiliente

- Persistir local primeiro.
- Tentar Langfuse.
- Falha marca `failed` e não reverte decisão válida de domínio.
- Job periódico reenvia resumo/trace conforme mecanismo já previsto no Fator R.
- Evitar duplicata via `trace_id`/idempotência.

> **Como foi implementado (TASK-009; D-022).**
> - A corrida põe o job `langfuse_sync` na fila na mesma transação.
> - O worker envia por OTLP/HTTP JSON e pela API de scores; uma falha marca `failed`.
> - A cada `LANGFUSE_RETRY_SECONDS` (padrão 300), o worker reenvia corridas e notas `failed`, e as `pending` paradas há mais tempo que isso.
> - Os IDs são fixos (span raiz derivado do trace; score com o ID da nota): reenviar não duplica.

## 6. Métricas Fator R

Preservar:
- corridas 24h/totais;
- pendências sem nota humana;
- acerto ouro/humano;
- por agente: volume, latência, confiança, erros, reviews;
- carteira/risco e cobertura 12/12.

> **Tela (TASK-014b; D-025).** Fator R → Observabilidade (`/factor-r/runs`):
> - painel com essas métricas;
> - corridas filtráveis por agente, resultado, nota humana e empresa;
> - detalhe da corrida com passos, decisão, nota ouro e nota humana.
>
> Custo e tokens vêm da trilha local (D-022). Carteira e cobertura ficam na própria carteira do Fator R.

## 7. Métricas Conciliação

Preservar/adicionar:
- % extrações aprovadas na primeira tentativa;
- tentativas médias;
- custo por documento;
- % conciliado por passe;
- itens por bucket;
- correções humanas pós-extração;
- erros detectados após fechamento (meta: zero);
- sucesso de playbook por banco.

> **Desde a TASK-016 (D-027):** os dados dessas métricas de extração já são gravados: o log de extração por tentativa (alias, modelo, tokens, custo, latência, resultado do validador), a importação com o número de tentativas e os playbooks com sucessos e falhas. O painel chega com a TASK-021/024. `make eval-extrator` mede o extrator contra o golden.

## 8. Healthchecks

No mínimo:
- API;
- Postgres principal;
- worker heartbeat;
- MinIO;
- LiteLLM;
- Langfuse (informativo, não bloqueia domínio);
- n8n (informativo para demo manual).

## 9. Runbooks mínimos

### Langfuse fora
- confirmar trace local;
- verificar fila `langfuse_sync_retry`;
- não reprocessar domínio somente para gerar trace.

> **Na implementação (D-022):**
> - a fila é a dos jobs `kind = 'langfuse_sync'`;
> - o estado de cada corrida está em `ai_runs.langfuse_sync_status` e `langfuse_last_error` (também em `GET /ai/runs/{trace_id}`);
> - quando o Langfuse volta, o reenvio periódico resolve sozinho;
> - para conferir a rede: `docker exec controle-worker-1 curl -fsS http://langfuse:3000/api/public/health`.

### LiteLLM/provider fora
- job entra em retry com backoff;
- após limite, estado de revisão/erro operacional;
- nenhuma regra determinística deve ser contornada.

> **Na Conciliação (TASK-016; D-027):**
> - o job da triagem ou da extração falha com `LLM_UNAVAILABLE` e a fila tenta de novo; esgotadas as tentativas, ele fica `dead` e o documento volta pela `POST /reconciliation/documents/{id}/retry`;
> - com o alias desligado na tela de IA, o documento vai direto para a revisão;
> - os leitores determinísticos (PDF do leiaute conhecido, CSV) funcionam sem o LLM.

### MinIO fora
- upload do Conciliador deve falhar antes de criar estado inconsistente;
- não armazenar original temporariamente em local alternativo silencioso.

> **Desde a TASK-015 (D-026):** o envio responde 503 `STORAGE_UNAVAILABLE` sem gravar documento nem job (testado com o MinIO fora e sem configuração). API e worker continuam saudáveis.

### Worker parado
- uploads continuam registrados apenas se storage + transação puderem garantir consistência;
- jobs permanecem pendentes;
- UI indica fila parada/pendente.
