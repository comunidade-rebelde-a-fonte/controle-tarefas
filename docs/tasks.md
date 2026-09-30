# Tasks — Sistema de Controle de Obrigações (MVP)

**Fontes:** `docs/PRD.md` v1.0 · `docs/plan.md` (marcos M0–M7, decisões D1–D10 e R1–R9, perguntas Q1–Q8)
**Data:** 29/09/2026

---

## 1. Como usar este arquivo

### 1.1 Formato de cada tarefa

```
### T-Mx-NN — Título
- [ ] Concluída
- **Depende de:** tarefas que precisam estar [x] antes de começar
- **Ref:** seções do plano (plan) e do PRD (§), critérios de aceite (CA) e regras (RN)
- **Arquivos:** onde o trabalho acontece (indicativo)
- **Fazer:** escopo objetivo
- **Aceite:** o que precisa ser verdade/testado para marcar [x]
```

### 1.2 Regras de execução

1. **Uma tarefa = um commit ou PR.** A mensagem começa com o ID (ex.: `T-M1-08: API de empresas`).
2. **Só inicie uma tarefa com todas as dependências marcadas `[x]`.**
3. **Regra de negócio fica no backend** [PRD §43]. O frontend só exibe derivados calculados pela API.
4. **Todo CA do PRD vira teste com o ID no nome**, por exemplo `TestCA010_ContabilDependency` ou `it("CA-003 copia CNPJ só com dígitos")`.
5. **Mudou o contrato HTTP? Atualize `backend/api/openapi.yaml` e regenere os tipos (`make gen`)** na mesma tarefa.
6. Valores dependentes de T-000 ainda sem resposta entram como **provisórios**, com o comentário `-- A VALIDAR (Qn)` no seed e uma linha em `docs/pendencias.md`.

### 1.3 Definition of Done (toda tarefa)

- [ ] Código, migrations e seeds da tarefa commitados.
- [ ] Testes da tarefa escritos e passando (`make test`).
- [ ] Lint limpo (`golangci-lint`, `eslint`, `tsc --noEmit`).
- [ ] OpenAPI e tipos gerados atualizados, quando aplicável.
- [ ] Checkbox da tarefa marcado neste arquivo.

---

## 2. Bloqueios externos

### T-000 — Validar decisões pendentes com o escritório
- [ ] Concluída
- **Depende de:** —
- **Ref:** plan §9 (Q1–Q8), §2 (decisões marcadas "validar")
- **Fazer:** obter as respostas e registrá-las em `docs/decisoes.md`.

| Pergunta | Assunto | Desbloqueia (valor definitivo) |
|---|---|---|
| Q1 | Regra de vencimento de cada obrigação | T-M1-06 (seed de `due_date_rules`) |
| Q2 | Coluna Fechamento fora dos percentuais; "Pronto para fechar" só considera os controles (R2/R3) | T-M2-07 |
| Q3 | ICMS Simples não bloqueia "Pronto para fechar" (R4) | T-M2-07 |
| Q4 | Módulo fechado bloqueia edição ou só alerta (R9) | T-M2-08 |
| Q5 | Empresa "Sem folha": `Sem movimento` × `Não se aplica` por controle (D7) | T-M1-06 |
| Q6 | Matriz declarações × regime | T-M1-06 |
| Q7 | UFs e municípios com feriados locais | T-M1-07 (opcional, pode ir até M7) |
| Q8 | Competência de entrada em produção e competências retroativas | T-M7-06 |

Nenhuma tarefa espera T-000 para **começar**. As afetadas usam valores provisórios (regra 1.2.6) e são revisitadas quando a resposta chegar.

---

## 3. Mapa de dependências e paralelismo

```text
M0 ──► M1 ──► M2 ──► M3 ──► M4 ──┐
                 │                 ├──► M6 ──► M7
                 └──► M5 (APIs) ───┘
```

- **Dentro de cada marco:** a trilha backend (API/SQL) e a trilha frontend correm em paralelo depois que o contrato OpenAPI da tarefa backend está definido.
- **M5 × M4:** as APIs do dashboard e do backlog (T-M5-01, 03, 04) dependem só de M2 e podem correr junto com M4.
- **M6:** a base de PDF (T-M6-06) e o template de importação (T-M6-01) podem começar cedo; o restante espera M4/M5.
- **Caminho crítico:** M0-03 → M0-04 → M1-01 → M1-02 → M1-04 → M1-05 → M1-06 → M1-10 → M2-03 → M2-04 → M2-07 → M2-08 → M2-12 → M3-07 → M3-09 → M4-01 → M4-06 → M7-01 → M7-07.

**Totais:** 1 bloqueio externo + 84 tarefas (M0: 8 · M1: 17 · M2: 14 · M3: 10 · M4: 11 · M5: 9 · M6: 8 · M7: 7).

---

## M0 — Infraestrutura do repositório

### T-M0-01 — Estrutura do monorepo
- [x] Concluída
- **Depende de:** —
- **Ref:** plan §3, M0
- **Arquivos:** `/backend`, `/frontend`, `/e2e`, `Makefile`, `.gitignore`, `.editorconfig`, `README.md`
- **Fazer:**
  - criar as pastas raiz e o `Makefile` com os alvos `help`, `tools`, `dev`, `db-up`, `db-down`, `db-reset`, `db-psql`, `test`, `lint`, `e2e`, `migrate`, `migrate-down`, `seed`, `seed-demo`, `gen` (inicialmente como placeholders);
  - `.gitignore` para Go, Node, `.env` e `storage/`;
  - README com propósito, pré-requisitos (Go 1.23+, Node 22+, Docker) e comandos.
- **Aceite:** `make help` lista os alvos; o repositório não versiona artefatos de build.

### T-M0-02 — Docker Compose de desenvolvimento (banco de dados)
- [x] Concluída
- **Depende de:** T-M0-01
- **Ref:** plan M0, D10 · PRD §42.3, §45.2, §45.3
- **Arquivos:** `docker-compose.yml`, `docker/postgres/init/01-databases.sql`, `.env.example`, `Makefile`
- **Fazer:**
  - Serviço `db`:
    - imagem oficial `postgres:16` fixada em versão menor (ex.: `postgres:16.x`, baseada em Debian e não em alpine, por causa das collations);
    - volume nomeado `pgdata`;
    - porta do host configurável `${POSTGRES_PORT:-5432}:5432`;
    - `POSTGRES_INITDB_ARGS="--encoding=UTF8 --locale-provider=icu --icu-locale=pt-BR"`, para ordenar razão social com acentos corretamente;
    - `TZ=UTC` e `PGTZ=UTC` (timestamps em UTC [§45.2]);
    - healthcheck `pg_isready -U $POSTGRES_USER -d $POSTGRES_DB` com intervalo curto e retries;
    - `restart: unless-stopped`.
  - Script de init montado em `/docker-entrypoint-initdb.d`, que cria os bancos `controle` (dev), `controle_e2e` (E2E, T-M0-07/T-M7-03) e `controle_perf` (carga, T-M3-10). Documentar que o init só roda com volume vazio: para recriar, `make db-reset`.
  - `.env.example`:
    - `POSTGRES_USER`, `POSTGRES_PASSWORD=dev-only-change-me`, `POSTGRES_DB=controle`, `POSTGRES_PORT`;
    - `DATABASE_URL` para uso no host (`localhost:${POSTGRES_PORT}`), com comentário de que dentro do compose o host é `db`;
    - `APP_TIMEZONE=America/Sao_Paulo`, `CORS_ORIGINS`, `FILE_STORAGE_DIR=./storage`, `HTTP_PORT`.
  - Alvos do `Makefile`:
    - `db-up`: sobe só o banco e espera healthy;
    - `db-down`;
    - `db-reset`: `down -v` + `up` + espera healthy; apaga os dados locais e avisa isso na saída;
    - `db-psql`: abre `psql` no container;
    - `db-logs`.
- **Aceite:**
  - `make db-up` deixa o `db` healthy;
  - `docker compose exec db psql -U $POSTGRES_USER -lqt` lista `controle`, `controle_e2e` e `controle_perf`;
  - `SHOW timezone` retorna `UTC`;
  - `SELECT x FROM (VALUES ('Épsilon'),('Eta'),('Íris'),('Iota')) t(x) ORDER BY x` retorna a ordem alfabética pt-BR (`Épsilon`, `Eta`, `Iota`, `Íris`);
  - `.env` está no `.gitignore` e nenhum segredo real foi versionado.

### T-M0-03 — Esqueleto da API Go
- [x] Concluída
- **Depende de:** T-M0-01, T-M0-02
- **Ref:** plan §3, M0 · PRD §42.2, §45.3
- **Arquivos:** `backend/cmd/api/main.go`, `backend/internal/platform/{config,db,http,clock}`
- **Fazer:**
  - `config` a partir do ambiente, com validação na carga;
  - `pgxpool`;
  - router `chi` com prefixo `/api/v1`;
  - `GET /api/v1/health` (checa o DB);
  - middlewares de request-id, recover, log `slog` (sem corpo de requisição) e CORS com origens explícitas;
  - envelope de erro padrão `{code, message, details}` com helpers `400/404/409/422/423`;
  - interface `Clock` com fuso configurável (D10), para testes determinísticos;
  - pool `pgxpool` com `timezone=UTC` na conexão e tamanho configurável (`DB_MAX_CONNS`, padrão 10);
  - serviço `api` no `docker-compose.yml`:
    - imagem `golang:1.23` com hot reload via `air`;
    - código montado como volume;
    - `depends_on: db (condition: service_healthy)`;
    - `DATABASE_URL` com host `db`;
    - `./storage` montado em `FILE_STORAGE_DIR`.
- **Aceite:** `go run ./cmd/api` (no host, com `make db-up`) e `docker compose up api` respondem 200 em `/health`; `/health` retorna 503 com o banco parado; teste unitário do envelope de erro; CORS nega origem não listada.

