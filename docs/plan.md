# Plano de Implementação — Sistema de Controle de Obrigações (MVP)

**Base:** `docs/PRD.md` v1.0 (27/09/2026)
**Data do plano:** 29/09/2026
**Stack (ADR-001 a ADR-004):** React + TypeScript + Vite · Go (REST, monólito modular) · PostgreSQL

Referências entre colchetes apontam para seções (§), regras de negócio (RN) e critérios de aceite (CA) do PRD.

---

## 1. Visão geral da entrega

O MVP é entregue em **8 marcos incrementais** (M0–M7), seguindo as 6 fases do PRD [§52], com o marco zero de infraestrutura explícito. Cada marco termina com algo executável e testado.

| Marco | Tema | Resultado verificável |
|---|---|---|
| M0 | Infraestrutura do repositório | `docker compose up` sobe Postgres + API + SPA; CI verde |
| M1 | Fundação de domínio | Empresas, regimes, módulos, catálogo e seed das 20 empresas via API e UI |
| M2 | Motor operacional | Competência gerada automaticamente, idempotente, com vencimentos e histórico |
| M3 | Grade genérica + Fiscal | Fiscal Normal e Simples operáveis inline e em massa, com fechamento/reabertura |
| M4 | Demais grades | Declarações, Folha, DCTFWeb, Contábil, Procurações/Certificados |
| M5 | Gestão | Dashboard, "Exige atenção", Backlog, drill-down, comparação histórica |
| M6 | Entrada/saída de dados | Importação XLSX/CSV, exportação XLSX, relatórios PDF |
| M7 | QA, carga e implantação | E2E completo, massa de 300 empresas, critérios de pronto [§54] atendidos |

Dependências: M0 → M1 → M2 → M3 → (M4 ∥ M5 parcial) → M6 → M7. O componente de grade construído em M3 é pré-requisito de M4; o dashboard (M5) depende apenas das consultas de agregação de M2 e pode começar em paralelo a M4.

---

## 2. Decisões de interpretação do PRD

O PRD deixa alguns pontos implícitos ou em tensão. As decisões abaixo destravam a implementação; as marcadas com **(validar)** devem ser confirmadas com o escritório antes de M3.

### 2.1 Modelo de dados

| # | Ponto no PRD | Decisão |
|---|---|---|
| D1 | A coluna **Fechamento** do Fiscal [§13.2, §11.1] tem status próprio, mas não está no seed [§50.1]. | Criar o tipo `FISCAL_FECHAMENTO` (status set `FISCAL_CLOSING`), aplicável a Normal e Simples. |
| D2 | O ICMS é compartilhado entre Normal e Simples, mas `counts_in_progress = false` só no Simples [§14.3, §50.2]. O flag no tipo não basta. | Criar a tabela `obligation_type_regime_rules` (tipo × regime → aplicável por padrão, conta no progresso). Na geração, o valor resolvido é gravado como **snapshot** em `obligation_instances.counts_in_progress`. |
| D3 | "Regime padrão aplicável" é multivalorado [§20.2]. | Resolvido pela mesma `obligation_type_regime_rules` de D2. Tipo sem regra para o regime da empresa = não gerado. |
| D4 | "Preservar snapshot da configuração utilizada" [§10.1 item 9, ADR-006] não tem tabela no modelo. | Criar `company_competence_snapshots` (empresa × competência: regime, tipo de folha, fator R, módulos ativos, responsáveis). Ela serve de base para o agrupamento Normal/Simples e para as colunas Regime, Tipo de Folha e Fator R das grades históricas. |
| D5 | REINF Retidos e REINF Lucros aparecem só na visão DCTFWeb, que não é um módulo contratual [§7.4]. | `primary_module = DECLARATIONS`, com contexto de exibição `DCTFWEB`. A ativação é regida pelo módulo Declarações; o fechamento conta no módulo Declarações. |
| D6 | Instâncias "Não se aplica" devem permanecer visíveis [§12.3]. | Para cada tipo compatível com regime e módulo, gerar a instância mesmo quando o override for `NOT_APPLICABLE`, com `applicable = false`. Tipos incompatíveis com o regime não são gerados. |
| D7 | Empresa `Sem folha` [§17.4]. | Novo campo no catálogo: `status_when_no_payroll` (NULL / `NO_MOVEMENT` / `NOT_APPLICABLE`), aplicado na geração. |
| D8 | Controle de concorrência [§49.4]. | Coluna `version INT` em `obligation_instances`, `company_module_periods`, `companies` e `credential_controls`. Os PATCH recebem `version` e a API retorna `409` em caso de divergência. |
| D9 | Usuário implícito [§34.1]. | O middleware injeta o `user_id` do `Administrador` (seed com UUID fixo) no contexto. Todas as escritas registram esse usuário, o que prepara a entrada de autenticação sem refatoração. |
| D10 | Fuso horário [§45.2]. | Timestamps em UTC no banco. Datas de negócio ("hoje", competência atual) calculadas em `America/Sao_Paulo`, configurável por `APP_TIMEZONE`. |

