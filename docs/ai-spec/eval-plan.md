# Eval Plan

## 1. Objetivo

Provar que o porte para Go/LiteLLM/OpenAI preserva ou melhora o comportamento observável sem relaxar guardas.

## 2. Fator R

### Parser PGDAS

O parser é determinístico (ADR-006). O golden compara a implementação Go com o parser de referência do Fator R nas 14 fixtures de `apps/api/fixtures/pgdas/`, campo a campo, com confiança e lacunas conhecidas.

Golden por documento:
- CNPJ;
- PA;
- RBT12;
- FS12;
- Fator R;
- Anexo quando presente.

Preservar meta original de >=80% de acerto ouro para extratos de texto vinculados antes de declarar o parser estável, sem usar essa meta para autorizar gravação insegura.

> **Como foi implementado (TASK-012; D-023).**
> - O golden (`backend/internal/factorr/pgdas/testdata/golden_gen.py`) roda o próprio `parse` da origem sobre as 14 fixtures e sobre 109 variações de borda. Os 122 resultados do porte são idênticos, inclusive a confiança.
> - O porte do `test_fixtures.py` confere cada fixture (campos, lacunas conhecidas, identificação, séries e faixa de confiança). O acerto nas fixtures é 108 de 112 campos (96%); os 4 erros são as lacunas do DAS no layout declaratório.
> - A nota ouro de cada extrato vinculado é gravada em `ai_gold_evals` e vai ao Langfuse como `gold_<campo>` e `gold_acerto` (D-022).

### Evals humanas

`acerto | parcial | erro`, comentário obrigatório em erro.

> **Consultor e priorizador (TASK-013; D-024).** Como na origem, não têm nota ouro. A nota humana vale para as corridas deles (`POST /ai/runs/{trace_id}/evaluation`), registrada no detalhe da corrida em Fator R → Observabilidade (TASK-014b).
> - O que garante a resposta é determinístico: o texto do LLM é validado contra a decisão (golden do `validar_texto` da origem, 40 casos) e, se reprovado, vira o template.
> - A classificação da intenção tem golden das palavras-chave e da validação da saída estruturada.

## 3. Conciliação

### Extrato

Golden set com PDF + JSON esperado:
- saldos;
- transações;
- data/valor/sentido/descrição;
- número de tentativas;
- custo/latência.

Gate soberano: saldo deve fechar R$ 0,00 independentemente do score do modelo.

> **Como foi implementado (TASK-016; D-027).**
> - Golden em `backend/internal/reconciliation/testdata/golden` (convenção da origem): os 2 extratos do cenário, com o gabarito tirado do CSV pelo `extrato_de_csv` da origem.
> - O relatório pula os leitores determinísticos e traz, por documento e no total: aprovação na 1ª tentativa, tentativas, custo, latência e as diferenças para o gabarito (saldos e, linha a linha, data, valor, sentido e descrição normalizada).
> - `make test` roda o relatório com o gateway simulado; `make eval-extrator` roda com o LiteLLM do ambiente (provedor real, só por ação explícita).
> - O schema, o validador e os leitores são travados por golden do Python da origem (256 casos), e o validador por testes de propriedade.

### Comprovantes

Acurácia por campo em tipos previstos pelo projeto original.

> **Como foi implementado (TASK-017; D-028).**
> - Gabarito dos 13 comprovantes do cenário da origem (PIX, boleto e ordem bancária) em `backend/internal/reconciliation/testdata/golden/comprovantes`, com os dados do gerador do cenário.
> - O relatório pula o leitor determinístico e traz, por documento e no total, a acurácia por campo (data, valor, sentido, contraparte normalizada, documento da contraparte e tipo), os lançamentos extraídos e parciais, as chamadas, o custo e a latência.
> - `make test` roda o relatório com o gateway simulado; `make eval-extrator` roda com o LiteLLM do ambiente (provedor real, só por ação explícita).
> - O schema, as regras de campo e o leitor determinístico são travados por golden do Python da origem (113 casos), e o leitor também de ponta a ponta nos 13 PDFs.
> - O mínimo de 5 comprovantes de cada tipo (M3.8 da origem), em PDF e imagem, depende de material real anonimizado, como o golden do extrato.

### Matching

Testes determinísticos por passe, inclusive ambiguidade e casos de borda.

## 4. Regressão de porte

Antes de substituir uma capacidade do projeto auxiliar pela implementação Go:
1. executar fixtures/golden no comportamento de referência quando possível;
2. executar implementação nova;
3. comparar saída normalizada;
4. registrar divergências intencionais somente com decisão documentada.

## 5. Provedor/modelo

Como modelos são configuráveis, evals devem registrar provider/model/version. Troca de modelo não altera critérios determinísticos.

## 6. Suítes

- `make test`: sem LLM real.
- suite marcada `ai-integration`: provider/LiteLLM real, somente por ação explícita.
- golden local: pode usar provider real quando autorizado e credencial disponível.
- Langfuse integration: valida correlação de `trace_id`.

## 7. Critérios de parada

Não promover slice se:
- parser cair abaixo do baseline aceito sem explicação;
- portão de saldo falhar;
- resolvedor persistir conciliação;
- classificação agentic dispensar confirmação indevidamente;
- trace local faltar em decisão persistente.
