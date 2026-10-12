# CLAUDE.md — Controle Integrado

## Contexto do projeto

Este repositório parte do `controle-tarefas` e consolida integralmente:
- Controle de Obrigações;
- Plataforma Fator R;
- Vera / Conciliação Contábil.

A aplicação-base é Go + React/TypeScript/Vite + PostgreSQL. Fator R e Conciliação devem ser portados para essa stack de aplicação, sem perda de comportamento. Serviços auxiliares dos sistemas de origem permanecem quando fazem parte do funcionamento: MinIO, LiteLLM, n8n e Langfuse.

## Fonte de especificação

Comece sempre em:
- `docs/ai-spec/00-index.md`

Para o comportamento atual do sistema-base, consulte também os documentos existentes do repositório. Não apague `AGENTS.md` existente apenas porque o agente-alvo desta consolidação é Claude Code.

### Prioridade de instruções

1. instrução explícita atual do usuário;
2. `docs/ai-spec/*` desta consolidação;
3. este `CLAUDE.md`;
4. documentos existentes do `controle-tarefas` para comportamento não alterado;
5. PRDs/planos dos repositórios Fator R e Conciliador como referência de comportamento portado.

Em conflito material, pare e reporte antes de alterar regra de negócio.

## Workflow obrigatório

1. Explore o código antes de editar.
2. Leia a próxima TASK no implementation plan.
3. Liste requisitos, ACs e testes.
4. Faça plano curto.
5. Implemente uma vertical slice.
6. Rode validações.
7. Compare com review checklist.
8. Reporte evidências e riscos.

## Comandos base

Confirme no Makefile real antes de usar. O baseline atual inclui comandos equivalentes a:
- `make dev`
- `make migrate`
- `make seed`
- `make seed-demo`
- `make gen`
- `make lint`
- `make test`
- `make e2e`

Desde a TASK-003: `make dev` também sobe worker, MinIO, LiteLLM e n8n; `make langfuse-up` / `make dev-all` sobem o Langfuse (compose dedicado); `make health` mostra a saúde de todos os serviços.

Não invente comando que não existe: crie/atualize Makefile quando a TASK exigir.

## Regras de engenharia

- OpenAPI 3.1 é o contrato frontend/backend.
- SQL via padrões existentes (`pgx/sqlc/goose`).
- Dinheiro nunca em `float`.
- Competência segue padrão existente.
- Reutilize serviços existentes antes de criar duplicatas.
- Não escrever diretamente em tabelas de obrigação quando existir serviço de domínio que preserva histórico/regras.
- Jobs assíncronos usam fila PostgreSQL e worker.
- LLM via LiteLLM; provider/modelo por alias/configuração.
- Trace local é obrigatório; Langfuse complementa.

## Regras de negócio invioláveis

- `companies` é cadastro canônico.
- Fator R é calculado por código.
- Corte legal 28%; meta operacional 30% até decisão humana em contrário.
- Fator R não conclui obrigação Fiscal automaticamente.
- Extrato só importa se saldo fechar exatamente.
- Resolvedor apenas sugere.
- Matching ambíguo não concilia automaticamente.
- Classificação: regra → histórico → agente.
- Razão é projeção derivada.
- Fechamento Contábil do Controle é autoridade única e trava Conciliação.
- Conciliação concluída/reaberta sincroniza a obrigação `Conciliação` via serviço de domínio.
- Originais do Conciliador ficam no MinIO versionado; não deletar.

## IA

Aliases esperados:
- `triador`
- `extrator`
- `extrator_forte`
- `resolvedor`
- `consultor`
- `priorizador`
- `classificador` quando necessário

`parser_pgdas` é agente **determinístico**: regex, sem chamada de modelo e sem alias no LiteLLM (ADR-006).

Provider inicial: OpenAI, mas modelo concreto não é hardcoded.

Desde a TASK-004 (D-014): catálogo de aliases e resolução em `backend/internal/ai`; cliente do gateway em `backend/internal/ai/litellm`. O app resolve o alias no início da corrida e chama o gateway com o modelo concreto. A lista de aliases é conferida contra `litellm/config.yaml`, `docker-compose.yml` e `.env.example` por teste.

Testes normais nunca chamam provider real. `make test-litellm` usa o LiteLLM real com provedor simulado.