### T-M0-04 — Migrations e sqlc
- [x] Concluída
- **Depende de:** T-M0-02, T-M0-03
- **Ref:** plan §3, §4 · PRD §42.2
- **Arquivos:** `backend/migrations/`, `backend/queries/`, `backend/sqlc.yaml`, `Makefile`
- **Fazer:**
  - `goose` com migrations SQL (up/down) e primeira migration com extensões `pgcrypto`, `btree_gist`, `pg_trgm`;
  - migrations embutidas no binário (`embed.FS` + goose como biblioteca), com `MIGRATE_ON_BOOT` (padrão `false` em dev, `true` em prod, usado em T-M7-06) e comando `api migrate up|down|status`;
  - `sqlc` configurado para `pgx/v5`;
  - alvos `make tools` (instala `goose`, `sqlc`, `golangci-lint` e `air` com versões fixadas), `make migrate`, `make migrate-down`, `make migrate-status` e `make gen` (sqlc);
  - os alvos `migrate*` aceitam `DB=e2e|perf` para apontar para os outros bancos locais.
- **Aceite:** `make migrate` aplica e `make migrate-down` reverte sem erro no banco `controle`; `make migrate DB=e2e` funciona; `make gen` gera código compilável.

### T-M0-05 — Esqueleto da SPA
- [x] Concluída
- **Depende de:** T-M0-02, T-M0-03
- **Ref:** plan §3 · PRD §6, §42.1, §44
- **Arquivos:** `frontend/` (Vite, `src/app`, `src/components`, `src/features/*`, `src/lib`, `src/services`, `src/types`)
- **Fazer:**
  - Vite + React + TS (strict), Tailwind, shadcn/ui, React Router e TanStack Query;
  - layout com menu lateral das 12 áreas do §6, cada rota com página placeholder;
  - formatadores em `src/lib/format.ts` (datas pt-BR, competência `MM/AAAA`);
  - serviço `web` no `docker-compose.yml`:
    - imagem `node:22`;
    - Vite em `0.0.0.0` com file watching por polling (necessário em bind mount no macOS);
    - `node_modules` em volume nomeado, não montado do host;
    - proxy `/api` → `api:${HTTP_PORT}`;
  - `make dev` = `docker compose up` (db + api + web).
- **Aceite:** `make dev` sobe os 3 serviços; a SPA em `http://localhost:5173` navega entre as 12 áreas e o health da API é visível via proxy; `tsc --noEmit` e `eslint` limpos.

### T-M0-06 — Contrato OpenAPI e cliente HTTP
- [x] Concluída
- **Depende de:** T-M0-03, T-M0-05
- **Ref:** plan §3 (Contrato)
- **Arquivos:** `backend/api/openapi.yaml`, `frontend/src/types/api.d.ts`, `frontend/src/lib/api.ts`
- **Fazer:**
  - OpenAPI 3.1 com `health`, o schema de erro e componentes comuns (`Competence` `AAAA-MM`, `Cnpj`, `Version`);
  - `openapi-typescript` no `make gen`;
  - cliente `fetch` tipado que converte o envelope de erro em exceção tipada.
- **Aceite:** o frontend chama `/health` com tipos gerados; alterar o YAML e rodar `make gen` atualiza os tipos.

### T-M0-07 — Harness de testes
- [x] Concluída
- **Depende de:** T-M0-04, T-M0-05
- **Ref:** plan §6
- **Arquivos:** `backend/internal/testutil/`, `frontend/vitest.config.ts`, `e2e/playwright.config.ts`
- **Fazer:**
  - helper Go que sobe Postgres com `testcontainers-go`, aplica migrations e oferece `Truncate`/`WithTx`;
  - helper de servidor HTTP de teste e `Clock` fixo;
  - testcontainers usa a mesma imagem `postgres:16.x` do compose e os mesmos `INITDB_ARGS`; README com a nota de compatibilidade (Docker Desktop; no Colima, configurar `DOCKER_HOST` e `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE`);
  - Vitest + Testing Library;
  - Playwright contra uma instância da API apontada para o banco **`controle_e2e`** (nunca para o banco `controle` de dev); `make e2e` faz reset, migrate e seed desse banco antes da suíte.
- **Aceite:** um teste de integração exemplo (health + DB), um de componente exemplo e um E2E exemplo passam localmente; rodar `make e2e` não altera os dados do banco `controle`.

### T-M0-08 — CI
- [x] Concluída
- **Depende de:** T-M0-07
- **Ref:** plan M0
- **Arquivos:** `.github/workflows/ci.yml`
- **Fazer:** jobs de lint (golangci-lint, eslint, tsc), testes Go com Docker, Vitest e build do frontend, com cache de módulos Go e npm.
- **Aceite:** o workflow passa em um PR trivial.

---

## M1 — Fundação de domínio

### T-M1-01 — Usuários, usuário implícito e responsáveis
- [x] Concluída
- **Depende de:** T-M0-04, T-M0-06
- **Ref:** plan D9 · PRD §34, §38.20, §7.1
- **Arquivos:** `backend/migrations/0002_users.sql`, `backend/internal/user/`
- **Fazer:**
  - tabela `users` (`id`, `name`, `email NULL`, `role`, `active`) com seed `Administrador` / `ADMIN` e UUID fixo;
  - middleware que injeta o `user_id` do Administrador no contexto, mais um helper `user.FromContext`;
  - `GET/POST/PATCH /users`: cadastro de responsáveis, sem login.
- **Aceite:** toda requisição tem usuário no contexto (teste); responsáveis podem ser criados e listados; não há endpoint de delete.

### T-M1-02 — Migrations de empresas e configurações com vigência
- [x] Concluída
- **Depende de:** T-M1-01
- **Ref:** plan §4 (2–4), D8 · PRD §7, §8, §9, §38.2–§38.4, §38.10
- **Arquivos:** `backend/migrations/0003_companies.sql`, `0004_company_settings.sql`
- **Fazer:**
  - `companies` com todos os campos do §38.2, `version`, `payroll_type` (enum `NO_PAYROLL`/`PRO_LABORE`/`PAYROLL`) e `factor_r`; UNIQUE em `cnpj` e `internal_code`; FKs de responsáveis → `users`;
  - `company_tax_regime_history` com competência como `DATE` (dia 1) e `EXCLUDE USING gist (company_id WITH =, daterange(...) WITH &&)`;
  - `company_module_settings` e `company_obligation_overrides` com vigência (o FK para `obligation_types` entra em T-M1-04);
  - FKs com `ON DELETE RESTRICT` [RN-008].
- **Aceite:** um teste de integração prova que inserir regimes sobrepostos falha e que CNPJ/código duplicados falham.

### T-M1-03 — Utilitários de competência e CNPJ
- [x] Concluída
- **Depende de:** T-M0-03
- **Ref:** PRD §5.3, §7.3, §53.1 (sanitização) · RN-014
- **Arquivos:** `backend/internal/platform/competence/`, `backend/internal/platform/cnpj/`, `frontend/src/lib/{competence,cnpj}.ts`
- **Fazer:**
  - Go: `Competence` (parse `AAAA-MM`, `String()`, `Display()` `MM/AAAA`, `Next`, `Prev`, `FirstDay`, `FromDate`, `Current(clock)`);
  - Go: `cnpj.Sanitize`, `cnpj.Valid` (dígitos verificadores) e `cnpj.Format`;
  - TS: os mesmos formatadores e sanitizador.
- **Aceite:** testes table-driven cobrindo virada de ano, `00.000.001/0001-01` → `00000001000101`, CNPJ inválido e CNPJ com espaços/pontuação.

### T-M1-04 — Migrations de catálogo, calendário, credenciais e settings
- [x] Concluída
- **Depende de:** T-M1-02
- **Ref:** plan §4 (5–7, 12), D2, D3, D7 · PRD §11, §20, §21, §38.8–§38.9, §38.13–§38.19
- **Arquivos:** `backend/migrations/0005_status.sql`, `0006_calendar.sql`, `0007_catalog.sql`, `0008_credentials_settings.sql`
- **Fazer:**
  - `status_sets` e `status_definitions` (`counts_as_started`, `counts_as_operational_progress`, `is_final`, `is_blocking`, `sort_order`);
  - `business_calendars`, `holidays` e `due_date_rules` (`month_offset`, `base_day`, `day_type`, `non_business_day_adjustment`, `calendar_id`);
  - `obligation_types` (+ `initial_status_code`, `status_when_no_payroll`, `periodicity` fixo `MONTHLY`), `obligation_display_contexts`, `obligation_type_regime_rules` (tipo × regime → `default_applicable`, `counts_in_progress`) e `obligation_dependencies` (HARD/SOFT);
  - FK de `company_obligation_overrides` → `obligation_types`;
  - `credential_controls` (UNIQUE `company_id + type`, `version`) e `settings` (chave/valor JSON: faixas de validade, fuso).
- **Aceite:** migrations sobem e descem; constraints testadas (tipo com status set inexistente falha).

### T-M1-05 — Seed dos conjuntos de status
- [x] Concluída
- **Depende de:** T-M1-04
- **Ref:** PRD §11.1–§11.5, §38.13–§38.14
- **Arquivos:** `backend/seed/status.sql` (ou `seed/*.go`), `make seed`
- **Fazer:**
  - `FISCAL_CLOSING`, `FISCAL_ITEM`, `DECLARATION`, `ACCOUNTING`, `PAYROLL` com códigos, labels, flags e ordem exatamente como nas tabelas do PRD;
  - `NOT_APPLICABLE` em `FISCAL_ITEM` fora do denominador;
  - `is_blocking` em `MISSING_DOCUMENTS`, `ERROR` e `PENDING`;
  - seed idempotente (`ON CONFLICT DO UPDATE`).
- **Aceite:** um teste compara o seed com as tabelas do §11; rodar o seed 2× não duplica.

