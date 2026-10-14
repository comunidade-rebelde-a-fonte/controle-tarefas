# ADR-006 — Parser PGDAS-D determinístico, sem LLM

Status: Accepted (07/10/2026, decisão do usuário: opção A do conflito C-001)

## Contexto

A spec de consolidação descrevia o `parser_pgdas` como agente que extrai campos do PGDAS-D por modelo de linguagem: fluxo F-002 ("Worker resolve alias `parser_pgdas` no LiteLLM"), `prompt-spec` §3, `ai-behavior-spec` §2, a lista de aliases do `CLAUDE.md` e do system design §8, e a variável `PARSER_PGDAS_MODEL`.

No Fator R de origem (commit `9dd7a74`), o parser é determinístico:

- `CLAUDE.md` §3.2: "O parse é determinístico. A extração do PGDAS-D é por regras (regex e heurísticas). O LLM nunca extrai campos de documento na v1."
- `docs/plan.md` §6 e §7.3: no `parser_pgdas`, o `plan` é "sem LLM", o `tool` é o parse e o match de CNPJ, e o `render` é template.
- Código: `parsing/pgdas.py` ("regras e regex, sem LLM"); `agents/parser_pgdas.py` não chama modelo.
- A meta de ≥ 80% de acerto ouro (PRD §7.9) mede esse parser determinístico.

Extrair por LLM seria trocar validação determinística por modelo, o que a regra soberana da consolidação proíbe (01-intake: "não substituir validação determinística por LLM").

## Decisão

1. O parser do PGDAS-D é portado para Go **como é na origem**: as mesmas regras de extração (regex com variantes de rótulo e formato BR), os mesmos campos, a mesma confiança por documento (pesos e bônus de consistência), a mesma `PARSER_VERSION` de referência (`2026.09.3`), os mesmos limites de isolamento (timeout, teto de páginas, texto e memória, no máximo 2 extrações simultâneas) e o mesmo tratamento de PDF sem camada de texto (`needs_review`, sem OCR).
2. `parser_pgdas` continua sendo o **nome do agente** para trace, decisão e avaliação (spans `plan`, `parse`, `decide`, `render`; `record_decision`; eval ouro e humana). **Nenhuma etapa chama modelo**: `plan` é o gatilho de upload e `render` é template.
3. Não existe alias de modelo `parser_pgdas` no LiteLLM, na tabela `ai_model_aliases` nem no `.env` (`PARSER_PGDAS_MODEL` sai da lista).
4. A equivalência é provada pelo golden das 14 fixtures de `fator-r/apps/api/fixtures/pgdas/` (campos, confiança e lacunas conhecidas) com a biblioteca de extração de texto escolhida em Go. Divergência só com decisão registrada.

## Consequências

- Aliases de modelo da consolidação: `triador`, `extrator`, `extrator_forte`, `resolvedor`, `classificador` (D-008), `consultor`, `priorizador`.
- O `prompt-spec` §3 não se aplica a prompt (não há prompt), mas as regras de conteúdo continuam valendo para o código: distinguir campo presente, ausente e "não se aplica"; nunca gerar divisão de folha que o documento não traz.
- O eval-plan §2 continua: golden por documento e meta de ≥ 80% de acerto ouro nos extratos de texto vinculados, agora explicitamente sobre o parser determinístico.
- Usar LLM como apoio para documentos que o parser não lê (opção B do C-001) seria capacidade nova e exige nova ADR.
