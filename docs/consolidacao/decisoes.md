# Registro de decisões da consolidação

Decisões tomadas durante a execução da spec (`docs/ai-spec/`), conflitos encontrados e pendências. As entradas mais recentes ficam no topo de cada seção. Decisão que muda contrato também atualiza a ADR correspondente.

## 1. Decisões

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

## 2. Conflitos que aguardam decisão

### C-001 — `parser_pgdas`: extração por LLM (spec) × parser determinístico (origem)

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
- **Status:** aguardando decisão do usuário, antes da TASK-004.

## 3. Pendências abertas

| ID | Pendência | Recomendação | Quando |
|---|---|---|---|
| P-001 | BL-001 (a11y `target-size` no Dashboard, `docs/consolidacao/baseline.md` §3) | Corrigir já, num commit `fix` separado antes da TASK-003. Enquanto o teste falha, ele esconde violações novas nas outras telas. | antes da TASK-003 |
| P-002 | BL-002 (`source-map-js`, npm audit alto) | `npm audit fix` no frontend, no mesmo commit de correções do baseline | antes da TASK-003 |
| P-003 | Capacidades não citadas pela spec (`fontes.md` §4): FR-15 rotina semanal do priorizador, FR-19 disclaimer em todas as telas do módulo, FR-21 backup completo, CC-08 importação determinística de extrato (CSV/TXT/texto do PDF), CC-12 lançamentos manuais/CSV, telas de playbooks e métricas | Incluir no escopo, nas TASKs indicadas, pela regra "preservar 100%" | confirmação do usuário |
| P-004 | FR-23 agente `echo` (ferramenta de dev do Fator R) | Não portar como agente; cobrir o tracer com testes automatizados | confirmação do usuário |

### Pendências herdadas (permanecem pendentes; não resolver por suposição)

| Origem | Pendência |
|---|---|
| Controle de Tarefas | Q1–Q8 de `docs/decisoes.md` (valores provisórios em `docs/pendencias.md`), conforme Q-002 do intake |
| Consolidação | Q-003: modelo OpenAI padrão de cada alias (vem do `.env`) |
| Fator R | P-04: conferência do CSV das tabelas do Simples contra a LC 123/2006 |
| Fator R | P-05: proporcionalização de empresa com menos de 12 meses (hoje devolve `empresa_nova_regra_pendente`) |
| Fator R | P-07: planilha de aceite (2 empresas × 12 meses); T-707: conferência com empresa real |
| Conciliador | golden com extratos reais (gate M0) e as questões abertas 1, 4, 5 e 6 do PRD |
