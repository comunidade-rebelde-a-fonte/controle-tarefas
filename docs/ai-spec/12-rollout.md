# 12 — Rollout Local e Estratégia de Consolidação

## 1. Princípio

Não existe rollout para usuários externos. O risco principal é regressão funcional durante a consolidação. A estratégia é incremental e reversível no repositório-base.

## 2. Preparação

1. Registrar baseline verde do `controle-tarefas`.
2. Congelar referências dos commits/branches dos três repositórios usados como fonte.
3. Criar migrations somente aditivas no início.
4. Não remover código funcional do Controle para “abrir espaço”.
5. Adicionar serviços auxiliares ao ambiente local sem alterar suas garantias.

## 3. Ordem de ativação

1. Infra compartilhada e settings de IA.
2. Fator R portado e isoladamente funcional.
3. Conciliação portado e isoladamente funcional.
4. Integrações entre módulos.
5. Dashboard/ficha/documentos consolidados.
6. Seed-demo e E2E ponta a ponta.
7. Styling final InovaSala quando o design system estiver disponível.

## 4. Compatibilidade

Os dois repositórios auxiliares permanecem intactos como referência. Não há migração de banco legado nem corte de produção.

## 5. Rollback

Por slice:
- reverter código/migration aditiva não utilizada;
- manter dados existentes do Controle intactos;
- não executar down migration destrutiva sem revisão.

[ROLLBACK] Se uma migration não puder ser revertida sem perda, ela deve ser redesenhada como aditiva antes de execução, salvo aprovação humana explícita.

## 6. Feature flags

Não são obrigatórias para a demo, mas rotas/itens de menu de um módulo incompleto podem ficar ocultos por configuração de desenvolvimento até o slice atingir aceite. Não usar flag para mascarar regra quebrada após considerar o módulo pronto.

## 7. Critério de interrupção

Parar o rollout do próximo marco se:
- suíte do Controle regredir;
- fórmula/golden do módulo portado divergir;
- portão do Conciliador for afrouxado;
- storage perder imutabilidade;
- houver secret no diff/log;
- integração alterar status de obrigação errada;
- migration destrutiva for necessária sem aprovação.