Desde a TASK-008 (D-021): a configuração da tela Configurações → IA fica em `ai_model_aliases` (pacote `backend/internal/ai/aliases`), sem nenhum segredo. Toda corrida de agente resolve o modelo com `aliases.Resolver.Resolve` no início e grava o snapshot. Alias novo exige linha nova numa migration (um teste confere tabela × catálogo).

## Auditoria de IA

Desde a TASK-009 (D-022): trilha local em `backend/internal/ai/tracer`, porte do `tracer.run` do Fator R.
- Toda corrida de agente usa `tracer.Do`, numa requisição, ou `Tracer.Begin` + `Run.Finish` na transação do job, devolvendo `Run.Fail(err)` em caso de erro.
- Decisão de agente só por `Run.RecordDecision`. Tabela de domínio escrita por agente referencia `ai_runs(trace_id)`.
- Spans só `plan|parse|tool|decide|render`, mais `Run.Generation` para as chamadas ao LLM. Nunca o documento inteiro em entrada ou saída.
- O envio ao Langfuse é do worker (job `langfuse_sync`); a API de consulta está em `backend/internal/ai/runs`.

## Jobs

Desde a TASK-005 (D-019): fila em `backend/internal/jobs` (SKIP LOCKED, tentativas, posse com prazo, efeito do handler na mesma transação da conclusão). Cada novo tipo de job registra o handler em `backend/internal/worker/handlers.go`; payload só com IDs, nunca documento.

## Conciliação

Desde a TASK-007 (D-020): schema em `backend/migrations/0020_reconciliation.sql` (mapeamento origem → consolidado na D-020); chave de dedupe e trava pelo fechamento Contábil em `backend/internal/reconciliation`. Toda mutação da Conciliação chama `reconciliation.RequireOpen` (AC-025).

Ingestão (TASK-015, D-026), em `backend/internal/reconciliation/documents`:
- original no MinIO pelo cliente `internal/platform/objectstore` (SigV4 só com a biblioteca padrão; sem operação de apagar), com a chave `originais/<sha256>` e a versão guardada no documento;
- arquivo repetido: 409 `DUPLICATE_DOCUMENT`; MinIO fora: 503 sem gravar nada; documento tardio com o Contábil fechado: `pendencia_periodo_fechado`, sem job;
- o documento nasce `pendente` com o job `reconciliation_triage` na mesma transação;
- testes com MinIO real: `testutil.NewMinIO` (mesma imagem e mesmo `init.sh` do compose).

Triagem e extração (TASK-016, D-027):
- domínio em `backend/internal/reconciliation/statement`: schema e validação do pydantic da origem, validador (portão R$ 0,00 e `validar_saldo`), dicas e leitores determinísticos (CSV, TXT, texto do PDF), travados por golden gerado pelo Python da origem (`testdata/statement_gen.py`);
- worker em `backend/internal/reconciliation/pipeline`: jobs `reconciliation_triage` (leitor do PDF e triador) e `statement_extract` (leitor do CSV ou extrator com até 3 tentativas e playbook); a importação é atômica e só existe com o portão aprovado;
- texto e render de PDF em `internal/platform/pdftext` (`Pages` no formato do PyMuPDF, `Render` em PNG), com golden do PyMuPDF (`testdata/mupdf_gen.py`);
- páginas renderizadas vão ao modelo como imagem (`litellm.Message.Images`);
- prompts em `internal/ai/prompts/conciliacao/`;
- `make eval-extrator` avalia o extrator contra o golden com o LiteLLM real (custo de provedor: só com autorização).

Comprovantes e lançamentos (TASK-017, D-028):
- domínio em `backend/internal/reconciliation/receipt`: schema do `ComprovanteExtraido`, `campos_abaixo_limiar`, regras de campo (M3.4) e o leitor determinístico, travados por golden gerado pelo Python da origem (`testdata/receipt_gen.py`); a validação no estilo do pydantic fica em `internal/platform/pymodel`;
- worker: o leitor de comprovante roda no job de triagem (só com a empresa do documento achada no comprovante); o job `receipt_extract` lê o texto e, com campo fraco ou sem texto, a imagem; campo essencial abaixo de `LIMIAR_CAMPO` deixa o lançamento `parcial`, fora do razão;
- um lançamento por comprovante, com confiança, trecho e bbox por campo, marcas de borda e a duplicata de leiaute diferente sinalizada (nunca descartada);
- lançamentos manuais (formulário e CSV, tudo ou nada) em `backend/internal/reconciliation/entries`, com a trava do Contábil;
- `consultar_cnpj` não existe: aguarda decisão (P-007).

