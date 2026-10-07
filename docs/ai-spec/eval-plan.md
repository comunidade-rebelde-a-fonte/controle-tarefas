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

### Evals humanas

`acerto | parcial | erro`, comentário obrigatório em erro.

## 3. Conciliação

### Extrato

Golden set com PDF + JSON esperado:
- saldos;
- transações;
- data/valor/sentido/descrição;
- número de tentativas;
- custo/latência.

Gate soberano: saldo deve fechar R$ 0,00 independentemente do score do modelo.

### Comprovantes

Acurácia por campo em tipos previstos pelo projeto original.

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
