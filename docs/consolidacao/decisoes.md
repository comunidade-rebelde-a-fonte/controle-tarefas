# Registro de decisões da consolidação

Decisões tomadas durante a execução da spec (`docs/ai-spec/`), conflitos encontrados e pendências. As entradas mais recentes ficam no topo de cada seção. Decisão que muda contrato também atualiza a ADR correspondente.

## 1. Decisões

### D-015 — Q-003 decidida: modelos padrão dos aliases (07/10/2026)

- **Decisão do usuário:** confirmados os modelos propostos na TASK-004.

  | Uso | Modelo | Aliases |
  |---|---|---|
  | barato | `openai/gpt-6-luna` | triador, consultor, priorizador |
  | forte com visão | `openai/gpt-6-sol` | extrator, resolvedor, classificador |
  | mais forte | `openai/gpt-6-astra` | extrator_forte |

- Continuam trocáveis no `.env` e, a partir da TASK-008, pela tela Configurações → IA.

### D-014 — Roteamento de modelos de IA (TASK-004, 07/10/2026; decisões técnicas dentro da autonomia da spec)

- **A aplicação resolve o alias e chama o gateway com o modelo concreto.**
  - O `ai.Resolver` devolve `provedor/modelo` no início de cada corrida; o resultado é o snapshot gravado nela.
  - O cliente (`internal/ai/litellm`) envia esse modelo pela rota `openai/*` (ou `anthropic/*`) do `litellm/config.yaml`.
  - Com isso, a corrida em andamento mantém o modelo do início (F-010), e a troca feita na tela (TASK-008) vale para as novas corridas sem reiniciar nada (AC-027).
  - Os aliases continuam declarados no gateway, também com o modelo do `.env`, como compatibilidade com o Conciliador.
- **Chaves.**
  - Nenhum alias tem chave própria (antes: `ANTHROPIC_API_KEY` fixa em todos). O LiteLLM usa a variável do provedor do modelo.
  - As chaves dos provedores existem só no container do LiteLLM. API e worker usam a chave do gateway (`LITELLM_API_KEY`), guardada como `config.Secret`, que se mascara em print, log e JSON.
  - Dois testes travam essas propriedades: `TestCompose_ProviderKeysOnlyInGateway` e `TestSecret_NeverPrinted`.
- **Modelos padrão (Q-003; propostos aqui e confirmados pelo usuário no D-015).** Família OpenAI gpt-6, que é a mais nova no LiteLLM fixado, mantendo o escalonamento dos sistemas de origem:

  | Uso | Modelo | Aliases |
  |---|---|---|
  | barato | `gpt-6-luna` | `triador`, `consultor`, `priorizador` |
  | forte com visão | `gpt-6-sol` | `extrator`, `resolvedor`, `classificador` |
  | mais forte, 3ª tentativa | `gpt-6-astra` | `extrator_forte` |

  Todos têm visão e saída estruturada segundo o próprio gateway. Ficam num único lugar do compose (âncora `x-ai-models`, usada por LiteLLM, API e worker). `TestCatalog_MatchesGatewayAndCompose` confere que código, `litellm/config.yaml`, `docker-compose.yml` e `.env.example` não divergem.
- **D-008 aplicado:** alias `classificador` com o mesmo modelo do `resolvedor`. No Conciliador, o agente de classificação usava o próprio alias `resolvedor`.
- **Tabela de modelos do LiteLLM travada** (`LITELLM_LOCAL_MODEL_COST_MAP=True`).
  - Sem isso, o gateway baixa a tabela do GitHub a cada início, ou seja, chamada externa e comportamento que muda sozinho.
  - Evidência: a tabela embutida não conhece o `gpt-6.1-sol` e o trata como sem suporte a saída estruturada, o que, com `drop_params`, poderia descartar o JSON Schema em silêncio.
  - **Regra para a TASK-008:** a validação da tela de IA deve consultar o gateway e avisar quando o modelo escolhido não tiver suporte a saída estruturada ou a visão, se o alias precisar.
