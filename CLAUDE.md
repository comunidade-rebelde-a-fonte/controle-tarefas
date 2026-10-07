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
- `parser_pgdas`
- `consultor`
- `priorizador`
- `classificador` quando necessário

Provider inicial: OpenAI, mas modelo concreto não é hardcoded.

Testes normais nunca chamam provider real.

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
