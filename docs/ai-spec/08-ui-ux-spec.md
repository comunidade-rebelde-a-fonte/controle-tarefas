# 08 — UI/UX Spec

## 1. Direção

Aplicação desktop-first, densa e operacional, com uma única navegação para os três domínios. A UI deve seguir o **Design System InovaSala** fornecido pelo usuário.

[PENDING] O arquivo/anexo oficial do Design System não está disponível no contexto atual. Portanto:

- não inventar paleta;
- não inventar tipografia;
- não inventar tokens de spacing/radius/shadow;
- não substituir pelo visual atual como decisão definitiva;
- implementar estrutura semântica e componentes funcionais;
- bloquear apenas o styling final/QA visual até o material ser disponibilizado.

## 2. Navegação principal

Ordem aprovada:

1. Dashboard
2. Empresas
3. Fiscal
4. Declarações
5. Contábil
6. Folha
7. DCTFWeb
8. Fator R
9. Conciliação
10. Procurações/Certificados
11. Backlog
12. Documentos
13. Relatórios
14. Configurações

A arquitetura visual (sidebar/topbar/submenus) deve ser derivada do Design System InovaSala quando disponível.

## 3. Dashboard

Preservar cards/indicadores do sistema-base e adicionar seções:

### Fator R
- Monitoradas.
- Anexo V / vermelho.
- 28–30% / amarelo.
- Seguras / verde.
- Economia em jogo.
- Ação sugerida/prioridades.

### Conciliação
- Processando.
- Revisão de triagem/extração.
- Extrato sem comprovante.
- Comprovante sem extrato.
- Divergências.
- Competências prontas para fechamento.

Todos com drill-down.

> **Como foi implementado (TASK-024; D-038).** As duas seções ficam abaixo dos cards dos módulos, com os filtros do Dashboard. O Fator R mostra monitoradas, Anexo V, no limite, seguras, dados insuficientes, economia em jogo, as que exigem ação e as cinco prioridades, com a ação sugerida e o link para a aba Fator R. A Conciliação mostra processando, em revisão e parados por período fechado (de todas as competências), a fila da competência por balde, as prontas para concluir e o Contábil bloqueado. Cada número abre a lista com o mesmo total: a carteira com o filtro de semáforo, os documentos, a fila e a tela nova "Empresas" da Conciliação. As telas mostram os filtros que vieram do Dashboard e permitem limpá-los.

## 4. Empresas

Listagem atual preservada. Acrescentar indicadores compactos:
- Fator R atual, quando aplicável.
- estado da conciliação da competência selecionada.
- alertas críticos.

Não sobrecarregar a listagem com todos os dados; detalhes ficam na ficha.

## 5. Ficha da empresa

Cabeçalho único com nome, código, CNPJ, regime e ações rápidas existentes.

Subáreas:
- Visão geral.
- Obrigações.
- Fator R.
- Conciliação.
- Documentos.
- Procurações/Certificados.
- Histórico.

> **Como foi implementado (TASK-021c; D-034).** A ficha ganhou a aba **Conciliação** (`?tab=reconciliation`): a competência com o Contábil aberto ou fechado (a trava), o percentual automático contra a meta, a fila por balde, a classificação, os lançamentos e os extratos que cobrem a competência; rodar a conciliação e os atalhos para as telas da Conciliação já filtradas pela empresa; e as contas bancárias (CC-11), com as lidas nos extratos para cadastrar com um clique.
>
> **Desde a TASK-022 (D-036):** o cartão "Conclusão da conciliação" na mesma aba mostra o estado (em andamento, pronta, concluída), o que falta (com o caminho para a fila), o aviso, a obrigação Conciliação do Contábil, concluir com confirmação e reabrir com motivo.
>
> **Desde a TASK-023 (D-037):** o cartão "Fechamento do Contábil", ao lado, mostra o estado, quem fechou e o histórico (com o motivo das reaberturas), e fecha e reabre com os diálogos da grade do Contábil. Usando a Conciliação, avisa que o Contábil só fecha com a obrigação Conciliação concluída.

### Visão geral

Cards operacionais:
- progresso por módulo;
- próximos vencimentos;
- Fator R atual e tendência;
- estado da conciliação;
- pendências recentes.

## 6. Fator R

### Carteira

Tabela densa com:
- ação sugerida;
- empresa;
- Fator R;
- anexo;
- RBT12;
- FS12;
- reforço mensal;
- economia 12 meses;
- pacote/honorário quando aplicável.

Ordenação padrão original.

### Ficha Fator R

- gráfico/série dos 12+ meses;
- valores do PA;
- janela usada;
- cálculo explicável;
- simulador;
- documentos PGDAS;
- traces/avaliações relacionados.

### Inbox PGDAS

Estados visuais claros:
- recebido;
- processando;
- parsed;
- precisa revisão;
- vinculado;
- rejeitado.

> **Como foi implementado (TASK-014a e TASK-014b; D-018 e D-025).** A área Fator R tem um submenu: Carteira, Inbox PGDAS-D, Agentes e Observabilidade (o menu da origem).
> - **Inbox** (`/factor-r/inbox`): envio por arrastar ou escolher arquivos e acompanhamento de cada documento até o worker terminar.
>   - Estados com ícone e texto: na fila, processando, nova tentativa, falha no processamento, extraído, em revisão, vinculado e rejeitado.
>   - Fila parada (sem worker ativo) aparece como aviso.
>   - O detalhe tem os campos com a confiança, os meses declarados, o original, o trace e as decisões: vincular, rejeitar e cadastrar pelo extrato (formulário canônico de empresa).
> - **Aba Fator R da ficha:** além do resultado, da grade e da configuração (TASK-014a), o simulador com histórico, o consultor da empresa, os extratos vinculados e as corridas da empresa.
> - **Agentes** (`/factor-r/agents`): o priorizador, com a fila do PA, e o consultor. Cada resposta tem o link do trace.
> - **Observabilidade** (`/factor-r/runs`): painel, corridas filtráveis e detalhe da corrida com passos, decisão, nota ouro e nota humana.
> - Disclaimer do PGDAS-D em todas as telas (FR-19).