- **Teste de alias simulado (TEST-004).**
  - O `mock_response` do LiteLLM vindo do cliente só funciona com chave virtual, que exige banco.
  - Por isso a evidência é: testes unitários do resolver e do cliente (`make test`) mais `make test-litellm`, que sobe o LiteLLM real (mesma imagem e mesmo `config.yaml`) diante de um provedor OpenAI simulado no próprio teste. Sem chamada externa e sem custo.
- **Cliente do gateway:**
  - poucas novas tentativas só em falhas transitórias (rede, 408, 429, 5xx), como os clientes de origem;
  - erro sempre sanitizado (sem chaves nem `Bearer`);
  - custo do gateway guardado como decimal textual, nunca `float`;
  - saída estruturada por JSON Schema (`strict`), que continua validada pelo código antes de qualquer uso.

### D-013 — Infraestrutura consolidada da TASK-003 (07/10/2026, decisões técnicas dentro da autonomia da spec)

A spec autoriza alterar Docker Compose e Makefile sem aprovação prévia (15-agent-execution-prompt); as escolhas abaixo ficam registradas para revisão.

- **MinIO.**
  - **Imagem:** `cgr.dev/chainguard/minio`, fixada por digest (MinIO `RELEASE.2026-09-22`).
  - **Motivo:** a MinIO deixou de publicar imagens da edição comunitária. `quay.io/minio/minio` e `quay.io/minio/mc` não existem mais, nem `latest` nem tags `RELEASE.*`. Por isso o compose do Conciliador de origem **não sobe hoje**.
  - O Langfuse do Fator R já usava a imagem da Chainguard; a mesma imagem serve aos dois MinIO e ao `minio-init` (ela traz `sh` e `mc`).
  - A Chainguard gratuita só publica `latest`, então a versão é fixada pelo digest.
- **Original imutável também no storage.**
  - Antes, o Conciliador usava a credencial raiz no app. Agora o `minio-init` cria um usuário da aplicação (`S3_ACCESS_KEY`).
  - A política desse usuário permite gravar, ler e listar versões, e **nega** apagar objeto, apagar versão, apagar o bucket, suspender o versionamento e mudar ciclo de vida ou política.
  - A credencial de administrador (`S3_ROOT_*`) só é usada pelo `minio-init`.
- **`minio-init` avulso.** Roda como etapa do `make dev` (`docker compose run --rm minio-init`) e é idempotente. Nenhum serviço depende do MinIO para subir: com ele fora, API e worker continuam saudáveis e só o upload da Conciliação falha (runbook 11 §9; testado).
- **Langfuse em compose dedicado** (`docker-compose.langfuse.yml`, projeto `controle-langfuse`).
  - Comandos: `make langfuse-up` ou `make dev-all`; `make dev-down` para tudo.
  - Motivo: são cerca de 2,4 GiB de RAM só para ele (RISK-008), e o domínio não depende dele (AC-030).
  - Organização e projeto passam a "Controle Integrado" / `controle-integrado-dev`.
  - Ganhou healthchecks no web e no worker.
- **Worker:**
  - subcomando `worker` do mesmo binário, em processo e container separados (04 §7);
  - healthcheck HTTP próprio em `/health`, porta 8090 só dentro do container;
  - hot reload com `.air.worker.toml` e diretório de build próprio (`tmp-worker/`).
- **Portas novas** publicadas só em `127.0.0.1` (09 §8). As portas existentes (Postgres, API, SPA) ficaram como estavam.
- **Versões fixadas:**

  | Serviço | Imagem |
  |---|---|
  | LiteLLM | `ghcr.io/berriai/litellm:v1.104.0` |
  | n8n | `n8nio/n8n:2.42.4` |
  | Langfuse | `langfuse:4.53.0` e `langfuse-worker:4.53.0` |
  | ClickHouse | `25.12.11.4` |
  | Redis | `7.4.11-bookworm` |
  | Postgres do Langfuse | `17.11-bookworm` |

  A origem usava `latest`/`main-latest` no Conciliador e só a versão maior (`4`, `7`, `17`) no Langfuse.
