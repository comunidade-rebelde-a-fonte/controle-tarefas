# CLAUDE.md — Controle Integrado

## Contexto do projeto

Este repositório parte do `controle-tarefas` e consolida integralmente:
- Controle de Obrigações;
- Plataforma Fator R;
- Vera / Conciliação Contábil.

A aplicação-base é Go + React/TypeScript/Vite + PostgreSQL. Fator R e Conciliação devem ser portados para essa stack de aplicação, sem perda de comportamento. Serviços auxiliares dos sistemas de origem permanecem quando fazem parte do funcionamento: MinIO, LiteLLM, n8n e Langfuse.

## Fonte de especificação

Comece sempre em:
- `docs/ai-spec/00-index.md`

Para o comportamento atual do sistema-base, consulte também os documentos existentes do repositório. Não apague `AGENTS.md` existente apenas porque o agente-alvo desta consolidação é Claude Code.

### Prioridade de instruções

1. instrução explícita atual do usuário;
2. `docs/ai-spec/*` desta consolidação;
3. este `CLAUDE.md`;
4. documentos existentes do `controle-tarefas` para comportamento não alterado;
5. PRDs/planos dos repositórios Fator R e Conciliador como referência de comportamento portado.

Em conflito material, pare e reporte antes de alterar regra de negócio.

## Workflow obrigatório

1. Explore o código antes de editar.
2. Leia a próxima TASK no implementation plan.
3. Liste requisitos, ACs e testes.
4. Faça plano curto.
5. Implemente uma vertical slice.
6. Rode validações.
7. Compare com review checklist.
8. Reporte evidências e riscos.

## Comandos base

Confirme no Makefile real antes de usar. O baseline atual inclui comandos equivalentes a:
- `make dev`
- `make migrate`
- `make seed`
- `make seed-demo`
- `make gen`
- `make lint`
- `make test`
- `make e2e`

Desde a TASK-003: `make dev` também sobe worker, MinIO, LiteLLM e n8n; `make langfuse-up` / `make dev-all` sobem o Langfuse (compose dedicado); `make health` mostra a saúde de todos os serviços.

Não invente comando que não existe: crie/atualize Makefile quando a TASK exigir.

## Regras de engenharia

- OpenAPI 3.1 é o contrato frontend/backend.
- SQL via padrões existentes (`pgx/sqlc/goose`).
- Dinheiro nunca em `float`.
- Competência segue padrão existente.
- Reutilize serviços existentes antes de criar duplicatas.
- Não escrever diretamente em tabelas de obrigação quando existir serviço de domínio que preserva histórico/regras.
- Jobs assíncronos usam fila PostgreSQL e worker.
- LLM via LiteLLM; provider/modelo por alias/configuração.
- Trace local é obrigatório; Langfuse complementa.

## Regras de negócio invioláveis

- `companies` é cadastro canônico.
- Fator R é calculado por código.
- Corte legal 28%; meta operacional 30% até decisão humana em contrário.
- Fator R não conclui obrigação Fiscal automaticamente.
- Extrato só importa se saldo fechar exatamente.
- Resolvedor apenas sugere.
- Matching ambíguo não concilia automaticamente.
- Classificação: regra → histórico → agente.
- Razão é projeção derivada.
- Fechamento Contábil do Controle é autoridade única e trava Conciliação.
- Conciliação concluída/reaberta sincroniza a obrigação `Conciliação` via serviço de domínio.
- Originais do Conciliador ficam no MinIO versionado; não deletar.

## IA

Aliases esperados:
- `triador`
- `extrator`
- `extrator_forte`
- `resolvedor`
- `consultor`
- `priorizador`
- `classificador` quando necessário

`parser_pgdas` é agente **determinístico**: regex, sem chamada de modelo e sem alias no LiteLLM (ADR-006).

Provider inicial: OpenAI, mas modelo concreto não é hardcoded.

Desde a TASK-004 (D-014): catálogo de aliases e resolução em `backend/internal/ai`; cliente do gateway em `backend/internal/ai/litellm`. O app resolve o alias no início da corrida e chama o gateway com o modelo concreto. A lista de aliases é conferida contra `litellm/config.yaml`, `docker-compose.yml` e `.env.example` por teste.

Testes normais nunca chamam provider real. `make test-litellm` usa o LiteLLM real com provedor simulado.

Desde a TASK-008 (D-021): a configuração da tela Configurações → IA fica em `ai_model_aliases` (pacote `backend/internal/ai/aliases`), sem nenhum segredo. Toda corrida de agente resolve o modelo com `aliases.Resolver.Resolve` no início e grava o snapshot. Alias novo exige linha nova numa migration (um teste confere tabela × catálogo).

## Auditoria de IA

Desde a TASK-009 (D-022): trilha local em `backend/internal/ai/tracer`, porte do `tracer.run` do Fator R.
- Toda corrida de agente usa `tracer.Do`, numa requisição, ou `Tracer.Begin` + `Run.Finish` na transação do job, devolvendo `Run.Fail(err)` em caso de erro.
- Decisão de agente só por `Run.RecordDecision`. Tabela de domínio escrita por agente referencia `ai_runs(trace_id)`.
- Spans só `plan|parse|tool|decide|render`, mais `Run.Generation` para as chamadas ao LLM. Nunca o documento inteiro em entrada ou saída.
- O envio ao Langfuse é do worker (job `langfuse_sync`); a API de consulta está em `backend/internal/ai/runs`.

## Jobs

Desde a TASK-005 (D-019): fila em `backend/internal/jobs` (SKIP LOCKED, tentativas, posse com prazo, efeito do handler na mesma transação da conclusão). Cada novo tipo de job registra o handler em `backend/internal/worker/handlers.go`; payload só com IDs, nunca documento.

