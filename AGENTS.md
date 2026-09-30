# AGENTS.md — Contrato de execução do projeto

Este arquivo é o contrato de trabalho para qualquer agente (IA ou pessoa) que implemente este repositório. Leia-o inteiro antes da primeira tarefa e releia-o sempre que houver dúvida sobre o que é permitido.

---

## 1. Objetivo final

Entregar o **MVP do Sistema de Controle de Obrigações para Escritório Contábil**: uma aplicação web que substitui as planilhas do escritório por uma base única, histórica e organizada por competência mensal.

O MVP só está pronto quando o escritório consegue executar o ciclo mensal inteiro dentro do sistema [PRD §58]:

> **cadastrar → configurar → abrir competência → executar obrigações → acompanhar prazos → tratar pendências → fechar cada setor → consultar histórico → exportar/relatar**

**Critério objetivo de conclusão:** todos os itens do PRD §54 atendidos com evidência, conforme a matriz da seção 4 de `docs/tasks.md`, e a tarefa T-M7-07 marcada `[x]`.

**Escopo operacional:** um escritório controlando cerca de 300 empresas-clientes (multiempresa), com um usuário implícito `Administrador`. **Não** é multi-escritório (multi-tenant).

---

## 2. Documentos do projeto e hierarquia de autoridade

| Ordem | Documento | Papel | Quem altera |
|---|---|---|---|
| 1 | `docs/PRD.md` | **O quê.** Requisitos, regras de negócio (RN), critérios de aceite (CA). | Somente o humano responsável |
| 2 | `docs/decisoes.md` | Respostas às perguntas Q1–Q8 (T-000). Complementa o PRD quando ele é omisso. | Somente o humano (o agente pode redigir a partir do que o humano disser) |
| 3 | `docs/plan.md` | **Como.** Arquitetura, decisões de interpretação (D1–D10, R1–R9), marcos M0–M7. | Somente com aprovação humana |
| 4 | `docs/tasks.md` | **Etapas.** As 84 tarefas executáveis, dependências e critérios de aceite. | O agente marca checkboxes; mudar escopo exige aprovação |
| 5 | `AGENTS.md` (este arquivo) | **Regras de execução.** Processo, permissões, convenções. | Somente com aprovação humana |

**Em caso de conflito**, vale o documento de ordem menor (PRD > decisões > plano > tarefas). Se o conflito afetar comportamento visível ao usuário ou dados, **pare e pergunte** (seção 9). Não escolha sozinho.

**Documentos gerados durante a execução:**

| Documento | Uso |
|---|---|
| `docs/pendencias.md` | Valores provisórios aguardando T-000 |
| `docs/perf.md` | Medições de performance |
| `docs/seguranca.md` | Checklist de segurança |
| `docs/operacao.md` | Implantação e backup |

---

## 3. Onde estão as etapas e como escolher a próxima

- **Fonte única das etapas:** `docs/tasks.md`. Cada tarefa tem ID (`T-Mx-NN`), dependências, referências, arquivos, o que fazer e critérios de aceite.
- **Status:** o checkbox `- [ ] Concluída` de cada tarefa. `[x]` = concluída e verificada.
- **Próxima tarefa:** a primeira, na ordem do arquivo, cuja lista "Depende de" esteja toda `[x]`. Siga o caminho crítico da seção 3 de `tasks.md` quando houver mais de uma opção.
- **T-000** (perguntas ao escritório) **não bloqueia o início** de nenhuma tarefa. Use valores provisórios, marcados conforme a seção 6.6.
- **Não invente tarefas fora de `tasks.md`.** Se descobrir trabalho necessário que não está listado, proponha a nova tarefa ao humano (ID, dependências e aceite) antes de executá-la.

---

## 4. Ciclo obrigatório de execução de cada tarefa