- **Preservado como na origem, adaptado depois:**
  - `litellm/config.yaml` (aliases e padrões Anthropic do Conciliador) → OpenAI configurável na TASK-004;
  - `n8n/workflows/ingestao.json`, que aponta para `http://app:8000/ingestao/arquivo` → endpoint novo na TASK-015. A pasta `n8n/inbox`, que o workflow observa e a origem não montava, agora é montada.
- **`.env.example`:** valores `dev-only` que funcionam localmente, na convenção do projeto-base. Não são segredos reais; para qualquer uso fora da máquina local, gerar valores próprios.
- **Ponto aberto para a TASK-009:**
  - API e worker (projeto `controle`) precisam alcançar o Langfuse (projeto `controle-langfuse`). Hoje estão em redes diferentes, e o Langfuse só publica em `127.0.0.1`.
  - Opções: rede externa compartilhada entre os dois projetos, ou `host.docker.internal`. Esta funciona no Docker Desktop, mas não no Linux com bind em loopback.
  - A decisão vai junto com a sincronização dos traces.

### D-012 — Agente `echo` fora do porte (07/10/2026)

- **Decisão do usuário:** o agente `echo` do Fator R (FR-23), ferramenta de dev para exercitar o tracer, não é portado. O tracer fica coberto por testes automatizados na TASK-009.

### D-011 — Capacidades não citadas pela spec entram no escopo (07/10/2026)

- **Decisão do usuário:** "se precisamos das capacidades, temos que desenvolver." Toda capacidade da origem não citada pela spec (`fontes.md` §4) é desenvolvida.
- **Distribuição:** `docs/ai-spec/13-implementation-plan.md` §12, assim:
  - FR-03 → TASK-014;
  - FR-07 → TASK-011/014;
  - FR-15 → TASK-013;
  - FR-19 → TASK-013/014;
  - CC-08 → TASK-016;
  - CC-09 → TASK-021;
  - CC-12 → TASK-017/021;
  - CC-22 → TASK-021/024.
- **Nova TASK-031 (M6, antes da TASK-030):** execução local completa fora do modo dev + backup e restauração de todos os armazenamentos (FR-21). A lacuna veio do próprio projeto-base: o `docker-compose.prod.yml` sobe só `app` + `db`, e o `scripts/backup.sh` cobre só o banco e os arquivos de procurações. Sem essa TASK, o produto consolidado instalado fora do modo dev ficaria sem worker, MinIO, LiteLLM, n8n e Langfuse.

### D-010 — Corrigir o baseline antes da TASK-003 (07/10/2026)

- **Decisão do usuário:** corrigir BL-001 (acessibilidade `target-size` no Dashboard) e BL-002 (`source-map-js`) agora, num commit `fix` separado, antes da TASK-003.
- **Motivo:** o teste de acessibilidade varre todas as telas num caso só, então enquanto falha esconde violações novas. Com o baseline todo verde, qualquer falha futura é regressão.
- **Execução:** a verificação da correção revelou mais dois problemas pré-existentes que também impediam um baseline verde e confiável. Os dois entraram no mesmo commit, com a mesma finalidade:
  - BL-003: asserção instável de aviso no E2E;
  - BL-004: detecção do Docker pelo testcontainers no Windows sob paralelismo.

  Detalhes e evidências em `baseline.md` §7.

### D-009 — Parser PGDAS-D determinístico: C-001 resolvido com a opção A (07/10/2026)

- **Decisão do usuário:** opção A, ou seja, portar o parser determinístico (regex), sem chamada de modelo em nenhuma etapa. `parser_pgdas` continua sendo o agente de trace, decisão e avaliação, mas não tem alias no LiteLLM.
- **Registro:** ADR-006. Documentos da spec ajustados: `CLAUDE.md` (IA), `04-system-design` §8, `07-flows` F-002, `10-env-and-secrets` §5, `ai-behavior-spec` §2, `prompt-spec` §3, `eval-plan` §2, `13-implementation-plan` (TASK-004).

### D-008 — Alias `classificador` (proposta técnica, TASK-004)

