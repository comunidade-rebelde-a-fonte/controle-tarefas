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
| `make demo-fixtures` | Prepara os fixtures da demonstração (cenário da Conciliação na 111 e extratos do PGDAS-D em `demo/pgdas`), com o `make dev` no ar |
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
| Extratores da Conciliação | Golden dos extratores com o LiteLLM do ambiente: no extrato, aprovação na 1ª tentativa, tentativas, custo e diferenças para o gabarito; no comprovante, acurácia por campo, chamadas e custo. **Usa o provedor real (custo)** | `make eval-extrator` |
| Langfuse | Langfuse local real (exige `make langfuse-up`): envia uma corrida e confere trace, spans e scores pela API pública | `make test-langfuse` |
| Carga | `backend/cmd/loadgen` | `make perf-generate` e `make perf-bench` |

- **MinIO nos testes:** `testutil.NewMinIO` sobe a mesma imagem do compose e roda o `docker/minio/init.sh` (bucket versionado e usuário sem permissão de apagar). Os testes da Conciliação gravam e leem originais de verdade e provam que a aplicação não apaga.
- **Integração Go:** `internal/testutil` sobe um Postgres descartável por pacote de teste, aplica as migrations num banco *template* e entrega a cada teste um banco clonado e isolado (`testutil.NewDB`). Há também servidor HTTP de teste (`testutil.NewServer`) e relógio fixo (`testutil.FixedClock`).
- **E2E:** `make e2e` recria o banco `controle_e2e`, sobe uma API própria (porta 8091) e uma SPA própria (porta 5174) e roda o Playwright. **Nunca usa o banco de desenvolvimento `controle`.** Seed e API usam o relógio fixo `E2E_FIXED_TIME` (padrão `2026-09-15T12:00:00-03:00`), então as datas não dependem do dia real. A suíte sobe também o worker (healthcheck na porta 8094), que lê os extratos do inbox, e um gateway LiteLLM simulado (`e2e/support/fake-litellm.mjs`, porta 8093) para a tela Configurações → IA e para os agentes do Fator R, com respostas determinísticas e sem provedor real. Para os originais da Conciliação, sobe um MinIO só do E2E (projeto compose `controle-e2e`, porta 9102, volume e credenciais de teste); **nunca usa o MinIO de desenvolvimento**. Os testes da Conciliação (`e2e/specs/reconciliation-*.spec.ts`) usam extratos diferentes do cenário Horizonte (ou nenhum): o mesmo original é recusado e as transações repetidas são ignoradas, porque a chave de dedupe é única no sistema. A suíte cobre os 11 fluxos do PRD §53.3 (títulos `§53.3-N`) e a acessibilidade (`e2e/specs/a11y.spec.ts`: axe WCAG 2.2 A/AA sem violações serious/critical e grade operável pelo teclado).
- **Fator R:** o motor é conferido dígito a dígito contra um golden gerado pelo próprio código Python da origem (`backend/internal/factorr/engine/testdata/golden_gen.py`, semente fixa). Só se regenera o golden se a origem mudar, com a referência congelada em `referencias/` (TASK-002).
- **Agentes do Fator R:** o validador do texto do LLM, os formatadores e a classificação da intenção são conferidos contra goldens gerados pelo código da origem (`backend/internal/factorr/agents/testdata/*_gen.py`; o da intenção exige o pydantic 2.13.5 do `uv.lock` da origem). Os testes nunca chamam provedor real: o gateway é simulado.
- **Parser PGDAS-D:** o parser é conferido contra um golden gerado pelo `parse` da origem (`backend/internal/factorr/pgdas/testdata/golden_gen.py`), e o texto do PDF contra o pdfplumber da origem (`backend/internal/platform/pdftext/testdata/text_gen.py`). Os dois exigem o Python com o pdfplumber 0.11.10 do `uv.lock` da origem.
- **Carga:** `make perf-generate` recria o banco `controle_perf` com massa sintética determinística (`COMPANIES=300 COMPETENCES=3 SEED=42`), e `make perf-bench` mede as grades, o dashboard, o backlog, as exportações, o `ensure`, o PATCH de status e a alteração em massa. O `loadgen` recusa qualquer banco que não seja o `controle_perf` local. Resultados e medição no navegador: `docs/perf.md`.
- **Compatibilidade do testcontainers:** funciona com o Docker Desktop sem configuração. No Colima, exporte `DOCKER_HOST=unix://$HOME/.colima/default/docker.sock` e `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`. No Windows (Docker Desktop), defina `DOCKER_HOST=npipe:////./pipe/docker_engine` no `.env`: sem isso, quando muitos pacotes sobem Postgres em paralelo, o testcontainers pode não achar o pipe do Docker momentaneamente ocupado e falhar com `rootless Docker is not supported on Windows`. O `testutil` ainda tenta de novo, até 3 vezes, quando esse erro acontece.

