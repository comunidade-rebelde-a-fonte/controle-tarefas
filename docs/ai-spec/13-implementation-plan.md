# 13 — Plano de Implementação para Claude Code

## 1. Estratégia

Executar por vertical slices verificáveis. Não iniciar pela reescrita total. O sistema-base deve permanecer executável durante todo o trabalho.

Princípios:
- explorar antes de editar;
- preservar contratos funcionais;
- escrever testes de equivalência antes/junto do porte de regras críticas;
- usar migrations aditivas;
- não executar operações destrutivas sem aprovação;
- atualizar OpenAPI e tipos TS na mesma entrega de contrato;
- manter serviços auxiliares.

## 2. Baseline obrigatório

### TASK-001 — Registrar baseline do Controle de Tarefas

**Objetivo:** provar que o repositório-base está saudável antes da consolidação.  
**Requisitos:** REQ-001, REQ-004.  
**Aceite:** AC-001, AC-004.  
**Ações:**
- ler README, AGENTS atual, PRD, plan, decisões;
- executar comandos existentes de lint/test/build/E2E aplicáveis;
- registrar falhas pré-existentes sem corrigi-las fora do escopo.

**Validação:** TEST-001.

### TASK-002 — Congelar referências dos projetos fonte

**Objetivo:** registrar commit/ref usado de Fator R e Conciliador para comparação de comportamento.  
**Requisitos:** REQ-008 a REQ-030.  
**Validação:** TEST-002: documento de referência com refs e arquivos fonte.

## 3. Marco M1 — Infraestrutura consolidada sem remoção

### TASK-003 — Expandir Docker/Makefile

**Requisitos:** REQ-038 a REQ-046.  
Adicionar/preservar:
- MinIO + init/versionamento do Conciliador;
- LiteLLM;
- n8n;
- worker Go;
- comandos que sobem Langfuse self-hosted existente;
- healthchecks.

Preferir manter Langfuse em compose dedicado se isso preservar melhor o stack atual; `make dev`/comando equivalente pode orquestrar ambos.

**Aceite:** AC-014, AC-028, AC-030.  
**Validação:** TEST-003 — stack local sobe e healthchecks esperados respondem.

### TASK-004 — Adaptar LiteLLM para OpenAI configurável

**Requisitos:** REQ-038, REQ-039, REQ-040.  
- manter aliases existentes;
- adicionar aliases de Fator R (`consultor`, `priorizador`; o `parser_pgdas` é determinístico e não tem alias, ADR-006);
- remover dependência de API key hardcoded por alias;
- aceitar provider/model string configurável;
- não remover compatibilidade que não conflite com a decisão atual.

**Aceite:** AC-027, AC-028.  
**Validação:** TEST-004 — resolução de alias mockada + integração manual opcional.

### TASK-005 — Portar/preservar fila de jobs em PostgreSQL

**Requisitos:** REQ-046.  
**Aceite:** AC-014.  
**Validação:** TEST-005 — concorrência com `SKIP LOCKED`, retry, dead state, idempotência.

## 4. Marco M2 — Dados compartilhados e configurações

### TASK-006 — Migrations de extensões Fator R

**Requisitos:** REQ-002, REQ-008 a REQ-015.  
Implementar entidades de `05-data-model.md` sem tabela de empresa paralela.

**Aceite:** AC-002, AC-005 a AC-010.  
**Validação:** TEST-006 — migrate do zero + constraints + upsert de movimento.

### TASK-007 — Migrations de Conciliação

**Requisitos:** REQ-018 a REQ-030, REQ-033.  
Portar entidades necessárias, substituindo `cliente` por `companies.id` e fechamento próprio pela autoridade Contábil canônica.

**Aceite:** AC-013 a AC-022, AC-025.  
**Validação:** TEST-007 — migrations + FKs + dedupe + período fechado.

### TASK-008 — Configuração de modelos de IA

**Requisitos:** REQ-039, REQ-040, REQ-047.  
Backend/API/UI de aliases sem secrets.

**Aceite:** AC-027, AC-028.  
**Validação:** TEST-008 — atualização versionada e ausência de chave em respostas.

### TASK-009 — Auditoria local de IA + sync status

**Requisitos:** REQ-041, REQ-042.  
**Aceite:** AC-029, AC-030, AC-031.  
**Validação:** TEST-009.

## 5. Marco M3 — Fator R portado integralmente

### TASK-010 — Portar motor de Fator R para Go

