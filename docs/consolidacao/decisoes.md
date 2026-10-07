# Registro de decisões da consolidação

Decisões tomadas durante a execução da spec (`docs/ai-spec/`), conflitos encontrados e pendências. As entradas mais recentes ficam no topo de cada seção. Decisão que muda contrato também atualiza a ADR correspondente.

## 1. Decisões

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
- **Status:** a confirmar na TASK-004.

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
| Consolidação | Q-003: modelo OpenAI padrão de cada alias (vem do `.env`) |
| Fator R | P-04: conferência do CSV das tabelas do Simples contra a LC 123/2006 |
| Fator R | P-05: proporcionalização de empresa com menos de 12 meses (hoje devolve `empresa_nova_regra_pendente`) |
| Fator R | P-07: planilha de aceite (2 empresas × 12 meses); T-707: conferência com empresa real |
| Conciliador | golden com extratos reais (gate M0) e as questões abertas 1, 4, 5 e 6 do PRD |
