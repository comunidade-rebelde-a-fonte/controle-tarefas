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
backend/    API Go (cmd/api, internal/<domínio>, migrations, queries, seed, api/openapi.yaml);
            o mesmo binário roda o worker dos jobs assíncronos (`api worker`)
frontend/   SPA React + TypeScript + Vite
e2e/        testes Playwright
docker/     inicialização do Postgres, do MinIO (bucket versionado) e configuração do Langfuse
litellm/    aliases de modelo do gateway LiteLLM
n8n/        workflows de ingestão e a pasta monitorada (n8n/inbox, não versionada)
scripts/    backup, restauração e saúde dos serviços
docs/       PRD, plano, tarefas, decisões, pendências; docs/ai-spec e docs/consolidacao (consolidação)
```

## Comandos

Execute `make help` para a lista completa.

| Comando | O que faz |
|---|---|
| `make tools` | Instala as ferramentas de desenvolvimento (sqlc, goose, golangci-lint, air) |
| `make dev` | Sobe banco, API, SPA, worker, MinIO, LiteLLM e n8n (e inicializa o bucket do MinIO) |
| `make langfuse-up` / `make langfuse-down` | Sobe ou para o Langfuse self-hosted (compose dedicado, mais pesado) |
| `make dev-all` | `make dev` + `make langfuse-up` |
| `make health` | Saúde de cada serviço local; sai com erro se um obrigatório estiver fora |
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
| Gateway de IA | LiteLLM real (container) + provedor OpenAI simulado, sem chamada externa | `make test-litellm` |
| Carga | `backend/cmd/loadgen` | `make perf-generate` e `make perf-bench` |

- **Integração Go:** `internal/testutil` sobe um Postgres descartável por pacote de teste, aplica as migrations num banco *template* e entrega a cada teste um banco clonado e isolado (`testutil.NewDB`). Há também servidor HTTP de teste (`testutil.NewServer`) e relógio fixo (`testutil.FixedClock`).
- **E2E:** `make e2e` recria o banco `controle_e2e`, sobe uma API própria (porta 8091) e uma SPA própria (porta 5174) e roda o Playwright. **Nunca usa o banco de desenvolvimento `controle`.** Seed e API usam o relógio fixo `E2E_FIXED_TIME` (padrão `2026-09-15T12:00:00-03:00`), então as datas não dependem do dia real. Para a tela Configurações → IA, a suíte sobe também um gateway LiteLLM simulado (`e2e/support/fake-litellm.mjs`, porta 8093). A suíte cobre os 11 fluxos do PRD §53.3 (títulos `§53.3-N`) e a acessibilidade (`e2e/specs/a11y.spec.ts`: axe WCAG 2.2 A/AA sem violações serious/critical e grade operável pelo teclado).
- **Fator R:** o motor é conferido dígito a dígito contra um golden gerado pelo próprio código Python da origem (`backend/internal/factorr/engine/testdata/golden_gen.py`, semente fixa). Só se regenera o golden se a origem mudar, com a referência congelada em `referencias/` (TASK-002).
- **Carga:** `make perf-generate` recria o banco `controle_perf` com massa sintética determinística (`COMPANIES=300 COMPETENCES=3 SEED=42`), e `make perf-bench` mede as grades, o dashboard, o backlog, as exportações, o `ensure`, o PATCH de status e a alteração em massa. O `loadgen` recusa qualquer banco que não seja o `controle_perf` local. Resultados e medição no navegador: `docs/perf.md`.
- **Compatibilidade do testcontainers:** funciona com o Docker Desktop sem configuração. No Colima, exporte `DOCKER_HOST=unix://$HOME/.colima/default/docker.sock` e `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`. No Windows (Docker Desktop), defina `DOCKER_HOST=npipe:////./pipe/docker_engine` no `.env`: sem isso, quando muitos pacotes sobem Postgres em paralelo, o testcontainers pode não achar o pipe do Docker momentaneamente ocupado e falhar com `rootless Docker is not supported on Windows`.