1. **Selecionar** a tarefa conforme a seção 3 e confirmar que todas as dependências estão `[x]`.
2. **Ler as referências** citadas no campo "Ref" (seções do PRD e do plano, CAs, RNs), não só o resumo da tarefa.
3. **Criar a branch** `task/T-Mx-NN-slug-curto` a partir da branch de integração.
4. **Escrever os testes primeiro** quando a tarefa envolver regra de negócio (motor, cálculos, dependências, validações). Todo CA leva o ID no nome do teste (seção 7).
5. **Implementar** somente o escopo da tarefa. Refatorações fora do escopo viram proposta, não código.
6. **Atualizar o contrato**: se mudou HTTP, editar `backend/api/openapi.yaml` e rodar `make gen`.
7. **Verificar** localmente, com tudo verde:
   ```bash
   make lint
   make test
   ```
   E, quando a tarefa tocar UI ou fluxo completo:
   ```bash
   make e2e
   ```
8. **Conferir o aceite**: cada bullet de "Aceite" da tarefa precisa estar comprovado por teste ou verificação reproduzível.
9. **Marcar** `[x]` na tarefa em `docs/tasks.md`, no mesmo commit.
10. **Commitar** com a mensagem no formato da seção 8.
11. **Reportar** usando o modelo da seção 11.

**Antes de M0 existir**, os comandos `make` ainda não estão disponíveis. A tarefa T-M0-01 é justamente criá-los.

---

## 5. O que o agente PODE fazer (autonomia concedida)

- Criar e editar arquivos dentro de `backend/`, `frontend/`, `e2e/`, `.github/`, `docker/`, `scripts/`, `docs/` (apenas os documentos gerados da seção 2), `Makefile`, `docker-compose*.yml`, `Dockerfile`, `.dockerignore`, `.env.example`, `.env.prod.example`, `.gitignore`, `README.md`.
- Criar **novas** migrations, seeds e queries SQL.
- Adicionar as dependências da **lista aprovada** (seção 6.1) sem pedir permissão.
- Subir containers locais (`docker compose`) e rodar migrations e seeds nos bancos **locais** (`controle`, `controle_e2e`, `controle_perf`) ou nos containers de teste (testcontainers).
- Recriar os bancos locais (`make db-reset`). Isso apaga os dados de dev, então avise no relatório quando fizer.
- Rodar testes, linters, formatadores, `go mod tidy`, `npm install` e geradores (`sqlc`, `openapi-typescript`).
- Criar branches locais e commits locais seguindo a seção 8.
- Marcar checkboxes em `docs/tasks.md` e registrar pendências em `docs/pendencias.md`.
- Usar valores provisórios para itens dependentes de T-000, seguindo a seção 6.6.
- Consultar documentação oficial de bibliotecas da stack.

---

## 6. O que o agente NÃO PODE fazer

### 6.1 Stack e dependências

A stack é fixa [PRD §42, ADR-001 a ADR-004]:

| Camada | Tecnologias |
|---|---|
| Backend | Go 1.23+, `chi`, `pgx/v5`, `sqlc`, `goose`, `excelize`, `maroto` v2, `go-playground/validator`, `golang.org/x/text` (encodings), `log/slog`, `testcontainers-go` |
| Ferramentas de dev | `air` (hot reload), `golangci-lint`, `govulncheck` |
| Banco | PostgreSQL 16 (extensões `pgcrypto`, `btree_gist`, `pg_trgm`), collation ICU `pt-BR`, timezone UTC |
| Imagens Docker | Somente oficiais e com versão fixada: `postgres:16.x` (Debian, não alpine), `golang:1.23`, `node:22`, `alpine` (imagem final de produção) |
| Frontend | Node 22+, React, TypeScript (strict), Vite, React Router, TanStack Query/Table/Virtual, React Hook Form, Zod, `@hookform/resolvers`, Tailwind, shadcn/ui (Radix), `lucide-react`, Recharts, `openapi-typescript` |
| Testes (frontend) | Vitest, Testing Library, Playwright, `@axe-core/playwright` |

**Proibido sem aprovação humana:**
- trocar qualquer item da stack;
- adicionar dependência fora desta lista;
- usar ORM (GORM, ent etc.);
- adotar microsserviços, filas, Redis, SSR/Next.js ou GraphQL;
- usar SQLite.

