# Sistema de Controle de Obrigações — Escritório Contábil

Aplicação web que substitui as planilhas do escritório por uma base única, histórica e organizada por competência mensal: cadastro único de empresas, geração automática das obrigações do mês, grades operacionais por setor (Fiscal, Declarações, Contábil, Folha, DCTFWeb, Procurações/Certificados), prazos, pendências, fechamento por setor, dashboard, backlog, importação/exportação e relatórios.

Documentação do projeto:

| Documento | Conteúdo |
|---|---|
| `docs/PRD.md` | Requisitos, regras de negócio e critérios de aceite |
| `docs/plan.md` | Arquitetura, decisões de interpretação e marcos |
| `docs/tasks.md` | Tarefas executáveis e rastreabilidade |
| `AGENTS.md` | Contrato de execução (processo, permissões, convenções) |

## Pré-requisitos

- Go 1.26+ (ver `docs/decisoes.md`, DT-01)
- Node 22+ e npm
- Docker (Docker Desktop, ou Colima configurado)
- `make`

## Estrutura

```text
backend/    API Go (cmd/api, internal/<domínio>, migrations, queries, seed, api/openapi.yaml)
frontend/   SPA React + TypeScript + Vite
e2e/        testes Playwright
docker/     scripts de inicialização do Postgres
scripts/    backup e restauração
docs/       PRD, plano, tarefas, decisões, pendências
```

## Comandos

Execute `make help` para a lista completa.

| Comando | O que faz |
|---|---|
| `make tools` | Instala as ferramentas de desenvolvimento (sqlc, goose, golangci-lint, air) |
| `make dev` | Sobe banco, API e SPA |
| `make db-up` / `make db-down` | Sobe ou para somente o banco |
| `make db-reset` | **Apaga** e recria os bancos locais |
| `make db-psql` | Abre o `psql` no banco de desenvolvimento |
| `make migrate` / `make migrate-down` | Aplica ou reverte migrations |
| `make seed` / `make seed-demo` | Carrega dados de referência / dados de demonstração |
| `make gen` | Gera código a partir do SQL e do OpenAPI |
| `make lint` | Lint do backend e do frontend |
| `make test` | Testes do backend e do frontend |
| `make e2e` | Testes ponta a ponta |

## Testes

| Nível | Ferramenta | Como rodar |
|---|---|---|
| Unidade e integração Go | `go test` + testcontainers (Postgres 16 real, mesma imagem do compose) | `make test` (exige Docker rodando) |
| Componentes | Vitest + Testing Library | `make test` |
| E2E | Playwright | `make e2e` |

- **Integração Go:** `internal/testutil` sobe um Postgres descartável por pacote de teste, aplica as migrations num banco *template* e entrega a cada teste um banco clonado e isolado (`testutil.NewDB`). Há também servidor HTTP de teste (`testutil.NewServer`) e relógio fixo (`testutil.FixedClock`).
- **E2E:** `make e2e` recria o banco `controle_e2e`, sobe uma API própria (porta 8091) e uma SPA própria (porta 5174) e roda o Playwright. **Nunca usa o banco de desenvolvimento `controle`.**
- **Compatibilidade do testcontainers:** funciona com o Docker Desktop sem configuração. No Colima, exporte `DOCKER_HOST=unix://$HOME/.colima/default/docker.sock` e `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`.

## Portas locais

| Serviço | Porta padrão | Variável |
|---|---|---|
| Postgres | 5432 | `POSTGRES_PORT` |
| API | 8080 | `HTTP_PORT` |
| SPA (Vite) | 5173 | — |
| API/SPA do E2E | 8091 / 5174 | `E2E_API_PORT` / `E2E_WEB_PORT` |

Se alguma porta estiver ocupada por outro programa, mude-a no `.env` (não pare o outro serviço).