- **Contexto:** a spec lista `classificador` "quando necessário". No Conciliador, o agente de classificação usa o alias `resolvedor` (`app/agentes/classificador.py`).
- **Proposta:** criar o alias `classificador` apontando, por padrão, para o mesmo modelo do `resolvedor`. O comportamento de origem não muda, e o modelo passa a ser trocável de forma independente pela tela de IA.
- **Status:** aplicado na TASK-004 (D-014).

### D-007 — Precedência sobre o `AGENTS.md` do projeto-base (07/10/2026)

- **Contexto:** o `AGENTS.md` do `controle-tarefas` proíbe filas, dependências fora da lista aprovada e integrações externas (§6.1, §6.2), e define commits em inglês no formato `T-Mx-NN` (§8.8). A consolidação exige fila/worker, LiteLLM, Langfuse, MinIO e n8n.
- **Decisão:** vale a ordem de prioridade do `CLAUDE.md`: instrução do usuário > `docs/ai-spec/*` > `CLAUDE.md` > documentos existentes do `controle-tarefas`. O `AGENTS.md` continua valendo para o que a consolidação não altera: regras de domínio (§7), convenções de código, API, testes e segurança (§6.3–§6.5, §8.2–§8.7). O arquivo não é editado nem apagado.
- **Origem:** `CLAUDE.md` (Prioridade de instruções) e 00-index (Arquivos de agente).

### D-006 — Chave da OpenAI (07/10/2026)

- **Decisão do usuário:** a chave será entregue por arquivo, na hora de usar. Ela nunca entra no repositório, no banco, em log ou na API. Testes padrão não precisam dela (NFR-007).

### D-005 — Design System InovaSala entregue: Q-001 resolvida (07/10/2026)

- **Material:**
  - `docs/design-system/design-system-inovasala.html`, idêntico (mesmo SHA-256 `a48e7fa0…`) a `fator-r/docs/design-system-inovasala.html`;
  - `docs/design-system/design-system-inovasala-luminous.md`, versão operacional com tokens em JSON, regras e checklist.
- **Logos:** ficam em `C:\Users\mauro\OneDrive\Documentos\CEATH\Inovasala\Inova Sala SVG\` (versões branca e preta, com e sem "by Ceath"). Só entram no repositório na TASK-029.
- **Referência de adaptação** do sistema visual (pensado para páginas de marketing) a uma interface operacional densa: o próprio Fator R já fez esse trabalho no commit `31e7fd4` ("aplicar tema visual CEATH com tokens e gate de cores"), documentado em `fator-r/.claude/sdd/features/DESIGN_TEMA_CEATH.md`.
- **Efeito:** o AC-035 deixa de estar bloqueado por falta de material e passa a ser trabalho normal da TASK-029. A ADR-005 foi atualizada.
- **Observação:** o repositório é público. Os dois arquivos ficam públicos no primeiro push; o HTML já é público no `fator-r`.

### D-004 — Cadência de validação (07/10/2026)

- **Decisão do usuário:** parar ao fim de cada TASK para validação. A TASK-001 e a TASK-002 foram executadas juntas, porque só registram estado e não alteram código.

### D-003 — Ambiente de execução no Windows (07/10/2026)

- **Decisão do usuário:** opção A, ou seja, Git Bash + GNU Make no Windows, com o código em `C:\Desenvolvimento`.
- **Aplicado:**
  - GNU Make 4.4.1 (`ezwinports.make`, via winget);
  - Docker Desktop (WSL2);
  - clone com `core.autocrlf=false`;
  - `.env` local com `HTTP_PORT=8088`, porque a 8080 está ocupada por outro processo do usuário.

### D-002 — Git (07/10/2026)

- **Decisão do usuário:**
  - branch local `feat/consolidacao` a partir de `develop`;
  - um commit local por TASK, **depois** da validação do usuário;
  - mensagens em Conventional Commits em português;
  - nenhuma operação remota (push, PR, merge, tag) sem pedido explícito.
- Para o trabalho da consolidação, isso substitui o formato `T-Mx-NN` e as branches `task/...` do `AGENTS.md` §8.8 (instrução do usuário tem prioridade 1). Também é compatível com a DT-02 do projeto-base: nada vai para `main`.

### D-001 — Organização do workspace (07/10/2026)

- **Decisão do usuário:**
  - o repositório final é o `controle-tarefas`, não um repositório novo;
  - o workspace fica assim:

```text
C:\Desenvolvimento\controle-a-fonte\
  controle-tarefas\                  repositório de trabalho (branch feat/consolidacao)
  referencias\fator-r\               só leitura, commit 9dd7a74 (TASK-002)
  referencias\conciliador-contabil\  só leitura, commit 2bc6e26 (TASK-002)
  spec\                              pacote original da spec (cópia versionada aqui)
