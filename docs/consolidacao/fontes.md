# Fontes de comportamento congeladas — TASK-002

Referências imutáveis dos projetos de origem usadas nos testes de equivalência do porte (eval-plan §4) e no inventário de capacidades que a TASK-030 vai conferir ("nenhuma capacidade foi silenciosamente perdida", 00-index).

## 1. Referências congeladas

| Projeto | Repositório | Branch | Commit | Data do commit | Cópia local (só leitura) |
|---|---|---|---|---|---|
| Controle de Tarefas (base) | `comunidade-rebelde-a-fonte/controle-tarefas` | `develop` (= `main`) | `a8b46802fcd1f88b7908de317ed34da97696999e` | 30/09/2026 | este repositório |
| Plataforma Fator R | `comunidade-rebelde-a-fonte/fator-r` | `main` | `9dd7a74a789cfa7af45dcf8c89a50a89390bc342` | 24/09/2026 | `../referencias/fator-r` |
| Vera / Conciliação Contábil | `comunidade-rebelde-a-fonte/conciliador-contabil` | `main` | `2bc6e2669ed12e40941b97762487c1099dfe0cd9` | 16/09/2026 | `../referencias/conciliador-contabil` |

Regras:

- Os clones de referência ficam fora deste repositório, em `C:\Desenvolvimento\controle-a-fonte\referencias\`, presos nesses commits. Não se edita nada neles.
- Comparação de comportamento é sempre contra esses commits. Se um projeto de origem mudar depois, a referência só avança por decisão registrada em `docs/consolidacao/decisoes.md`.
- Para recriar: `git clone <repo> && git checkout <commit>`.

## 2. Estado das suítes de referência

| Projeto | Suíte | Resultado no commit congelado | Observação |
|---|---|---|---|
| Conciliador | `pytest` (Python 3.12.13, venv isolado fora do clone) | **74 passed** | O clone continuou intacto (`git status` vazio) |
| Fator R | `pytest` | **adiada para a TASK-010** | A suíte exige Postgres com a role dona e a role da aplicação (RLS) migradas pelo próprio projeto (`tests/conftest.py`, fixture `_migrated_database` autouse). Vai rodar na TASK-010, onde a saída do motor de referência vira golden. |
| Conciliador | golden de extratos (`tests/golden/extratos/`) | **vazio no repositório** | O gate M0 do projeto ("golden ≥ 3 extratos reais") ficou aberto. Os únicos insumos de golden são os fixtures fictícios de `conciliacao/` (seção 3.2). |

## 3. Onde está o comportamento de cada módulo

### 3.1 Fator R (`../referencias/fator-r`)

| Tipo | Arquivos |
|---|---|
| Contrato de produto | `docs/prd.md` (v0.3), `docs/plan.md` (§5 fórmulas, §6 parser, §7 agentes e tracing, §13 registro de decisões), `docs/tasks.md`, `CLAUDE.md` §3 (16 regras invioláveis) |
| Motor e domínio | `apps/api/src/fator_r/domain/{janela,fator_r,simulador,carteira,tabelas}.py` |
| Parser PGDAS-D | `apps/api/src/fator_r/parsing/{pgdas,texto,isolado}.py` (`PARSER_VERSION 2026.09.3`) |
| Agentes | `apps/api/src/fator_r/agents/{parser_pgdas,consultor,priorizador,etapas,intencao,render,guardas,llm,echo}.py` e `agents/prompts/{plan,render}.md` |
| Tracing e evals | `apps/api/src/fator_r/tracing/{tracer,sync,scores,client,consulta}.py`, `services/ouro_pgdas.py` |
| Serviços e API | `apps/api/src/fator_r/services/*.py`, `apps/api/src/fator_r/api/*.py` |
| Banco | `apps/api/alembic/versions/*` (10 migrations) |
| Testes | `apps/api/tests/{domain,parsing,agents,api,integration}` |
| Fixtures | `apps/api/fixtures/pgdas/` (14 casos com `expected.json`), `pdf/` (extratos fictícios por mês) |
| Infra | `docker-compose.yml`, `infra/langfuse/docker-compose.langfuse.yml` (+ `.env.example`), `infra/backup/{backup.sh,testar_restore.sh}` |
| Interface | `apps/web/app/(app)/{carteira,empresas,inbox,agentes,observabilidade}`; tema InovaSala aplicado no commit `31e7fd4` (`.claude/sdd/features/DESIGN_TEMA_CEATH.md`) |

### 3.2 Conciliador (`../referencias/conciliador-contabil`)

| Tipo | Arquivos |
|---|---|
| Contrato de produto | `docs/prd.md` (v0.1), `docs/plan.md` (§4 contratos congelados, §5 portão), `docs/task.md` (desatualizado: só M0 marcado, embora o código cubra M0–M6), `AGENTS.md` |
| Domínio puro | `app/domain/{schemas,validador,dedupe,conciliacao,classificacao,bordas,similaridade,money,csv_extrato,pdf_extrato}.py` |
| Agentes | `app/agentes/{triador,extrator_extrato,extrator_comprovante,resolvedor,classificador,ferramentas,llm}.py` e `app/agentes/prompts/*.md` (7 prompts) |
| Serviços | `app/servicos/{ingestao,pipeline,importacao,comprovantes,matching,razao,fechamento,fila,processar_local,auth}.py` |
| Fila e worker | `app/servicos/fila.py`, `app/workers/runner.py` |
| Storage | `app/storage.py` (MinIO/S3 versionado, sem delete) |
| Banco | `app/migrations/versions/{001_inicial,002_m1_m6}.py`, `app/db/models.py` |
| Interface | `app/web/rotas.py` + `app/web/templates/*.html` (documentos, fila, clientes, lançamentos, playbooks, razão, métricas, fechamento) |
| Testes | `tests/unit/*.py` (22 arquivos, 74 testes) |
| Fixtures | `conciliacao/` (2 extratos em PDF/CSV/TXT, 13 comprovantes PDF, contas a pagar e a receber, `gabarito_conciliacao.md`, `razao_banco_a_fonte.csv`) |
| Infra | `docker-compose.yml` (postgres, minio, minio-init, litellm, app, worker, n8n), `litellm/config.yaml`, `n8n/workflows/ingestao.json`, `.env.example` |
| Scripts | `scripts/avaliar_extrator.py` (golden), `scripts/carregar_horizonte.py` (cenário de demonstração) |

### 3.3 Controle de Tarefas (base)

O comportamento do sistema-base está em `docs/PRD.md`, `docs/plan.md`, `docs/decisoes.md`, `docs/pendencias.md` e no próprio código. A matriz CA → teste está em `docs/tasks.md` §4. O baseline executável está em `docs/consolidacao/baseline.md`.

## 4. Inventário de capacidades

Legenda da coluna **Spec**:

- **coberta**: a spec de consolidação cita a capacidade;
- **não citada**: existe na origem, mas a spec não menciona; entra no escopo pela regra soberana ("preservar 100%") e precisa de confirmação;
- **adaptada**: a spec decidiu mudar a forma;
- **conflito**: a spec contradiz a origem; decisão pendente em `decisoes.md`;
- **incluída (D-011)**: era "não citada"; entrou no escopo por decisão do usuário (07/10/2026), na TASK indicada (`13-implementation-plan.md` §12);
- **fora (D-012)**: não será portada, por decisão do usuário.

### 4.1 Fator R

| ID | Capacidade (comportamento de origem) | Fonte | TASK | Spec |
|---|---|---|---|---|
| FR-01 | Configuração Fator R por empresa: CNAE, atividade, sujeita a Fator R (Sim/Não), quantidade de sócios, contato, pacote (monitoramento, correção, retainer), honorário, início de atividade, notas, ativo | PRD §7.2; plan §4 | 006, 011, 014 | coberta; a sujeição é o `companies.factor_r` do cadastro (D-017) |
| FR-02 | Série mensal: receita, pró-labore, salários + 13º/férias, CPP, FGTS, folha do mês calculada, origem (`manual`, `pgdas`, `folha`, `agente`), observação; upsert por empresa + competência; valores ≥ 0 | PRD §7.3; T-102, T-107 | 006, 011 | coberta |
| FR-03 | Grade mensal de 12+ competências editável, com a origem de cada linha. Linhas `pgdas` mostram a célula "Folha declarada no PGDAS-D" com "Detalhar folha" | T-109, T-810 | 014 | coberta; detalhe da célula incluído (D-011) |
| FR-04 | Motor: janela PA−12 a PA−1; meses válidos desde o início de atividade; faltantes; RBT12; FS12 com política de CPP; fator; Anexo III se ≥ 0,28; folha mínima 28%/meta; gap e reforço (28% e meta); alíquota efetiva pela tabela vigente no PA; economia em 12 meses; semáforo; `dados_insuficientes` se RBT12 = 0; `empresa_nova_regra_pendente` (proporcionalização aguardando P-05) | plan §5.1; `domain/fator_r.py` | 010 | coberta |
| FR-05 | Tabelas dos Anexos III e V com vigência por data; seed a partir do CSV de 2018 (conferência legal P-04 pendente) | plan §2.2-4, §5.3; T-201 | 006, 010 | coberta |
| FR-06 | Política única: meta 0,30; limiar do parser 0,40; piso de economia R$ 6.000; CPP do DAS integra a FS12 = sim; tolerância ouro = 1% | plan §4 (`firms`), §13 | 006 (`factor_r_policy`) | coberta |
| FR-07 | Carteira: só empresas ativas e sujeitas; PA padrão = competência corrente; ordem vermelho → amarelo → verde e, dentro do grupo, maior economia; ação sugerida (reunião de correção, vigiar/simular, manter, completar lançamentos); KPIs (monitoradas, no V, no limite, seguras, economia em jogo, honorários); coluna "Aumento de folha por mês" rotulada; meta por linha; 200 empresas em < 1 s numa consulta | PRD §7.5; T-301..T-306, T-809 | 011, 014 | coberta; ação sugerida e rótulos incluídos (D-011) |
| FR-08 | Inbox PGDAS-D: upload PDF/TXT validado pelo conteúdo, limite de tamanho, dedupe por SHA-256, arquivo fora da web root, download só por endpoint | PRD §7.6, §8; T-502, T-503 | 012 | coberta |
| FR-09 | **Parser determinístico** (regex, sem LLM): pdfplumber → pypdf; sem camada de texto → `needs_review`; campos CNPJ, PA (mês/ano ou intervalo dentro do mesmo mês), RBT12, RPA, FS12, Fator R, DAS e anexo (linha estruturada primeiro); nome empresarial e sugestão de sujeição fora da confiança; confiança ponderada com bônus de consistência; extração isolada com timeout e tetos de páginas, texto e memória; no máximo 2 extrações simultâneas | CLAUDE.md §3.2; plan §6; T-505..T-507, T-801 | 012 | coberta: determinístico, sem LLM (ADR-006; C-001 resolvido) |
| FR-10 | Regras de gravação: `linked` só com confiança ≥ limiar **e** CNPJ encontrado; RPA na competência do PA só se vazia; 12 meses anteriores só se as tabelas conferem com RBT12/FS12 (tolerância R$ 0,01); folha declarada inteira em salários, demais campos zerados, com observação; atividade sem fator r → folha zero; nunca sobrescreve | CLAUDE.md §3.8, §3.9; plan §13 (22/09) | 012 | coberta |
| FR-11 | Ações manuais no inbox: vincular (novo trace), rejeitar, cadastrar empresa pelo extrato (pré-checagens; 409 → vincular à existente) | T-512, T-803..T-805 | 012, 014 | coberta |
| FR-12 | Simulador: reforço, pró-labore extra, custo INSS + IRRF, economia no horizonte, líquido, vereditos `ja_na_meta`, `corrigir`, `nao_forcar`, piso; aviso "INSS sem teto na v1"; persistido com trace | plan §5.2; T-601..T-604 | 010, 013 | coberta |
| FR-13 | Consultor: o LLM só classifica intenção (`status_empresa`, `simular`, `explicar`, `priorizar`) e extrai parâmetros, com regras de palavras-chave como alternativa; ferramentas motor e simulador; guarda "`corrigir` exige `simulation_id`"; texto redigido pelo LLM com validador de números e template como alternativa | plan §7.3; T-605..T-609 | 013 | coberta |
| FR-14 | Priorizador: fila de vermelhos e amarelos por economia, pelo chat | T-610 | 013 | coberta |
| FR-15 | **Rotina semanal do priorizador** (todo domingo, gatilho `rotina`, sem LLM) | plan §7.3; T-611 | 013 | incluída (D-011) |
| FR-16 | Tracing: `tracer.run`; `trace_id` de 32 hex igual no banco local e no Langfuse; spans `plan → parse/tool → decide → render`; `record_decision` como único caminho; sem trace, não grava (`trace_id NOT NULL`); mascaramento (sem texto bruto de PDF, CNPJ mascarado, payload truncado); metadados; `langfuse_sync` com job de reenvio | plan §7.2; T-401..T-407 | 009 | coberta |
| FR-17 | Evals: ouro automático após `linked` (exclui `origem=pgdas`, `ouro_indisponivel`, tolerância) com scores `gold_<campo>` e `gold_acerto`; humana `acerto`, `parcial` ou `erro` com comentário obrigatório (CHECK) espelhada como `human_eval`; dataset `pgdas_extratos` e `parser-experiment` | plan §7.4; T-408, T-511, T-513 | 009, 012 | coberta |
| FR-18 | Painel de observabilidade: corridas 24 h e totais, pendências sem nota, acerto ouro e humano, métricas por agente; custo e tokens pela API do Langfuse (cache de 5 min); lista de traces filtrável; detalhe com spans, evals, nota e link para o Langfuse | PRD §7.9; T-409, T-410 | 009, 014 | coberta |
| FR-19 | **Disclaimer permanente** "o PGDAS-D da Receita Federal prevalece" em toda tela do módulo e em toda resposta de agente | PRD §8, §13; T-704 | 013, 014 | incluída (D-011); a spec só citava o consultor (prompt-spec §7) |
| FR-20 | LLM determinístico para teste e E2E (`LLM_PROVIDER=fake`, proibido em produção) | `agents/llm.py` | 004, 013 | coberta (ai-behavior-spec §4) |
| FR-21 | **Backup diário** (banco da aplicação + Postgres e ClickHouse do Langfuse + uploads), com rotação e teste de restore | T-701; `infra/backup/` | 031 | incluída (D-011) |
| FR-22 | Retenção: nenhum job apaga traces ou movimentos; Langfuse com retenção ≥ 24 meses | T-702 | 009 | coberta (09-security §9) |
| FR-23 | Agente `echo`, só para dev e teste do tracer | T-411 | — | fora (D-012); o tracer fica coberto por testes na TASK-009 |
| FR-24 | Seed de carga (200 empresas × 24 meses) e teste de performance da carteira | T-304, T-305 | 011 | coberta (11 Performance) |
| FR-25 | Login, sessões, `firm_id` e RLS por escritório; testes de isolamento entre escritórios | T-008..T-010, T-103, T-110 | — | **adaptada**: sem login e escritório único (REQ-003, DEC-002); o isolamento passa a ser N/A |

### 4.2 Conciliação (Vera)

| ID | Capacidade (comportamento de origem) | Fonte | TASK | Spec |
|---|---|---|---|---|
| CC-01 | Ingestão burra: `POST /ingestao/arquivo` (n8n) + upload manual; SHA-256; duplicata → 409 + status `duplicado`; original no MinIO versionado sem delete; `documento_origem` `pendente`; enfileira triagem; não interpreta | PRD §6.1; plan M1.1 | 015 | coberta (D-026): `POST /reconciliation/documents`; a duplicata é recusada com 409 e, como na origem, não vira documento `duplicado` |
| CC-02 | Fluxo n8n "pasta monitorada → endpoint" (`n8n/workflows/ingestao.json`) | plan M1.2 | 003, 015 | coberta (D-026): workflow refeito para o n8n 2.x e provado; `make n8n-import` |
| CC-03 | Ferramentas `extrair_texto_pdf` e `renderizar_pagina` (DPI ajustável, cache do render no storage), via PyMuPDF | plan §1, M1.3 | 016 | coberta (D-027): PDFium com as regras de linha do PyMuPDF (golden de 67 PDFs) e render PNG; `cache_render` não portado (a origem não o usava) |
| CC-04 | Triador: primeiras N páginas (texto ou imagem); saída `{tipo, banco, conta, período, confiança}`; abaixo do limiar → `revisao_triagem` | PRD §6.2; M1.4 | 016 | coberta (D-027): texto, imagem ou conteúdo do arquivo; `LIMIAR_TRIAGEM` |
| CC-05 | Extrator de extrato: schema PRD §7; até 3 tentativas com feedback numérico e dicas (linha pulada, separador decimal); estratégias texto nativo → render 150 DPI → render 300 DPI + `extrator_forte`; JSON inválido conta como tentativa | PRD §6.3; M1.8 | 016 | coberta (D-027): as páginas vão como imagem (a origem mandava só o tamanho do PNG) |
| CC-06 | Validador puro: 6 regras, `ResultadoValidacao`, `Decimal`, saldo como última regra e única que rejeita o lote, tolerância R$ 0,00; `validar_saldo` usa a mesma função | PRD §7; plan §5 | 016 | coberta (D-027): golden da origem + testes de propriedade |
| CC-07 | Importação atômica + dedupe `sha256(conta + data + valor + sentido + normalizar(descricao) + ordem_no_dia)`; reprovado → `revisao_extracao` com diagnóstico | M1.9 | 016 | coberta (D-027): + empresa pela conta e trava do Contábil |
| CC-08 | **Caminhos determinísticos paralelos ao agente**: importador de extrato CSV/TXT (`csv_extrato.py`) e leitura de extrato pelo texto nativo do PDF (`pdf_extrato.py`, `processar_local.py`) | código | 016 | coberta (D-027): PDF no job de triagem; CSV no de extração; leitor de comprovante na TASK-017 |
| CC-09 | Playbooks por banco: schema congelado; persistir só se o portão aprovou; carregar no prompt; invalidar na falha da 1ª tentativa; tela com versão e taxa de sucesso | PRD §6.7; M1.5 | 016, 021 | coberta (D-027); tela na TASK-021 (D-011) |
| CC-10 | `extracao_log` por tentativa (hash, alias, modelo real, hash do prompt, saída bruta, validador, tentativa, custo, tokens, latência) + intervenção humana (quem, quando, o quê) | PRD §11; M1.10, M2.7 | 007, 009, 016 | coberta (D-027): + `leitor_local` e `humano` (reprocessamento) |
| CC-11 | Cadastro de cliente e conta bancária; extrato vinculado por banco + agência + conta | M2.2 | 007, 021 | **adaptada**: cliente → `companies` (REQ-002) |
| CC-12 | **Lançamentos do cliente inseridos à mão** (formulário + CSV) | M2.3; rotas `/lancamentos`, `/lancamentos/csv` | 017, 021 | incluída (D-011) |
| CC-13 | Extrator de comprovante: texto primeiro, render + visão quando falta texto ou confiança; confiança e trecho/bbox por campo; validador de campos; registro `parcial` | PRD §6.4; M3.2..M3.4 | 017 | coberta |
| CC-14 | Comprovante duplicado com leiaute diferente (`valor + data + contraparte normalizada`) → sinaliza, não descarta | M3.5 | 017 | coberta |
| CC-15 | `consultar_cnpj` | PRD §6.6 | 017 | coberta; **não implementada na origem** (só aparece nos prompts) |
| CC-16 | Cascata de 4 passes: exato; D±3; fuzzy de contraparte com limiar; N:1 com subconjunto ≤ 8, D±3 e solução única; ambiguidade → divergência; 4 baldes | PRD §8; M2.4, M4.1, M4.2 | 018 | coberta |
| CC-17 | Marcação de casos de borda: transferência interna, estorno (janela de 7 dias), bruto × líquido | M4.6; `domain/bordas.py` | 018 | coberta |
| CC-18 | Resolvedor: hipótese, justificativa, pares e confiança gravados como proposta; aceitar → conciliação `manual` com método `resolvedor`; rejeitar | M4.4, M4.5 | 019 | coberta |
| CC-19 | Fila de revisão: documento à esquerda; ações conciliar, desfazer, "sem comprovante — confirmado", editar campo, "que documento é esse?", aceitar ou rejeitar hipótese; filtros por balde, cliente e período | M2.6; rotas `/fila/*` | 021 | coberta |
| CC-20 | Classificação regra → histórico → agente, com a origem registrada; o agente `classificador` usa o alias `resolvedor`; correção vira regra após confirmação; plano de contas por cliente com cópia de padrão e importação CSV | PRD §9; M5.1..M5.5 | 020 | coberta (alias: ver D-008) |
| CC-21 | Razão como projeção + drill-down até o documento + exportação CSV | M5.6, M5.7; `/razao.csv` | 020, 028 | coberta (+ XLSX nova) |
| CC-22 | Painel de métricas: aprovação na 1ª tentativa, tentativas médias, custo, % por passe, itens por balde | M2.9, M4.7; `/metricas` | 021, 024 | coberta (11-observability §7) |
| CC-23 | Fechamento por cliente e competência, reabertura com justificativa, `pendencia_periodo_fechado` | M6.1, M6.2 | 023 | **adaptada**: autoridade passa ao Contábil (ADR-003) |
| CC-24 | Fila de jobs: `SKIP LOCKED`, retry com backoff, dead-letter, `JOB_MAX_TENTATIVAS=3`, `WORKER_ID` | M0.6 | 005 | coberta |
| CC-25 | `scripts/avaliar_extrator.py`: relatório do golden (aprovação na 1ª tentativa, tentativas, custo, diff) | M1.12 | 016 | coberta (D-027): `make eval-extrator`; golden do cenário |
| CC-26 | Cenário de demonstração (`scripts/carregar_horizonte.py` + `conciliacao/`) | código | 027 | coberta (REQ-048) |
| CC-27 | Login (`ADMIN_USER`) | M2.1 | — | **adaptada**: sem login, ator local (REQ-003) |
| CC-28 | Tema próprio claro/escuro com alternador (`.interface-design/system.md`) | código | 029 | **adaptada**: Design System InovaSala (ADR-005) |

### 4.3 Controle de Tarefas

Preservado por inteiro (REQ-004, REQ-006). O inventário é a matriz do `docs/tasks.md` §4.2 (critérios do PRD §54) e o baseline executável de `docs/consolidacao/baseline.md`.

## 5. Notas técnicas para o porte

Registradas agora para não se perderem; cada uma é tratada na TASK indicada.

1. **Extração de texto de PDF (TASK-012 e TASK-016).** O Fator R usa pdfplumber/pypdf e o Conciliador usa PyMuPDF. O parser do Fator R é por regex sobre o texto extraído, então outra biblioteca pode mudar espaços e ordem do texto e quebrar a equivalência. O golden das 14 fixtures de `fixtures/pgdas/` precisa passar com a biblioteca escolhida em Go.
2. **Imagens sem versão fixa no Conciliador (TASK-003).** `quay.io/minio/minio:latest`, `minio/mc:latest`, `litellm:main-latest` e `n8nio/n8n:latest`. O `AGENTS.md` do projeto-base exige imagens com versão fixada. **Resolvido na TASK-003 (D-013).** Na verificação, as imagens `quay.io/minio/*` já não existiam: o compose de origem do Conciliador não sobe mais.
3. **Modelos padrão do Conciliador eram Anthropic** (Haiku 4.5 no triador, Sonnet 4.5 no extrator e no resolvedor). A ADR-004 troca para OpenAI; os nomes concretos ficam em `.env` (Q-003).
4. **Escalada do extrator:** na 3ª tentativa, o alias `extrator` resolve o modelo de `extrator_forte` (`app/agentes/llm.py`). Preservar.
5. **O Fator R usa a Anthropic direto**, sem LiteLLM e só para `plan` e `render`. A ADR-004 passa esse uso para o LiteLLM.