### 2.2 Regras de negócio

| # | Ponto | Decisão |
|---|---|---|
| R1 | Quais instâncias pertencem a um módulo para fins de fechamento. | As de `primary_module = módulo`. A visão DCTFWeb não possui fechamento próprio. |
| R2 | "Pronto para fechar" [§10.5]. | Todas as instâncias aplicáveis do módulo, com `counts_in_progress = true`, estão em status `is_final`. No Fiscal, `FISCAL_FECHAMENTO` fica fora dessa checagem [§13.5]. **(validar)** |
| R3 | Os percentuais da linha Fiscal incluem a coluna Fechamento? | Não. Fechamento é o estágio, não um controle. Os percentuais consideram só os controles [§13.3, §14.2]. **(validar)** |
| R4 | O ICMS Simples bloqueia "Pronto para fechar"? | Não. Como está fora do progresso (D2), também fica fora da checagem. **(validar)** |
| R5 | Uma empresa cadastrada no meio do mês recebe a competência corrente? | Sim. `ensureCurrentCompetence` cria o snapshot e as instâncias das empresas elegíveis que ainda **não têm snapshot** na competência. Empresas já com snapshot nunca são reprocessadas (RN-004). |
| R6 | Alteração de módulo, override ou regime com a competência corrente já gerada. | A UI sugere como vigência a próxima competência ainda não gerada. Vigência sobre competência já gerada é aceita, mas não reescreve as instâncias existentes [CA-004]. |
| R7 | Ajuste manual de aplicabilidade em uma competência já gerada. | Permitido por instância (PATCH details), com registro no histórico (`change_source = INLINE`). Não altera o cadastro. |
| R8 | Backlog [§25.1]. | Instâncias aplicáveis e não finais **ou** `company_module_periods` em `OPEN`, de competências anteriores à selecionada. |
| R9 | Ao fechar um módulo, as instâncias ficam travadas? | Sim. Em período `CLOSED`, a API rejeita edição de status das instâncias daquele módulo (`423`). Para editar, é preciso reabrir com motivo [§10.6]. **(validar)** |

---

## 3. Estrutura do repositório

```text
/backend
  /cmd/api                 # main: config, DI, router, scheduler diário
  /internal
    /platform              # config, db (pgxpool), http (chi, middlewares, erros), clock, tz
    /user                  # usuário implícito, contexto de requisição
    /company               # cadastro, regimes, módulos, overrides, responsáveis
    /catalog               # obligation_types, status_sets, regras de regime, dependências
    /calendar              # calendários, feriados, motor de vencimento
    /competence            # ensureCurrentCompetence, snapshots, navegação
    /obligation            # instâncias, status, histórico, bulk, dependências
    /period                # company_module_periods, fechar/reabrir, eventos
    /grid                  # montagem de grades por módulo (fiscal, decl., folha, dctf, contábil)
    /credential            # procurações/certificados, validação de arquivos, storage
    /dashboard             # KPIs, atenção, histórico, backlog
    /importer              # preview/commit XLSX/CSV
    /exporter              # XLSX
    /report                # PDF
  /migrations              # goose, SQL puro
  /queries                 # SQL para sqlc
  /seed                    # catálogo, feriados nacionais, 20 empresas fictícias
/frontend
  /src
    /app                   # rotas, providers, layout
    /components            # DataGrid, StatusCell, CompetenceNav, Filters, BulkBar, CopyButton
    /features              # dashboard, companies, fiscal, declarations, accounting, payroll,
                           # dctfweb, credentials, backlog, settings, imports, reports
    /lib                   # formatadores (CNPJ, competência), api client
    /services              # hooks TanStack Query por recurso
    /types                 # tipos gerados do OpenAPI
/e2e                       # Playwright
/docker-compose.yml
/Makefile
```

