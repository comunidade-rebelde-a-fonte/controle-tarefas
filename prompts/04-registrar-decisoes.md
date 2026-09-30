# Prompt 04 — Registrar decisões do escritório (T-000)

O escritório respondeu a uma ou mais perguntas pendentes (Q1–Q8 do `docs/plan.md` §9). Registre as respostas e aplique-as ao projeto seguindo o `AGENTS.md`.

## Entrada (fornecida pelo humano junto com este prompt)

As respostas, em texto livre, por exemplo:

```
Q2: sim, Fechamento fica fora dos percentuais.
Q5: Folha e eSocial = Sem movimento; FGTS e INSS/IRRF = Não se aplica.
Q1: (planilha de prazos anexada/colada)
```

---

## 1. Entender e confirmar

1. Releia o `AGENTS.md` (§2 e §6.6) e as perguntas em `docs/plan.md` §9, e as decisões relacionadas no §2 (R2, R3, R4, R9, D7…).
2. Para cada resposta, escreva em uma linha **como você a interpretou em termos de sistema**. Exemplo:
   > Q5 → `status_when_no_payroll`: PAYROLL_PROCESSING = NO_MOVEMENT, ESOCIAL = NO_MOVEMENT, FGTS = NOT_APPLICABLE, INSS_IRRF = NOT_APPLICABLE.
3. **Se alguma resposta for ambígua, incompleta ou contrariar o PRD, pare e pergunte** antes de alterar qualquer arquivo. Isso vale principalmente para prazos (Q1): nunca complete ou deduza um prazo legal que o escritório não informou.

## 2. Registrar

1. Em `docs/decisoes.md`, na seção de cada pergunta respondida:
   - `**Status:** respondida em DD/MM/AAAA`;
   - `**Resposta do escritório:**` com o texto original, **literal**;
   - `**Interpretação no sistema:**` com a linha confirmada no passo 1.
2. Se a resposta **confirma** a decisão provisória do `plan.md`, troque apenas o marcador "(validar)" daquela decisão por "(confirmado — ver docs/decisoes.md Qn)". Nenhuma outra edição no `plan.md`.
3. Se a resposta **muda** uma decisão do `plan.md` (D*/R*), isso altera o documento de autoridade. Mostre o trecho atual e o trecho proposto e **peça aprovação explícita** antes de editar [AGENTS.md §6.8].

## 3. Aplicar ao código

1. Localize tudo o que depende da pergunta:
   ```bash
   grep -rn "A VALIDAR (Qn)" backend frontend
   ```
   Confira também as linhas correspondentes em `docs/pendencias.md`.
2. Para cada ocorrência:
   - **Tarefa ainda não executada:** nada a mudar no código. Apenas registre em `docs/pendencias.md` que a tarefa já usará o valor definitivo.
   - **Tarefa já concluída:** aplique o valor definitivo:
     - **seeds** (idempotentes): atualize o arquivo de seed e rode `make seed` duas vezes no banco local, para confirmar que aplica o valor novo sem duplicar;
     - **schema:** crie uma **migration nova**; nunca edite uma existente. Valide com `make migrate-down && make migrate`;
     - **regras em código:** ajuste a função e os testes (atualize as expectativas e adicione o caso da decisão);
     - remova o marcador `A VALIDAR (Qn)` e a linha de `docs/pendencias.md`.
3. **Competências já geradas não são reescritas** [RN-004]. Se a mudança afetar vencimentos ou aplicabilidade de instâncias existentes (ex.: novo prazo para a competência aberta), **não** recalcule automaticamente. Meça o impacto com uma consulta **somente leitura** no banco **local** (`make db-psql`), nunca em outro ambiente. Informe quantas instâncias e quais competências são afetadas e pergunte se o humano quer:
   - deixar como está (vale a partir da próxima competência), ou
   - aplicar um ajuste pontual pela funcionalidade de override de vencimento, com motivo "Decisão Qn".

## 4. Verificar e commitar

```bash
make lint
make test
```

Tudo verde. Depois:
- branch `task/T-000-qN-slug` a partir de `develop`;
- commit:
  ```
  T-000: apply office decisions Qx, Qy

  - decisoes.md updated
  - <seeds/migrations/regras alteradas>
  ```
- merge `--no-ff` em `develop`.

Se **todas** as perguntas Q1–Q8 estiverem respondidas e aplicadas, marque `[x]` na T-000 do `docs/tasks.md`.

## 5. Relatório

- Perguntas registradas e a interpretação de cada uma.
- Arquivos alterados e testes afetados.
- Pendências que continuam abertas (perguntas sem resposta).
- Decisões do `plan.md` que aguardam aprovação, se houver.
- Impacto em competências existentes, se houver.