### T-M1-06 — Seed do catálogo de obrigações
- [x] Concluída
- **Depende de:** T-M1-05 · provisório até T-000 (Q1, Q5, Q6)
- **Ref:** plan D1, D2, D3, D5, D7 · PRD §13–§18, §50, §38.9, §38.15
- **Arquivos:** `backend/seed/catalog.sql`
- **Fazer:**
  - tipos do §50 + `FISCAL_FECHAMENTO` (D1), com `primary_module`, status set, `sort_order`, `allows_protocol`/`allows_transmission_date` (Declarações) e `initial_status_code`;
  - `obligation_type_regime_rules`:
    - os 12 controles fiscais valem para os regimes Normais;
    - no Simples valem Entradas, Saídas, Prestados, Tomados, Simples Nacional, ISSQN, ICMS (`counts_in_progress=false`) e Retidos;
    - Declarações seguem Q6 (provisório);
    - Folha e Contábil valem para todos;
  - display contexts: `MIT` → DECLARATIONS/DCTFWEB; `DCTFWEB` → DECLARATIONS/DCTFWEB; `ESOCIAL` → PAYROLL/DCTFWEB; `REINF_RETIDOS`/`REINF_LUCROS` → DCTFWEB com `primary_module = DECLARATIONS` (D5);
  - dependências: cadeia HARD das 6 etapas contábeis; SOFT de `DCTFWEB` sobre MIT, REINF_RETIDOS, REINF_LUCROS e ESOCIAL;
  - `status_when_no_payroll` para os 4 controles de Folha (Q5, provisório);
  - `due_date_rules` provisórias por obrigação (Q1), marcadas `A VALIDAR`.
- **Aceite:** um teste verifica que o Simples tem 8 controles fiscais com 7 no progresso [§14.4]; `ESOCIAL` tem 2 contextos; a cadeia contábil tem 5 arestas HARD.

### T-M1-07 — Seed de feriados nacionais
- [x] Concluída
- **Depende de:** T-M1-04 · Q7 opcional
- **Ref:** PRD §21.2, §38.17–§38.18
- **Arquivos:** `backend/internal/calendar/holidays.go`, `backend/seed/calendar.go`
- **Fazer:**
  - calendário `Nacional`;
  - feriados fixos + móveis (Carnaval seg/ter, Sexta-feira Santa, Corpus Christi) calculados pela Páscoa (algoritmo de Meeus) para 2025–2030;
  - estrutura para calendários de UF e município (vazios até Q7).
- **Aceite:** teste unitário da Páscoa (2025-04-20, 2026-04-05, 2027-03-28) e contagem de feriados por ano.

### T-M1-08 — API de empresas
- [x] Concluída
- **Depende de:** T-M1-02, T-M1-03
- **Ref:** plan M1 · PRD §7, §26.5, §41.1 · CA-001, CA-002 · RN-001, RN-002, RN-008
- **Arquivos:** `backend/internal/company/{handler,service,repo}.go`, `backend/queries/companies.sql`, `openapi.yaml`
- **Fazer:**
  - `GET /companies` com filtros `q`, `cnpj`, `code`, `tax_regime`, `active` e `module`; `q` casa nome, razão, fantasia, código e CNPJ com ou sem máscara (índice `pg_trgm`);
  - `POST /companies` exige o regime inicial (vigência = competência de `service_start_date`); o CNPJ é sanitizado e validado;
  - `GET /companies/{id}`;
  - `PATCH /companies/{id}` com `version` → `409` se divergente;
  - `POST /companies/{id}/deactivate`; sem DELETE;
  - conflito de CNPJ/código → `409` com `details.conflicting_company {id, internal_code, legal_name}`.
- **Aceite:** `TestCA001_CreateCompany`, `TestCA002_DuplicateCnpj`; busca por `00.000.001/0001-01` e por `00000001000101` retorna a mesma empresa.

### T-M1-09 — API de regime tributário
- [x] Concluída
- **Depende de:** T-M1-08
- **Ref:** PRD §8, §41.2 · UC-04 · RN-003
- **Arquivos:** `backend/internal/company/regime.go`, `backend/queries/regimes.sql`
- **Fazer:**
  - `GET /companies/{id}/tax-regimes` (linha do tempo);
  - `POST` com `{tax_regime, effective_from}` encerra a vigência aberta em `effective_from - 1 mês` na mesma transação e registra `created_by`;
  - rejeita vigência anterior ou igual ao início da vigência aberta atual (`422`);
  - função `RegimeAt(companyID, competence)` para uso no motor.
- **Aceite:** testes da troca Simples → Presumido (exemplo do §8.2) e de `RegimeAt` antes e depois da troca.

### T-M1-10 — API de módulos e overrides de aplicabilidade
- [x] Concluída
- **Depende de:** T-M1-08, T-M1-06
- **Ref:** plan R6 · PRD §7.4, §9, §41.3–§41.4 · RN-015
- **Arquivos:** `backend/internal/company/{modules,overrides}.go`
- **Fazer:**
  - `GET /companies/{id}/modules` e `PUT /companies/{id}/modules/{module}` com `{enabled, effective_from}`; o default sugerido é a próxima competência não gerada;
  - `GET /companies/{id}/obligation-overrides` e `PUT …/{obligationTypeId}` com `{applicability: DEFAULT|APPLICABLE|NOT_APPLICABLE, effective_from}`;
  - fechar a vigência anterior ao abrir uma nova;
  - funções `ModulesAt(company, competence)` e `OverridesAt(company, competence)`.
- **Aceite:** testes de vigência (override de 10/2026 não aparece em 09/2026) e de persistência da dispensa nas competências seguintes.

### T-M1-11 — API de catálogo, calendário e settings
- [x] Concluída
- **Depende de:** T-M1-06, T-M1-07
- **Ref:** PRD §20, §21, §19.3, §41.10
- **Arquivos:** `backend/internal/catalog/`, `backend/internal/calendar/handler.go`, `backend/internal/platform/settings/`
- **Fazer:**
  - `GET/POST/PATCH /obligation-types` (inclui contextos, regras por regime e dependências);
  - `GET /status-sets`;
  - `GET/POST/PATCH /due-date-rules`;
  - `GET/POST/DELETE /holidays` (feriados estaduais e municipais custom) e `GET /calendars`;
  - `GET/PUT /settings/credential-thresholds` (faixas 7/15/30/60 configuráveis).
- **Aceite:** criar um tipo de declaração novo via API faz ele aparecer no catálogo sem migration [§15.2]; testes de validação de payload.

### T-M1-12 — Seed das 20 empresas fictícias
- [x] Concluída
- **Depende de:** T-M1-09, T-M1-10
- **Ref:** PRD §37 · plan M1 (seeds)
- **Arquivos:** `backend/seed/companies.go`
- **Fazer:**
  - as 10 empresas Normais e as 10 do Simples do §37, com código, razão, CNPJ e regime;
  - variar município/UF, módulos ativos (algumas sem Folha ou Contábil), overrides (ex.: 102 sem IPI, 106 sem ICMS), `payroll_type` (Sem folha, Pró-labore, Folha), `factor_r` em parte do Simples e responsáveis (criar 3–4 responsáveis fictícios);
  - uma empresa (ex.: 205) com troca de regime histórica;
  - `service_start_date` = 01/2025;
  - comando `make seed-demo` idempotente.
- **Aceite:** após o seed, `GET /companies` retorna 20; os CNPJs do seed são armazenados com 14 dígitos. Os CNPJs fictícios do PRD podem não ter DV válido: o seed usa um bypass explícito e documentado, sem afrouxar a validação da API.

### T-M1-13 — FE: listagem de empresas e CopyButton
- [x] Concluída
- **Depende de:** T-M1-08, T-M0-06
- **Ref:** PRD §7.3, §27.1–§27.2, §26.5 · CA-003 · RN-014
- **Arquivos:** `frontend/src/features/companies/CompaniesPage.tsx`, `frontend/src/components/CopyButton.tsx`, `frontend/src/services/companies.ts`
- **Fazer:**
  - tabela com as colunas do §27.1, busca global com debounce e filtros (regime, situação, módulo);
  - ações rápidas: abrir ficha, copiar código, copiar CNPJ sem máscara, editar e inativar;
  - `CopyButton` com feedback acessível ("Copiado").
- **Aceite:** teste de componente `CA-003`: o clipboard recebe `00000001000101`; a busca por CNPJ formatado e sem formatação funciona.

### T-M1-14 — FE: formulário de empresa e inativação
- [x] Concluída
- **Depende de:** T-M1-13
- **Ref:** PRD §7.1–§7.2, §7.5 · CA-001, CA-002
- **Arquivos:** `frontend/src/features/companies/CompanyForm.tsx`, `schemas.ts`
- **Fazer:**
  - React Hook Form + Zod com todos os campos obrigatórios do §7.1, regime inicial, período de atendimento e máscara de CNPJ;
  - erro `409` exibe a empresa conflitante com link;
  - inativação com confirmação;
  - envio de `version` no PATCH e tratamento de `409` de versão.
- **Aceite:** teste de componente de validação e de exibição do conflito de CNPJ.

### T-M1-15 — FE: ficha da empresa com abas
- [x] Concluída
- **Depende de:** T-M1-14, T-M1-09, T-M1-10
- **Ref:** PRD §27.3, §7.3, §8, §9, §17.1–§17.2, §34.4
- **Arquivos:** `frontend/src/features/companies/detail/*`
- **Fazer:**
  - cabeçalho com Copiar CNPJ e Copiar código;
  - abas: Dados gerais; Regime (linha do tempo + incluir novo regime); Módulos (toggle + vigência); Obrigações/aplicabilidade (Padrão / Aplicável / Não se aplica por tipo + vigência); Folha (tipo de folha, fator R só no Simples); Responsáveis;
  - Procurações/Certificados e Histórico de competências ficam como placeholders (entram em T-M4-11 e T-M5-08).
- **Aceite:** teste E2E curto: trocar o regime com vigência futura e ver a linha do tempo atualizada.