Matching (TASK-018, D-029, ADR-008):
- domínio em `backend/internal/reconciliation/matching`: Jaro-Winkler, cascata dos passes 1 a 4, pré-filtro e bordas da origem e `buscar_candidatos`, travados por golden gerado pelo Python da origem (`testdata/matching_gen.py`);
- regra dos dois lados (ADR-008): só concilia o que não é ambíguo nem para a linha nem para o comprovante; casos de borda vão à fila marcados; parcial fica fora do automático até a revisão. A regra da origem (`Options.OneSided`) é só para o golden;
- rodada em `backend/internal/reconciliation/reconcile`: `POST /companies/{id}/reconciliation/run?competence=` e o job `reconciliation_run` (o worker enfileira ao importar extrato ou gravar comprovante, via `reconciliation.EnqueueRun`); idempotente, serializada por empresa, com a trava do Contábil;
- conciliação automática com origem `auto` e sem autor; marcas da fila em `statement_transactions.flags` (`divergencia` ou `borda`), trocadas a cada rodada.

Resolvedor de exceções (TASK-019, D-030):
- domínio em `backend/internal/reconciliation/resolver`: schema do `HipoteseResolvedor` (golden da origem, `testdata/resolver_gen.py`), contexto do modelo e plano do aceite;
- agente, API e aceite em `backend/internal/reconciliation/proposals`: `POST /reconciliation/review/{id}/proposal` enfileira o job `agent_resolver`; o agente grava só a proposta, com a corrida no tracer;
- só o aceite humano (`accept-proposal`) cria conciliação: origem `manual`, método `resolvedor` ou `sem_comprovante`, autor e proposta, só com linhas e lançamentos livres da empresa. Nunca chame `InsertManualReconciliation` ou `InsertAutoReconciliation` de outro lugar (o TEST-019 confere no código);
- `RESOLVEDOR_AUTOMATICO` (padrão `false`): a rodada pede hipótese uma só vez para cada linha que sobrou.

Classificação e razão (TASK-020, D-031):
- domínio em `backend/internal/reconciliation/classification`: `classificar` (regra → histórico → agente), regra derivada da correção, `re.escape` e o schema da sugestão, travados por golden do Python da origem (`testdata/classification_gen.py`);
- a classificação fica na linha do extrato conciliada (uma conta por linha do razão); regra e histórico rodam ao conciliar (`classify.Competence` na rodada, `classify.Lines` no aceite) e só preenchem o que não foi confirmado. Toda conciliação nova feita à mão deve passar por `classify.Lines`;
- plano de contas e regras em `backend/internal/reconciliation/chart` (a regra mais nova vale; a do mesmo tipo e padrão substitui a antiga; regex só pelo `pytext`); correção, sugestão e agente `classificador` (job `agent_classifier`, só sugere) em `backend/internal/reconciliation/classify`;
- a correção e o aceite só **propõem** a regra; ela vale quando confirmada em `POST /companies/{id}/classification-rules`;
- razão em `backend/internal/reconciliation/ledger`: consulta sobre as conciliações ativas (nunca tabela), drill-down em `GET /reconciliation/reconciliations/{id}` e CSV; o XLSX é da TASK-028;
- `CLASSIFICADOR_AUTOMATICO` (padrão `false`): a rodada pede sugestão de conta uma só vez por linha.

Fila de revisão e documentos (TASK-021a, D-032; a TASK-021 vai em três partes: 021a, 021b e 021c):
- API da fila em `backend/internal/reconciliation/review`: baldes (revisão, divergência, extrato sem comprovante, comprovante sem extrato, classificação), a linha com os candidatos da origem, conciliar à mão (N:1), sem comprovante, desfazer (também a automática, com motivo; a linha ganha a marca `desfeita` e sai do automático) e a correção campo a campo do comprovante (roda a competência de novo);
- "que documento é esse?" (`POST /reconciliation/documents/{id}/identify`) e o original como imagem (`.../pages/{page}`, PNG no DPI do extrator, para o bbox dos campos) em `documents`;
- telas em `frontend/src/features/reconciliation` (`review/`, `documents/`), área Conciliação no menu com submenu;
- `make e2e` sobe um MinIO só do E2E (projeto compose `controle-e2e`, porta 9102): nunca use o MinIO de dev em teste.