**Bibliotecas escolhidas** (concretizando o §42):

- **Backend:** `chi`, `pgx/v5`, `sqlc`, `goose` (migrations), `excelize` (XLSX in/out), `encoding/csv`, `maroto` v2 (PDF), `go-playground/validator`, `slog` (logs estruturados, sem segredos).
- **Frontend:** React Router, TanStack Query, TanStack Table, TanStack Virtual, React Hook Form, Zod, shadcn/ui (Radix, acessível), Tailwind, Recharts.
- **Contrato:** OpenAPI 3.1 escrito à mão em `/backend/api/openapi.yaml`, com tipos TS gerados via `openapi-typescript`. É a única fonte de verdade do contrato.
- **Testes:** `go test` + `testcontainers-go` (Postgres real), Vitest + Testing Library, Playwright.

---

## 4. Modelo de dados — migrations

Ordem das migrations (a base é o §38, somada às decisões da seção 2):

1. `users`, seed `Administrador` (UUID fixo).
2. `companies` com `version`, `payroll_type` (enum `NO_PAYROLL`/`PRO_LABORE`/`PAYROLL`) e `factor_r`.
3. `company_tax_regime_history`, com constraint `EXCLUDE USING gist` contra sobreposição de intervalos [§38.3]. A competência é armazenada como `DATE` no primeiro dia do mês para permitir `daterange`; a representação `AAAA-MM` fica na API.
4. `company_module_settings` (vigência), `company_obligation_overrides` (vigência).
5. `status_sets`, `status_definitions` (flags `counts_as_started`, `counts_as_operational_progress`, `is_final`, `is_blocking`, `sort_order`) [§11].
6. `business_calendars`, `holidays`, `due_date_rules`.
7. `obligation_types` (+ `status_when_no_payroll`, `initial_status_code`), `obligation_display_contexts`, `obligation_type_regime_rules` (D2/D3), `obligation_dependencies` (HARD/SOFT).
8. `competencies`, `company_competence_snapshots` (D4).
9. `company_module_periods` (+ `version`), `company_module_period_events`.
10. `obligation_instances` (+ `counts_in_progress`, `version`, `due_date_overridden_by/at`), UNIQUE `(company_id, competence_id, obligation_type_id)` [RN-009].
11. `obligation_status_history` (+ `change_source`, `override_reason`, e também mudanças de applicable/vencimento, com `field` e valores antigo/novo).
12. `credential_controls` (UNIQUE `company_id + type`) [§38.19] e `settings` (faixas de validade configuráveis [§19.3], fuso).
13. Índices do §51, mais `pg_trgm` em `legal_name`/`trade_name` (barato e já útil para a busca global).

Exclusão física de empresas é bloqueada por FK `ON DELETE RESTRICT` e pela ausência de endpoint DELETE [RN-008].

---

## 5. Marcos detalhados

### M0 — Infraestrutura do repositório

- [ ] Monorepo com `/backend` e `/frontend`; `Makefile` (`dev`, `db-*`, `test`, `migrate`, `seed`, `gen`).
- [ ] `docker-compose.yml`: Postgres 16 (imagem Debian fixada, ICU pt-BR, UTC, healthcheck, volume nomeado, bancos `controle`, `controle_e2e` e `controle_perf` criados no init), API com hot reload (air) e Vite. Os testes de integração usam testcontainers com a mesma imagem; o E2E usa o banco `controle_e2e`, nunca o de dev.
- [ ] Config por variáveis de ambiente (`DATABASE_URL`, `APP_TIMEZONE`, `CORS_ORIGINS`, `FILE_STORAGE_DIR`) [§45.3].
- [ ] Esqueleto da API: `/api/v1`, healthcheck, middleware de erros padronizados (`{code, message, details}`), CORS explícito, request-id, logs `slog`.
- [ ] Esqueleto da SPA: layout com menu das 12 áreas [§6], roteamento, cliente HTTP, TanStack Query.
- [ ] Pipeline de geração: `sqlc generate` e `openapi-typescript`.
- [ ] CI (GitHub Actions): lint (golangci-lint, eslint), testes Go com testcontainers, Vitest, build.

**Pronto quando:** `make dev` sobe o ambiente completo e o CI passa em um PR trivial.