## Portas locais

| Serviço | Porta padrão | Variável |
|---|---|---|
| Postgres | 5432 | `POSTGRES_PORT` |
| API | 8080 | `HTTP_PORT` |
| SPA (Vite) | 5173 | — |
| API/SPA do E2E | 8091 / 5174 | `E2E_API_PORT` / `E2E_WEB_PORT` |
| Worker/gateway simulado do E2E | 8094 / 8093 | `E2E_WORKER_HEALTH_PORT` / `E2E_LITELLM_PORT` |
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
| worker | Jobs assíncronos: fila em PostgreSQL (`backend/internal/jobs`) | Mesmo binário da API (`api worker`), processo e container separados; healthcheck em `/health`. Processa o inbox PGDAS-D e a Conciliação: triagem e extração do extrato e do comprovante, com o original lido do MinIO e os limiares `LIMIAR_TRIAGEM` e `LIMIAR_CAMPO`, a rodada do matching da competência (`LIMIAR_FUZZY`) o resolvedor de exceções, que só grava a proposta (a pedido, ou depois da rodada com `RESOLVEDOR_AUTOMATICO`), e o classificador contábil, que só sugere a conta (a pedido, ou com `CLASSIFICADOR_AUTOMATICO`). Até 3 tentativas com espera de 2ⁿ s. Um job de worker que caiu volta para a fila pelo prazo de posse. `GET /api/v1/jobs/{id}` mostra o estado e se há worker ativo; `POST …/retry` reenvia um job `dead`. Os tipos de job ficam registrados em `backend/internal/worker/handlers.go` |
| MinIO | Originais da Conciliação, imutáveis | Bucket `S3_BUCKET` versionado; o usuário da aplicação (`S3_ACCESS_KEY`) não tem permissão de apagar (`docker/minio/init.sh`). A API grava cada original com o SHA-256 como chave e guarda a versão (`POST /api/v1/reconciliation/documents`); com o MinIO fora, só esse envio falha (503). Imagem da Chainguard fixada por digest (a MinIO deixou de publicar imagens da edição comunitária) |
| LiteLLM | Gateway de modelos dos agentes, por alias (`litellm/config.yaml`) | A aplicação resolve o alias (`internal/ai`) e chama o gateway com o modelo concreto. Provedor inicial: OpenAI; modelos padrão no `.env` (`<ALIAS>_MODEL`, decididos na Q-003), trocáveis em **Cadastros e Configurações → IA** sem reiniciar nada (a troca é validada no gateway e vale para as próximas execuções). A chave do provedor (`OPENAI_API_KEY`) só chega ao container do LiteLLM; a aplicação usa a chave do gateway |
| n8n | Ingestão automatizada: arquivos colocados em `n8n/inbox/` são enviados à Conciliação (`POST /api/v1/reconciliation/documents`) | A demonstração usa upload manual. `make n8n-import` importa e ativa o workflow (`n8n/workflows/ingestao.json`); o gatilho de pasta, desligado por padrão no n8n 2.x, é religado no compose |
| Langfuse | Traces e avaliações dos agentes (self-hosted) | Compose próprio (`docker-compose.langfuse.yml`); informativo: fora do ar, o domínio continua. A trilha local (`ai_runs`, `GET /api/v1/ai/runs`) é a fonte de verdade; o worker espelha cada corrida e as notas no Langfuse pela rede `controle-obs` (`make obs-net`, criada por `make dev` e `make langfuse-up`) e reenvia o que falhou a cada 5 minutos |

