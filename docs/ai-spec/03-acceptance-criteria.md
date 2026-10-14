# 03 — Critérios de Aceite

## Escopo validado

Aplicam-se ao produto consolidado no repositório `controle-tarefas`, com Claude Code como agente de implementação.

## Critérios funcionais

### Fundação

**AC-001 — Aplicação única**  
Dado que a stack local está subida, quando o operador abre a aplicação, então todas as áreas dos três domínios estão disponíveis sob uma única navegação e origem web.

**AC-002 — Empresa canônica**  
Dado uma empresa do seed do Controle de Tarefas, quando Fator R ou Conciliação registram dados, então todos os registros referenciam a mesma `companies.id` e não criam cadastro paralelo.

**AC-003 — Sem login principal**  
Dado o ambiente local, quando o operador abre a aplicação, então não existe tela de login da aplicação principal, e ações auditáveis usam ator local implícito.

**AC-004 — Regressão zero do Controle de Tarefas**  
Dado o baseline anterior à consolidação, quando a suíte existente é executada após cada marco, então os testes e os 11 fluxos E2E atuais continuam passando.

### Fator R

**AC-005 — Cálculo determinístico**  
Dado uma empresa com 12 competências de receita/folha, quando um PA é selecionado, então RBT12, FS12, Fator R, anexo, gaps e metas coincidem com a regra do PRD original e com o golden esperado.

**AC-006 — Janela móvel**  
Dado dois PAs consecutivos, quando o usuário troca o PA, então exatamente o mês correto sai e o novo mês entra na janela, sem edição manual.

**AC-007 — Semáforo**  
Dado um Fator R abaixo de 28%, entre 28% e 30% ou a partir de 30%, quando a carteira é exibida, então a classificação operacional corresponde respectivamente a vermelho, amarelo ou verde e possui indicador não dependente somente de cor.

**AC-008 — PGDAS conhecido**  
Dado um PGDAS-D de CNPJ conhecido, quando o parser o processa com confiança suficiente, então o documento é vinculado e as regras de gravação de PA/série respeitam o PRD original, sem sobrescrever competência já preenchida indevidamente.

**AC-009 — PGDAS desconhecido/revisão**  
Dado CNPJ não encontrado ou confiança abaixo do limiar, quando o parser termina, então o item fica em revisão e nenhuma folha é inventada/persistida automaticamente.

**AC-010 — Simulador**  
Dado dados válidos, quando uma simulação é executada, então retorna um dos vereditos definidos e todos os valores são calculados por código, não pelo LLM.

**AC-011 — Fator R não muda obrigação**  
Dado alteração do semáforo ou conclusão de análise de Fator R, quando o estado muda, então nenhuma obrigação Fiscal é atualizada automaticamente.

**AC-012 — Dashboard Fator R**  
Dado empresas em risco, quando o Dashboard é aberto, então vermelho/amarelo e economia em jogo aparecem com drill-down.

### Conciliação

**AC-013 — Upload e original**  
Dado um extrato/comprovante, quando é enviado manualmente, então o original é preservado no storage correto, hash é calculado, duplicata é tratada conforme o sistema original e um job é enfileirado.

**AC-014 — Processamento assíncrono**  
Dado um upload aceito, quando a requisição termina, então ela não espera a extração completa; a UI exibe o estado do job até sucesso/revisão/falha.

**AC-015 — Portão aritmético**  
Dado um extrato cuja soma não fecha exatamente, quando o extrator termina todas as tentativas, então nenhuma transação do lote é persistida como importada.

**AC-016 — Tolerância zero**  
Dado diferença diferente de R$ 0,00, quando o validador é executado, então `ok=false` para o portão de saldo.

**AC-017 — Playbook**  
Dado um banco com playbook válido, quando novo extrato compatível é processado, então o playbook é usado; se falhar na primeira tentativa de validação, é invalidado e ocorre redescoberta conforme contrato original.

**AC-018 — Matching determinístico**  
Dado candidatos de conciliação, quando os passes 1–4 são executados, então somente soluções não ambíguas autorizadas são persistidas automaticamente.

**AC-019 — Resolvedor não decide**  
Dado item não conciliado, quando o resolvedor propõe pares, então nenhum registro `conciliacao` é criado até ação humana de aceite.

**AC-020 — Classificação**  
Dado lançamento conciliado, quando classificação ocorre, então a precedência é regra → histórico → agente, e sugestão de agente sem base determinística requer confirmação.

**AC-021 — Razão rastreável**  
Dado uma linha do razão, quando o operador abre o drill-down, então consegue chegar à classificação, conciliação, transação e documento original.

**AC-022 — Exportação**  
Dado um razão filtrado, quando exportado, então XLSX e CSV preservam valores/competência/conta/contraparte/origem de forma determinística.

### Integração entre módulos

**AC-023 — Conciliação conclui obrigação**  
Dado que a conciliação de empresa/competência atingiu seu critério de conclusão, quando o evento é confirmado, então a obrigação canônica `Conciliação` no módulo Contábil passa ao status final configurado, com origem de sistema e sem duplicidade.

**AC-024 — Reabertura sincronizada**  
Dado uma conciliação previamente concluída, quando ela é reaberta, então a obrigação `Conciliação` volta a estado não final apropriado e o histórico registra motivo/origem.

**AC-025 — Fechamento trava conciliação**  
Dado o módulo Contábil fechado, quando uma mutação de conciliação/classificação é tentada, então a API rejeita com erro de período fechado até reabertura com motivo.

**AC-026 — Fechamentos independentes preservados**  
Dado fechamento Contábil, quando Fiscal/Folha/Declarações são consultados, então seus estados não são fechados automaticamente.

### IA e observabilidade