### M1 — Fundação de domínio [§7, §8, §9, §20, §34, §37, §50]

**Backend**
- [ ] Migrations 1–7 e 12.
- [ ] CRUD de empresas [§41.1] com validação de CNPJ (dígitos verificadores), sanitização para 14 dígitos, unicidade de CNPJ e código com resposta indicando a empresa conflitante [CA-002], e `POST /deactivate` (sem DELETE).
- [ ] Cadastro inicial exige o regime inicial, com vigência = competência de `service_start_date`.
- [ ] Regimes [§41.2]: `POST` encerra a vigência anterior (`effective_to = nova - 1 mês`) na mesma transação [UC-04] e valida contra sobreposição.
- [ ] Módulos [§41.3] e overrides [§41.4] com vigência (R6).
- [ ] Catálogo: `GET/POST/PATCH /obligation-types`, `GET /status-sets`, CRUD de `/due-date-rules` [§41.10].
- [ ] Busca global [§26.5]: `q` casa nome, razão social, nome fantasia, código, CNPJ formatado e CNPJ sem máscara (normalizar `q` removendo não dígitos quando o termo for numérico/pontuado).
- [ ] Seeds:
  - status sets e definições do §11 (5 conjuntos, conforme as tabelas);
  - catálogo do §50, mais `FISCAL_FECHAMENTO` (D1) e as regras tipo × regime (D2/D3);
  - `obligation_display_contexts` (ex.: `ESOCIAL` → `PAYROLL`, `DCTFWEB`; `MIT` → `DECLARATIONS`, `DCTFWEB`; `DCTFWEB` → `DECLARATIONS`, `DCTFWEB`);
  - dependências: Contábil HARD em cadeia (6 etapas); DCTFWeb SOFT sobre MIT, REINF Retidos, REINF Lucros e eSocial [§16.4, §38.15];
  - calendário nacional com feriados fixos e móveis (Carnaval, Sexta-feira Santa, Corpus Christi) de 2025 a 2030, gerados por algoritmo (Páscoa);
  - as 20 empresas fictícias do §37, com módulos variados, alguns overrides (ex.: IPI não se aplica), tipos de folha variados e credenciais com datas espalhadas pelas faixas de validade, para exercitar todas as telas.

**Frontend**
- [ ] Tela Empresas [§27.1] com listagem, busca global, filtros e ações rápidas [§27.2].
- [ ] `CopyButton` de CNPJ (só dígitos) e código [§7.3, RN-014, CA-003].
- [ ] Ficha da empresa com as 8 abas [§27.3]; a aba "Histórico de competências" fica como placeholder até M2.
- [ ] Tela de Configurações: catálogo de obrigações, regras de vencimento, feriados estaduais/municipais e faixas de validade.
- [ ] Estado vazio de "sem empresas" com três CTAs [§49.1]; o CTA de importação é ativado em M6.

**Testes:** sanitização e validação de CNPJ; resolução de regime por competência; sobreposição de vigência rejeitada; resolução de aplicabilidade (padrão × override × vigência).
**Pronto quando:** CA-001, CA-002 e CA-003 passam; o seed carrega as 20 empresas.

### M2 — Motor operacional [§10, §11, §12, §21, §23, §39]

**Motor de vencimento (`/internal/calendar`)**
- [ ] `DueDate(rule, competence, calendar) → date` com deslocamento de mês, dia-base, dia calendário ou útil, e ajuste (anterior / próximo / nenhum) [§21.1].
- [ ] O calendário efetivo de uma empresa é a união de nacional + UF + município [§21.2].
- [ ] Tabela de testes cobrindo fim de mês (dia 31 em fevereiro), dia-base útil (ex.: "15º dia útil"), feriado móvel e véspera de fim de semana.

**Geração de competência (`/internal/competence`)**
- [ ] `ensureCurrentCompetence(ctx, competenceKey)` transacional e idempotente [§10.2, §39, RN-005]:
  1. `INSERT … ON CONFLICT DO NOTHING` em `competencies`;
  2. seleciona as empresas elegíveis (ativas, dentro do período de atendimento [§7.5]) **sem snapshot** na competência (R5);
  3. grava o snapshot (D4);
  4. cria `company_module_periods` para cada módulo ativo, com evento `OPENED`;
  5. resolve os tipos: módulo ativo ∩ regra de regime ∩ override → `applicable`, `counts_in_progress`;
  6. calcula `due_date_original = due_date_effective`;
  7. define o status inicial (`initial_status_code`, `status_when_no_payroll`, `NOT_APPLICABLE` quando o set suporta);
  8. `INSERT … ON CONFLICT DO NOTHING` nas instâncias.