**Requisitos:** REQ-009, REQ-010, REQ-011, REQ-013.  
Portar funções puras e tabelas vigentes do projeto fonte; dinheiro em precisão exata. Não introduzir LLM.

**Aceite:** AC-005, AC-006, AC-007, AC-010.  
**Validação:** TEST-010 — golden/manual do projeto de origem + fronteiras 28/30 + RBT12=0.

### TASK-011 — Portar carteira e APIs Fator R

**Requisitos:** REQ-008 a REQ-011.  
**Aceite:** AC-005 a AC-007.  
**Validação:** TEST-011 — integração DB/API/performance.

### TASK-012 — Portar Inbox/Parser PGDAS

**Requisitos:** REQ-012, REQ-014, REQ-015, REQ-046.  
Upload manual → job → parser → revisão/vínculo/gravação segura.

**Aceite:** AC-008, AC-009, AC-014, AC-029 a AC-031.  
**Validação:** TEST-012 — fixtures sintéticas/anonimizadas e estados de revisão.

### TASK-013 — Portar simulador, consultor e priorizador

**Requisitos:** REQ-013, REQ-014, REQ-015.  
**Aceite:** AC-010, AC-029, AC-031.  
**Validação:** TEST-013 — cálculo independente do texto do LLM; saída estruturada mockada.

### TASK-014 — UI completa do Fator R em React

**Requisitos:** REQ-011 a REQ-017, REQ-036, REQ-050, REQ-051, REQ-052.  
Criar carteira, ficha, inbox, simulador e traces na aplicação única.

**Aceite:** AC-007 a AC-012, AC-033 a AC-035.  
**Validação:** TEST-014 — component/E2E; styling final bloqueado se Design System ausente.

## 6. Marco M4 — Conciliação portada integralmente

### TASK-015 — Storage MinIO e ingestão manual

**Requisitos:** REQ-018, REQ-043, REQ-045, REQ-046.  
Preservar hash, versionamento, original imutável e endpoints compatíveis com n8n.

**Aceite:** AC-013, AC-014.  
**Validação:** TEST-015 — upload, duplicata, versionamento, falha de storage.

### TASK-016 — Portar triador e extrator de extrato

**Requisitos:** REQ-019, REQ-020, REQ-021, REQ-022.  
Implementar ferramentas equivalentes em Go/bibliotecas adequadas. Não mudar schema lógico nem 3 tentativas.

**Aceite:** AC-015, AC-016, AC-017.  
**Validação:** TEST-016 — golden + property tests do validador.

> **Como foi implementado (D-027).** Domínio em `backend/internal/reconciliation/statement` e processamento no worker em `backend/internal/reconciliation/pipeline` (jobs `reconciliation_triage` e `statement_extract`).
> - As ferramentas de PDF ficam em `internal/platform/pdftext`: texto no formato do PyMuPDF e render PNG.
> - O schema lógico e as 3 tentativas foram mantidos. A origem não mandava a página renderizada ao modelo; aqui ela vai como imagem.
> - Goldens gerados pelo Python da origem (domínio e texto do PDF), testes de propriedade do validador e integração com Postgres e MinIO reais.
> - O relatório do golden roda em `make eval-extrator`.

### TASK-017 — Portar extração de comprovantes

**Requisitos:** REQ-023, REQ-030.  
**Aceite:** AC-013, AC-020.  
**Validação:** TEST-017 — fixtures por tipo/campo + baixa confiança.

### TASK-018 — Portar matching passes 1–4

**Requisitos:** REQ-024, REQ-030.  
**Aceite:** AC-018.  
**Validação:** TEST-018 — casos determinísticos, ambiguidade, N:1 única.

### TASK-019 — Portar resolvedor de exceções

**Requisitos:** REQ-025, REQ-038 a REQ-042.  
**Aceite:** AC-019, AC-029, AC-030.  
**Validação:** TEST-019 — prova de que nenhum caminho cria conciliação sem ação humana.

### TASK-020 — Portar classificação e razão

**Requisitos:** REQ-026 a REQ-029.  
**Aceite:** AC-020, AC-021, AC-022.  
**Validação:** TEST-020 — precedência, regras aprendidas confirmadas, view/query do razão.

### TASK-021 — UI completa da Conciliação em React

**Requisitos:** REQ-018 a REQ-030, REQ-036, REQ-050 a REQ-052.  
Fila de revisão é a prioridade de UX.