Lançamentos, razão, plano de contas e regras (TASK-021b, D-033), só no frontend:
- telas em `frontend/src/features/reconciliation` (`entries/`, `ledger/`, `chart/`, `rules/`), no submenu da Conciliação; a empresa escolhida (`company_id`) acompanha a troca de seção;
- "rodar a conciliação da competência" (`RunReconciliation` em `common.tsx`) fica nos lançamentos e na fila;
- o drill-down do razão (AC-021) é o diálogo `Trail` em `ledger/LedgerPage.tsx`, sobre `GET /reconciliation/reconciliations/{id}`;
- no E2E, o extrato do Banco A Fonte é do TEST-021a e o do Banco Beta é do TEST-021b: o mesmo arquivo (sha256) é recusado e as transações repetidas são ignoradas (a chave de dedupe é única no sistema). Teste novo não reusa extrato do cenário.

Métricas, playbooks, aba da ficha e contas bancárias (TASK-021c, D-034):
- painel do 11 §7, playbooks e o resumo da ficha (`GET /companies/{id}/reconciliation?competence=`) em `backend/internal/reconciliation/metrics`: consultas sobre as tabelas da Conciliação, sem tabela de métrica; a meta da automática vem de `META_CONCILIACAO_AUTO` (80, da origem); os itens por balde usam `review.Counts`, a mesma contagem da fila;
- contas bancárias (CC-11) em `backend/internal/reconciliation/bankaccount`: a conta cadastrada decide a empresa do extrato (D-027); banco, agência e conta precisam ser os que a leitura grava, então a aba oferece as contas lidas nos extratos;
- telas em `frontend/src/features/reconciliation` (`metrics/`, `playbooks/`, `company/`); a aba Conciliação da ficha é `company/ReconciliationTab.tsx`.

Conclusão da Conciliação e a obrigação Conciliação (TASK-022, D-036):
- `backend/internal/reconciliation/completion`: pronta (o extrato todo resolvido), concluir (o contador confirma; a obrigação `ACCOUNTING_RECONCILIATION` vai para OK) e reabrir com motivo (volta para Pendente, se ninguém a mudou depois);
- a obrigação muda só por `obligation.ApplyIntegrationStatus` (as regras do `ChangeStatus`, origem `SYSTEM_INTEGRATION`, evento no histórico); nunca escreva status de obrigação direto no banco;
- concluída, a competência trava: `reconciliation.RequireOpen` devolve 423 `RECONCILIATION_COMPLETED`, `LockReason` manda o documento novo para pendência e os jobs ignoram a competência travada com `IsLocked`;
- a conclusão pega a trava exclusiva da empresa e `RequireOpen` a compartilhada: toda mutação nova da Conciliação passa por `RequireOpen` dentro da sua transação.

Fechamento do Contábil e a trava da Conciliação (TASK-023, D-037):
- o fechamento é o do Controle (`period.Service.Close`); quando a empresa usa a Conciliação na competência (extrato que a cobre ou documento dela), o Contábil só fecha com a obrigação Conciliação final ou não aplicável (422 `RECONCILIATION_NOT_FINAL`); sem Conciliação, fecha como sempre;
- o fechamento pega a mesma trava exclusiva da conclusão: não crie outro caminho de fechamento do Contábil fora do `period.Service`;
- reaberto o Contábil, o documento tardio segue em `pendencia_periodo_fechado` até alguém reprocessar;
- na ficha, o cartão "Fechamento do Contábil" (`company/AccountingClosure.tsx`) usa os diálogos e os endpoints da grade, sobre `GET /companies/{id}/reconciliation/closure`.

## Fator R

Fatia da TASK-006a/010/011/014a (D-016 a D-018):
- motor em `backend/internal/factorr/engine`: porte fiel do `fator_r/domain` da origem, travado por um golden gerado pelo próprio Python da origem (`testdata/golden_gen.py`);
- API em `backend/internal/factorr`; dinheiro em `internal/platform/decimal` (ADR-007);
- "sujeita ao Fator R" é o `companies.factor_r` (D-017);
- telas do inbox, do simulador, dos agentes e da observabilidade na TASK-014b (D-025): submenu da área Fator R e rotas em `frontend/src/features/factor-r` (`inbox/`, `simulator/`, `agents/`, `runs/`);
- o E2E sobe o worker e um gateway LiteLLM simulado que responde os agentes de forma determinística (`e2e/support/fake-litellm.mjs`); nunca provedor real.