- [ ] `pg_advisory_xact_lock` por competência, contra execuções concorrentes (boot + cron + acesso simultâneos).
- [ ] Gatilhos: boot da API, agendador diário às 00:05 no fuso configurado e middleware/handler ao acessar a competência corrente inexistente [§10.2].
- [ ] Endpoints [§41.5]: `GET /competencies`, `/latest`, `POST /ensure-current`.

**Instâncias e status (`/internal/obligation`)**
- [ ] `PATCH /status` valida o status contra o set do tipo, grava o histórico e retorna a instância com derivados (`is_overdue`, `is_final`).
- [ ] Dependências HARD (Contábil): bloqueia `OK` se alguma etapa anterior aplicável estiver em status não final (`A iniciar`/`Pendente`); `Sem movimento` satisfaz [§18.2]. Retorna `422` com a etapa pendente [§18.3, CA-010].
- [ ] Dependências SOFT (DCTFWeb): ao marcar status final, sem `confirm_exception`, retorna `409 DEPENDENCY_WARNING` com a lista de pendências; com `confirm_exception: true` e motivo, aceita e grava `override_reason` [§16.4, CA-011].
- [ ] `PATCH /details`: data de transmissão, protocolo, vencimento efetivo (exige motivo [§21.3]) e aplicabilidade (R7).
- [ ] `POST /bulk-status`: aplica a mesma regra por instância, em uma transação, com histórico individual `BULK`. Retorna `{updated, skipped: [{id, reason}]}`. Instâncias bloqueadas por HARD ou em período fechado são puladas, sem abortar o lote [§26.3, CA-016].
- [ ] `GET /history` [§23].

**Períodos (`/internal/period`)**
- [ ] `close`: registra usuário e data/hora [§10.4]. No Fiscal, se `FISCAL_FECHAMENTO ≠ CLOSED`, exige `confirm_set_closing: true` e atualiza na mesma transação [§13.6].
- [ ] `reopen`: exige `reason` não vazio e grava evento `REOPENED` [§10.6, RN-007, CA-015].
- [ ] Fechar um módulo não afeta os demais [RN-006, CA-013].

**Cálculos derivados (SQL/views, sem regra na UI [§43])**
- [ ] `is_overdue = applicable AND NOT is_final AND due_date_effective < today(tz)` [§12.4, RN-013].
- [ ] Progresso operacional e conclusão final por empresa × módulo, excluindo `applicable = false` e `counts_in_progress = false` do denominador [§12].
- [ ] `ready_to_close` (R2).

**Testes:** unidade para vencimento, dias úteis, atraso, progresso, conclusão, dependências HARD/SOFT e regra ICMS/Simples [§53.1]. Integração para geração sem duplicação (rodar 3× e em paralelo), CA-004 a CA-007, fechamento/reabertura e CA-012 a CA-015.
**Pronto quando:** a competência corrente e as 3 anteriores (seed de histórico) são geradas para as 20 empresas e os testes de §53.1 passam.

### M3 — Grade genérica + Fiscal [§13, §14, §26, §28]

**Backend**
- [ ] `GET /modules/fiscal/grid?competence&group=normal|simples` [§41.6]: uma única consulta que devolve linhas (empresa, snapshot, período, nota, progresso, conclusão, prazo crítico mais próximo) e células (instância, status, flags `overdue`, `applicable`, `due_overridden`, `has_history`).
- [ ] Colunas vindas do catálogo, por `display_context` e `sort_order`; a UI não tem colunas fixas.
- [ ] Nota geral: `PATCH /company-module-periods/{id}/note` [§22, RN-016].
- [ ] Filtros server-side do §26.4 que dependem de derivados (somente atrasadas, prontas para fechar, aberto/fechado, com/sem observação). Busca e filtros simples também podem rodar client-side, dado o volume (~300 linhas).