## Portas locais

| Serviço | Porta padrão | Variável |
|---|---|---|
| Postgres | 5432 | `POSTGRES_PORT` |
| API | 8080 | `HTTP_PORT` |
| SPA (Vite) | 5173 | — |
| API/SPA do E2E | 8091 / 5174 | `E2E_API_PORT` / `E2E_WEB_PORT` |
| MinIO (API S3 / console) | 9000 / 9001 | `MINIO_PORT` / `MINIO_CONSOLE_PORT` |
| LiteLLM | 4000 | `LITELLM_PORT` |
| n8n | 5678 | `N8N_PORT` |
| Langfuse (web / MinIO interno) | 3100 / 9190 | `LANGFUSE_WEB_PORT` / `LANGFUSE_MINIO_PORT` |
| Worker (healthcheck, só dentro do container) | 8090 | — |

Se alguma porta estiver ocupada por outro programa, mude-a no `.env` (não pare o outro serviço). As portas dos serviços auxiliares ficam publicadas só em `127.0.0.1`.

## Serviços auxiliares (consolidação)

O produto consolida o Controle de Tarefas com a Plataforma Fator R e a Conciliação Contábil (`docs/ai-spec/`). Os serviços dos módulos de origem continuam fazendo parte do funcionamento [ADR-002]:

| Serviço | Para quê | Observações |
|---|---|---|
| worker | Jobs assíncronos: fila em PostgreSQL (`backend/internal/jobs`) | Mesmo binário da API (`api worker`), processo e container separados; healthcheck em `/health`. Até 3 tentativas com espera de 2ⁿ s. Um job de worker que caiu volta para a fila pelo prazo de posse. `GET /api/v1/jobs/{id}` mostra o estado e se há worker ativo; `POST …/retry` reenvia um job `dead`. Os tipos de job ficam registrados em `backend/internal/worker/handlers.go` |
| MinIO | Originais da Conciliação, imutáveis | Bucket `S3_BUCKET` versionado; o usuário da aplicação (`S3_ACCESS_KEY`) não tem permissão de apagar (`docker/minio/init.sh`). Imagem da Chainguard fixada por digest (a MinIO deixou de publicar imagens da edição comunitária) |
| LiteLLM | Gateway de modelos dos agentes, por alias (`litellm/config.yaml`) | A aplicação resolve o alias (`internal/ai`) e chama o gateway com o modelo concreto. Provedor inicial: OpenAI; modelos padrão no `.env` (`<ALIAS>_MODEL`, decididos na Q-003), trocáveis em **Cadastros e Configurações → IA** sem reiniciar nada (a troca é validada no gateway e vale para as próximas execuções). A chave do provedor (`OPENAI_API_KEY`) só chega ao container do LiteLLM; a aplicação usa a chave do gateway |
| n8n | Ingestão automatizada: arquivos colocados em `n8n/inbox/` são enviados à API | A demonstração usa upload manual; o workflow (`n8n/workflows/ingestao.json`) é adaptado ao endpoint novo na TASK-015 |
| Langfuse | Traces e avaliações dos agentes (self-hosted) | Compose próprio (`docker-compose.langfuse.yml`); informativo: fora do ar, o domínio continua |

## Dados de referência e demonstração