## Dados de referência e demonstração

- `make seed` carrega os dados de referência (conjuntos de status, catálogo de obrigações, feriados nacionais e as tabelas dos Anexos III e V do Simples usadas pelo Fator R). É idempotente e reaplica os valores do seed; as tabelas do Simples só ganham linhas novas, nunca têm uma vigência alterada.
- `make seed-demo` carrega também as **21 empresas fictícias** e os responsáveis de exemplo: as 20 do PRD §37 e a **111 Construtora Horizonte**, a empresa do cenário de aula da Conciliação (agosto de 2026). Empresas já existentes (mesmo CNPJ) não são alteradas.
- O mesmo seed grava dados do **Fator R** (24 meses de receita e folha, configuração e pacotes) para as 5 empresas sujeitas ao Fator R. No PA corrente, a carteira mostra um caso de cada: vermelho, amarelo, verde, meses faltantes, meses declarados no PGDAS-D e dados insuficientes.
- **Bypass de CNPJ, restrito ao seed:** os CNPJs fictícios do PRD não têm dígitos verificadores válidos. Por isso o seed de demonstração grava as empresas direto no banco, sempre com 14 dígitos. Todo cadastro pela API (tela, importação) continua validando os dígitos verificadores.
- `make demo-fixtures` prepara o resto da demonstração (TASK-027), depois do `make seed-demo` e com o `make dev` no ar (o worker lê os documentos e o MinIO guarda os originais). Rodar de novo não duplica nada.
  - **Conciliação:** carrega na 111 o cenário completo de 08/2026, como o `carregar_horizonte.py` da origem: as contas bancárias, um plano de contas fictício, os lançamentos de contas a pagar e a receber, os 2 extratos e os 13 comprovantes (lidos sem o modelo) e a rodada da conciliação. A competência abre em andamento: as automáticas feitas e os casos difíceis na fila de revisão.
  - **Fator R:** grava em `demo/pgdas` três extratos do PGDAS-D para o inbox, com um `LEIA-ME.txt`. Como os CNPJs do seed não têm dígito verificador válido, o extrato da empresa da carteira cai em revisão e se vincula pela ação Vincular. O de CNPJ fora do cadastro oferece o cadastro pelo extrato, e o terceiro, da mesma empresa, se vincula sozinho.
  - **Jornada (F-012):** Dashboard → ficha da 111 → Fator R (carteira e inbox) → Conciliação (fila de 08/2026) → fechamento do Contábil (recusado até a conciliação concluída) → histórico.
- Valores provisórios (vencimentos, "Sem folha", matriz de declarações, feriados locais) estão marcados `A VALIDAR (Qn)` e listados em `docs/pendencias.md`.

## Operação diária

O ciclo mensal do escritório [PRD §58] acontece todo dentro do sistema: cadastrar, configurar, abrir a competência, executar as obrigações, acompanhar prazos, tratar pendências, fechar cada setor, consultar o histórico e exportar ou relatar.

