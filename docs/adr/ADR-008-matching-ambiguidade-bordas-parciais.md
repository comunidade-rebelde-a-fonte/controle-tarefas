# ADR-008 — Matching da Conciliação: ambiguidade dos dois lados, bordas fora do automático e parciais só depois da revisão

Status: Accepted (09/10/2026, decisão do usuário na TASK-018)

## Contexto

"Passes 1–4" são contrato congelado da consolidação (04-system-design §10): mudança exige ADR e aprovação humana. No porte do matching do Conciliador (commit `2bc6e26`; `app/domain/conciliacao.py`, `app/domain/bordas.py`, `app/servicos/matching.py`) apareceram três pontos em que o código da origem deixa o automático decidir sem certeza, contra as regras que a própria origem e a spec declaram:

- **Ambiguidade de um lado só.** A `cascata` olha os candidatos de cada linha do extrato, na ordem do extrato. Um comprovante que serve igualmente a duas linhas (mesmo valor, datas na janela) vai para a primeira da ordem, sem olhar a contraparte. A spec diz que só soluções não ambíguas são persistidas (AC-018, REQ-024) e a origem, que ambiguidade vira divergência, "não chute" (AGENTS.md).
- **Casos de borda sem uso.** `marcar_transferencia_interna`, `marcar_estorno` e `marcar_bruto_liquido` existem, mas nada as chama. O PRD §12 da origem diz que nenhum caso de borda é resolvido pelo matching automático; a M4.6, que eles são sinalizados, não resolvidos.
- **Lançamento parcial no automático.** O `rodar_matching` usa todo lançamento com data, valor e sentido, inclusive o parcial (campo essencial abaixo do limiar). O contrato da origem diz que campo abaixo do limiar não entra no razão (M3.4, AGENTS.md).

## Decisão

1. **Ambiguidade dos dois lados.** Nos passes 1 a 3, a linha concilia só quando tem um único comprovante candidato **e** esse comprovante tem só essa linha como candidata. No passe 4 (N:1), a linha concilia só com solução única **e** se nenhum lançamento da solução entra em solução de outra linha. Os candidatos são calculados no início de cada passe; a ordem do extrato não decide nada. A disputa vira divergência do passe (`ambiguo_passe_N`), com as linhas concorrentes; o passe seguinte ainda pode resolvê-la (o passe 3, pelo nome da contraparte). Os critérios de cada passe (mesma data; D±3; Jaro-Winkler ≥ `LIMIAR_FUZZY`; de 2 a 8 lançamentos em D±3) não mudam.
2. **Casos de borda marcados e fora do automático.** Antes da cascata, as linhas da competência envolvidas em transferência entre contas (par C/D de mesmo valor no mesmo dia), estorno (par de mesmo valor e sentidos opostos em até 7 dias, olhando também as linhas vizinhas e as já conciliadas) ou bruto × líquido (comprovante maior que a linha em até R$ 20,00) saem do automático e vão à fila com o motivo e os relacionados. O bruto × líquido só vale em D±3 (a função da origem não olhava a data); o comprovante dele também sai do automático.
3. **Parciais só depois da revisão.** Entram no automático os lançamentos `extraido` e `manual`. O `parcial` espera a correção campo a campo na fila (TASK-021) e conta no balde "aguardando revisão".

O resto do `rodar_matching` é o da origem: o pré-filtro `_ids_divergentes` (comprovante com juros, ISS retido ou parcelado; nota fiscal repetida; valor duplicado; contraparte repetida) e os quatro passes, com os scores e os métodos dela.

## Consequências

- Sem disputa, o resultado é idêntico ao da origem: provado no golden gerado pelo Python da origem (os casos sem disputa saem iguais; os com disputa, mútuos) e no cenário Horizonte inteiro, que dá os mesmos 25 casamentos, as mesmas 14 linhas tiradas pelo pré-filtro e a mesma disputa da Escola Crescer.
- Mais itens podem ir à fila: o comprovante disputado não é dado a ninguém, e a coincidência de valor de um estorno tira as duas linhas do automático. Em troca, nenhum casamento é decidido pela ordem do extrato nem por coincidência de valor.
- A regra da origem continua implementada (`matching.Options.OneSided`) só para o golden, que prova a fidelidade do porte.
- Mudar qualquer um dos três pontos exige nova ADR.