## 7. Conciliação

### Documentos

Lista com status, tipo, banco, período, empresa e diagnóstico.

### Fila de revisão — tela prioritária

Preservar o objetivo original da UI:
- documento/render à esquerda;
- dados/candidatos/ações à direita;
- filtros por bucket, empresa e competência;
- ações de teclado onde úteis;
- justificativa do resolvedor visível como sugestão, não como fato.

> **Como foi implementado (TASK-021a; D-032).** Área Conciliação no menu, com o submenu Fila de revisão (`/reconciliation`) e Documentos (`/reconciliation/documents`).
> - **Fila:** os baldes com a contagem (revisão, divergências, extrato sem comprovante, comprovante sem extrato, classificação), filtros por empresa e competência, o motivo em texto. O item aberto mostra o original à esquerda e, à direita, os dados, os candidatos e as ações: conciliar (N:1), não há comprovante, desfazer com motivo, corrigir campo, "que documento é esse?", a hipótese do resolvedor e a classificação. J e K passam de item; Esc fecha.
> - **Original:** as páginas do PDF como imagem, com a marcação de cada campo lido (bbox); imagem como está; CSV e TXT como texto.
> - **Documentos:** envio por arrastar ou escolher, acompanhamento até o worker terminar (fila parada e falha com reenvio), lista filtrável e o detalhe com o original, os dados, a importação, os campos e as tentativas.

### Razão

Tabela com filtros e drill-down em cadeia:
`razão → classificação → conciliação → transação → documento`.

> **Como foi implementado (TASK-021b; D-033).** Submenu Lançamentos (`/reconciliation/entries`), Razão (`/reconciliation/ledger`), Plano de contas (`/reconciliation/chart`) e Regras (`/reconciliation/rules`); a empresa escolhida acompanha a troca de seção.
> - **Razão:** filtros por empresa, competência inicial e final e conta (bancária ou contábil), os totais do período e a exportação CSV do filtro. A linha abre o drill-down em diálogo, na cadeia acima, com os caminhos para a linha na fila e para os originais.
> - **Lançamentos:** lista com a origem e o status, formulário e CSV do cliente (tudo ou nada, com o erro de cada linha) e "rodar a conciliação da competência", também na fila.
> - **Plano de contas e regras:** incluir, renomear, desativar e reativar contas, importar e copiar o plano; regras da mais nova para a mais antiga, confirmar, desativar e aplicar às linhas pendentes.

> **Como foi implementado (TASK-021c; D-034).** Submenu Métricas (`/reconciliation/metrics`) e Playbooks (`/reconciliation/playbooks`).
> - **Métricas:** filtros por empresa e competência; o percentual automático contra a meta, os métodos, os baldes (com o caminho para a fila), a leitura dos documentos, as correções, o depois do fechamento (meta zero), o custo dos agentes e os playbooks por banco.
> - **Playbooks:** as versões de cada banco com o estado, os sucessos, as falhas, a taxa e a receita. Somente leitura.

## 8. Documentos consolidados

A área Documentos é um agregador de navegação, não uma promessa de backend de storage único.

Filtros:
- módulo/origem;
- empresa;
- competência/período;
- tipo;
- status;
- banco quando aplicável.

A ação “abrir” leva à experiência específica do módulo.

## 9. Configurações → IA

Tabela de aliases:
- alias/agente;
- provider;
- modelo;
- status;
- versão;
- última alteração.

Edição:
- provider/model string;
- parâmetros permitidos conforme capability;
- botão validar configuração;
- sem campo de API key.

Exibir texto: “Credenciais são configuradas no ambiente e não ficam salvas nesta tela.”

> **Como foi implementado (TASK-008; D-021).** Aba **IA** em Cadastros e Configurações (`/settings?tab=ai`).
> - Cada linha mostra se o modelo é o "padrão do ambiente" ou foi "escolhido aqui".
> - O status é Ativo, Desligado ou Sem modelo.
> - A edição abre em diálogo:
>   - padrão do ambiente ou provedor + modelo;
>   - `temperature` e `max_tokens`;
>   - agente ligado;
>   - "Validar configuração", que lista cada conferência e os parâmetros aceitos pelo modelo.
> - Sem gateway configurado, um aviso explica que só dá para ligar e desligar agentes.

## 10. Estados obrigatórios

Toda tela nova deve prever:
- loading;
- empty;
- success;
- validation error;
- domain error;
- service unavailable;
- processing assíncrono;
- needs review;
- period closed quando aplicável.

## 11. Acessibilidade

- Preservar suporte por teclado nas grades atuais.
- Não usar apenas cor para semáforo/status.
- Foco visível.
- Labels em controles.
- Axe sem violações serious/critical no conjunto E2E definido.

## 12. Responsividade

Prioridade desktop/notebook. Telas de tabela e revisão podem exigir viewport largo; em viewport menor devem degradar de forma utilizável, sem ser requisito de operação mobile completa.

## 13. Critério visual

**AC-035** só pode ser concluído com o Design System InovaSala real disponível. Claude Code deve registrar screenshots/evidência visual após aplicar tokens/componentes oficiais.