**Frontend — componentes reutilizáveis**
- [ ] `CompetenceNav`: ← anterior | seletor mês/ano | próxima →, e "voltar à mais recente". A competência fica na URL (`?competence=2026-09`), para links de drill-down [§10.7].
- [ ] `DataGrid` (TanStack Table + Virtual): cabeçalho fixo, primeira coluna fixa [§28.2], seleção de linhas e navegação por teclado (setas, Enter abre o dropdown) [§45.4].
- [ ] `StatusCell`: dropdown inline com label textual e ícone, e indicadores de atraso, não aplicável, histórico e prazo alterado sem depender só de cor [§28.3]. Atualização otimista com rollback e toast em erro [§49.3]; em `409` de versão, recarrega a linha.
- [ ] `BulkActionBar`: empresas selecionadas → coluna → status → prévia da contagem → confirmar [§26.3].
- [ ] Modais: `DependencyWarningDialog` (SOFT), `CloseModuleDialog`, `ReopenDialog` (motivo obrigatório), `HistoryDrawer` e `DueDateOverrideDialog`.
- [ ] `NoteCell` com edição rápida.
- [ ] Filtros do §26.4 refletidos na URL.

**Tela Fiscal**
- [ ] Alternador `Normais | Simples Nacional`, baseado em `regime_snapshot` [§28.1].
- [ ] Banner/badge "Todos os controles concluídos — sugerido encerrar o Fiscal desta competência" [§13.5].
- [ ] Botão "Fechar competência" por linha, com a regra do §13.6.

**Pronto quando:** UC-02 é executável ponta a ponta; grade com 300 linhas abre em até ~2 s [§45.1] (medido com massa sintética).

### M4 — Demais grades [§15–§19, §29–§33]

Todas reutilizam o `DataGrid` de M3; cada uma é um endpoint de grade e uma configuração de colunas.

