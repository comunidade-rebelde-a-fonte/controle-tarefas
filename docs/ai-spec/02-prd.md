# 02 — PRD: Sistema Integrado de Operação Contábil

**Nome provisório:** Controle Integrado  
**Repositório:** `controle-tarefas`  
**Status:** especificação para implementação  
**Plataforma:** web local, desktop-first  
**Usuário:** operador único

## 1. Visão

Transformar o `controle-tarefas` no único sistema operacional da demonstração, incorporando integralmente os domínios de Fator R e Conciliação Contábil sem perder as capacidades existentes de nenhum dos três projetos.

O produto final deve apresentar uma única navegação, um único cadastro de empresas, uma única competência operacional e um único dashboard, mas preservar motores, regras, filas, auditoria, documentos e observabilidade específicos de cada domínio.

## 2. Problemas resolvidos

1. Obrigações contábeis/fiscais dispersas em controles mensais.
2. Monitoramento mensal do Fator R em múltiplos CNPJs do Simples Nacional.
3. Extração e conciliação de extratos/comprovantes não estruturados.
4. Falta de uma visão única por empresa e por competência.
5. Duplicação de cadastro entre sistemas especializados.
6. Necessidade de operar e demonstrar IA com auditabilidade e guardas determinísticas.

## 3. Objetivos

- Manter o `controle-tarefas` como fonte única de verdade para empresas e competências.
- Expor Fator R e Conciliação como módulos nativos do mesmo produto.
- Preservar 100% do comportamento funcional definido nos PRDs de origem.
- Integrar alertas e estados entre módulos sem criar automações tributárias indevidas.
- Permitir configurar provedores/modelos de IA por alias sem alteração de código.
- Manter observabilidade local + Langfuse.
- Demonstrar o sistema com dados e documentos fictícios.

## 4. Não objetivos

- [OUT-OF-SCOPE] SaaS e multi-tenant real.
- [OUT-OF-SCOPE] Autenticação da aplicação principal.
- [OUT-OF-SCOPE] Migração de bases reais dos projetos legados.
- [OUT-OF-SCOPE] Portal do cliente.
- [OUT-OF-SCOPE] ERP contábil específico.
- [OUT-OF-SCOPE] Transmissão oficial de obrigações ou PGDAS-D.
- [OUT-OF-SCOPE] LLM efetuando cálculo tributário, validação aritmética, conciliação ambígua, classificação definitiva ou fechamento.

## 5. Requisitos funcionais

### Fundação e identidade

- **REQ-001** — O produto final deve ser executado a partir do repositório `controle-tarefas`.
- **REQ-002** — `companies` do `controle-tarefas` é a entidade canônica de empresa para todos os módulos.
- **REQ-003** — O sistema deve operar sem login da aplicação principal e assumir um único operador local para ações auditáveis.
- **REQ-004** — Todo comportamento atual do Controle de Tarefas deve continuar funcionando sem regressão.
- **REQ-005** — A competência mensal canônica do Controle de Tarefas deve ser usada como referência temporal também por Fator R e Conciliação quando os conceitos coincidirem.

### Controle de obrigações

- **REQ-006** — Preservar cadastro, regimes, módulos, aplicabilidade, catálogo, vencimentos, grades, fechamento/reabertura, dashboard, backlog, credenciais, importação/exportação e relatórios do sistema-base.
- **REQ-007** — As pendências operacionais já registradas em `docs/decisoes.md` continuam pendentes; a consolidação não pode resolvê-las por suposição.

### Fator R

- **REQ-008** — Incorporar cadastro/configuração específica de Fator R por empresa sem duplicar a empresa.
- **REQ-009** — Manter série mensal de receita e folha, com origem e observação.
- **REQ-010** — Manter motor determinístico de RBT12, FS12, Fator R, Anexo III/V, gaps, metas 28%/30%, alíquotas e economia estimada.
- **REQ-011** — Manter carteira com semáforo vermelho `<28%`, amarelo `28%–<30%` e verde `>=30%`.
- **REQ-012** — Manter Inbox PGDAS-D, parser, vínculo por CNPJ, revisão, cadastro assistido a partir do extrato quando aplicável e regras de gravação da série.
- **REQ-013** — Manter simulador de correção, custos e vereditos `ja_na_meta | corrigir | nao_forcar`.
- **REQ-014** — Manter agentes `parser_pgdas`, `consultor` e `priorizador` com saídas estruturadas e traceáveis.
- **REQ-015** — Manter avaliação ouro e humana dos agentes de Fator R.
- **REQ-016** — Fator R nunca deve alterar automaticamente status de obrigação fiscal.
- **REQ-017** — Alertas vermelho/amarelo de Fator R devem aparecer no Dashboard consolidado e na ficha da empresa.

### Conciliação Contábil