| Quando | O que fazer | Onde |
|---|---|---|
| Virada do mês | Nada manual. A competência nova é gerada sozinha na verificação diária (00:05, no fuso `APP_TIMEZONE`) ou quando a aplicação sobe. Competências já geradas não são reescritas. | Dashboard (seletor de competência) |
| Empresa nova | Cadastre pela tela Empresas ou pela Importação (planilha com pré-validação). Na ficha, configure regime, módulos, aplicabilidade, folha e responsáveis. A empresa entra na competência corrente na próxima geração; veja `docs/operacao.md` §2. | Empresas, Importação/Exportação |
| Dia a dia | Atualize os status nas grades de cada setor, com o mouse ou pelo teclado (Tab até a grade, setas, Enter abre o status, Esc fecha). O atraso é calculado sozinho. Busque por nome, código ou CNPJ; o CNPJ copiado vai sem máscara. | Fiscal, Declarações, Folha, Contábil, DCTFWeb |
| Acompanhar | Indicadores e "Atenção agora" no Dashboard (cada número abre a lista que o explica); pendências de meses anteriores no Backlog; vencimentos de procurações e certificados. | Dashboard, Backlog, Procurações e Certificados |
| Fator R | Carteira do PA por semáforo e economia, com a ação sugerida. Na ficha, aba Fator R: resultado do PA com o cálculo explicado, grade mensal de receita e folha e configuração (pacote, honorário, início de atividade). "Sujeita ao Fator R" é marcada na aba Folha. Na aba também ficam o simulador de correção, o consultor da empresa, os extratos e as corridas da empresa. Extratos PGDAS-D entram pelo **Inbox PGDAS-D**: o worker lê, vincula pelo CNPJ e lança a receita só em competência vazia; o resto vai para revisão (vincular, rejeitar ou cadastrar a empresa pelo extrato). Em **Agentes**, o priorizador monta a fila do PA e o consultor responde sobre a empresa citada; os números vêm sempre do motor, e aos domingos o worker registra a fila da semana. Em **Observabilidade**, o painel dos agentes, as corridas e a nota humana. O PGDAS-D da Receita Federal prevalece. | Fator R (Carteira, Inbox PGDAS-D, Agentes, Observabilidade); ficha → aba Fator R |
| Fechar o setor | Quando a linha mostrar "Pronto para fechar", use **Fechar**. Cada módulo fecha por empresa, independente dos outros. Para corrigir depois, **Reabrir** exige motivo e fica registrado no histórico da empresa. | Grades dos setores; ficha → Histórico de competências |
| Relatar | **Exportar** baixa o XLSX da grade com a competência, os filtros e a busca atuais. Relatórios gera os 6 PDFs (situação geral, pendências, atrasadas, backlog, credenciais e controle do módulo). | Grades, Relatórios |
| Servidor, todo dia | Backup de todos os armazenamentos no cron do host (`scripts/backup.sh`): confira o log e mantenha uma cópia fora do servidor. | `docs/operacao.md` §4 |

Prazos, a matriz de declarações por regime e outros valores ainda provisórios (`A VALIDAR`) estão em `docs/pendencias.md`, até a resposta do escritório (T-000).

## Produção

Execução local completa, fora do modo dev (TASK-031): o `docker-compose.prod.yml` sobe o produto inteiro. São o app (SPA + API no mesmo binário, imagem única e usuário não-root), o worker com a mesma imagem, o Postgres, o MinIO, o LiteLLM, o n8n e o Langfuse. Só a aplicação, o n8n e a interface do Langfuse têm porta no host, em `127.0.0.1` por padrão, e o compose recusa subir sem os segredos. Implantação, atualização, backup de todos os armazenamentos (`scripts/backup.sh`, com contagens e rotação 7/4/12), restauração conferida (`scripts/restore.sh`) e rotação de senha: `docs/operacao.md`.

```bash
cp .env.prod.example .env.prod   # preencha os segredos vazios (openssl rand -hex 24)
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build --wait
```

`make test-restore` (TEST-031) ensaia o ciclo inteiro numa instalação descartável, sem porta no host e sem internet: sobe com dados em todos os armazenamentos, faz o backup, destrói, restaura numa instalação vazia e confere tudo.