**Aceite:** AC-013 a AC-022, AC-033 a AC-035.  
**Validação:** TEST-021 — E2E de fila + acessibilidade + QA visual pendente do design system.

## 7. Marco M5 — Integrações reais entre módulos

### TASK-022 — Sincronizar conclusão/reabertura da Conciliação com obrigação Contábil

**Requisitos:** REQ-031, REQ-032.  
Usar o serviço existente de obrigação; não escrever status direto no banco.

**Aceite:** AC-023, AC-024.  
**Validação:** TEST-022 — idempotência + histórico + concorrência.

### TASK-023 — Unificar fechamento Contábil e trava

**Requisitos:** REQ-033, REQ-034.  
**Aceite:** AC-025, AC-026.  
**Validação:** TEST-023 — mutações bloqueadas/permitidas após reabertura.

### TASK-024 — Dashboard consolidado

**Requisitos:** REQ-017, REQ-035.  
**Aceite:** AC-012, AC-033.  
**Validação:** TEST-024 — KPIs e drill-down batem com consultas fonte.

### TASK-025 — Ficha da empresa como hub

**Requisitos:** REQ-002, REQ-036.  
**Aceite:** AC-002, AC-034.  
**Validação:** TEST-025 — E2E troca de abas/empresa/competência sem identidade divergente.

### TASK-026 — Documentos consolidados

**Requisitos:** REQ-037, REQ-043, REQ-044.  
Criar agregação sem migrar storages.

**Validação:** TEST-026 — documentos dos módulos aparecem e abrem no detalhe correto.

## 8. Marco M6 — Demonstração, relatórios e acabamento

### TASK-027 — Seed-demo integrado

**Requisitos:** REQ-048, REQ-049.  
Associar cenários fictícios existentes às empresas do Controle.

**Aceite:** AC-032.  
**Validação:** TEST-027 — banco limpo → seed → jornada reproduzível.

### TASK-028 — Exportações XLSX/CSV do razão

**Requisitos:** REQ-029.  
**Aceite:** AC-022.  
**Validação:** TEST-028 — arquivos comparados a fixture.

### TASK-029 — Navegação final e Design System InovaSala

**Requisitos:** REQ-050, REQ-051, REQ-052.  
Implementar tokens/componentes oficiais somente após material disponível.

**Aceite:** AC-033, AC-034, AC-035.  
**Validação:** TEST-029 — screenshots + E2E + axe.

### TASK-030 — E2E consolidado e revisão final

**Requisitos:** REQ-001 a REQ-052.  
**Aceite:** AC-001 a AC-035.  
**Validação:** TEST-030 — `make lint`, `make test`, `make e2e`, golden/evals aplicáveis e review checklist.

## 9. Paralelização

Pode ocorrer após M2:
- parte do Fator R e parte da Conciliação podem ser portadas em paralelo conceitualmente, mas Claude Code deve executar uma tarefa de cada vez para manter contexto/testes claros.
- UI de um módulo só começa quando seus contratos/API básicos estiverem estáveis.

## 10. Critérios de parada para Claude Code

Parar e pedir decisão humana se:
- for necessário alterar fórmula/limiar/regra tributária;
- houver conflito entre PRDs de origem não resolvido por esta spec;
- for necessário remover MinIO/LiteLLM/Langfuse/n8n;
- uma migration destrutiva parecer necessária;
- precisar executar `db-reset` em dados não descartáveis;
- precisar apagar arquivo/original/volume;
- precisar fazer operação Git remota;
- o Design System InovaSala não estiver disponível no momento de finalizar styling;
- uma ferramenta/biblioteca Go não conseguir preservar contrato crítico e exigir mudança de comportamento.

## 11. Definition of Done final

- TASK-001 a TASK-031 concluídas ou explicitamente dispensadas por decisão humana.
- TEST-001 a TEST-031 com evidência aplicável.
- AC-001 a AC-035 atendidos; AC-035 não pode ser falsamente marcado se o anexo visual continuar ausente.
- Regressões proibidas inexistentes.
- Documentação atualizada.

## 12. Adendo de 07/10/2026 — capacidades incluídas (D-011)

O inventário da TASK-002 (`docs/consolidacao/fontes.md` §4) encontrou capacidades da origem que este plano não citava. Por decisão do usuário (D-011), todas entram no escopo. Cada uma passa a fazer parte do aceite da TASK indicada.