- `make seed` carrega os dados de referência (conjuntos de status, catálogo de obrigações, feriados nacionais e as tabelas dos Anexos III e V do Simples usadas pelo Fator R). É idempotente e reaplica os valores do seed; as tabelas do Simples só ganham linhas novas, nunca têm uma vigência alterada.
- `make seed-demo` carrega também as **20 empresas fictícias** do PRD §37 e os responsáveis de exemplo. Empresas já existentes (mesmo CNPJ) não são alteradas.
- O mesmo seed grava dados do **Fator R** (24 meses de receita e folha, configuração e pacotes) para as 5 empresas sujeitas ao Fator R. No PA corrente, a carteira mostra um caso de cada: vermelho, amarelo, verde, meses faltantes, meses declarados no PGDAS-D e dados insuficientes.
- **Bypass de CNPJ, restrito ao seed:** os CNPJs fictícios do PRD não têm dígitos verificadores válidos. Por isso o seed de demonstração grava as empresas direto no banco, sempre com 14 dígitos. Todo cadastro pela API (tela, importação) continua validando os dígitos verificadores.
- Valores provisórios (vencimentos, "Sem folha", matriz de declarações, feriados locais) estão marcados `A VALIDAR (Qn)` e listados em `docs/pendencias.md`.

## Operação diária

O ciclo mensal do escritório [PRD §58] acontece todo dentro do sistema: cadastrar, configurar, abrir a competência, executar as obrigações, acompanhar prazos, tratar pendências, fechar cada setor, consultar o histórico e exportar ou relatar.

| Quando | O que fazer | Onde |
|---|---|---|
| Virada do mês | Nada manual. A competência nova é gerada sozinha na verificação diária (00:05, no fuso `APP_TIMEZONE`) ou quando a aplicação sobe. Competências já geradas não são reescritas. | Dashboard (seletor de competência) |
| Empresa nova | Cadastre pela tela Empresas ou pela Importação (planilha com pré-validação). Na ficha, configure regime, módulos, aplicabilidade, folha e responsáveis. A empresa entra na competência corrente na próxima geração; veja `docs/operacao.md` §2. | Empresas, Importação/Exportação |
| Dia a dia | Atualize os status nas grades de cada setor, com o mouse ou pelo teclado (Tab até a grade, setas, Enter abre o status, Esc fecha). O atraso é calculado sozinho. Busque por nome, código ou CNPJ; o CNPJ copiado vai sem máscara. | Fiscal, Declarações, Folha, Contábil, DCTFWeb |
| Acompanhar | Indicadores e "Atenção agora" no Dashboard (cada número abre a lista que o explica); pendências de meses anteriores no Backlog; vencimentos de procurações e certificados. | Dashboard, Backlog, Procurações e Certificados |
| Fator R | Carteira do PA por semáforo e economia, com a ação sugerida. Na ficha, aba Fator R: resultado do PA com o cálculo explicado, grade mensal de receita e folha e configuração (pacote, honorário, início de atividade). "Sujeita ao Fator R" é marcada na aba Folha. O PGDAS-D da Receita Federal prevalece. | Fator R; ficha → aba Fator R |
| Fechar o setor | Quando a linha mostrar "Pronto para fechar", use **Fechar**. Cada módulo fecha por empresa, independente dos outros. Para corrigir depois, **Reabrir** exige motivo e fica registrado no histórico da empresa. | Grades dos setores; ficha → Histórico de competências |
| Relatar | **Exportar** baixa o XLSX da grade com a competência, os filtros e a busca atuais. Relatórios gera os 6 PDFs (situação geral, pendências, atrasadas, backlog, credenciais e controle do módulo). | Grades, Relatórios |
| Servidor, todo dia | Backup no cron do host (`scripts/backup.sh`): confira o log e mantenha uma cópia fora do servidor. | `docs/operacao.md` §4 |

Prazos, a matriz de declarações por regime e outros valores ainda provisórios (`A VALIDAR`) estão em `docs/pendencias.md`, até a resposta do escritório (T-000).

## Produção

Imagem única (`Dockerfile`: SPA + API no mesmo binário, usuário não-root) e `docker-compose.prod.yml` (Postgres sem porta publicada no host). Implantação, atualização, backup (`scripts/backup.sh`), restauração (`scripts/restore.sh`) e rotação de senha: `docs/operacao.md`.

```bash
cp .env.prod.example .env.prod   # defina a senha do banco (openssl rand -hex 24)
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
```

