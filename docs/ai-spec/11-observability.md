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

## 6. Métricas Fator R

Preservar:
- corridas 24h/totais;
- pendências sem nota humana;
- acerto ouro/humano;
- por agente: volume, latência, confiança, erros, reviews;
- carteira/risco e cobertura 12/12.

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

### LiteLLM/provider fora
- job entra em retry com backoff;
- após limite, estado de revisão/erro operacional;
- nenhuma regra determinística deve ser contornada.

### MinIO fora
- upload do Conciliador deve falhar antes de criar estado inconsistente;
- não armazenar original temporariamente em local alternativo silencioso.

### Worker parado
- uploads continuam registrados apenas se storage + transação puderem garantir consistência;
- jobs permanecem pendentes;
- UI indica fila parada/pendente.