### T-M1-16 — FE: Configurações
- [x] Concluída
- **Depende de:** T-M1-11
- **Ref:** PRD §20, §21, §19.3, §6 (item 10)
- **Arquivos:** `frontend/src/features/settings/*`
- **Fazer:** telas de catálogo de obrigações (lista + edição de flags, contextos e dependências), regras de vencimento, feriados (lista por calendário + inclusão), faixas de validade de credenciais e responsáveis (usuários sem login).
- **Aceite:** editar uma regra de vencimento e reler o valor atualizado; teste de componente do formulário de regra.

### T-M1-17 — FE: estado vazio sem empresas
- [x] Concluída
- **Depende de:** T-M1-13
- **Ref:** PRD §49.1
- **Arquivos:** `frontend/src/components/EmptyState.tsx`, `CompaniesPage.tsx`
- **Fazer:**
  - CTAs Cadastrar empresa, Importar planilha (desabilitado até T-M6-04) e Carregar dados de demonstração;
  - endpoint `POST /demo/seed`, disponível apenas quando `APP_ENV != production`.
- **Aceite:** com o banco vazio, o CTA de demonstração carrega as 20 empresas.

---

## M2 — Motor operacional

### T-M2-01 — Motor de vencimento
- [x] Concluída
- **Depende de:** T-M1-03, T-M1-07
- **Ref:** PRD §21.1–§21.2, §38.16 · §53.1 (vencimento, dia útil)
- **Arquivos:** `backend/internal/calendar/duedate.go`, `duedate_test.go`
- **Fazer:**
  - `DueDate(rule, competence, cal) (time.Time, error)` com `month_offset`, `base_day`, `day_type` (CALENDAR: dia N, limitado ao último dia do mês; BUSINESS: N-ésimo dia útil) e ajuste (`PREVIOUS`/`NEXT`/`NONE`);
  - `Calendar` como união de nacional + UF + município da empresa;
  - `IsBusinessDay`.
- **Aceite:** testes table-driven cobrindo dia 31 em fevereiro, 15º dia útil, feriado móvel, sábado com PREVIOUS e NEXT, virada de ano e `NONE`.

### T-M2-02 — Migrations de competência, snapshots, períodos, instâncias e histórico
- [x] Concluída
- **Depende de:** T-M1-04
- **Ref:** plan §4 (8–11), D2, D4, D8 · PRD §38.5–§38.7, §38.11–§38.12, §51
- **Arquivos:** `backend/migrations/0009_competencies.sql` … `0012_status_history.sql`
- **Fazer:**
  - `competencies` (UNIQUE `competence_key`) e `company_competence_snapshots` (UNIQUE `company_id + competence_id`; regime, payroll_type, factor_r, módulos ativos em array, responsáveis);
  - `company_module_periods` (UNIQUE `company + competence + module`; state, closed_at/by, general_note, version) e `company_module_period_events`;
  - `obligation_instances` (UNIQUE `company + competence + type`; `applicable`, `counts_in_progress`, `regime_snapshot`, vencimentos original/efetivo, override reason/by/at, transmissão, protocolo, `version`, last_changed_by/at);
  - `obligation_status_history` (`field`, old/new, `change_source`, `override_reason`, `user_id`);
  - índices do §51.
- **Aceite:** migrations sobem e descem; um teste de integração prova que o UNIQUE de instância impede duplicação.

### T-M2-03 — Resolvedor de aplicabilidade (função pura)
- [x] Concluída
- **Depende de:** T-M1-06, T-M1-10
- **Ref:** plan D2, D3, D6, D7 · PRD §5.6, §9, §14.3, §17.4, §39 · RN-010
- **Arquivos:** `backend/internal/competence/resolve.go`, `resolve_test.go`
- **Fazer:** `Resolve(snapshot, catalog, overrides) []PlannedInstance`:
  - considera só os tipos de módulos ativos com regra para o regime (D3);
  - `applicable` resolvido por override > regra padrão; `NOT_APPLICABLE` gera a instância com `applicable=false` (D6);
  - `counts_in_progress` vem da regra tipo × regime (D2);
  - status inicial: `initial_status_code`; com `NO_PAYROLL`, usa `status_when_no_payroll`; se não aplicável e o set tiver `NOT_APPLICABLE`, usa esse código.
- **Aceite:** testes unitários (§53.1 aplicabilidade e ICMS/Simples): o Simples gera ICMS com `counts_in_progress=false`; a empresa com IPI dispensado gera IPI `applicable=false`; Fiscal desativado não gera tipos fiscais; Sem folha aplica o status configurado.

### T-M2-04 — `ensureCurrentCompetence`
- [x] Concluída
- **Depende de:** T-M2-01, T-M2-02, T-M2-03
- **Ref:** plan M2, R5 · PRD §10.1–§10.2, §39 · CA-005, CA-006, CA-007 · RN-004, RN-005
- **Arquivos:** `backend/internal/competence/ensure.go`, `ensure_test.go`
- **Fazer:**
  - uma transação com `pg_advisory_xact_lock(hash(competence_key))`;
  - upsert da competência;
  - seleção das empresas ativas, dentro de `service_start/end`, **sem snapshot** na competência;
  - por empresa: snapshot → `company_module_periods` + evento `OPENED` → `Resolve` → cálculo do vencimento → `INSERT … ON CONFLICT DO NOTHING`;
  - retorno de um resumo (empresas processadas e instâncias criadas).
- **Aceite:** `TestCA005_OnlyActiveInServicePeriod`, `TestCA006_DisabledModuleNoInstances`, `TestCA007_NotApplicableKeepsPercent` (este em conjunto com T-M2-07); rodar 3× sequencialmente e 5× em paralelo produz o mesmo número de linhas; uma empresa criada depois da geração recebe as instâncias na próxima chamada, sem alterar as demais.

### T-M2-05 — Gatilhos e endpoints de competência
- [x] Concluída
- **Depende de:** T-M2-04
- **Ref:** PRD §10.2, §10.7, §41.5 · UC-01
- **Arquivos:** `backend/cmd/api/main.go`, `backend/internal/competence/{scheduler,handler}.go`
- **Fazer:**
  - chamar `ensure` no boot;
  - scheduler diário às 00:05 no fuso configurado, com goroutine e cancelamento no shutdown;
  - `GET /competencies` (lista com contagens), `GET /competencies/latest` e `POST /competencies/ensure-current`;
  - handlers que recebem `?competence=` igual à competência corrente inexistente disparam `ensure`.
- **Aceite:** teste de integração: API iniciada com `Clock` em 2026-10-01 cria 10/2026 sem chamada manual.

### T-M2-06 — Seed de competências históricas de demonstração
- [x] Concluída
- **Depende de:** T-M2-04, T-M1-12
- **Ref:** plan M2 (Pronto quando) · PRD §24.8, §25
- **Arquivos:** `backend/seed/history.go`
- **Fazer:**
  - gerar as 3 competências anteriores à corrente;
  - aplicar status variados, de forma determinística por seed: a maioria concluída, algumas pendentes ou atrasadas, um módulo fechado e reaberto e um override de vencimento, gravando histórico `SYSTEM`.
- **Aceite:** após `make seed-demo`, há backlog não vazio, a comparação com a competência anterior tem dados e existe ao menos um evento `REOPENED`.

### T-M2-07 — Cálculos derivados
- [x] Concluída
- **Depende de:** T-M2-04 · Q2 e Q3 (provisório)
- **Ref:** plan R2, R3, R4, D10 · PRD §12, §10.5, §13.5 · CA-007, CA-012, CA-014 · RN-013
- **Arquivos:** `backend/internal/obligation/derived.go`, `backend/queries/derived.sql` (ou view `v_obligation_instances`)
- **Fazer:**
  - `is_overdue = applicable AND NOT is_final AND due_date_effective < today(tz)`;
  - `is_final`, `is_started` e `is_blocking` via join com `status_definitions`;
  - agregação por empresa × módulo × competência: progresso operacional, conclusão final e `ready_to_close` (denominador = aplicáveis com `counts_in_progress`, sem `FISCAL_FECHAMENTO`);
  - prazo crítico mais próximo da linha (menor vencimento não concluído);
  - o "hoje" vem do `Clock`.
- **Aceite:** `TestCA012_OverdueComputed`, `TestCA014_ReadyToClose`, `TestCA007_NotApplicableKeepsPercent`; teste de ICMS no Simples fora dos percentuais.

### T-M2-08 — Alteração de status individual
- [x] Concluída
- **Depende de:** T-M2-07 · Q4 (provisório)
- **Ref:** plan D8, R9 · PRD §23, §41.7, §49.4
- **Arquivos:** `backend/internal/obligation/{handler,service}.go`
- **Fazer:** `PATCH /obligation-instances/{id}/status` com `{status_code, version, confirm_exception?, exception_reason?}`:
  - valida o código contra o status set do tipo (`422`);
  - `version` divergente → `409`;
  - período do `primary_module` fechado → `423` (R9);
  - grava o histórico (`INLINE`), `last_changed_by/at` e incrementa `version`;
  - retorna a instância com os derivados e o agregado atualizado da linha.
- **Aceite:** testes de status inválido, conflito de versão, período fechado e registro correto no histórico.

### T-M2-09 — Dependências HARD (Contábil)
- [x] Concluída
- **Depende de:** T-M2-08
- **Ref:** PRD §18.2–§18.3, §38.15 · CA-010 · RN-011
- **Arquivos:** `backend/internal/obligation/dependencies.go`
- **Fazer:**
  - ao mudar para status `is_final` diferente de `NO_MOVEMENT`, verificar os pré-requisitos HARD aplicáveis na mesma empresa e competência;
  - pré-requisito satisfeito = `OK` ou `NO_MOVEMENT` (qualquer `is_final`);
  - bloqueio → `422 DEPENDENCY_BLOCKED` com a lista de etapas e a mensagem do §18.3;
  - função pura `CheckHard(...)` reaproveitável no bulk e na grade (flag `blocked_by`).
- **Aceite:** `TestCA010_ContabilDependency`; `Sem movimento` satisfaz a dependência; um pré-requisito não aplicável é ignorado.