### 6.2 Escopo do produto

Não implementar nada do PRD §3.2 e §55, nem de forma "preparatória":
- autenticação/login e multiusuário simultâneo;
- notificações (internas, e-mail, WhatsApp);
- anexos em obrigações;
- integrações externas e sincronização com outros sistemas;
- MEI, pessoa física, Carnê-Leão;
- matriz/filiais, multi-escritório/SaaS;
- periodicidades não mensais na UI;
- filtros salvos;
- segunda aprovação;
- backups pela interface;
- múltiplas procurações ou certificados ativos por empresa.

A **estrutura preparada** exigida pelo PRD continua valendo: `user_id` nas escritas, `version`, tabela `users` com `role`, campo `periodicity`.

### 6.3 Dados, banco e histórico

- **Não editar migration já commitada.** Toda correção de schema é uma migration nova.
- **Não criar exclusão física** (DELETE) de empresas, instâncias, históricos, eventos ou competências [RN-008]. Use inativação.
- **Não reescrever competências já geradas** a partir de mudança de cadastro, regime, módulo ou aplicabilidade [RN-004, ADR-006].
- **Não criar uma segunda instância** para obrigação compartilhada entre telas [RN-009, ADR-005]. Telas são visões de `obligation_instances`.
- **Não tratar "Atrasado" como status manual** [RN-013]. É sempre calculado.
- **Não rodar comandos contra bancos que não sejam os locais ou os de teste.** Nada de produção, homologação ou bancos remotos.
- **E2E e testes de carga nunca usam o banco `controle` de dev.** Usam `controle_e2e` e `controle_perf`.
- **Não publicar a porta do Postgres** no compose de produção nem usar a senha de exemplo fora de dev.

### 6.4 Segurança

- **Nunca** armazenar senha, chave privada, `.pfx`/`.p12` ou segredo de certificado, nem em teste ou fixture versionada fora de `testdata/` com material sintético [RN-018].
- Não enfraquecer a validação de upload (T-M4-08) nem a de CNPJ da API. O bypass para os CNPJs fictícios existe **apenas** no seed, e documentado.
- Não commitar `.env`, credenciais, tokens ou dados reais de clientes. Dados de exemplo são sempre fictícios.
- Não concatenar SQL com entrada do usuário. Use apenas queries parametrizadas (`sqlc`/`pgx`).
- Não logar corpo de requisição, conteúdo de arquivos ou dados sensíveis.
- Não abrir CORS com `*`.

### 6.5 Arquitetura e código

- **Regra de negócio não vai para o frontend** [PRD §43]. Cálculos de atraso, progresso, conclusão, "pronto para fechar", aplicabilidade e dependências são do backend. O frontend só exibe e faz UX (ex.: desabilitar opção bloqueada, sempre revalidado pela API).
- Não colocar colunas de obrigação fixas no banco nem na UI. Colunas vêm do catálogo [PRD §15.2, §29].
- Não pular, desabilitar, marcar `skip` ou afrouxar testes, lint ou checagens de tipo para "fazer passar".
- Não usar `any` no TypeScript nem `//nolint`/`@ts-ignore` sem justificativa em comentário e menção no relatório.

### 6.6 Valores provisórios (dependentes de T-000)

- Regras de vencimento, matriz declarações × regime, status "Sem folha" e as regras R2/R3/R4/R9 **não são inventadas como definitivas**.
- Use o valor provisório do `plan.md` e marque no código ou seed:
  ```sql
  -- A VALIDAR (Qn)
  ```
- Registre cada valor provisório em `docs/pendencias.md` (tarefa, valor, pergunta).
- Não apresente prazos legais como fato: não afirme "o prazo da DCTFWeb é X" sem que esteja em `docs/decisoes.md`.

### 6.7 Git e ações externas

**Exigem autorização humana explícita:**
- `git push`, abrir PR, merge em `main`/`master`;
- `git push --force`, `git reset --hard` em branch compartilhada, reescrever histórico publicado;
- criar ou alterar repositórios remotos, issues ou releases;
- chamar serviços externos com dados do projeto;
- publicar imagens.