```

## 2. Conflitos

### C-001 — `parser_pgdas`: extração por LLM (spec) × parser determinístico (origem) — **resolvido (D-009, ADR-006)**

- **Spec de consolidação:**
  - F-002 diz "Worker resolve alias `parser_pgdas` no LiteLLM → extrair campos + confiança";
  - `prompt-spec` §3 define um prompt de extração;
  - `ai-behavior-spec` §2 trata o agente como extrator;
  - o `CLAUDE.md` lista `parser_pgdas` como alias de modelo.
- **Origem (Fator R):**
  - o `CLAUDE.md` §3.2 diz "O parse é determinístico. A extração do PGDAS-D é por regras (regex e heurísticas). O LLM nunca extrai campos de documento na v1";
  - no `plan.md` §7.3, a etapa `plan` do `parser_pgdas` é "sem LLM" e o `render` é template;
  - o código confirma: `parsing/pgdas.py` abre com "regras e regex, sem LLM";
  - a meta de ≥ 80% de acerto ouro mede esse parser determinístico.
- **Por que é material:** usar LLM para extrair é uma mudança de comportamento e de risco que a regra soberana da própria spec proíbe ("não substituir validação determinística por LLM", "PRD de origem é contrato comportamental"). Afeta a TASK-004 (lista de aliases), a TASK-012 (parser) e o eval-plan §2.
- **Opções:**
  - **A (recomendada):** portar o parser determinístico para Go, com a mesma regex, confiança e versão, provado pelo golden das 14 fixtures. `parser_pgdas` continua sendo o nome do agente, mas sem chamada de modelo; o alias não é criado, ou fica reservado e desabilitado.
  - **B:** parser determinístico primeiro; LLM só como sugestão para documento que o regex não lê, sempre em `needs_review` e sem gravar nada. Isso é capacidade nova e exige ADR.
  - **C:** extração por LLM como a spec descreve. Exige ADR, golden novo e aceite explícito da troca de comportamento.
- **Status:** resolvido em 07/10/2026 com a opção A (D-009, ADR-006).

## 3. Pendências

| ID | Pendência | Situação |
|---|---|---|
| P-001 | BL-001 (a11y `target-size` no Dashboard, `baseline.md` §3) | aprovado: corrigir antes da TASK-003 (D-010) |
| P-002 | BL-002 (`source-map-js`, npm audit alto) | aprovado: corrigir antes da TASK-003 (D-010) |
| P-003 | Capacidades não citadas pela spec | aprovado: entram no escopo (D-011) |
| P-004 | FR-23 agente `echo` | aprovado: não portar (D-012) |

### Pendências herdadas (permanecem pendentes; não resolver por suposição)

| Origem | Pendência |
|---|---|
| Controle de Tarefas | Q1–Q8 de `docs/decisoes.md` (valores provisórios em `docs/pendencias.md`), conforme Q-002 do intake |
| Consolidação | ~~Q-003: modelo OpenAI padrão de cada alias~~ — **decidida em 07/10/2026 (D-015)** |
| Fator R | P-04: conferência do CSV das tabelas do Simples contra a LC 123/2006 |
| Fator R | P-05: proporcionalização de empresa com menos de 12 meses (hoje devolve `empresa_nova_regra_pendente`) |
| Fator R | P-07: planilha de aceite (2 empresas × 12 meses); T-707: conferência com empresa real |
| Conciliador | golden com extratos reais (gate M0) e as questões abertas 1, 4, 5 e 6 do PRD |