### T-M2-10 — Dependências SOFT (DCTFWeb)
- [x] Concluída
- **Depende de:** T-M2-08
- **Ref:** PRD §16.4 · CA-011 · RN-012
- **Arquivos:** `backend/internal/obligation/dependencies.go`
- **Fazer:**
  - ao marcar `DCTFWEB` como final com componentes aplicáveis não finais, sem `confirm_exception`, retornar `409 DEPENDENCY_WARNING` com a lista de pendências e a mensagem ("DCTFWeb possui pré-requisitos pendentes: …");
  - com `confirm_exception=true` + `exception_reason`, aceitar e gravar o `override_reason` no histórico;
  - flag `dependency_status` (apto/pendente + lista) disponível para a grade.
- **Aceite:** `TestCA011_DctfwebWarning`, cobrindo os caminhos de alerta e de exceção registrada.

### T-M2-11 — Detalhes da instância e histórico
- [x] Concluída
- **Depende de:** T-M2-08
- **Ref:** plan R7 · PRD §15.3, §21.3, §23, §41.7
- **Arquivos:** `backend/internal/obligation/details.go`
- **Fazer:**
  - `PATCH /obligation-instances/{id}/details` com `{transmission_date?, protocol?, due_date_effective?, due_date_override_reason?, applicable?, version}`;
  - protocolo e transmissão só se o tipo permitir;
  - mudança de vencimento exige motivo e grava by/at;
  - mudança de aplicabilidade grava histórico;
  - `GET /obligation-instances/{id}/history` com todas as mudanças, mais recentes primeiro.
- **Aceite:** override sem motivo → `422`; o histórico lista a mudança de vencimento e a de status.

### T-M2-12 — Alteração em massa
- [x] Concluída
- **Depende de:** T-M2-09, T-M2-10
- **Ref:** PRD §26.3, §41.7 · CA-016
- **Arquivos:** `backend/internal/obligation/bulk.go`
- **Fazer:**
  - `POST /obligation-instances/bulk-status` com `{instance_ids[] | {competence, obligation_type_id, company_ids[]}, status_code, dry_run?}`;
  - `dry_run` devolve a contagem de afetadas e puladas (usada na prévia);
  - execução em uma transação, pulando instâncias bloqueadas por HARD, em período fechado ou não aplicáveis;
  - SOFT exige `confirm_exception` global;
  - histórico individual `BULK`;
  - resposta `{updated, skipped:[{id, reason}]}`.
- **Aceite:** `TestCA016_BulkUpdate`: 20 selecionadas geram 20 históricos; um item bloqueado é pulado sem abortar o lote.

### T-M2-13 — Fechamento, reabertura e observação geral
- [x] Concluída
- **Depende de:** T-M2-08
- **Ref:** plan R1 · PRD §10.3–§10.6, §13.6, §22, §41.8 · CA-013, CA-015 · RN-006, RN-007, RN-016
- **Arquivos:** `backend/internal/period/`
- **Fazer:**
  - `POST /company-module-periods/{id}/close` com `{version, confirm_set_closing?}`: no Fiscal, se `FISCAL_FECHAMENTO ≠ CLOSED`, sem a confirmação → `409 CLOSING_STATUS_REQUIRED`; com a confirmação, atualiza na mesma transação;
  - `POST …/reopen` com `{reason}` obrigatório;
  - eventos `CLOSED`/`REOPENED` com usuário;
  - `PATCH …/note` (observação geral);
  - `GET …/events`.
- **Aceite:** `TestCA013_IndependentClose` (fechar o Fiscal não altera os outros 3 módulos); `TestCA015_ReopenRequiresReason`.

### T-M2-14 — Teste de integração: mudança de regime futura
- [x] Concluída
- **Depende de:** T-M2-04, T-M1-09
- **Ref:** PRD §8.3 · CA-004 · RN-003, RN-004
- **Arquivos:** `backend/internal/competence/regime_change_test.go`
- **Fazer:**
  - gerar 09/2026 para uma empresa do Simples;
  - registrar Lucro Presumido com vigência 10/2026;
  - gerar 10/2026.
- **Aceite:** `TestCA004_FutureRegimeChange`: as instâncias e o snapshot de 09/2026 ficam intactos (regime e tipos do Simples) e 10/2026 usa o catálogo Normal.

---

## M3 — Grade genérica + Fiscal

### T-M3-01 — Endpoint da grade Fiscal
- [x] Concluída
- **Depende de:** T-M2-07, T-M2-13
- **Ref:** plan M3 · PRD §13.2, §14.1, §26.4, §41.6
- **Arquivos:** `backend/internal/grid/{builder,fiscal}.go`, `backend/queries/grid.sql`
- **Fazer:**
  - construtor genérico `BuildGrid(ctx, module, competence, filters)`, usado por todas as grades;
  - colunas vindas de `obligation_display_contexts` + `sort_order`;
  - linhas com empresa, snapshot, período, nota, progresso, conclusão, prazo crítico, `ready_to_close` e células `{instance_id, status, version, overdue, applicable, due_overridden, has_history, blocked_by, dependency_status}`;
  - `GET /modules/fiscal/grid?competence&group=normal|simples` filtrando pelo `regime_snapshot`;
  - filtros server-side: somente atrasadas, pendentes, não iniciadas, prontas para fechar, aberto/fechado, com/sem observação, regime, responsável e status;
  - no máximo 3 queries por grade (sem N+1).
- **Aceite:** o teste de integração com o seed retorna 10 linhas em cada grupo; o Simples tem 8 colunas de controle + Fechamento.

### T-M3-02 — FE: CompetenceNav
- [x] Concluída
- **Depende de:** T-M0-05, T-M2-05
- **Ref:** PRD §10.7, §24.1
- **Arquivos:** `frontend/src/components/CompetenceNav.tsx`, `frontend/src/lib/useCompetence.ts`
- **Fazer:**
  - `← MM/AAAA | seletor | MM/AAAA →` e botão "Mais recente";
  - competência sincronizada com `?competence=` na URL;
  - default = `GET /competencies/latest`.
- **Aceite:** teste de componente: navegar anterior/próxima atualiza a URL; o seletor direto funciona na virada de ano.

### T-M3-03 — FE: DataGrid reutilizável
- [x] Concluída
- **Depende de:** T-M0-05
- **Ref:** PRD §26.1, §28.2, §45.1, §45.4
- **Arquivos:** `frontend/src/components/DataGrid/*`
- **Fazer:**
  - TanStack Table + TanStack Virtual, com colunas definidas por configuração;
  - cabeçalho e primeira coluna fixos, seleção de linhas (checkbox + shift-click);
  - navegação por teclado entre células (setas; Enter/Espaço abre o editor);
  - slot para células customizadas e estado vazio "Nenhuma obrigação aplicável para os filtros selecionados." [§49.2].
- **Aceite:** teste de componente de teclado e seleção; render de 300 × 20 com dados falsos sem travar (virtualização ativa).

### T-M3-04 — FE: StatusCell
- [x] Concluída
- **Depende de:** T-M3-03, T-M2-08
- **Ref:** PRD §26.2, §28.3, §45.4, §49.3–§49.4
- **Arquivos:** `frontend/src/components/StatusCell.tsx`, `frontend/src/services/obligations.ts`
- **Fazer:**
  - dropdown com os status do set, mostrando label e ícone;
  - indicadores textuais/ícone para atrasado, não aplicável, histórico disponível e prazo alterado, sem depender só de cor;
  - mutation otimista com rollback e toast em erro;
  - `409` de versão recarrega a linha e avisa;
  - `422`/`409` de dependência são repassados para os modais de T-M3-05;
  - célula não aplicável ou bloqueada fica desabilitada com tooltip.
- **Aceite:** testes de componente: rollback em erro de API; indicadores têm `aria-label`.

### T-M3-05 — FE: modais operacionais
- [x] Concluída
- **Depende de:** T-M3-04, T-M2-10, T-M2-11, T-M2-13
- **Ref:** PRD §10.4, §10.6, §13.6, §16.4, §21.3, §23 · CA-011, CA-015
- **Arquivos:** `frontend/src/components/dialogs/*`
- **Fazer:**
  - `HistoryDrawer`;
  - `DueDateOverrideDialog` (motivo obrigatório);
  - `DependencyWarningDialog` (lista de pendências + motivo da exceção);
  - `CloseModuleDialog` (inclui a confirmação do status Fechamento no Fiscal);
  - `ReopenDialog` (motivo obrigatório);
  - `CellDetailsPopover` (vencimento, transmissão, protocolo).
- **Aceite:** testes de componente: não é possível confirmar a reabertura ou a exceção sem motivo.

### T-M3-06 — FE: NoteCell
- [x] Concluída
- **Depende de:** T-M3-03, T-M2-13
- **Ref:** PRD §22 · RN-016
- **Arquivos:** `frontend/src/components/NoteCell.tsx`
- **Fazer:** exibição truncada com tooltip; edição inline (Enter salva, Esc cancela) via `PATCH …/note`; otimista com rollback.
- **Aceite:** teste de componente de edição e de cancelamento.

### T-M3-07 — FE: BulkActionBar
- [x] Concluída
- **Depende de:** T-M3-03, T-M2-12
- **Ref:** PRD §26.3 · CA-016
- **Arquivos:** `frontend/src/components/BulkActionBar.tsx`
- **Fazer:**
  - barra que aparece com linhas selecionadas: escolher coluna/obrigação → status → prévia (`dry_run`: "18 empresas → Retidos → Sem movimento", N puladas com motivo) → confirmar;
  - ao concluir, invalida a grade.
- **Aceite:** teste de componente do fluxo completo com a API mockada.

### T-M3-08 — FE: filtros de grade na URL
- [x] Concluída
- **Depende de:** T-M3-03
- **Ref:** PRD §26.4–§26.5
- **Arquivos:** `frontend/src/components/GridFilters.tsx`, `frontend/src/lib/useGridFilters.ts`
- **Fazer:**
  - todos os filtros do §26.4 serializados na URL;
  - busca livre aplicada no cliente sobre nome, código e CNPJ (com e sem máscara);
  - filtros derivados enviados à API;
  - botão "Limpar filtros".