**AC-027 — Modelo configurável**  
Dado um alias de agente, quando o operador troca seu modelo em Configurações → IA, então novas corridas usam a nova configuração sem rebuild e sem expor API key.

**AC-028 — OpenAI inicial**  
Dado a configuração inicial válida, quando LiteLLM resolve um alias, então o provider selecionado é OpenAI; a escolha pode ser alterada por configuração suportada sem mudança de domínio.

**AC-029 — Trace local obrigatório**  
Dado corrida de agente que produz efeito persistente, quando ela termina, então existe trace/auditoria local com `trace_id`, agente, entrada sanitizada/metadados, decisão, status, latência e vínculo de domínio.

**AC-030 — Langfuse resiliente**  
Dado Langfuse indisponível, quando um agente executa, então a operação de domínio não falha apenas por isso; o status de sincronização fica registrado e o mecanismo de retry preservado tenta reenviar.

**AC-031 — Evals preservados**  
Dado traces elegíveis de Fator R, quando golden/human eval roda, então scores e comentários seguem o contrato do projeto original e podem ser consultados.

### Demonstração/UI

**AC-032 — Seed demonstrável**  
Dado banco limpo e serviços disponíveis, quando `make seed-demo` e preparação de fixtures são executados, então existem empresas do Controle com cenários coerentes de obrigações, Fator R e Conciliação.

**AC-033 — Navegação consolidada**  
Dado qualquer tela principal, quando o usuário navega pelo menu, então as 14 áreas definidas no PRD estão acessíveis sem abrir outra aplicação.

**AC-034 — Ficha hub**  
Dado uma empresa, quando sua ficha é aberta, então obrigações, Fator R, Conciliação, documentos e histórico estão acessíveis a partir da mesma identidade de empresa.

**AC-035 — Design InovaSala**  
Dado que o arquivo oficial do Design System InovaSala esteja disponível, quando a camada visual final for implementada, então tokens/componentes/tipografia/espaçamento seguem esse material e passam revisão visual. Se o arquivo não estiver disponível, este critério permanece bloqueado e nenhuma paleta/tipografia deve ser inventada.

## Requisitos não funcionais e critérios

**NFR-001 — Volume:** manter operação percebida adequada para aproximadamente 300 empresas e os volumes de demonstração dos módulos.  
**NFR-002 — Integridade:** dinheiro sempre em tipos decimais/inteiros de centavos adequados; nunca `float` em regras financeiras.  
**NFR-003 — Auditabilidade:** toda automação que grava estado relevante deve ser rastreável.  
**NFR-004 — Resiliência:** queda de Langfuse não interrompe domínio; queda de LLM encaminha job para retry/revisão conforme política.  
**NFR-005 — Segurança local:** segredos somente em env/config segura, nunca em repositório/API/UI.  
**NFR-006 — Acessibilidade:** preservar ao menos o baseline WCAG 2.2 A/AA já testado no Controle de Tarefas para componentes críticos.  
**NFR-007 — Testabilidade:** chamadas reais de LLM não rodam na suíte unitária/integração padrão.  
**NFR-008 — Reprodutibilidade:** fixtures/golden sets determinísticos e independentes do dia real quando aplicável.  
**NFR-009 — Compatibilidade:** o OpenAPI continua sendo contrato entre frontend e backend.  
**NFR-010 — Local-only:** serviços de aplicação devem ser acessíveis localmente e não exigir deploy externo.

## Casos de borda obrigatórios

- RBT12 = 0.
- Empresa com menos de 12 meses.
- PGDAS com PA em intervalo válido e inválido entre meses.
- PGDAS com competência já preenchida.
- Extrato com linha perdida e diferença igual a uma transação.
- Erro de separador decimal.
- Ordem de transação com lacuna.
- Transferência entre contas próprias.
- Valor bruto versus líquido.
- PIX sem contraparte.
- Competência versus caixa.
- Pagamento parcelado.
- Estorno/devolução.
- Comprovantes duplicados com layouts diferentes.
- Período contábil fechado.
- Langfuse indisponível.
- LiteLLM/LLM indisponível.
- MinIO indisponível.
- Job excedendo tentativas máximas.
- Troca de modelo durante jobs já em execução: job em andamento mantém configuração resolvida no início; novos jobs usam nova configuração.

## Regressões proibidas

1. Perder qualquer tela/fluxo atual do Controle de Tarefas.
2. Alterar fórmula ou limiares do Fator R sem decisão humana.
3. Permitir que LLM calcule/persista resultado final sem guarda determinística.
4. Relaxar tolerância de saldo do Conciliador.
5. Remover MinIO/LiteLLM/Langfuse/n8n por simplificação arquitetural.
6. Duplicar cadastro de empresa.
7. Criar segundo fechamento Contábil.
8. Expor segredo na UI/log.
9. Rodar teste padrão consumindo API real de LLM.
10. Inventar Design System InovaSala.

## Evidências esperadas

- Saída de `make lint`, `make test`, `make e2e`.
- Testes de integração de migrations em banco vazio.
- Golden report do parser PGDAS.
- Golden report dos extratores do Conciliador.
- Trace IDs locais e correspondentes no Langfuse em teste de integração dedicado.
- Capturas/QA visual das telas quando o Design System estiver disponível.
- Arquivos XLSX/CSV de exportação comparados com fixture esperado.

## Definition of Done

- Todos os critérios aplicáveis acima passam.
- Nenhum critério bloqueado é marcado como concluído.
- Suítes existentes do Controle continuam verdes.
- Golden/evals não apresentam regressão acima do limiar aceito do módulo.
- Não há secrets reais no diff.
- OpenAPI e tipos TypeScript foram regenerados.
- Migrations aplicam do zero.
- Documentação de mudanças está atualizada.
- Claude Code relata arquivos alterados, comandos executados, testes e riscos restantes.