Também não apague arquivos que não foram criados na tarefa corrente sem autorização.

### 6.8 Documentos de autoridade

- Não editar `docs/PRD.md`, `docs/plan.md` nem este `AGENTS.md` sem aprovação.
- Em `docs/tasks.md`, o agente pode **apenas** marcar checkboxes e corrigir erros de digitação. Mudar escopo, dependências ou aceite exige aprovação.

---

## 7. Regras de domínio invioláveis (resumo operacional)

Detalhes no PRD §46 e nas decisões do `plan.md` §2. Toda implementação deve respeitar:

| Tema | Regra |
|---|---|
| Cadastro único | Uma empresa, um cadastro, usado por todos os módulos. CNPJ e código interno únicos (RN-001, RN-002). |
| CNPJ | Armazenado com 14 dígitos; copiar = só dígitos (RN-014). Validar os dígitos verificadores na API. |
| Competência | Armazenamento `AAAA-MM` (ou `DATE` dia 1), exibição `MM/AAAA`. |
| Regime | Resolvido por vigência na competência (RN-003); intervalos não se sobrepõem. |
| Snapshot | A competência preserva regime, tipo de folha, fator R, módulos e responsáveis usados na geração (D4). |
| Geração | `ensureCurrentCompetence` transacional, idempotente, com advisory lock; só empresas ativas e dentro do período de atendimento (RN-005). |
| Aplicabilidade | Override da empresa > regra padrão tipo × regime. Não aplicável fica visível, fora do denominador (D2, D3, D6, RN-015). |
| ICMS no Simples | Visível, fora do progresso (RN-010, D2). |
| Progresso | Operacional = itens que contam / aplicáveis relevantes; conclusão = finais / aplicáveis [§12]. |
| Atraso | `aplicável AND NOT final AND vencimento_efetivo < hoje(fuso)` (RN-013). |
| Contábil | Dependência HARD sequencial; `Sem movimento` satisfaz (RN-011). |
| DCTFWeb | Dependência SOFT: alerta forte + exceção com motivo registrada no histórico (RN-012). |
| Fechamento | Por empresa + módulo + competência, independente (RN-006). Reabertura exige motivo e gera evento (RN-007). |
| Observação | Uma por empresa + módulo + competência; sem comentário por obrigação (RN-016). |
| Histórico | Toda mudança de status, vencimento ou aplicabilidade gera linha em `obligation_status_history` com usuário e origem. |
| Concorrência | `version` em entidades editáveis; divergência → `409`. |
| Tempo | UTC no banco; "hoje" e "competência atual" calculados em `APP_TIMEZONE` (padrão `America/Sao_Paulo`) via `Clock` injetável. |
| Credenciais | Uma procuração RFB e um certificado ativos por empresa; novo substitui o anterior (RN-017). |

---

## 8. Convenções de código, API, testes e Git

### 8.1 Estrutura do repositório

Conforme `plan.md` §3:

```text
backend/      cmd/api, cmd/loadgen, internal/<domínio>, migrations, queries, seed, api/openapi.yaml
frontend/     src/{app,components,features,lib,services,types}
e2e/          specs Playwright
docker/       postgres/init (criação dos bancos locais)
scripts/      backup.sh, restore.sh
docs/         PRD, plan, tasks, decisões, pendências, perf, segurança, operação
```

**Ambiente Docker:**

| Contexto | Arquivo | Serviços | Banco |
|---|---|---|---|
| Dev | `docker-compose.yml` (`make dev`) | `db`, `api` (air), `web` (Vite) | `controle` |
| E2E | `docker-compose.yml` + `make e2e` | `db` + API apontada para E2E | `controle_e2e` (resetado a cada execução) |
| Carga | `docker-compose.yml` + `loadgen` | `db` | `controle_perf` |
| Integração Go | testcontainers | container efêmero por suíte | efêmero |
| Produção | `docker-compose.prod.yml` | `db` (sem porta exposta), `app` (imagem única) | definido em `.env.prod` |

