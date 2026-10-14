# 00 — Índice do Pacote de Especificação

## Projeto

**Controle Integrado — Obrigações, Fator R e Conciliação Contábil**  
Nome provisório; repositório final: `comunidade-rebelde-a-fonte/controle-tarefas`.

## Objetivo

Consolidar os três sistemas existentes em um único produto local, usando a stack de aplicação do `controle-tarefas`, sem remover capacidades dos sistemas de origem e com integração real entre os domínios.

## Status

**Em execução por Claude Code.**

- Design System InovaSala: entregue em 07/10/2026 (`docs/design-system/`, ADR-005). A antiga pendência `[PENDING]` do artefato visual está resolvida.
- Execução, baseline, fontes congeladas e decisões: `docs/consolidacao/`.

## Ordem de leitura para Claude Code

1. `/CLAUDE.md`
2. `00-index.md` (este arquivo)
3. `01-intake.md`
4. `02-prd.md`
5. `03-acceptance-criteria.md`
6. `07-flows.md`
7. `04-system-design.md`
8. `05-data-model.md`
9. `06-api-contract.md`
10. `08-ui-ux-spec.md`
11. `09-security-privacy.md`
12. `10-env-and-secrets.md`
13. `11-observability.md`
14. `ai-behavior-spec.md`
15. `prompt-spec.md`
16. `tool-contract.md`
17. `eval-plan.md`
18. `12-rollout.md`
19. `13-implementation-plan.md`
20. `16-traceability-matrix.md`
21. `14-review-checklist.md`
22. `15-agent-execution-prompt.md`

## Arquivos incluídos

| Arquivo | Finalidade |
|---|---|
| `01-intake.md` | solicitação, decisões, escopo, fontes e pendências |
| `02-prd.md` | produto e requisitos REQ-001 a REQ-052 |
| `03-acceptance-criteria.md` | AC-001 a AC-035, NFRs e DoD |
| `04-system-design.md` | arquitetura consolidada e decisões |
| `05-data-model.md` | modelo relacional e migrations |
| `06-api-contract.md` | contratos de API a adicionar/preservar |
| `07-flows.md` | fluxos e estados críticos |
| `08-ui-ux-spec.md` | navegação, telas e gate InovaSala |
| `09-security-privacy.md` | dados, secrets e operações destrutivas |
| `10-env-and-secrets.md` | inventário de ambiente sem valores reais |
| `11-observability.md` | logs, auditoria local, Langfuse e runbooks |
| `12-rollout.md` | consolidação incremental e rollback |
| `13-implementation-plan.md` | TASK-001 a TASK-030 |
| `14-review-checklist.md` | revisão final |
| `15-agent-execution-prompt.md` | prompt mestre para Claude Code |
| `16-traceability-matrix.md` | REQ → AC → TASK → TEST |
| `ai-behavior-spec.md` | contratos dos agentes |
| `prompt-spec.md` | regras de prompts/versionamento |
| `tool-contract.md` | tools permitidas e limites |
| `eval-plan.md` | golden/evals e gates de qualidade |

## Arquivos de agente

- `CLAUDE.md` — gerado na raiz do pacote.
- `AGENTS.md` — **não gerado**, porque o usuário definiu Claude Code como único agente-alvo. O `AGENTS.md` já existente no repositório-base não deve ser apagado automaticamente; ele continua como referência histórica do sistema atual.

## ADRs

- `ADR-001-aplicacao-base-controle-tarefas.md`
- `ADR-002-preservar-servicos-auxiliares.md`
- `ADR-003-identidade-e-fechamento-canonicos.md`
- `ADR-004-ia-via-litellm-openai-configuravel.md`
- `ADR-006-parser-pgdas-deterministico.md` (07/10/2026)
- `ADR-007-decimal-exato-semantica-python.md` (07/10/2026)
- `ADR-008-matching-ambiguidade-bordas-parciais.md` (09/10/2026)
- `ADR-005-design-system-inovasala.md`

## Decisões principais

1. O repositório-base é `controle-tarefas`.
2. A aplicação usa Go/React/PostgreSQL.
3. O funcionamento de Fator R e Conciliador deve ser preservado integralmente.
4. MinIO, LiteLLM, n8n e Langfuse não são removidos por simplificação.
5. Cadastro de empresa é único.
6. Fechamento Contábil é único e canônico.
7. Provider inicial de IA é OpenAI via LiteLLM; modelo é configurável.
8. LLM interpreta/sugere; código determinístico valida/calcula/persiste.
9. Upload principal da demo é manual e processamento é assíncrono.
10. Design visual final usa InovaSala, sem tokens inventados.

## Riscos principais

- RISK-001: regressão do Controle de Tarefas.
- RISK-002: divergência ao portar Python → Go.
- RISK-003: unificação semântica incorreta de entidades/estados.
- RISK-004: remoção indevida de serviço auxiliar.
- RISK-005: inconsistência trace local/Langfuse.
- RISK-006: chamadas reais de LLM em teste padrão.
- RISK-007: styling sem o material InovaSala.
- RISK-008: stack local pesada; usar comandos/perfis, não remover capacidades.

## Como iniciar

Claude Code deve usar `15-agent-execution-prompt.md`, executar primeiro TASK-001 e não iniciar o porte de domínio antes de registrar o baseline do sistema-base.

## Critério soberano de pronto

O produto só é considerado consolidado quando os três domínios são operáveis em uma única aplicação local, os testes/goldens comprovam equivalência de comportamento, as integrações cruzadas funcionam e nenhuma capacidade foi silenciosamente perdida.
