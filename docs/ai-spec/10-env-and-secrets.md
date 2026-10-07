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
| `S3_ACCESS_KEY` | usuário de storage | sim |
| `S3_SECRET_KEY` | credencial de storage | sim |
| `S3_BUCKET` | bucket de originais | não |
| `S3_REGION` | compatibilidade S3 | não |

Preservar versionamento do bucket e ausência de delete no fluxo normal.

## 4. LiteLLM / OpenAI

| Nome | Finalidade | Secret? |
|---|---|---:|
| `LITELLM_BASE_URL` | gateway local | não |
| `LITELLM_API_KEY` | autenticação app → gateway | sim |
| `LITELLM_MASTER_KEY` | administração do proxy | sim |
| `OPENAI_API_KEY` | provider inicial | sim |
| `ANTHROPIC_API_KEY` | compatibilidade opcional futura/existente | sim |

Aliases/modelos devem ter defaults configuráveis por env e overrides não secretos pelo banco/UI. Não hardcode modelo no domínio.

## 5. Aliases sugeridos

Variáveis de bootstrap, caso o LiteLLM continue usando env para defaults:

- `TRIADOR_MODEL`
- `EXTRATOR_MODEL`
- `EXTRATOR_FORTE_MODEL`
- `RESOLVEDOR_MODEL`
- `CONSULTOR_MODEL`
- `PRIORIZADOR_MODEL`
- `CLASSIFICADOR_MODEL`

O valor é uma string de modelo suportada pelo LiteLLM; não faz parte da regra de negócio.

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

Manter variáveis necessárias ao serviço atual. Não adicionar credenciais de canal externo até existir fluxo solicitado.

## 8. Jobs

- `WORKER_ID`
- `JOB_MAX_TENTATIVAS`
- parâmetros de polling/backoff se já existirem ou forem necessários.

## 9. Regras

- `.env.example` usa somente placeholders.
- `.env` real permanece gitignored.
- Frontend recebe somente config pública.
- Não gravar `OPENAI_API_KEY` ou outras chaves em `ai_model_aliases`.
- Validação de Configurações → IA consulta o gateway sem ecoar credenciais.