- **Aceite:** recarregar a página mantém os filtros; testes do hook de serialização.

### T-M3-09 — FE: tela Fiscal
- [x] Concluída
- **Depende de:** T-M3-01, T-M3-02, T-M3-04, T-M3-05, T-M3-06, T-M3-07, T-M3-08
- **Ref:** PRD §13, §14, §28 · UC-02 · CA-014
- **Arquivos:** `frontend/src/features/fiscal/*`
- **Fazer:**
  - topo com competência, busca, filtros, alternador `Normais | Simples Nacional` (na URL), exportar (desabilitado até T-M6-05) e ações em massa;
  - grade com as colunas do §13.2 / §14.1;
  - badge "Pronto para fechar" com o texto do §13.5;
  - botão Fechar/Reabrir por linha;
  - badge de módulo fechado.
- **Aceite:** E2E UC-02: filtrar → alterar status inline → ver o progresso mudar → fechar a empresa pronta.

### T-M3-10 — Gerador de massa e medição da grade
- [x] Concluída
- **Depende de:** T-M3-01
- **Ref:** PRD §45.1 · plan M3/M7
- **Arquivos:** `backend/cmd/loadgen/main.go`, `docs/perf.md`
- **Fazer:**
  - CLI que cria N empresas (default 300) com distribuição realista (≈60% Simples, módulos e overrides variados) e gera K competências com status aleatórios determinísticos (`--seed`), em banco separado (`controle_perf`);
  - benchmark do endpoint de grade Fiscal e do PATCH de status.
- **Aceite:** `docs/perf.md` registra os tempos: grade com 300 linhas ≤ ~2 s ponta a ponta e PATCH p95 < 200 ms. Se não atingir, abrir tarefa de otimização antes de M4.

---

## M4 — Demais grades

### T-M4-01 — Endpoints das grades de Declarações, Folha, DCTFWeb e Contábil
- [x] Concluída
- **Depende de:** T-M3-01
- **Ref:** PRD §29–§32, §41.6 · plan R1
- **Arquivos:** `backend/internal/grid/{declarations,payroll,dctfweb,accounting}.go`
- **Fazer:** reusar `BuildGrid`.
  - **Declarações:** colunas dinâmicas do catálogo, com vencimento, transmissão e protocolo na célula.
  - **Folha:** colunas Tipo de Folha e Fator R (do snapshot; Fator R só no Simples).
  - **DCTFWeb:** instâncias de outros módulos via contexto `DCTFWEB`, coluna `dependencies`, sem período próprio.
  - **Contábil:** ordem sequencial e `blocked_by` por célula.
- **Aceite:** testes de integração: cada grade retorna as colunas esperadas com o seed; a DCTFWeb não expõe fechamento.

### T-M4-02 — FE: tela Declarações
- [x] Concluída
- **Depende de:** T-M4-01, T-M3-09
- **Ref:** PRD §15, §29
- **Arquivos:** `frontend/src/features/declarations/*`
- **Fazer:** grade com colunas dinâmicas; `CellDetailsPopover` com edição de data de transmissão e protocolo; fechar/reabrir por linha.
- **Aceite:** E2E: marcar MIT como Transmitida com protocolo e ver o protocolo no popover após recarregar.

### T-M4-03 — FE: tela Folha
- [x] Concluída
- **Depende de:** T-M4-01, T-M3-09
- **Ref:** PRD §17, §30
- **Arquivos:** `frontend/src/features/payroll/*`
- **Fazer:** colunas do §30; Fator R exibido só no Simples; indicação visual das empresas Sem folha.
- **Aceite:** teste de componente da coluna Fator R condicional; E2E de alteração do eSocial.

### T-M4-04 — FE: tela DCTFWeb
- [x] Concluída
- **Depende de:** T-M4-01, T-M3-09
- **Ref:** PRD §16, §31 · CA-011
- **Arquivos:** `frontend/src/features/dctfweb/*`
- **Fazer:** colunas do §31; coluna Dependências ("Apta" ou "Pendente: eSocial, REINF Retidos"); a conclusão da DCTFWeb passa pelo `DependencyWarningDialog`; sem botão de fechamento.
- **Aceite:** E2E CA-011: com eSocial pendente, concluir a DCTFWeb exibe o alerta; com exceção, o histórico mostra o motivo.

### T-M4-05 — FE: tela Contábil
- [x] Concluída
- **Depende de:** T-M4-01, T-M3-09
- **Ref:** PRD §18, §32 · CA-010
- **Arquivos:** `frontend/src/features/accounting/*`
- **Fazer:** colunas na ordem da sequência com indicação visual de etapa (1→6); a opção "OK" fica desabilitada com tooltip quando `blocked_by` não está vazio; o erro `422` da API também é tratado.
- **Aceite:** E2E CA-010: tentar a Conciliação com a etapa anterior "A iniciar" é bloqueado com a mensagem do §18.3.

### T-M4-06 — Integração: obrigação canônica entre telas
- [x] Concluída
- **Depende de:** T-M4-01
- **Ref:** PRD §16.1, §40 · CA-008, CA-009 · RN-009 · ADR-005
- **Arquivos:** `backend/internal/grid/canonical_test.go`
- **Fazer:**
  - alterar o MIT pela instância lida na grade de Declarações e ler a grade DCTFWeb;
  - o mesmo para o eSocial (Folha → DCTFWeb);
  - query de auditoria garantindo que nenhuma combinação empresa + competência + tipo aparece duplicada.
- **Aceite:** `TestCA008_MitShared` e `TestCA009_EsocialShared` passam; `instance_id` idêntico nas duas grades.

### T-M4-07 — API de Procurações e Certificados
- [x] Concluída
- **Depende de:** T-M1-04, T-M1-11
- **Ref:** PRD §19, §33, §41.11 · UC-05 · RN-017
- **Arquivos:** `backend/internal/credential/{handler,service,validity}.go`
- **Fazer:**
  - `GET /credential-controls` (grade com filtros rápidos: vencidos, 7/15/30/60 dias, aguardando cliente, sem data) e `GET /companies/{id}/credential-controls`;
  - `PUT /companies/{id}/credential-controls/{type}` com `{expires_at, renewal_status, notes, version}`;
  - situação calculada pelas faixas de `settings`;
  - `renewal_status = RENEWED` exige um novo `expires_at` e volta para `CURRENT` [§19.4].
- **Aceite:** testes da função de situação em todas as faixas e "sem data"; Renovado sem data → `422`.

### T-M4-08 — Upload seguro de documentos
- [x] Concluída
- **Depende de:** T-M4-07
- **Ref:** PRD §19.1–§19.2, §45.3 · CA-020 · RN-018
- **Arquivos:** `backend/internal/credential/{upload,validate}.go`, `validate_test.go`
- **Fazer:** `PUT …/{type}/file` (multipart, limite de 10 MB) e `DELETE …/file`.
  - **Procuração:** aceita PDF, PNG e JPEG por magic bytes.
  - **Certificado:**
    - recusa `.pfx`/`.p12` por extensão e por assinatura PKCS#12;
    - `.pem`/`.crt`/`.cer`: parse PEM aceitando só blocos `CERTIFICATE`, recusando qualquer `*PRIVATE KEY*`;
    - DER aceito só se `x509.ParseCertificate` tiver sucesso.
  - Armazenamento em `FILE_STORAGE_DIR/<uuid>`; substituição remove o arquivo anterior; logs sem conteúdo.
  - Mensagem de recusa: "Chaves privadas não são armazenadas pelo sistema."
- **Aceite:** `TestCA020_RejectPrivateKey` com fixtures de `.pfx`, `.p12` renomeado para `.cer`, PEM só com chave, PEM com certificado + chave, `.cer` válido (aceito) e PDF válido (aceito).

### T-M4-09 — Seed de credenciais
- [x] Concluída
- **Depende de:** T-M4-07, T-M1-12
- **Ref:** PRD §19, §37
- **Arquivos:** `backend/seed/credentials.go`
- **Fazer:** para as 20 empresas, datas de vencimento relativas ao `Clock` cobrindo todas as faixas (vencido, 7, 15, 30, 60, vigente, sem data) e etapas de renovação variadas.
- **Aceite:** cada filtro rápido do §33 retorna ao menos uma empresa no seed.

### T-M4-10 — FE: tela Procurações e Certificados
- [x] Concluída
- **Depende de:** T-M4-08, T-M3-03
- **Ref:** PRD §33 · UC-05 · CA-019, CA-020
- **Arquivos:** `frontend/src/features/credentials/*`
- **Fazer:**
  - grade com as colunas do §33 e chips de filtro rápido;
  - edição inline de vencimento e etapa; Renovado abre um diálogo pedindo a nova validade;
  - upload/substituição/remoção com a mensagem de recusa do backend.
- **Aceite:** E2E UC-05 completo; E2E de upload de `.pfx` recusado.

### T-M4-11 — FE: aba de credenciais na ficha da empresa
- [x] Concluída
- **Depende de:** T-M4-10, T-M1-15
- **Ref:** PRD §27.3 (aba 7)
- **Arquivos:** `frontend/src/features/companies/detail/CredentialsTab.tsx`
- **Fazer:** reusar os componentes de T-M4-10 para uma empresa.
- **Aceite:** editar a validade na ficha reflete na tela geral.

---

## M5 — Gestão

### T-M5-01 — API do dashboard: KPIs e cards
- [x] Concluída
- **Depende de:** T-M2-07
- **Ref:** PRD §24.1–§24.5, §41.9
- **Arquivos:** `backend/internal/dashboard/{handler,kpis}.go`, `backend/queries/dashboard.sql`
- **Fazer:** `GET /dashboard?competence&regime_group&regime&responsible`:
  - KPIs do §24.2 (total aplicáveis, concluídas, em andamento, não iniciadas, pendentes, atrasadas, % operacional, % conclusão, empresas prontas para fechar, empresas com competências anteriores abertas);
  - cards por módulo (Fiscal, Declarações, Folha, Contábil) e card de vencimentos de credenciais;
  - progresso por responsável (do snapshot);
  - cada número acompanha um `drilldown` (rota + filtros).