- [ ] **Declarações** [§29]: colunas dinâmicas do catálogo; célula com popover de vencimento, data de transmissão, protocolo e histórico. Edição de transmissão/protocolo via `PATCH /details`.
- [ ] **Folha** [§30]: colunas Tipo de Folha e Fator R (este só para Simples [§17.2]), vindas do snapshot. O eSocial é a mesma instância da DCTFWeb [CA-009].
- [ ] **DCTFWeb** [§31]: visão sobre instâncias de Declarações e Folha; coluna "dependências" com indicação de apto/pendente e lista das pendências. Sem botão de fechamento (R1).
- [ ] **Contábil** [§32]: colunas na ordem da sequência, com etapas bloqueadas visualmente desabilitadas no dropdown (o "OK" traz um tooltip com a etapa pendente). A validação final continua no backend.
- [ ] **Procurações e Certificados** [§19, §33]:
  - `PUT /companies/{id}/credential-controls/{type}` (vencimento, etapa de renovação, nota). `Renovado` exige nova validade e retorna a `Vigente` [§19.4, UC-05];
  - situação calculada pelas faixas configuráveis [§19.3];
  - upload (`PUT …/file`) com armazenamento em disco (`FILE_STORAGE_DIR`, chave UUID, nunca o nome original como path) e limite de tamanho (ex.: 10 MB);
  - validação de segurança [§19.2, RN-018, CA-020]:
    - recusar `.pfx`/`.p12` por extensão **e** por assinatura (ASN.1 PKCS#12);
    - para `.pem`/`.crt`/`.cer`, fazer parse e aceitar só blocos `CERTIFICATE`, rejeitando qualquer `PRIVATE KEY`/`ENCRYPTED PRIVATE KEY`/`RSA PRIVATE KEY`/`EC PRIVATE KEY`; DER é aceito apenas se `x509.ParseCertificate` tiver sucesso;
    - para procuração, aceitar somente PDF/imagens, com checagem por magic bytes;
    - mensagem: "Chaves privadas não são armazenadas pelo sistema.";
  - novo upload substitui o anterior e remove o arquivo antigo do disco [RN-017];
  - filtros rápidos do §33.

**Testes de integração:** CA-008 e CA-009 (editar em uma tela e ler na outra via API), rejeição de `.pfx`, PEM com chave e PEM misto (certificado + chave).
**Pronto quando:** todas as telas operacionais funcionam sobre o mesmo cadastro, sem instâncias duplicadas (verificado por query de auditoria no teste).

### M5 — Gestão [§24, §25]

- [ ] `GET /dashboard`: KPIs do §24.2, cards por módulo [§24.3], card de vencimentos de credenciais, filtros por regime/agrupamento [§24.4] e progresso por responsável [§24.5], usando o responsável do snapshot.
- [ ] `GET /dashboard/attention`: as 11 categorias do §24.6, cada uma com contagem e link de drill-down.
- [ ] `GET /dashboard/history`: competência selecionada × anterior (Δ % de conclusão, atrasos, pendências) [§24.8].
- [ ] `GET /backlog` (R8), com filtros do §25.3 e ordenação por dias em atraso [UC-03].
- [ ] Frontend: dashboard abrindo na competência mais recente [§24.1]; números clicáveis que abrem uma lista filtrada ou a grade do módulo com filtros na URL [§24.7]; gráficos Recharts.
- [ ] "Empresas com competências anteriores em aberto" como KPI e item de atenção.
- [ ] Aba "Histórico de competências" da ficha da empresa (matriz competência × módulo com estado aberto/fechado).

**Testes:** o dashboard bate com contagens independentes em SQL sobre o seed; CA-019 e CA-021.
**Pronto quando:** UC-03 é executável ponta a ponta.

### M6 — Entrada e saída de dados [§35, §36]

**Importação**
- [ ] `GET /imports/companies/template` (XLSX com cabeçalhos, validações de lista e aba de instruções).
- [ ] `POST /preview`: lê XLSX ou CSV (detecta separador `;`/`,` e encoding UTF-8/Windows-1252), valida linha a linha e classifica: Nova empresa, Empresa existente, CNPJ inválido, Código duplicado ou Campo obrigatório ausente [§35.5]. Detecta também duplicidade **dentro do próprio arquivo**. O preview fica guardado com token e TTL.
- [ ] `POST /commit`: recebe o token e a lista de linhas "existentes" que o usuário confirmou atualizar; importa em uma transação [§35.3, CA-018].
- [ ] UI em wizard: modelo → upload → prévia com erros por linha → confirmação → resumo.

**Exportação XLSX** [§36.1, CA-017]
- [ ] `GET /exports/{module}.xlsx` recebe os mesmos parâmetros de filtro da grade (reaproveita a mesma query) e inclui cabeçalho com competência e filtros, status em texto, atraso e vencimento.

**Relatórios PDF** [§36.2, §36.3]
- [ ] Layout comum: título, competência, data de geração, filtros aplicados e resumo de indicadores.
- [ ] Os 6 relatórios: situação geral, pendências por módulo, atrasadas, backlog, credenciais a vencer e controle completo de um módulo. Paisagem para grades largas.

**Testes:** planilhas com CNPJ inválido, duplicado no arquivo, duplicado na base e com campo ausente; export com filtros, reabrindo o XLSX gerado e conferindo linhas e colunas.

### M7 — QA, carga e implantação [§52 Fase 6, §53, §54]

- [ ] Gerador de massa: 300 empresas com distribuição realista de regimes e módulos e 12 competências de histórico.
- [ ] Performance: grade de 300 linhas em até ~2 s; alteração de status com p95 < 200 ms na API; `EXPLAIN ANALYZE` das consultas de grade e dashboard, com ajuste de índices [§51].
- [ ] E2E Playwright com os 11 fluxos do §53.3.
- [ ] Acessibilidade: axe no Playwright para grades e modais; navegação por teclado na grade.
- [ ] Checklist de segurança do §45.3 (grep de logs, CORS, queries parametrizadas, upload).
- [ ] Empacotamento: imagem única (API Go servindo o build estático da SPA, usuário não-root) + Postgres sem porta exposta; `docker-compose.prod.yml`; migrations embutidas rodando no boot; backup via `scripts/backup.sh` (`pg_dump -Fc` + arquivos) agendado no cron do host, com restauração testada. O backup é operacional, fora da UI [§3.2].
- [ ] Revisão final dos critérios de pronto do §54 (seção 7 deste plano).

---

## 6. Estratégia de testes

| Nível | Ferramenta | Cobertura mínima |
|---|---|---|
| Unidade (Go) | `go test`, table-driven | Todos os itens do §53.1 |
| Integração (Go) | testcontainers-go + Postgres 16 | Itens do §53.2 e CA-001 a CA-021 via HTTP |
| Frontend | Vitest + Testing Library | `StatusCell` (otimista + rollback), `CopyButton`, `CompetenceNav`, formatadores |
| E2E | Playwright | 11 fluxos do §53.3 sobre o seed das 20 empresas |
| Carga | script Go + k6 opcional | 300 empresas × 12 competências |

Cada CA do PRD vira ao menos um teste com o ID no nome (ex.: `TestCA010_ContabilDependency`), para dar rastreabilidade.

---

## 7. Rastreabilidade — critérios de pronto do MVP [§54]

| Critério | Marco | Evidência |
|---|---|---|
| 20 empresas fictícias carregadas | M1 | seed + E2E |
| Todos os módulos sobre o mesmo cadastro | M4 | integração |
| Sem duplicação de obrigação compartilhada | M2/M4 | UNIQUE + CA-008/009 |
| Competência gerada automaticamente | M2 | teste de idempotência + boot |
| Fechamento individual por módulo | M2/M3 | CA-013 |
| Reabertura com histórico | M2/M3 | CA-015 |
| Filtros por empresa, código e CNPJ | M1/M3 | E2E |
| CNPJ copiado sem máscara | M1 | CA-003 |
| Dashboard reflete o estado real | M5 | teste de contagem |
| Atrasos automáticos | M2 | CA-012 |
| Backlog | M5 | CA-021 |
| Dependências contábeis | M2/M4 | CA-010 |
| DCTFWeb sinaliza pendências | M2/M4 | CA-011 |
| Importação com pré-validação | M6 | CA-018 |
| Exportação Excel | M6 | CA-017 |
| PDFs básicos | M6 | teste de geração |
| 300 empresas sem lentidão | M7 | teste de carga |

---

## 8. Riscos e mitigações

| Risco | Impacto | Mitigação |
|---|---|---|
| Regras de vencimento reais não fornecidas | Datas erradas geram falsos atrasos | Seed com regras marcadas como "a validar"; tela de catálogo permite correção sem deploy; validar com o escritório antes de M3 (pergunta Q1) |
| Performance da grade com muitas colunas × 300 linhas | Operação lenta | Consulta única por grade, virtualização de linhas, medição desde M3 |
| Ambiguidade de "Pronto para fechar" e progresso do Fiscal | Retrabalho de regra | Decisões R2–R4 isoladas em funções puras, com validação antecipada |
| Upload de material sensível de certificado | Vazamento de chave privada | Validação por conteúdo, não só extensão; testes negativos; armazenamento fora do webroot |
| Geração concorrente de competência | Duplicação | UNIQUE + `ON CONFLICT` + advisory lock |
| Evolução para multiusuário | Refatoração | `user_id` em todas as escritas, `version` para concorrência otimista e middleware de usuário substituível (D8/D9) |

---

## 9. Perguntas em aberto para o escritório

Precisam de resposta antes de M3, exceto quando indicado.

1. **Q1 — Vencimentos:** qual a regra concreta (dia, deslocamento, útil/corrido, ajuste) de cada obrigação do catálogo? O PRD delega isso ao catálogo [§21.1]; o seed trará valores provisórios.
2. **Q2 — Fechamento Fiscal (R2/R3):** confirmar que a coluna Fechamento não entra nos percentuais e que "Pronto para fechar" considera só os controles.
3. **Q3 — ICMS Simples (R4):** confirmar que não bloqueia a sugestão de fechamento.
4. **Q4 — Edição em período fechado (R9):** bloquear edição até a reabertura, ou apenas alertar?
5. **Q5 — Folha "Sem folha" (D7):** para cada controle (Folha, eSocial, FGTS, INSS/IRRF), o padrão é `Sem movimento` ou `Não se aplica`?
6. **Q6 — Aplicabilidade padrão por regime:** matriz tipo × regime para Declarações (ex.: PGDAS e DeSTDA só no Simples? EFD Contribuições só em Normais?).
7. **Q7 — Feriados estaduais e municipais:** quais UFs e municípios concentram a carteira, para o seed inicial? (Pode ficar para M7.)
8. **Q8 — Competência de início:** a partir de qual competência o sistema entra em produção, e há necessidade de gerar competências retroativas vazias para lançar backlog real?

---

## 10. Fora do escopo (confirmação)

Segue integralmente o §3.2 e o §55: sem autenticação, notificações, anexos em obrigações, integrações externas, periodicidades não mensais, MEI, pessoa física, matriz/filial ou multi-tenant. O domínio, porém, já expõe como estados consultáveis os eventos futuros de notificação [§55.1] (atrasos, vencimentos próximos, pendências, credenciais vencendo).