## Conciliação

Desde a TASK-007 (D-020): schema em `backend/migrations/0020_reconciliation.sql` (mapeamento origem → consolidado na D-020); chave de dedupe e trava pelo fechamento Contábil em `backend/internal/reconciliation`. Toda mutação da Conciliação chama `reconciliation.RequireOpen` (AC-025).

Ingestão (TASK-015, D-026), em `backend/internal/reconciliation/documents`:
- original no MinIO pelo cliente `internal/platform/objectstore` (SigV4 só com a biblioteca padrão; sem operação de apagar), com a chave `originais/<sha256>` e a versão guardada no documento;
- arquivo repetido: 409 `DUPLICATE_DOCUMENT`; MinIO fora: 503 sem gravar nada; documento tardio com o Contábil fechado: `pendencia_periodo_fechado`, sem job;
- o documento nasce `pendente` com o job `reconciliation_triage` na mesma transação;
- testes com MinIO real: `testutil.NewMinIO` (mesma imagem e mesmo `init.sh` do compose).

Triagem e extração (TASK-016, D-027):
- domínio em `backend/internal/reconciliation/statement`: schema e validação do pydantic da origem, validador (portão R$ 0,00 e `validar_saldo`), dicas e leitores determinísticos (CSV, TXT, texto do PDF), travados por golden gerado pelo Python da origem (`testdata/statement_gen.py`);
- worker em `backend/internal/reconciliation/pipeline`: jobs `reconciliation_triage` (leitor do PDF e triador) e `statement_extract` (leitor do CSV ou extrator com até 3 tentativas e playbook); a importação é atômica e só existe com o portão aprovado;
- texto e render de PDF em `internal/platform/pdftext` (`Pages` no formato do PyMuPDF, `Render` em PNG), com golden do PyMuPDF (`testdata/mupdf_gen.py`);
- páginas renderizadas vão ao modelo como imagem (`litellm.Message.Images`);
- prompts em `internal/ai/prompts/conciliacao/`;
- `make eval-extrator` avalia o extrator contra o golden com o LiteLLM real (custo de provedor: só com autorização).

## Fator R

Fatia da TASK-006a/010/011/014a (D-016 a D-018):
- motor em `backend/internal/factorr/engine`: porte fiel do `fator_r/domain` da origem, travado por um golden gerado pelo próprio Python da origem (`testdata/golden_gen.py`);
- API em `backend/internal/factorr`; dinheiro em `internal/platform/decimal` (ADR-007);
- "sujeita ao Fator R" é o `companies.factor_r` (D-017);
- telas do inbox, do simulador, dos agentes e da observabilidade na TASK-014b (D-025): submenu da área Fator R e rotas em `frontend/src/features/factor-r` (`inbox/`, `simulator/`, `agents/`, `runs/`);
- o E2E sobe o worker e um gateway LiteLLM simulado que responde os agentes de forma determinística (`e2e/support/fake-litellm.mjs`); nunca provedor real.

Inbox PGDAS-D (TASK-012, D-023):
- parser em `backend/internal/factorr/pgdas`: porte fiel de `parsing/pgdas.py`, travado por golden gerado pelo próprio Python da origem (`testdata/golden_gen.py`). Regex da origem só por `pytext.Compile`, que mantém `\s`, `\d`, `\b`, IGNORECASE e lookahead do Python;
- texto do PDF em `backend/internal/platform/pdftext` (PDFium em WebAssembly + algoritmo do pdfplumber, com os limites da origem);
- inbox em `backend/internal/factorr/inbox`: upload na API (202) e job `pgdas_parse` no worker, com corrida do `parser_pgdas` e nota ouro;
- gravação do extrato só por `InsertFactorRMovementFromPgdas`: competência vazia, nunca sobrescreve, folha só total declarado e conferido.

Simulador e agentes consultor/priorizador (TASK-013, D-024), em `backend/internal/factorr/agents`:
- todo número vem do motor; o LLM (aliases `consultor` e `priorizador`) só classifica a intenção e redige o texto;
- texto do LLM sempre por `ValidateText` (porte do `validar_texto`, com golden da origem); reprovado, vira o template;
- "corrigir" só com simulação gravada em `factor_r_simulations` (guarda no decide);
- prompts versionados em `backend/internal/ai/prompts/`: mudar o texto exige versão nova (o teste fixa o hash).

## UI

A interface final usa Design System InovaSala. O material oficial está em `docs/design-system/` (entregue em 07/10/2026; ver ADR-005 e `docs/consolidacao/decisoes.md` D-005).

Não invente cores, fontes ou tokens fora desse material. Estrutura e comportamento podem ser implementados antes da TASK-029.

## Execução da consolidação

- Baseline de regressão: `docs/consolidacao/baseline.md`.
- Fontes congeladas e inventário de capacidades: `docs/consolidacao/fontes.md`.
- Decisões, conflitos e pendências: `docs/consolidacao/decisoes.md`.

## Segurança

Nunca:
- commitar secret;
- imprimir API key;
- expor segredo via API/frontend;
- relaxar validação para “fazer teste passar”;
- executar `db-reset`, apagar dados/arquivos/volumes ou migration destrutiva sem aprovação;
- executar operação Git remota sem aprovação explícita.

## Definition of Done de uma TASK

- ACs associados atendidos.
- Testes associados passando.
- Lint/typecheck aplicáveis passando.
- OpenAPI/tipos atualizados se necessário.
- Sem regressão detectada.
- Docs/ADR atualizados quando contrato mudou.
- Relatório final da TASK entregue ao usuário.
