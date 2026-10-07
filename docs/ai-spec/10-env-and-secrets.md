# 10 — Ambiente e Secrets

## 1. Ambientes

A entrega alvo é local. Manter:
- desenvolvimento local;
- bancos separados de teste/E2E/performance quando já existirem;
- stack auxiliar local.

Não definir deploy cloud nesta consolidação.

## 2. Variáveis do produto-base

Preservar as variáveis existentes do `controle-tarefas`, incluindo:
- `DATABASE_URL`
- `APP_ENV`
- `APP_TIMEZONE`
- `HTTP_PORT`
- `CORS_ORIGINS`
- `FILE_STORAGE_DIR`
- `DB_MAX_CONNS`

## 3. MinIO / Conciliação

Variáveis necessárias, sem valores:

| Nome | Finalidade | Secret? |
|---|---|---:|
| `S3_ENDPOINT` | endpoint interno MinIO | não |
| `S3_ACCESS_KEY` | usuário de storage da aplicação, sem permissão de apagar (TASK-003) | sim |
| `S3_SECRET_KEY` | credencial de storage | sim |
| `S3_BUCKET` | bucket de originais | não |
| `S3_REGION` | compatibilidade S3 | não |
| `S3_ROOT_USER` / `S3_ROOT_PASSWORD` | administrador do MinIO; usado só pelo `minio-init` (bucket, versionamento, usuário e política), nunca pela aplicação | sim |

Preservar versionamento do bucket e ausência de delete no fluxo normal. Desde a TASK-003, a ausência de delete também é garantida no storage: a política do usuário da aplicação nega exclusão de objeto, de versão e do bucket, e a suspensão do versionamento (`docker/minio/init.sh`, D-013).

## 4. LiteLLM / OpenAI

| Nome | Finalidade | Secret? |
|---|---|---:|
| `LITELLM_BASE_URL` | gateway local | não |
| `LITELLM_API_KEY` | autenticação app → gateway | sim |
| `LITELLM_MASTER_KEY` | administração do proxy | sim |
| `OPENAI_API_KEY` | provider inicial | sim |
| `ANTHROPIC_API_KEY` | compatibilidade opcional futura/existente | sim |
| `LITELLM_TIMEOUT_SECONDS` | limite de cada chamada da aplicação ao gateway (padrão 120) | não |
| `LITELLM_LOCAL_MODEL_COST_MAP` | `True` no compose: o LiteLLM usa a tabela de modelos embutida na versão fixada, sem download a cada início (D-014) | não |

Aliases/modelos devem ter defaults configuráveis por env e overrides não secretos pelo banco/UI. Não hardcode modelo no domínio.

Desde a TASK-004 (D-014): `OPENAI_API_KEY` e `ANTHROPIC_API_KEY` só chegam ao container do LiteLLM; API e worker recebem `LITELLM_BASE_URL` e `LITELLM_API_KEY`. A aplicação resolve o alias e chama o gateway com o modelo concreto (`provedor/modelo`).

## 5. Aliases sugeridos

Variáveis de bootstrap, caso o LiteLLM continue usando env para defaults:

- `TRIADOR_MODEL`
- `EXTRATOR_MODEL`
- `EXTRATOR_FORTE_MODEL`
- `RESOLVEDOR_MODEL`
- `CONSULTOR_MODEL`
- `PRIORIZADOR_MODEL`
- `CLASSIFICADOR_MODEL`

O valor é uma string de modelo suportada pelo LiteLLM; não faz parte da regra de negócio. Formato `provedor/modelo` (ex.: `openai/gpt-6-luna`), validado pela aplicação (`internal/ai`). Valores padrão (Q-003, decidida em 07/10/2026, D-015) no `docker-compose.yml` (âncora `x-ai-models`) e no `.env.example`.

## 6. Langfuse

Preservar as variáveis do stack self-hosted atual, como:
- `LANGFUSE_TRACING_ENABLED`
- `LANGFUSE_PUBLIC_KEY`
- `LANGFUSE_SECRET_KEY`
- `LANGFUSE_HOST`
- `LANGFUSE_PUBLIC_URL`
- `LANGFUSE_PROJECT_ID`
- `NEXTAUTH_URL`
- `NEXTAUTH_SECRET`
- `SALT`
- `ENCRYPTION_KEY`
- `LF_POSTGRES_PASSWORD`
- `CLICKHOUSE_PASSWORD`
- `MINIO_ROOT_PASSWORD`
- `REDIS_AUTH`
- `LANGFUSE_ADMIN_EMAIL`
- `LANGFUSE_ADMIN_PASSWORD`

## 7. n8n

Manter variáveis necessárias ao serviço atual. Não adicionar credenciais de canal externo até existir fluxo solicitado. `N8N_PORT`: porta do editor no host (só 127.0.0.1).

## 8. Jobs

- `WORKER_ID`
- `WORKER_HEALTH_PORT` (healthcheck do worker; padrão 8090, só dentro do container) e `WORKER_DB_MAX_CONNS` (pool do worker no compose), desde a TASK-003
- `JOB_MAX_TENTATIVAS`
- parâmetros de polling/backoff se já existirem ou forem necessários.

## 8.1 Portas dos serviços auxiliares (host, só 127.0.0.1)

`MINIO_PORT`, `MINIO_CONSOLE_PORT`, `LITELLM_PORT`, `N8N_PORT`, `LANGFUSE_WEB_PORT`, `LANGFUSE_MINIO_PORT`. Valores padrão no `README.md` (Portas locais).

## 9. Regras

- `.env.example` usa somente placeholders.
- `.env` real permanece gitignored.
- Frontend recebe somente config pública.
- Não gravar `OPENAI_API_KEY` ou outras chaves em `ai_model_aliases`.
- Validação de Configurações → IA consulta o gateway sem ecoar credenciais.