- **Aceite:** teste de integração com números conferidos contra o seed determinístico.

### T-M5-02 — API "Exige atenção"
- [x] Concluída
- **Depende de:** T-M5-01, T-M4-07
- **Ref:** PRD §24.6 · CA-019
- **Arquivos:** `backend/internal/dashboard/attention.go`
- **Fazer:** `GET /dashboard/attention?competence` com as 11 categorias do §24.6 (atrasadas, pendências, documentos faltantes, erros, certificados vencidos e a vencer, procurações vencidas e a vencer, competências anteriores abertas, dependências bloqueando DCTFWeb e Contábil), cada uma com contagem, amostra (top 5) e drill-down.
- **Aceite:** `TestCA019_ExpiringCertificateInAttention`; cada categoria tem ao menos um caso no seed.

### T-M5-03 — API de comparação histórica
- [x] Concluída
- **Depende de:** T-M5-01
- **Ref:** PRD §24.8
- **Arquivos:** `backend/internal/dashboard/history.go`
- **Fazer:** `GET /dashboard/history?competence` com a competência selecionada × a anterior: % de conclusão, Δ, atrasos e pendências (opcional: série das últimas 6 competências para o gráfico).
- **Aceite:** teste com as competências históricas do seed.

### T-M5-04 — API de backlog
- [x] Concluída
- **Depende de:** T-M2-07
- **Ref:** plan R8 · PRD §25 · CA-021
- **Arquivos:** `backend/internal/dashboard/backlog.go`
- **Fazer:** `GET /backlog?before=AAAA-MM&company&code&cnpj&module&responsible&status&overdue_min&overdue_max&sort`:
  - instâncias aplicáveis não finais e períodos `OPEN` de competências anteriores;
  - campos do §25.2 com `days_overdue`;
  - ordenação por dias em atraso; paginação.
- **Aceite:** `TestCA021_BacklogShowsPreviousOpen`.

### T-M5-05 — FE: Dashboard
- [x] Concluída
- **Depende de:** T-M5-01, T-M5-03, T-M3-02
- **Ref:** PRD §24 · UC-01, UC-03
- **Arquivos:** `frontend/src/features/dashboard/*`
- **Fazer:**
  - `CompetenceNav` com default na mais recente;
  - linha de KPIs (stat tiles) clicáveis → rota de drill-down com filtros na URL;
  - cards por módulo com barras de progresso, filtro por regime ou grupo, progresso por responsável e comparação com a competência anterior (Recharts).
- **Aceite:** E2E UC-03 (parte 1): clicar em "Atrasadas: N" abre a lista filtrada com N itens.

### T-M5-06 — FE: "Exige atenção"
- [x] Concluída
- **Depende de:** T-M5-02, T-M5-05
- **Ref:** PRD §24.6–§24.7
- **Arquivos:** `frontend/src/features/dashboard/AttentionPanel.tsx`
- **Fazer:** painel com as 11 categorias, contagem, amostra e link "ver todos".
- **Aceite:** teste de componente com categorias vazias ocultas e links corretos.

### T-M5-07 — FE: Backlog e lista de obrigações filtrada
- [x] Concluída
- **Depende de:** T-M5-04, T-M3-03
- **Ref:** PRD §25 · UC-03
- **Arquivos:** `frontend/src/features/backlog/*`
- **Fazer:**
  - tela Backlog com os filtros do §25.3 e ordenação por atraso;
  - clicar no item abre a grade do módulo na competência dele, filtrada na empresa;
  - reusar o componente como "lista filtrada" do drill-down do dashboard.
- **Aceite:** E2E UC-03 completo: dashboard → lista → empresa/módulo → atualizar status.

### T-M5-08 — Histórico de competências da empresa
- [x] Concluída
- **Depende de:** T-M2-13, T-M1-15
- **Ref:** PRD §27.3 (aba 8), §41.1 (`/companies/{id}/history`)
- **Arquivos:** `backend/internal/company/history.go`, `frontend/src/features/companies/detail/HistoryTab.tsx`
- **Fazer:**
  - `GET /companies/{id}/history`: matriz competência × módulo com estado, % de conclusão e eventos;
  - aba na ficha com link para cada grade.
- **Aceite:** a empresa com reabertura no seed mostra os eventos `CLOSED` → `REOPENED`.

### T-M5-09 — Teste de consistência do dashboard
- [x] Concluída
- **Depende de:** T-M5-02, T-M5-04, T-M2-06
- **Ref:** PRD §54 ("dashboard refletir o estado real")
- **Arquivos:** `backend/internal/dashboard/consistency_test.go`
- **Fazer:** para cada KPI, calcular o valor por uma query SQL independente e simples (sem views) e comparar; executar em 2 competências.
- **Aceite:** o teste passa com o seed demo e com a massa `loadgen --seed 42`.

---

## M6 — Entrada e saída de dados

### T-M6-01 — Template de importação
- [x] Concluída
- **Depende de:** T-M1-08
- **Ref:** PRD §35.3, §41.12
- **Arquivos:** `backend/internal/importer/template.go`
- **Fazer:** `GET /imports/companies/template` gera, via `excelize`, um XLSX com cabeçalhos dos campos do §7.1 + regime inicial + módulos + tipo de folha, validações de lista (regime, UF, sim/não) e aba "Instruções".
- **Aceite:** o arquivo abre no Excel/LibreOffice; teste que reabre o XLSX e confere os cabeçalhos.

### T-M6-02 — Parser e validação da importação
- [x] Concluída
- **Depende de:** T-M6-01, T-M1-03
- **Ref:** PRD §35.2–§35.5
- **Arquivos:** `backend/internal/importer/{parse,validate}.go`
- **Fazer:**
  - leitura de XLSX e CSV (detecta `;`/`,` e UTF-8/Windows-1252);
  - mapeamento por cabeçalho e normalização (CNPJ sanitizado, datas `dd/mm/aaaa`);
  - classificação por linha: Nova empresa, Empresa existente, CNPJ inválido, Código duplicado ou Campo obrigatório ausente;
  - detecção de duplicidade dentro do próprio arquivo.
- **Aceite:** testes com fixtures cobrindo cada classificação, CSV com `;` em Latin-1 e XLSX com linhas vazias no fim.

### T-M6-03 — Preview e commit da importação
- [x] Concluída
- **Depende de:** T-M6-02
- **Ref:** PRD §35.3, §35.5, §41.12 · CA-018
- **Arquivos:** `backend/internal/importer/handler.go`, `backend/migrations/0013_import_previews.sql`
- **Fazer:**
  - `POST /imports/companies/preview` (multipart) guarda o resultado com token e TTL de 1 h e retorna as linhas classificadas;
  - `POST /imports/companies/commit` recebe `{token, update_existing_rows[]}` e importa em uma transação só linhas válidas (novas + existentes confirmadas);
  - retorna um resumo.
- **Aceite:** `TestCA018_ImportRejectsInvalidBeforeCommit`; commit com token expirado → `410`; existentes não confirmadas não são alteradas.

### T-M6-04 — FE: wizard de importação
- [x] Concluída
- **Depende de:** T-M6-03, T-M1-17
- **Ref:** PRD §35.3, §49.1
- **Arquivos:** `frontend/src/features/imports/*`
- **Fazer:**
  - passos: baixar modelo → upload → prévia (tabela com classificação, erros por linha e filtro por tipo) → confirmar (checkbox por empresa existente) → resumo;
  - habilitar o CTA "Importar planilha" do estado vazio.
- **Aceite:** E2E: importar um arquivo com uma linha inválida mostra o erro e importa só as válidas.

### T-M6-05 — Exportação XLSX das grades
- [x] Concluída
- **Depende de:** T-M4-01, T-M3-08
- **Ref:** PRD §36.1, §41.13 · CA-017
- **Arquivos:** `backend/internal/exporter/xlsx.go`, botão "Exportar" nas telas
- **Fazer:**
  - `GET /exports/{module}.xlsx?competence&…filtros` reaproveitando `BuildGrid` com os mesmos filtros;
  - cabeçalho com módulo, competência, filtros e data de geração;
  - status em texto, colunas de atraso e vencimento, primeira linha congelada e autofiltro;
  - botões das telas passam os filtros atuais da URL.
- **Aceite:** `TestCA017_ExportRespectsFilters` reabre o XLSX e confere linhas e colunas.

### T-M6-06 — Base de relatórios PDF
- [x] Concluída
- **Depende de:** T-M0-03
- **Ref:** PRD §36.2
- **Arquivos:** `backend/internal/report/layout.go`
- **Fazer:** layout comum com `maroto` v2: cabeçalho (título, competência, data de geração, filtros aplicados), bloco de resumo de indicadores, tabela paginada com cabeçalho repetido, paisagem opcional e rodapé com página X/Y; fontes com acentuação pt-BR.
- **Aceite:** teste gera um PDF de exemplo e verifica o texto extraído (título, acentos).

### T-M6-07 — Os 6 relatórios PDF
- [x] Concluída
- **Depende de:** T-M6-06, T-M5-01, T-M5-04, T-M4-07, T-M6-05
- **Ref:** PRD §36.3, §41.13
- **Arquivos:** `backend/internal/report/{general,pending,overdue,backlog,credentials,module}.go`
- **Fazer:** `GET /reports/{report}.pdf?competence&…` para:
  - situação geral da competência;
  - pendências por módulo;
  - obrigações atrasadas;
  - backlog;
  - procurações e certificados a vencer;
  - controle completo de um módulo (`?module=`).
- **Aceite:** um teste por relatório gera o PDF com o seed e confere a presença das seções mínimas do §36.2.

### T-M6-08 — FE: tela de Relatórios
- [x] Concluída
- **Depende de:** T-M6-07
- **Ref:** PRD §6 (item 12), §36
- **Arquivos:** `frontend/src/features/reports/*`
- **Fazer:** lista dos 6 relatórios com parâmetros (competência, módulo, filtros) e download; atalho "Gerar PDF" no dashboard e no backlog.
- **Aceite:** E2E baixa cada relatório com status 200 e `content-type: application/pdf`.