- **REQ-018** — Incorporar ingestão manual de extratos e comprovantes, mantendo hash, dedupe e original imutável.
- **REQ-019** — Manter triagem agentic de documento e estados de revisão.
- **REQ-020** — Manter extrator de extrato com até três tentativas, mudança de estratégia e feedback determinístico da diferença de saldo.
- **REQ-021** — O portão aritmético deve manter tolerância R$ 0,00 e impedir persistência de transações quando o saldo não fecha.
- **REQ-022** — Manter playbooks por banco, invalidação e redescoberta.
- **REQ-023** — Manter extração de comprovantes com confiança por campo e revisão de campo de baixa confiança.
- **REQ-024** — Manter cascata de conciliação em quatro passes, com ambiguidade encaminhada à revisão humana.
- **REQ-025** — Manter agente resolvedor apenas como proponente; nunca efetivar conciliação sozinho.
- **REQ-026** — Manter classificação contábil na ordem regra determinística → histórico → sugestão de agente.
- **REQ-027** — Toda correção manual de classificação deve poder originar uma regra, mediante confirmação.
- **REQ-028** — Razão deve continuar sendo projeção derivada, rastreável até conciliação, transação e documento original.
- **REQ-029** — Manter exportação genérica do razão em XLSX e CSV.
- **REQ-030** — Casos de borda conhecidos do Conciliador devem ser sinalizados e nunca “resolvidos” automaticamente por heurística não autorizada.

### Integrações internas

- **REQ-031** — Conclusão da conciliação de empresa/competência deve concluir automaticamente a obrigação canônica `Conciliação` do módulo Contábil, de forma idempotente e auditada.
- **REQ-032** — Reabertura da conciliação deve retirar a conclusão automática da obrigação `Conciliação`, preservando histórico e motivo.
- **REQ-033** — O fechamento canônico da competência contábil é o fechamento do módulo Contábil do Controle de Tarefas.
- **REQ-034** — Fechar o módulo Contábil deve travar alterações de conciliação/classificação da mesma empresa/competência; reabertura exige motivo.
- **REQ-035** — O Dashboard principal deve consolidar KPIs/atenções de obrigações, Fator R e Conciliação.
- **REQ-036** — A ficha da empresa deve centralizar acesso a Dados Gerais, Obrigações, Fator R, Conciliação, Documentos e Histórico.
- **REQ-037** — Deve existir uma área Documentos consolidada que permita navegar pelos documentos de Fator R e Conciliação sem misturar suas regras de storage/estado.

### IA e infraestrutura

- **REQ-038** — LiteLLM deve permanecer como gateway dos agentes configuráveis.
- **REQ-039** — O provedor inicial deve ser OpenAI, sem hardcode de modelo concreto no domínio.
- **REQ-040** — A interface deve permitir escolher o modelo de cada alias/agente e parâmetros permitidos, com persistência local e validação.
- **REQ-041** — Langfuse self-hosted deve permanecer disponível para tracing/evals dos agentes.
- **REQ-042** — A trilha local de auditoria continua sendo fonte de verdade operacional; indisponibilidade do Langfuse não pode interromper o processamento de domínio.
- **REQ-043** — MinIO permanece no fluxo do Conciliador para originais/versionamento conforme o projeto atual.
- **REQ-044** — O storage local existente permanece para documentos que já são mantidos pelo Controle de Tarefas e pelo fluxo local do Fator R; não consolidar storages por conveniência.
- **REQ-045** — n8n permanece na stack conforme o Conciliador atual; o caminho obrigatório da demonstração é upload manual.
- **REQ-046** — Processamentos longos devem usar fila/worker e estados observáveis, sem bloquear requisição HTTP até o fim.
- **REQ-047** — Nenhum segredo real deve ser persistido no banco de configuração ou retornado pela API.

### Demonstração e interface

- **REQ-048** — Os seeds devem usar as empresas fictícias do Controle de Tarefas como base e associar cenários fictícios de Fator R e Conciliação.
- **REQ-049** — Deve haver uma jornada demonstrável ponta a ponta após `make seed-demo` e preparação dos fixtures.
- **REQ-050** — A navegação principal deve conter Dashboard, Empresas, Fiscal, Declarações, Contábil, Folha, DCTFWeb, Fator R, Conciliação, Procurações/Certificados, Backlog, Documentos, Relatórios e Configurações.
- **REQ-051** — A UI final deve seguir o Design System InovaSala fornecido pelo usuário.
- **REQ-052** — Enquanto o arquivo do Design System não estiver disponível, não inventar tokens visuais; estruturar componentes semanticamente e adiar styling final.

## 6. Regras de negócio críticas

### 6.1 Regra de preservação

O PRD de origem de cada módulo é contrato comportamental. Ao portar para Go/React, Claude Code deve criar testes que reproduzam as regras antes de considerar a migração pronta.

### 6.2 Regra de IA

> O agente interpreta. O código determinístico valida, calcula e persiste.

Aplicações práticas:

- Fator R é sempre calculado por código.
- Simulações tributárias são calculadas por código.
- Saldo de extrato é validado por código.
- Matching automático segue regras determinísticas.
- Agente resolvedor apenas sugere.
- Classificação por agente exige confirmação quando não houver regra/histórico suficiente.
- Fechamento nunca é decidido pelo modelo.

### 6.3 Regra de empresa canônica

Não criar `firm`, `cliente`, `company` ou equivalente paralelo para representar a mesma empresa. Extensões específicas devem referenciar `companies.id`.

### 6.4 Regra de fechamento

O estado de fechamento contábil não pode existir duplicado em Conciliação. A Conciliação deriva/consulta o `company_module_period` canônico do módulo Contábil.

### 6.5 Regra de storage

“Sistema único” não significa “um único storage”. Preservar o backend de armazenamento quando ele for parte do comportamento existente:

- documentos/credenciais já locais continuam locais;
- originais da Conciliação continuam no MinIO versionado;
- PGDAS-D mantém retenção e acesso conforme o módulo Fator R, podendo usar volume/diretório dedicado sem migrar para MinIO apenas por uniformidade.

### 6.6 Regra de auditabilidade

Mudanças automáticas entre módulos devem registrar origem explícita (`SYSTEM_INTEGRATION` ou equivalente), timestamp, empresa, competência e evento causador.

## 7. Histórias principais

- Como operador, quero abrir uma empresa e ver todas as dimensões operacionais sem trocar de sistema.
- Como operador, quero ver obrigações atrasadas, Fator R em risco e conciliações pendentes no mesmo Dashboard.
- Como operador, quero subir PGDAS-D e ter a série de Fator R atualizada com as mesmas guardas existentes.
- Como operador, quero subir extrato/comprovantes, acompanhar o processamento assíncrono e revisar somente exceções.
- Como operador, quero que o fim da conciliação reflita automaticamente na obrigação Contábil correspondente.
- Como operador, quero trocar o modelo de um agente sem editar código ou Docker Compose.
- Como operador, quero consultar traces/evals e entender de onde veio uma decisão assistida por IA.
- Como demonstrador, quero carregar fixtures fictícios e percorrer os três domínios numa única aplicação.

## 8. Dashboard consolidado

O Dashboard deve manter os blocos atuais do Controle de Tarefas e acrescentar:

### Fator R
- empresas monitoradas;
- vermelho `<28%`;
- amarelo `28–<30%`;
- verde `>=30%`;
- economia em jogo;
- itens que exigem ação.

### Conciliação
- documentos aguardando processamento;
- falhas/revisão de extração;
- itens em fila de revisão;
- competências prontas para fechamento;
- competências contábeis bloqueadas por conciliação incompleta.

Todo KPI deve permitir drill-down para a lista que o explica.

## 9. Ficha da empresa

Abas/áreas mínimas:

1. Dados gerais.
2. Obrigações/configurações atuais.
3. Histórico de competências.
4. Fator R.
5. Conciliação.
6. Documentos.
7. Procurações/Certificados, quando aplicável.

A UI pode organizar algumas áreas em subnavegação conforme o Design System InovaSala, mas não duplicar páginas com cadastros divergentes.

## 10. Métricas de sucesso

- Zero regressões conhecidas nas suítes do Controle de Tarefas.
- Fator R do golden/demo coincide com cálculo manual esperado.
- Portão do extrato mantém erro pós-fechamento esperado em zero.
- 100% das decisões de agente persistentes possuem trace local; Langfuse recebe/sincroniza traces quando disponível.
- Fluxo demonstrativo dos três módulos executável em uma instalação local.
- Troca de alias/modelo pela UI afeta novas corridas sem rebuild.
- Conciliação concluída/reaberta sincroniza corretamente a obrigação `Conciliação`.

## 11. Critério principal de aceite do produto

A consolidação está aceita quando um operador consegue, numa única instalação e interface:

1. navegar e operar as obrigações mensais existentes;
2. abrir uma empresa Simples e operar integralmente o Fator R;
3. importar/revisar documentos de conciliação e gerar razão;
4. observar os efeitos cruzados entre módulos;
5. fechar/reabrir Contábil com trava coerente na conciliação;
6. consultar dashboard, documentos e histórico consolidados;
7. alterar modelos de IA por configuração;
8. executar a demonstração com dados fictícios sem acessar os sistemas antigos.

## 12. Dependências

- Docker/Docker Compose.
- Go/Node conforme versões do projeto-base.
- PostgreSQL.
- MinIO.
- LiteLLM.
- n8n.
- Langfuse + dependências self-hosted (Postgres, ClickHouse, Redis, MinIO interno do Langfuse).
- Credencial OpenAI fornecida externamente por ambiente; nunca versionada.

## 13. Riscos de produto

- Ver `01-intake.md` e `14-review-checklist.md`.
- A maior fonte de risco é regressão semântica durante o porte Python → Go; a mitigação obrigatória é teste de equivalência/golden antes de remover qualquer dependência do módulo portado.