### 8.2 Go

- Pacotes por domínio em `internal/`, com `handler.go` (HTTP), `service.go` (regra) e `repo.go`/sqlc (dados). Handlers não contêm regra de negócio.
- Funções de regra **puras e testáveis** sempre que possível: `Resolve`, `DueDate`, `CheckHard`, cálculo de situação de credencial.
- `context.Context` como primeiro parâmetro; transações explícitas (`pgx.Tx`) em operações compostas.
- Erros de domínio tipados, mapeados para o envelope HTTP no handler.
- Tempo sempre via `Clock`, nunca `time.Now()` direto em regra de negócio.
- `gofmt`/`goimports` obrigatórios; `golangci-lint` limpo.

### 8.3 SQL e migrations

- Nome: `NNNN_descricao.sql` (goose), sempre com `-- +goose Up` e `-- +goose Down`.
- Constraints no banco para invariantes: UNIQUE, FK `ON DELETE RESTRICT`, `EXCLUDE` de vigência, CHECKs.
- Seeds idempotentes (`ON CONFLICT`).

### 8.4 API

- Prefixo `/api/v1`, JSON, rotas e campos em inglês `snake_case`, como no PRD §41.
- Erro padrão:
  ```json
  {"code": "...", "message": "...", "details": {...}}
  ```
  A `message` vem em português, para a UI.
- Códigos de status:

| Código | Uso |
|---|---|
| `400` | Payload inválido |
| `404` | Recurso não encontrado |
| `409` | Conflito de versão, unicidade ou `DEPENDENCY_WARNING` |
| `410` | Token de importação expirado |
| `422` | Regra de negócio violada (`DEPENDENCY_BLOCKED`, status inválido) |
| `423` | Período fechado |

- O OpenAPI em `backend/api/openapi.yaml` é a fonte do contrato. Tipos TS são gerados, nunca escritos à mão.

### 8.5 Frontend

- Dados remotos só via TanStack Query (hooks em `src/services`).
- Estado de competência e filtros sempre na URL.
- Componentes de grade, status, filtros e competência são **reutilizáveis** (`src/components`), nunca copiados entre módulos.
- Atualização otimista com rollback em erro [§49.3].
- Acessibilidade [§45.4]: status nunca só por cor (texto e/ou ícone + `aria-label`), navegação por teclado na grade, contraste adequado.
- Desktop-first, largura mínima de 1280 px.

### 8.6 Idioma

- **Código, identificadores, rotas, tabelas e commits técnicos:** inglês.
- **Textos da interface, mensagens de erro exibidas ao usuário e documentos em `docs/`:** português do Brasil.

### 8.7 Testes

| Nível | Ferramenta | Obrigatório para |
|---|---|---|
| Unidade Go | `go test` table-driven | Toda regra pura (PRD §53.1) |
| Integração Go | `testcontainers-go` + Postgres 16 real | Endpoints, geração, CAs |
| Componentes | Vitest + Testing Library | Componentes reutilizáveis e formulários |
| E2E | Playwright | Fluxos do PRD §53.3 e UCs citados nas tarefas |

- Nome de teste com o ID do critério: `TestCA010_ContabilDependency`, `it("CA-003 ...")`.
- Nada de mock do banco em teste de integração; use o container.
- Relógio fixo em testes; nada de testes dependentes da data real ou da ordem de execução.
- Teste instável (flaky) é bug: corrija, não reexecute até passar.

### 8.8 Git

- Branch: `task/T-Mx-NN-slug-curto`.
- Uma tarefa = um commit (ou uma série pequena de commits na mesma branch).
- Mensagem de commit:
  ```
  T-M2-04: implement ensureCurrentCompetence

  - transactional generation with advisory lock
  - idempotency tests (CA-005, CA-006)
  ```
- Nunca commitar arquivos gerados de build, `node_modules`, `.env` ou `storage/`.

---

## 9. Quando PARAR e escalar para o humano

Interrompa a tarefa e pergunte quando:

1. Houver conflito entre PRD, decisões, plano, tarefas ou este contrato que afete comportamento ou dados.
2. O aceite da tarefa só puder ser atingido violando alguma regra das seções 6 ou 7.
3. For necessária dependência, serviço ou tecnologia fora da stack aprovada.
4. For necessário mudar uma decisão D1–D10/R1–R9, o schema de forma incompatível ou quebrar o contrato OpenAPI já usado.
5. Uma meta de performance [§45.1] não for atingida após uma tentativa razoável de otimização (T-M3-10, T-M7-01).
6. For preciso um segredo, credencial, acesso remoto ou dado real.
7. A tarefa exigir apagar ou reescrever trabalho de tarefas já concluídas.
8. Surgir trabalho necessário não previsto em `tasks.md`.

Ao escalar, descreva:
- o problema;
- o que já foi tentado;
- as opções, com uma recomendação;
- o impacto de cada opção.

---

## 10. Definition of Done

### 10.1 Por tarefa

- [ ] Todas as dependências estavam `[x]` antes de começar.
- [ ] Todos os bullets de "Aceite" comprovados.
- [ ] `make lint` e `make test` verdes (e `make e2e` quando aplicável).
- [ ] OpenAPI e tipos gerados atualizados, se o contrato mudou.
- [ ] Nenhuma regra das seções 6 e 7 violada.
- [ ] Valores provisórios marcados e registrados em `docs/pendencias.md`.
- [ ] Checkbox marcado em `docs/tasks.md` no mesmo commit.
- [ ] Relatório enviado (seção 11).

### 10.2 Por marco

- [ ] Todas as tarefas do marco `[x]`.
- [ ] A frase "Pronto quando" do marco em `plan.md` §5 demonstrada.
- [ ] CI verde na branch de integração.
- [ ] Resumo do marco ao humano, com pendências abertas.

### 10.3 Do projeto

- [ ] Todos os critérios do PRD §54 com evidência na matriz de `tasks.md` §4.2.
- [ ] `docs/pendencias.md` vazio, ou cada item aceito explicitamente pelo humano como pós-MVP.
- [ ] Implantação reproduzível conforme `docs/operacao.md` (T-M7-06).

---

## 11. Modelo de relatório ao concluir cada tarefa

```markdown
## T-Mx-NN — <título>

**Status:** concluída | bloqueada | parcial

**O que foi feito**
- …

**Como foi verificado**
- `make test`: N testes, todos passando (incluindo TestCAxxx_…)
- `make lint`: limpo
- Verificação manual/E2E: …

**Valores provisórios introduzidos**
- … (Qn), registrado em docs/pendencias.md

**Desvios ou decisões tomadas**
- … (ou "nenhum")

**Próxima tarefa sugerida:** T-…
```

Se algo falhou ou foi pulado, diga isso explicitamente, com a saída relevante. Não relate como concluído o que não foi verificado.

---

## 12. Referência rápida

| Preciso de… | Onde está |
|---|---|
| Requisito ou regra de negócio | `docs/PRD.md` (RN §46, CA §47, modelo §38, API §41) |
| Decisão de interpretação | `docs/plan.md` §2 (D1–D10, R1–R9) |
| Resposta do escritório | `docs/decisoes.md` |
| Próxima etapa | `docs/tasks.md` |
| Rastreabilidade CA → tarefa | `docs/tasks.md` §4 |
| Valores ainda provisórios | `docs/pendencias.md` |
| Stack e estrutura | este arquivo, §6.1 e §8.1; `docs/plan.md` §3 |

**Glossário mínimo:**

| Termo | Significado |
|---|---|
| Competência | Mês de referência (`MM/AAAA`) |
| Instância | Uma obrigação para uma empresa numa competência |
| Normal | Grupo de interface do Fiscal (Presumido, Real, Arbitrado, Imune/Isenta); não é um regime |
| Período de módulo | Estado aberto ou fechado de empresa + módulo + competência |
| Snapshot | Cópia da configuração usada na geração da competência |