---

## M7 — QA, carga e implantação

### T-M7-01 — Teste de carga com 300 empresas
- [x] Concluída
- **Depende de:** T-M3-10, T-M4-06, T-M5-09
- **Ref:** PRD §45.1, §52 (Fase 6), §54
- **Arquivos:** `backend/cmd/loadgen`, `docs/perf.md`
- **Fazer:** massa de 300 empresas × 12 competências; medir todas as grades, dashboard, attention, backlog, bulk de 300 itens, `ensure` de uma competência nova e exportação XLSX.
- **Aceite:** metas registradas em `docs/perf.md`: grades ≤ ~2 s, PATCH p95 < 200 ms, `ensure` < 10 s.

### T-M7-02 — Otimização de consultas
- [ ] Concluída
- **Depende de:** T-M7-01
- **Ref:** PRD §51
- **Arquivos:** `backend/migrations/00xx_indexes.sql`, `docs/perf.md`
- **Fazer:** `EXPLAIN (ANALYZE, BUFFERS)` das consultas de grade, dashboard e backlog; ajustar índices (inclusive os do §51 e o `pg_trgm`) e reescrever queries lentas.
- **Aceite:** nenhuma consulta crítica com seq scan em tabelas grandes; métricas de T-M7-01 mantidas ou melhores.

### T-M7-03 — E2E dos fluxos mínimos
- [ ] Concluída
- **Depende de:** T-M6-08, T-M5-07, T-M4-11
- **Ref:** PRD §53.3 · UC-01…UC-05
- **Arquivos:** `e2e/specs/*.spec.ts`
- **Fazer:** os 11 fluxos do §53.3 em specs independentes sobre banco resetado + seed demo, com `Clock` fixo via variável de ambiente de teste.
- **Aceite:** suíte verde no CI (job E2E adicionado a `ci.yml`).

### T-M7-04 — Acessibilidade
- [ ] Concluída
- **Depende de:** T-M7-03
- **Ref:** PRD §28.3, §45.4
- **Arquivos:** `e2e/specs/a11y.spec.ts`
- **Fazer:** `@axe-core/playwright` em todas as telas e modais; teste de teclado na grade (navegar, abrir dropdown, alterar status sem mouse); checar que status e atraso têm texto ou ícone além da cor.
- **Aceite:** zero violações `serious`/`critical` no axe.

### T-M7-05 — Checklist de segurança
- [x] Concluída
- **Depende de:** T-M4-08, T-M6-03
- **Ref:** PRD §45.3 · RN-018
- **Arquivos:** `docs/seguranca.md`
- **Fazer:** verificar e documentar:
  - segredos só em env;
  - queries parametrizadas (sqlc) e ausência de SQL concatenado (grep);
  - CORS explícito e validação de payload em todos os handlers;
  - logs sem conteúdo de arquivos ou dados sensíveis;
  - limites de upload e armazenamento fora do webroot;
  - `govulncheck` e `npm audit`.
- **Aceite:** checklist completo sem itens em aberto de severidade alta.

### T-M7-06 — Empacotamento e implantação
- [ ] Concluída
- **Depende de:** T-M0-08, T-M7-02 · Q8
- **Ref:** plan M7 · PRD §42.4, §45.2
- **Arquivos:** `Dockerfile`, `.dockerignore`, `docker-compose.prod.yml`, `.env.prod.example`, `scripts/backup.sh`, `scripts/restore.sh`, `docs/operacao.md`
- **Fazer:**
  - imagem multi-stage única: stage `node:22` compila a SPA, stage `golang:1.23` compila o binário, imagem final mínima (`alpine` oficial) com usuário não-root, `HEALTHCHECK` em `/api/v1/health` e o binário servindo os estáticos e `/api/v1`;
  - `.dockerignore` excluindo `.env*`, `storage/`, `node_modules`, `.git`;
  - `MIGRATE_ON_BOOT=true`;
  - `docker-compose.prod.yml`:
    - `db`: mesma imagem `postgres:16.x` e `INITDB_ARGS` do dev, volume nomeado e **porta não publicada no host**;
    - `app`: `depends_on` com healthcheck, volume nomeado para `FILE_STORAGE_DIR`;
    - ambos com `restart: unless-stopped`;
    - senhas somente via `.env.prod` (não versionado; `.env.prod.example` versionado).
  - Backup sem dependências novas:
    - `scripts/backup.sh`: `docker compose exec db pg_dump -Fc` + `tar` do volume de arquivos, com data no nome e retenção configurável;
    - `scripts/restore.sh`: `pg_restore --clean` + restauração dos arquivos;
    - agendamento documentado via cron do host;
    - alvos `make db-dump` e `make db-restore FILE=…` para dev.
  - `docs/operacao.md`: subir, atualizar versão, backup/restauração, rotação de senha e competência inicial de produção (Q8).
- **Aceite:** subir do zero com `docker compose -f docker-compose.prod.yml --env-file .env.prod up -d` e executar UC-01; a porta 5432 não fica exposta no host; o container da app roda como não-root; um backup gerado por `scripts/backup.sh` é restaurado com sucesso em ambiente limpo e os dados e arquivos conferem.

### T-M7-07 — Revisão dos critérios de pronto do MVP
- [ ] Concluída
- **Depende de:** todas as anteriores
- **Ref:** PRD §54 · plan §7
- **Arquivos:** `docs/tasks.md` (seção 4), `README.md`
- **Fazer:** percorrer a matriz da seção 4 marcando a evidência de cada critério; atualizar o README com a operação diária; confirmar que as pendências de T-000 foram incorporadas ou registradas como pós-MVP.
- **Aceite:** todos os critérios do §54 com evidência; `make test` e CI E2E verdes.

---

## 4. Matriz de rastreabilidade

### 4.1 Critérios de aceite do PRD (§47)

| CA | Descrição curta | Tarefas |
|---|---|---|
| CA-001 | Cadastro de empresa | T-M1-08, T-M1-14 |
| CA-002 | CNPJ duplicado | T-M1-08, T-M1-14 |
| CA-003 | Copiar CNPJ | T-M1-13 |
| CA-004 | Mudança de regime futura | T-M2-14 |
| CA-005 | Abertura de competência | T-M2-04 |
| CA-006 | Módulo desativado | T-M2-03, T-M2-04 |
| CA-007 | Obrigação não aplicável | T-M2-03, T-M2-07 |
| CA-008 | Compartilhamento de MIT | T-M4-06 |
| CA-009 | Compartilhamento de eSocial | T-M4-06 |
| CA-010 | Dependência contábil | T-M2-09, T-M4-05 |
| CA-011 | Alerta DCTFWeb | T-M2-10, T-M3-05, T-M4-04 |
| CA-012 | Atraso automático | T-M2-07 |
| CA-013 | Fechamento independente | T-M2-13 |
| CA-014 | Sugestão de fechamento | T-M2-07, T-M3-09 |
| CA-015 | Reabertura | T-M2-13, T-M3-05 |
| CA-016 | Alteração em massa | T-M2-12, T-M3-07 |
| CA-017 | Exportação | T-M6-05 |
| CA-018 | Importação | T-M6-02, T-M6-03 |
| CA-019 | Certificado vencendo | T-M4-07, T-M5-02, T-M4-10 |
| CA-020 | Segurança do arquivo de certificado | T-M4-08, T-M4-10 |
| CA-021 | Backlog | T-M5-04, T-M5-07 |

### 4.2 Critérios de pronto do MVP (§54)

| Critério | Tarefas | Evidência |
|---|---|---|
| 20 empresas fictícias carregadas | T-M1-12 | seed + E2E |
| Todos os módulos sobre o mesmo cadastro | T-M4-01, T-M4-06 | integração |
| Sem duplicação de obrigação compartilhada | T-M2-02, T-M4-06 | UNIQUE + auditoria |
| Nova competência gerada automaticamente | T-M2-04, T-M2-05 | idempotência + boot |
| Fechamento individual por módulo | T-M2-13, T-M3-09 | CA-013 |
| Reabertura com histórico | T-M2-13, T-M3-05 | CA-015 |
| Filtros por empresa, código e CNPJ | T-M1-08, T-M3-08 | E2E |
| CNPJ copiado sem máscara | T-M1-13 | CA-003 |
| Dashboard reflete o estado real | T-M5-01, T-M5-09 | teste de consistência |
| Atrasos automáticos | T-M2-07 | CA-012 |
| Backlog | T-M5-04, T-M5-07 | CA-021 |
| Dependências contábeis respeitadas | T-M2-09, T-M4-05 | CA-010 |
| DCTFWeb sinaliza componentes pendentes | T-M2-10, T-M4-04 | CA-011 |
| Importação com pré-validação | T-M6-02, T-M6-03, T-M6-04 | CA-018 |
| Exportação Excel | T-M6-05 | CA-017 |
| Relatórios PDF básicos | T-M6-06, T-M6-07 | testes de geração |
| ~300 empresas sem lentidão | T-M3-10, T-M7-01, T-M7-02 | `docs/perf.md` |

### 4.3 Testes obrigatórios (§53)

| Item do §53 | Tarefa |
|---|---|
| Resolução de regime por competência | T-M1-09 |
| Aplicabilidade | T-M2-03 |
| Cálculo de vencimento / ajuste de dia útil | T-M2-01 |
| Atraso, progresso e conclusão | T-M2-07 |
| Dependências contábeis / DCTFWeb | T-M2-09, T-M2-10 |
| Regra ICMS/Simples | T-M2-03, T-M2-07 |
| Sanitização de CNPJ | T-M1-03 |
| Geração sem duplicação | T-M2-04 |
| MIT e eSocial refletidos na DCTFWeb | T-M4-06 |
| Fechamento/reabertura | T-M2-13 |
| Importação / exportação | T-M6-03, T-M6-05 |
| E2E (11 fluxos) | T-M7-03 |
