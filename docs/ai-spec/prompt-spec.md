# Prompt Spec

## 1. Armazenamento

Prompts de runtime devem ser versionados no repositório, em diretório equivalente a `backend/internal/ai/prompts/` ou estrutura coerente com o código existente.

Cada corrida registra:
- nome lógico do prompt;
- versão;
- hash do conteúdo;
- alias/modelo resolvido.

> **Como foi implementado (TASK-013; D-024).** Os prompts ficam em `backend/internal/ai/prompts/` (pacote `prompts`, embutidos no binário).
> - A referência `nome@versão#hash` vai para `ai_runs.prompt_version` e `metadata.prompts`; o alias e o modelo resolvidos ficam nas colunas da corrida.
> - Um teste fixa o hash de cada versão: mudar o texto exige versão nova.
> - Outro teste confere os itens do contrato comum (§2).

## 2. Contrato comum

Todo prompt deve conter:
1. papel específico do agente;
2. schema de saída;
3. ferramentas permitidas;
4. regras que o agente não pode decidir;
5. política de abstenção;
6. instrução anti-prompt-injection de documento;
7. regra de não inventar campos ausentes;
8. distinção entre dado extraído e inferência.

## 3. Parser PGDAS

Não há prompt: o parser é determinístico (ADR-006). As regras abaixo valem para o código do parser.

Deve extrair somente os campos definidos no PRD de origem e distinguir:
- campo presente;
- campo ausente;
- “não se aplica”;
- confiança.

Nunca gerar split de folha não fornecido pelo documento.

## 4. Extrator de extrato

Deve respeitar o schema congelado. A cada nova tentativa recebe feedback determinístico:
- soma créditos;
- soma débitos;
- saldo calculado;
- saldo esperado;
- diferença;
- lacunas/linhas suspeitas.

Estratégias por tentativa devem preservar o comportamento original: texto nativo → render intermediário → render maior/modelo forte, conforme implementável na stack final.

## 5. Extrator de comprovante

Saída com confiança por campo e evidência/trecho ou bbox quando disponível. Campo incerto deve ser `null`/marcado, nunca inventado.

## 6. Resolvedor

A linguagem deve ser propositiva: “hipótese”, “parece corresponder”, “candidatos”. Não usar linguagem de conclusão quando o sistema ainda exige decisão humana.

## 7. Consultor Fator R

Recebe números calculados e deve explicá-los. Não efetua aritmética normativa como fonte de verdade. Deve manter disclaimer de que PGDAS-D prevalece.

## 8. Priorizador

Só prioriza a carteira fornecida pelas tools/serviços. Não cria empresa nem altera semáforo.

## 9. Testes de prompt

- Mudança de prompt do parser/extrato/comprovante exige golden/eval antes de merge.
- Teste padrão não usa provider real.
- Integração real com provider é suíte marcada/manual.