| ID | Capacidade | TASK |
|---|---|---|
| FR-03 | Grade mensal: nas linhas `origem=pgdas`, célula única "Folha declarada no PGDAS-D" com "Detalhar folha" | 014 |
| FR-07 | Carteira: ação sugerida por linha e coluna "Aumento de folha por mês" rotulada (28% e meta), com a meta vinda da política | 011, 014 |
| FR-15 | Rotina semanal do priorizador (todo domingo, gatilho `rotina`, sem LLM no `plan`), agendada pelo worker | 013 |
| FR-19 | Disclaimer "o PGDAS-D da Receita Federal prevalece" em todas as telas do Fator R e em toda resposta de agente | 013, 014 |
| FR-21 | Backup e restauração de todos os armazenamentos | 031 |
| CC-08 | Importação determinística de extrato (CSV/TXT e texto nativo do PDF), paralela ao agente e sujeita ao mesmo portão | 016 |
| CC-09 | Tela de playbooks (versão, validade, taxa de sucesso por banco) | 021 |
| CC-12 | Lançamentos do cliente inseridos à mão (formulário + importação CSV) | 017 (API), 021 (UI) |
| CC-22 | Painel de métricas da Conciliação | 021 (tela), 024 (KPIs no Dashboard) |

Fica de fora, também por decisão do usuário (D-012): o agente `echo` do Fator R (ferramenta de desenvolvimento). O tracer é coberto por testes automatizados na TASK-009.

`consultar_cnpj` (CC-15) nunca foi implementada na origem; a forma de implementação sem chamada externa nova (tool-contract §2) é decidida na TASK-017.

### TASK-031 — Execução local completa e backup de todos os armazenamentos

**Marco:** M6, executada antes da TASK-030.  
**Requisitos:** REQ-001, REQ-004, REQ-006, REQ-043, REQ-044, REQ-045.  
**Motivo:** o `docker-compose.prod.yml` do projeto-base sobe só `app` + `db`, e o `scripts/backup.sh` cobre só o banco e os arquivos de procurações. O produto consolidado depende também de worker, MinIO, LiteLLM, n8n e Langfuse; o backup completo é capacidade do Fator R (FR-21, origem T-701).  
**Fazer:**
- execução local fora do modo dev (compose de execução) com todos os serviços, preservando o que o compose atual garante (Postgres sem porta publicada, imagem única, usuário não-root);
- backup e restauração do banco da aplicação, dos arquivos locais (procurações e PGDAS-D), do bucket MinIO versionado (com versões) e do Langfuse (Postgres e ClickHouse), com somas SHA-256 e retenção;
- teste de restauração em ambiente descartável;
- `docs/operacao.md` atualizado.

**Aceite:** a instalação local sobe com todos os serviços; um backup completo é restaurado em ambiente limpo e as contagens conferem.  
**Validação:** TEST-031.

## 13. Adendo de 07/10/2026 — fatia vertical do Fator R (D-016)

Para mostrar cedo a tela de outro sistema, o Fator R sem IA foi adiantado e validado como uma fatia única: TASK-006a → TASK-010 → TASK-011 → TASK-014a. Um commit local por TASK, depois da validação. As TASKs 006 e 014 foram divididas assim:

| Parte | Escopo | Quando |
|---|---|---|
| TASK-006a | `company_factor_r_settings`, `factor_r_movements`, `factor_r_policy`, `factor_r_simples_tables` (+ seed das tabelas), tipo decimal exato (ADR-007) | fatia |
| TASK-006b | `factor_r_pgdas_documents` (FK de `factor_r_movements.pgdas_document_id`), `factor_r_simulations` | `factor_r_pgdas_documents` com a TASK-012 (D-023) e `factor_r_simulations` com a TASK-013 (D-024) |
| TASK-014a | Carteira (menu "Fator R"), aba "Fator R" da ficha (resultado do PA, cálculo explicável, janela, grade mensal com FR-03, configuração), FR-07, FR-19 | fatia |
| TASK-014b | Inbox PGDAS-D, simulador (exige simulação persistida com trace), traces e avaliações | depois da TASK-012/013, com os agentes e a observabilidade (D-025) |

"Sujeita ao Fator R" é o `companies.factor_r` do cadastro (D-017), e a carteira só traz empresas do Simples Nacional no PA. O motor do simulador (REQ-013) já foi portado e provado na TASK-010; só a API e a tela ficam para a TASK-013/014b.