Inbox PGDAS-D (TASK-012, D-023):
- parser em `backend/internal/factorr/pgdas`: porte fiel de `parsing/pgdas.py`, travado por golden gerado pelo próprio Python da origem (`testdata/golden_gen.py`). Regex da origem só por `pytext.Compile`, que mantém `\s`, `\d`, `\b`, IGNORECASE e lookahead do Python;
- texto do PDF em `backend/internal/platform/pdftext` (PDFium em WebAssembly + algoritmo do pdfplumber, com os limites da origem);
- inbox em `backend/internal/factorr/inbox`: upload na API (202) e job `pgdas_parse` no worker, com corrida do `parser_pgdas` e nota ouro;
- gravação do extrato só por `InsertFactorRMovementFromPgdas`: competência vazia, nunca sobrescreve, folha só total declarado e conferido.

Simulador e agentes consultor/priorizador (TASK-013, D-024), em `backend/internal/factorr/agents`:
- todo número vem do motor; o LLM (aliases `consultor` e `priorizador`) só classifica a intenção e redige o texto;
- texto do LLM sempre por `ValidateText` (porte do `validar_texto`, com golden da origem); reprovado, vira o template;
- "corrigir" só com simulação gravada em `factor_r_simulations` (guarda no decide);
- prompts versionados em `backend/internal/ai/prompts/`: mudar o texto exige versão nova (o teste fixa o hash).

## Dashboard

Seções Fator R e Conciliação (TASK-024, D-038):
- `GET /dashboard` ganhou `factor_r` e `reconciliation` (`backend/internal/dashboard/sections.go`). Cada número sai da mesma função da lista do drill-down: a carteira (`factorr.PortfolioWith`), os documentos, a fila (`review.CountsFor`) e a Conciliação por empresa (`backend/internal/reconciliation/overview`, `GET /reconciliation/companies`);
- os filtros de empresa (agrupamento, regime e responsável) ficam em `company.Filter`, pelo snapshot da competência, com o responsável do Fiscal no Fator R e o do Contábil na Conciliação. A carteira, a fila, os documentos (com `filter_competence`) e a Conciliação por empresa aceitam os mesmos filtros;
- número novo no Dashboard leva os filtros para a lista do drill-down e entra no teste de consistência (TEST-024);
- no frontend, `CompanyFilterChips` mostra e limpa os filtros que vieram do Dashboard.

## Ficha da empresa

A ficha como hub (TASK-025, D-039):
- seções e subabas em `frontend/src/features/companies/detail/CompanyDetailPage.tsx`. A URL guarda a subaba (`?tab=`), e a competência da ficha (`?competence=`) vale para todas as seções (no Fator R, é o PA; o seletor vai além da competência gerada). Link novo para a ficha usa `competence`, nunca `pa`;
- trocar de empresa refaz as abas (o `key` é a empresa): aba nova não guarda estado de outra empresa;
- Visão geral em `backend/internal/companyhub` (`GET /companies/{id}/overview`), sempre a partir dos serviços de cada módulo; os indicadores da listagem em `GET /company-indicators`;
- tabela nova com `company_id` referencia `companies(id)`, direto ou pela chave composta `(id, company_id)` da tabela-mãe (o teste do AC-002 confere).

## UI

A interface final usa Design System InovaSala. O material oficial está em `docs/design-system/` (entregue em 07/10/2026; ver ADR-005 e `docs/consolidacao/decisoes.md` D-005).

Não invente cores, fontes ou tokens fora desse material. Estrutura e comportamento podem ser implementados antes da TASK-029.

## Execução da consolidação

- Baseline de regressão: `docs/consolidacao/baseline.md`.
- Fontes congeladas e inventário de capacidades: `docs/consolidacao/fontes.md`.
- Decisões, conflitos e pendências: `docs/consolidacao/decisoes.md`.

## Segurança

Nunca:
- commitar secret;
- imprimir API key;
- expor segredo via API/frontend;
- relaxar validação para “fazer teste passar”;
- executar `db-reset`, apagar dados/arquivos/volumes ou migration destrutiva sem aprovação;
- executar operação Git remota sem aprovação explícita.

## Definition of Done de uma TASK

- ACs associados atendidos.
- Testes associados passando.
- Lint/typecheck aplicáveis passando.
- OpenAPI/tipos atualizados se necessário.
- Sem regressão detectada.
- Docs/ADR atualizados quando contrato mudou.
- Relatório final da TASK entregue ao usuário.
