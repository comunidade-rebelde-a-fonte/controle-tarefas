# Registro de decisões da consolidação

Decisões tomadas durante a execução da spec (`docs/ai-spec/`), conflitos encontrados e pendências. As entradas mais recentes ficam no topo de cada seção. Decisão que muda contrato também atualiza a ADR correspondente.

## 1. Decisões

### D-022 — Auditoria local de IA e sincronização com o Langfuse (TASK-009; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte do tracing do Fator R** (`tracing/{tracer,client,sync,scores}.py`, commit `9dd7a74`), com os nomes do 05 §8.
  - Tabelas:
    - `ai_runs`, que corresponde a `agent_traces`;
    - `ai_decisions` (`agent_decisions`);
    - `ai_gold_evals` (`evals_gold`);
    - `ai_human_evals` (`evals_human`).
  - Vocabulários da origem:
    - status `ok`, `error` e `needs_review`;
    - nota humana `acerto`, `parcial` e `erro`;
    - nota ouro `ok`, `erro` e `ouro_indisponivel`.
  - Contrato do Langfuse da origem:
    - o trace leva o nome do agente;
    - spans só `plan`, `parse`, `tool`, `decide` e `render`, mais as gerações do LLM;
    - scores `gold_<campo>` (BOOLEAN), `gold_acerto` (NUMERIC) e `human_eval` (CATEGORICAL).
- **Semântica preservada:**
  - a corrida nasce na transação do agente;
  - decisão de agente só se grava por `RecordDecision`, e o banco exige `trace_id`;
  - um erro desfaz as decisões e grava a corrida de erro, com o código e nunca a mensagem;
  - a corrida que termina sem `Finish` fica como erro `NOT_FINISHED`;
  - a nota "erro" exige comentário, no código e no banco.
- **Diferenças da origem:**
  - **Envio sem SDK:** OTLP/HTTP em JSON e a API de scores. O Langfuse 4.53 aceita o formato (confirmado por sonda e pelo `make test-langfuse`), sem dependência nova.
  - **Spans guardados na trilha local:** o detalhe da corrida não depende do Langfuse. Na origem, os spans vinham da API do Langfuse e ficavam "indisponíveis" com ele fora. O reenvio manda o trace completo; na origem, só um resumo de metadados.
  - **Fila de jobs como outbox:**
    - o job `langfuse_sync` entra na fila na mesma transação da corrida;
    - se o Langfuse falha, o status fica `failed`, com o erro sanitizado;
    - o worker reenvia periodicamente: `LANGFUSE_RETRY_SECONDS`, padrão 5 minutos, 50 por vez, como a origem.
  - **IDs fixos:**
    - o span raiz é derivado do trace;
    - cada score usa o ID da própria nota;
    - o `gold_acerto` usa um UUID derivado do trace.
    - O reenvio não cria observação nem score novos (provado no Langfuse real).
    - O Langfuse v4 grava cada envio e consolida depois as versões da mesma observação (ClickHouse). Até isso acontecer, a API pública pode listar a mesma observação duas vezes, com o mesmo ID. Na origem, cada reenvio criava um span raiz novo.
  - **Notas com status de sincronização próprio:** na origem, o `create_score` era "dispara e esquece".
  - **Comentário da nota humana:** vai ao Langfuse com CNPJ e CPF mascarados; na origem ia cru.
  - **Painel:** custo e tokens vêm da trilha local (informados pelo gateway), e não da API de métricas do Langfuse.
  - **Sem multiescritório:** as etiquetas são agente, gatilho e módulo; não há `firm:<id>`.
  - **Gancho genérico na fila (`jobs.AfterRollback`):** num job, a corrida de erro é gravada depois do rollback da tentativa, junto com a falha do job.
- **Mascaramento.**
  - Porte de `mascarar_cnpj`, `_limpar_payload` e `mascarar_atributo`, com as classes Unicode do Python (`\d`, `\s` e `\b`; pacote `pytext`, que também passou a servir a chave de dedupe da Conciliação).
  - Provado por golden gerado com o próprio código da origem (`testdata/mask_gen.py`).
- **Rede (ponto aberto da D-013).**
  - Rede externa `controle-obs`, criada pelo Makefile (`make obs-net`, pré-requisito de `dev` e `langfuse-up`).
  - Ficam nela o worker e o `langfuse-web` (apelido `langfuse`), com `HOSTNAME=0.0.0.0` no `langfuse-web`, como no compose oficial; sem isso, o Next.js só atende na rede padrão dele.
  - O Langfuse continua publicado só em `127.0.0.1`. Funciona também no Linux.
- **Segredos.** `LANGFUSE_SECRET_KEY` vai só para o worker (um teste confere no compose). A API recebe apenas `LANGFUSE_PUBLIC_URL` e `LANGFUSE_PROJECT_ID`, para montar o link do trace.
- **Tela.** Traces e avaliações entram na TASK-014b (plano). Aqui fica a API do 06 §9, mais `GET /ai/summary` com as métricas do painel (FR-18).

### D-021 — Configuração dos modelos de IA (TASK-008; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Tabela `ai_model_aliases` [05 §8]**, criada na migration `0021_ai_model_aliases.sql`.
  - Uma linha por alias do `ai.Catalog`, criada pela própria migration (versão 1). Um teste confere que tabela e catálogo não divergem.
  - `provider` e `model` nulos significam "vale o padrão do `.env`" (`<ALIAS>_MODEL`); a escolha da tela vence o padrão.
    - O `.env` continua sendo o padrão da instalação (D-014, D-015).
    - A tela mostra de onde vem cada modelo: `settings` (escolhido na tela), `env` (padrão do ambiente) ou `none` (sem modelo).
  - Parâmetros: lista fechada com `temperature` (0 a 2) e `max_tokens` (1 a 1.000.000).
    - A lista é conferida no código e por CHECK no banco.
    - Nenhum outro parâmetro entra, inclusive `api_key`, nem por SQL direto.
  - Nome de modelo com cara de chave (`sk-…`, `pk-…`, `rk-…`) é recusado no código e no banco, sem ecoar o valor.
  - Histórico em `ai_model_alias_history`: uma linha por versão (o quê, quem e quando), gravada na mesma transação, como os demais históricos do Controle. Ainda sem tela.
- **API [06 §7].**
  - `GET /settings/ai/models` traz:
    - os aliases na ordem do catálogo;
    - a escolha da tela, o padrão do ambiente e o modelo das novas corridas, com a origem;
    - os parâmetros, ligado/desligado, a versão e a última alteração;
    - `gateway_configured`.
  - `PUT /settings/ai/models/{alias}` substitui a configuração, com `version` (409 `VERSION_CONFLICT`).
    - Mudança no modelo efetivo ou nos parâmetros é conferida no gateway antes de gravar. Reprovada: 422 `AI_MODEL_INVALID`, com as conferências em `details.checks`. Gateway fora do ar ou não configurado: 503 (`AI_GATEWAY_UNAVAILABLE`, `AI_GATEWAY_NOT_CONFIGURED`).
    - Ligar e desligar não dependem do gateway.
    - Campo fora do contrato, como `api_key`, é recusado com 400.
  - `POST /settings/ai/models/{alias}/validate` confere sem gravar.
- **Validação no gateway (regra do D-014).**
  - Consulta `GET /utils/model_info` do LiteLLM, que lê só a tabela de modelos do gateway. Não chama o provedor e não tem custo, então não há "chamada real" a sinalizar [06 §7].
  - Conferências:
    - o gateway conhece o modelo;
    - é um modelo de chat;
    - aceita saída estruturada (todos os agentes);
    - aceita imagens, para triador, extrator e extrator_forte, que leem a página renderizada quando o PDF não tem texto;
    - aceita os parâmetros escolhidos, e `max_tokens` cabe no limite do modelo.
  - Reprovação bloqueia a gravação. Com `drop_params`, o gateway descartaria em silêncio o JSON Schema ou o parâmetro não aceito.
  - **Limitação conhecida:** um modelo que a versão fixada do LiteLLM não conhece (ex.: `gpt-6.1-sol`) só pode ser escolhido depois de atualizar o LiteLLM. É o mesmo motivo de travar a tabela de modelos (D-014).
- **Resolução [AC-027, F-010].**
  - `aliases.Resolver.Resolve(ctx, db, alias)` aplica a configuração gravada sobre o `.env` e devolve o snapshot `ai.Resolved`: modelo, origem, parâmetros e versão.
  - As corridas dos agentes (TASK-012 em diante) resolvem por aqui, no início. A corrida em andamento mantém o snapshot.
  - Alias desligado não resolve (`ErrAliasDisabled`).
- **Chaves [REQ-047].**
  - A tela não tem campo de credencial.
  - A validação usa só a chave do gateway (`LITELLM_API_KEY`).
  - Os erros do gateway chegam sanitizados.
  - TEST-008 confere todas as respostas da tela.
- **E2E.** A suíte sobe um gateway LiteLLM simulado (`e2e/support/fake-litellm.mjs`), com a mesma rota e os mesmos corpos do LiteLLM v1.104.0. O contrato do gateway real fica com `make test-litellm`.

### D-020 — Schema da Conciliação (TASK-007; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte do schema do Conciliador** (`app/db/models.py`, commit `2bc6e26`) para a migration `0020_reconciliation.sql`.
  - Tabelas e colunas em inglês, na convenção do projeto-base. O 05 §5 nomeia as tabelas em inglês e descreve os campos sem fixar nomes.
  - Os vocabulários da origem ficam como estavam:
    - estados do documento (contrato congelado): `pendente`, `triado`, `extraido`, `importado`, `revisao_triagem`, `revisao_extracao`, `duplicado` e `pendencia_periodo_fechado`;
    - sentido: `C` e `D`;
    - métodos: `passe_1` a `passe_4`, `manual`, `sem_comprovante` e `resolvedor`;
    - origem da conciliação: `auto` e `manual`;
    - origem da classificação: `regra`, `historico`, `agente` e `manual`;
    - tipos de regra: `cnpj`, `contraparte` e `regex_descritivo`;
    - estados da proposta: `pendente`, `aceita` e `rejeitada`;
    - estados do lançamento: `pendente`, `parcial`, `extraido` e `manual`;
    - tipos de documento: `extrato`, `comprovante` e `desconhecido`.

  | Origem | Consolidado |
  |---|---|
  | `cliente` | `companies` (canônica, REQ-002) |
  | `usuario` | `users` |
  | `conta_bancaria` | `bank_accounts` |
  | `documento_origem` | `reconciliation_documents` |
  | `extrato_importacao` | `statement_imports` |
  | `extrato_transacao` | `statement_transactions` |
  | `lancamento_cliente` | `client_entries` |
  | `conciliacao` | `reconciliations` + `reconciliation_items` |
  | `plano_contas` | `chart_accounts` |
  | `regra_classificacao` | `classification_rules` |
  | `playbook_banco` | `bank_playbooks` (schema lógico congelado mantido) |
  | `extracao_log` | `extraction_logs` |
  | `proposta_resolvedor` | `resolver_proposals` |
  | `periodo_fechamento` | **não existe**: a trava é o fechamento do Contábil em `company_module_periods` (REQ-033, 05 §6) |
  | `job` | `jobs` (TASK-005) |
  | `cnpj_cache` | adiado: a consulta de CNPJ (CC-15) é decidida na TASK-017 |

- **Empresa em tudo.**
  - Toda entidade tem `company_id`. Só o documento pode ficar sem empresa durante a triagem; sem empresa definida, ele não é importado e fica em revisão.
  - Transação, lançamento, conciliação, item e proposta usam FKs compostas com `company_id`, então um vínculo nunca cruza empresas.
  - Na origem, a transação não tinha cliente, e o lançamento feito à mão ou por CSV podia ficar sem cliente.
- **O portão também no banco.**
  - `statement_imports.validation_ok` tem `CHECK (validation_ok)`: importação reprovada não existe (AC-015).
  - `attempts` vai de 0 a 3; 0 é a importação determinística, como na origem.
- **Dedupe.**
  - A chave segue a origem: sha256 de `conta|data|valor|sentido|descrição normalizada|ordem no dia`, única no sistema.
  - Foi portada para Go (`reconciliation.DedupeKey`). Um golden gerado pelo código Python da origem prova igualdade caractere a caractere em 45 casos difíceis de Unicode.
- **Conciliação N:1 com integridade.**
  - Os vínculos ficam em `reconciliation_items`, e não em arrays JSON (05 §5).
  - Há uma conciliação ativa por transação e um lançamento em no máximo uma conciliação ativa, ambos garantidos por índices únicos parciais.
  - "Sem comprovante" é uma conciliação sem itens.
  - Origem `auto` exige passe e não tem autor; origem `manual` exige autor.
  - Desfazer não apaga, ao contrário da origem: marca `undone_at` e o autor e libera os itens. A auditoria se mantém, e o razão é projeção das conciliações ativas.
- **Outras unicidades.**
  - Conta bancária única no sistema. Na origem a unicidade era por cliente, mas a importação buscava a conta só por banco, agência e conta, o que ficava ambíguo.
  - No máximo um playbook válido por banco; a origem já assumia isso.
  - Uma importação por documento.
- **Log de extração.** O custo é gravado em dólares com 8 casas. O `MONEY` de 2 casas da origem zerava o custo de cada chamada.
- **Trava.** `reconciliation.RequireOpen` consulta o fechamento do módulo Contábil e devolve 423 `PERIOD_CLOSED`, o mesmo das obrigações.
  - Fiscal fechado não trava a Conciliação (AC-026).
  - Sem período gerado, não há trava.
  - Os serviços das TASK-015 em diante usam a trava em cada mutação (AC-025).

### D-019 — Fila de jobs em PostgreSQL (TASK-005; 08/10/2026, dentro da autonomia da spec; validada pelo usuário com a TASK)

- **Porte da fila do Conciliador** (`app/servicos/fila.py` e `app/workers/runner.py`, commit `2bc6e26`):
  - aquisição atômica em ordem de chegada com `SELECT … FOR UPDATE SKIP LOCKED`;
  - 3 tentativas por padrão (`JOB_MAX_TENTATIVAS`);
  - espera de 2ⁿ segundos entre tentativas (limitada a 1 h);
  - estado terminal;
  - espera de 1 s com a fila vazia (`JOB_POLL_SECONDS`);
  - job `ping` de verificação.
  - As transições são as mesmas da origem; um teste confere os casos de `test_fila.py`.
- **Nomes do 05 §7:** tabela `jobs`; estados `pending`, `running`, `retry`, `done` e `dead` (`dead_letter` na origem); colunas `kind`, `attempts`, `max_attempts`, `run_after`, `last_error_code` e `last_error_message`.
- **Diferenças da origem, todas para não perder nem duplicar job:**
  - **Worker que cai.** A tentativa conta na aquisição, e a posse do job tem prazo (`JOB_LEASE_SECONDS`, padrão 300). O worker renova a posse a cada 1/3 do prazo enquanto executa. Sem renovação, outro worker devolve o job à fila com o código `WORKER_LOST` e a mesma regra de tentativas.
    - Na origem, um job em `running` de um worker que caiu ficava assim para sempre.
  - **Efeito uma vez por aquisição [06 §11].** O handler grava na mesma transação que marca o job como concluído, e a conclusão exige que o worker ainda tenha a posse. Uma tentativa que falha ou perde a posse desfaz os próprios efeitos.
  - **Enfileiramento idempotente.** Com `dedupe_key`, existe um job ativo por (`kind`, `dedupe_key`), garantido por índice único parcial. O job pode ser gravado na transação de quem chama, para nascer junto com o dado que o motivou.
  - **Tipos sem handler.** O worker só adquire os tipos que sabe executar. Um tipo sem handler espera na fila sem gastar tentativa; na origem, ele falhava até `dead_letter`.
  - **Pânico e erros.** Pânico no handler vira falha da tentativa (`HANDLER_PANIC`), sem derrubar o worker. O handler pode dar um código à falha (`Coded`) ou declará-la permanente (`Permanent`), e a falha permanente vai direto para `dead`.
  - **Mensagem de erro sanitizada.** Sem chaves, tokens e quebras de linha, até 300 caracteres. O sanitizador passou a ser compartilhado (`internal/platform/redact`) com o cliente do LiteLLM.
- **API [06 §8].**
  - `GET /jobs/{id}` traz o estado, as tentativas, os horários e o erro sanitizado, sem o payload.
  - `worker_online` diz se algum worker deu sinal nos últimos 30 s (tabela `worker_heartbeats`). Assim a interface pode mostrar "fila parada" (runbook 11 §9).
  - `POST /jobs/{id}/retry` vale só para `dead`: zera as tentativas e registra quem reenviou e quando. Não pula portões de domínio.
- **Encerramento ordenado.** O job em andamento tem 30 s para terminar; depois disso, a tentativa é registrada como `WORKER_SHUTDOWN` e o job volta para a fila.
- **Relógio.** A fila usa o relógio real, que é tempo operacional (espera entre tentativas, posse, batimento), mesmo com `APP_FIXED_TIME`, que congela só as datas de negócio.
- **Configuração:** `WORKER_CONCURRENCY` (padrão 1, como a origem), `JOB_MAX_TENTATIVAS`, `JOB_POLL_SECONDS` e `JOB_LEASE_SECONDS`.
- **Evidências:**
  - testes contra Postgres real: 60 jobs com 6 workers concorrentes, cada job executado uma vez; espera, `dead` e sanitização; efeitos desfeitos; posse vencida e posse renovada; encerramento ordenado;
  - detector de corrida limpo (Linux, `-race`);
  - teste manual no ambiente de dev com o worker real: job concluído; com o worker parado, job pendente e `worker_online: false`; ao religar, concluído.

### D-018 — Decisões técnicas da fatia do Fator R (TASK-006a, 010, 011 e 014a; 07/10/2026, dentro da autonomia da spec)

- **Decimal exato com a semântica do Python (ADR-007).**
  - Tipo `decimal.Decimal` sobre `cockroachdb/apd/v3`, com 28 dígitos e half-even.
  - A divisão exata vai ao expoente ideal e o zero é escrito como no Python, porque o apd difere nesses dois pontos.
  - Na API, decimal é string em ponto fixo. O motor recebe os valores com a escala da coluna.
  - Evidência: golden gerado pelo código Python da origem, com 233 cálculos, 18 carteiras e 160 simulações idênticos dígito a dígito.
- **Folha do PGDAS-D sem divisão.**
  - A folha total declarada fica em `folha_total_declarada_pgdas` (05 §4), não mais em `salarios` como na origem. Quando existe, é a folha do mês na FS12.
  - Um CHECK no banco exige `origem = 'pgdas'` e a divisão zerada, o que impede dupla contagem.
  - O lançamento manual ("Detalhar folha") apaga a folha total e grava a divisão.
  - Um teste prova que o resultado é igual ao da convenção da origem, com e sem a CPP na política.
- **Modelo de dados.**
  - **Configuração por empresa:** os padrões são os da origem (`quantidade_socios` 1, `honorario_mensal` 0, ambos NOT NULL), e não os nulos do 05 §4. Empresa sem configuração não tem linha. `pacote_comercial` nulo equivale ao `none` do 05 §4.
  - **Movimentos:** ganham `version` (concorrência otimista, padrão do projeto-base) e `pgdas_document_id`, cuja FK chega na TASK-006b, como na origem. `folha_mes` é coluna gerada.
  - **Tabelas do Simples:** `factor_r_simples_tables`, carregadas pelo `make seed` a partir do CSV idêntico ao da origem. Só insere; nunca altera uma vigência gravada.
  - **Política:** `factor_r_policy` tem uma linha só, com os padrões da origem (meta 30%, limiar do parser 0,40, piso R$ 6.000,00, CPP integra a FS12, tolerância ouro 1%). Não há tela de edição, porque na origem ela era da Fase 2.
- **Nomes.**
  - Colunas como no 05 §4. O JSON usa os mesmos nomes e, no cálculo e na carteira, os nomes da origem (`rbt12`, `fs12`, `semaforo`…).
  - Assim, o AGENTS.md §8.4 (campos em inglês) cede ao 05 §4 (D-007).
  - Os identificadores Go ficam em inglês, com o nome da origem no comentário.
- **Erros.**
  - Parâmetro de query ou caminho inválido → 400 `VALIDATION_ERROR` (convenção do projeto-base; a origem usava 422).
  - PA sem tabela do Simples vigente → 422 `SIMPLES_TABLE_NOT_IN_FORCE`, como na origem.
  - Dinheiro de entrada: string, ≥ 0, até 12 dígitos inteiros e 2 casas, como o `Dinheiro` da origem. Número JSON é recusado.
- **Interface.**
  - Carteira no menu "Fator R" e aba "Fator R" na ficha. O PA fica na URL (`?pa=`) e o padrão é a competência corrente.
  - A grade mensal acompanha a janela do PA; na origem ela tinha navegação própria de 12 em 12 meses.
  - "Como foi calculado" mostra o passo a passo das contas (cálculo explicável, 08 §6).
  - Semáforo com ícone e texto; disclaimer nas duas telas (FR-19).
  - A formatação é half-even por string, sem float.
- **Trava de contrato.** `TestOpenAPI_DocumentsEveryRoute` falha se alguma rota da API não estiver no `openapi.yaml`.
- **Demonstração.** O `make seed-demo` grava dados do Fator R para as 5 empresas sujeitas do seed. No PA corrente, cobrem:
  - vermelho, com meses faltantes;
  - amarelo;
  - verde, com meses do PGDAS-D;
  - dados insuficientes.

### D-017 — "Sujeita ao Fator R" é o `companies.factor_r` (07/10/2026; diverge do 05 §4; validada pelo usuário com a fatia em 08/10/2026)

- **Conflito:** o 05 §4 lista `sujeita_factor_r` em `company_factor_r_settings`. O projeto-base já tem `companies.factor_r`, o indicador "Fator R: Sim/Não" do Simples (PRD §17.2). Ele é copiado para o snapshot das competências e editado na aba Folha.
- **Por que não duplicar:** duas colunas seriam semântica paralela, o que o 05 §3 e o REQ-002 proíbem.
- **Decisão:**
  - Não existe coluna de sujeição na configuração do Fator R.
  - O indicador continua sendo editado só pelo `PATCH /companies/{id}` (aba Folha), que preserva histórico e snapshot.
  - A aba Fator R mostra o valor e leva até a aba Folha.
- **Carteira:** empresa ativa, com `factor_r = true` e no Simples Nacional no PA.
  - Na origem toda empresa era do Simples. No projeto-base, o `factor_r` pode continuar `true` depois de uma mudança de regime, porque a tela só o esconde.
  - O filtro de regime garante que só empresas do Simples entram.
  - O cálculo informa `monitorada` e `regime_no_pa`, e a aba explica por que a empresa está fora da carteira.

### D-016 — Fatia vertical do Fator R (07/10/2026)

- **Decisão do usuário:** adiantar a primeira tela de outro sistema ("só tem um dos 3 sistemas ali") e validar a fatia inteira de uma vez ("segue a fatia e valido inteira").
- **Ordem:** TASK-006a → TASK-010 → TASK-011 → TASK-014a. Um commit local por TASK, depois da validação.
- **Divisão das TASKs.**
  - **TASK-006:** a 006a traz configuração, movimentos, política e tabelas do Simples. A 006b traz documentos PGDAS-D e simulações, e fica com as TASK-012/013, porque depende da fila de jobs (TASK-005) e do trace local (TASK-009).
  - **TASK-014:** a 014a traz carteira, ficha com cálculo, grade mensal, configuração e disclaimer. A 014b traz inbox PGDAS-D, simulador, traces e avaliações.
  - **Por que o simulador espera:** na origem, `corrigir` só vale com a simulação persistida num trace (CLAUDE §3.11).
  - O motor do simulador já foi portado e provado na TASK-010 (REQ-013).
- Registrado no plano (13 §13).

### D-015 — Q-003 decidida: modelos padrão dos aliases (07/10/2026)

- **Decisão do usuário:** confirmados os modelos propostos na TASK-004.

  | Uso | Modelo | Aliases |
  |---|---|---|
  | barato | `openai/gpt-6-luna` | triador, consultor, priorizador |
  | forte com visão | `openai/gpt-6-sol` | extrator, resolvedor, classificador |
  | mais forte | `openai/gpt-6-astra` | extrator_forte |

- Continuam trocáveis no `.env` e, a partir da TASK-008, pela tela Configurações → IA.

### D-014 — Roteamento de modelos de IA (TASK-004, 07/10/2026; decisões técnicas dentro da autonomia da spec)

- **A aplicação resolve o alias e chama o gateway com o modelo concreto.**
  - O `ai.Resolver` devolve `provedor/modelo` no início de cada corrida; o resultado é o snapshot gravado nela.
  - O cliente (`internal/ai/litellm`) envia esse modelo pela rota `openai/*` (ou `anthropic/*`) do `litellm/config.yaml`.
  - Com isso, a corrida em andamento mantém o modelo do início (F-010), e a troca feita na tela (TASK-008) vale para as novas corridas sem reiniciar nada (AC-027).
  - Os aliases continuam declarados no gateway, também com o modelo do `.env`, como compatibilidade com o Conciliador.
- **Chaves.**
  - Nenhum alias tem chave própria (antes: `ANTHROPIC_API_KEY` fixa em todos). O LiteLLM usa a variável do provedor do modelo.
  - As chaves dos provedores existem só no container do LiteLLM. API e worker usam a chave do gateway (`LITELLM_API_KEY`), guardada como `config.Secret`, que se mascara em print, log e JSON.
  - Dois testes travam essas propriedades: `TestCompose_ProviderKeysOnlyInGateway` e `TestSecret_NeverPrinted`.
- **Modelos padrão (Q-003; propostos aqui e confirmados pelo usuário no D-015).** Família OpenAI gpt-6, que é a mais nova no LiteLLM fixado, mantendo o escalonamento dos sistemas de origem:

  | Uso | Modelo | Aliases |
  |---|---|---|
  | barato | `gpt-6-luna` | `triador`, `consultor`, `priorizador` |
  | forte com visão | `gpt-6-sol` | `extrator`, `resolvedor`, `classificador` |
  | mais forte, 3ª tentativa | `gpt-6-astra` | `extrator_forte` |

  Todos têm visão e saída estruturada segundo o próprio gateway. Ficam num único lugar do compose (âncora `x-ai-models`, usada por LiteLLM, API e worker). `TestCatalog_MatchesGatewayAndCompose` confere que código, `litellm/config.yaml`, `docker-compose.yml` e `.env.example` não divergem.
- **D-008 aplicado:** alias `classificador` com o mesmo modelo do `resolvedor`. No Conciliador, o agente de classificação usava o próprio alias `resolvedor`.
- **Tabela de modelos do LiteLLM travada** (`LITELLM_LOCAL_MODEL_COST_MAP=True`).
  - Sem isso, o gateway baixa a tabela do GitHub a cada início, ou seja, chamada externa e comportamento que muda sozinho.
  - Evidência: a tabela embutida não conhece o `gpt-6.1-sol` e o trata como sem suporte a saída estruturada, o que, com `drop_params`, poderia descartar o JSON Schema em silêncio.
  - **Regra para a TASK-008:** a validação da tela de IA deve consultar o gateway e avisar quando o modelo escolhido não tiver suporte a saída estruturada ou a visão, se o alias precisar.
- **Teste de alias simulado (TEST-004).**
  - O `mock_response` do LiteLLM vindo do cliente só funciona com chave virtual, que exige banco.
  - Por isso a evidência é: testes unitários do resolver e do cliente (`make test`) mais `make test-litellm`, que sobe o LiteLLM real (mesma imagem e mesmo `config.yaml`) diante de um provedor OpenAI simulado no próprio teste. Sem chamada externa e sem custo.
- **Cliente do gateway:**
  - poucas novas tentativas só em falhas transitórias (rede, 408, 429, 5xx), como os clientes de origem;
  - erro sempre sanitizado (sem chaves nem `Bearer`);
  - custo do gateway guardado como decimal textual, nunca `float`;
  - saída estruturada por JSON Schema (`strict`), que continua validada pelo código antes de qualquer uso.

### D-013 — Infraestrutura consolidada da TASK-003 (07/10/2026, decisões técnicas dentro da autonomia da spec)

A spec autoriza alterar Docker Compose e Makefile sem aprovação prévia (15-agent-execution-prompt); as escolhas abaixo ficam registradas para revisão.

- **MinIO.**
  - **Imagem:** `cgr.dev/chainguard/minio`, fixada por digest (MinIO `RELEASE.2026-09-22`).
  - **Motivo:** a MinIO deixou de publicar imagens da edição comunitária. `quay.io/minio/minio` e `quay.io/minio/mc` não existem mais, nem `latest` nem tags `RELEASE.*`. Por isso o compose do Conciliador de origem **não sobe hoje**.
  - O Langfuse do Fator R já usava a imagem da Chainguard; a mesma imagem serve aos dois MinIO e ao `minio-init` (ela traz `sh` e `mc`).
  - A Chainguard gratuita só publica `latest`, então a versão é fixada pelo digest.
- **Original imutável também no storage.**
  - Antes, o Conciliador usava a credencial raiz no app. Agora o `minio-init` cria um usuário da aplicação (`S3_ACCESS_KEY`).
  - A política desse usuário permite gravar, ler e listar versões, e **nega** apagar objeto, apagar versão, apagar o bucket, suspender o versionamento e mudar ciclo de vida ou política.
  - A credencial de administrador (`S3_ROOT_*`) só é usada pelo `minio-init`.
- **`minio-init` avulso.** Roda como etapa do `make dev` (`docker compose run --rm minio-init`) e é idempotente. Nenhum serviço depende do MinIO para subir: com ele fora, API e worker continuam saudáveis e só o upload da Conciliação falha (runbook 11 §9; testado).
- **Langfuse em compose dedicado** (`docker-compose.langfuse.yml`, projeto `controle-langfuse`).
  - Comandos: `make langfuse-up` ou `make dev-all`; `make dev-down` para tudo.
  - Motivo: são cerca de 2,4 GiB de RAM só para ele (RISK-008), e o domínio não depende dele (AC-030).
  - Organização e projeto passam a "Controle Integrado" / `controle-integrado-dev`.
  - Ganhou healthchecks no web e no worker.
- **Worker:**
  - subcomando `worker` do mesmo binário, em processo e container separados (04 §7);
  - healthcheck HTTP próprio em `/health`, porta 8090 só dentro do container;
  - hot reload com `.air.worker.toml` e diretório de build próprio (`tmp-worker/`).
- **Portas novas** publicadas só em `127.0.0.1` (09 §8). As portas existentes (Postgres, API, SPA) ficaram como estavam.
- **Versões fixadas:**

  | Serviço | Imagem |
  |---|---|
  | LiteLLM | `ghcr.io/berriai/litellm:v1.104.0` |
  | n8n | `n8nio/n8n:2.42.4` |
  | Langfuse | `langfuse:4.53.0` e `langfuse-worker:4.53.0` |
  | ClickHouse | `25.12.11.4` |
  | Redis | `7.4.11-bookworm` |
  | Postgres do Langfuse | `17.11-bookworm` |

  A origem usava `latest`/`main-latest` no Conciliador e só a versão maior (`4`, `7`, `17`) no Langfuse.
- **Preservado como na origem, adaptado depois:**
  - `litellm/config.yaml` (aliases e padrões Anthropic do Conciliador) → OpenAI configurável na TASK-004;
  - `n8n/workflows/ingestao.json`, que aponta para `http://app:8000/ingestao/arquivo` → endpoint novo na TASK-015. A pasta `n8n/inbox`, que o workflow observa e a origem não montava, agora é montada.
- **`.env.example`:** valores `dev-only` que funcionam localmente, na convenção do projeto-base. Não são segredos reais; para qualquer uso fora da máquina local, gerar valores próprios.
- **Ponto aberto para a TASK-009:**
  - API e worker (projeto `controle`) precisam alcançar o Langfuse (projeto `controle-langfuse`). Hoje estão em redes diferentes, e o Langfuse só publica em `127.0.0.1`.
  - Opções: rede externa compartilhada entre os dois projetos, ou `host.docker.internal`. Esta funciona no Docker Desktop, mas não no Linux com bind em loopback.
  - A decisão vai junto com a sincronização dos traces.
  - **Resolvido na D-022 (TASK-009):** rede externa `controle-obs`, criada pelo Makefile.

### D-012 — Agente `echo` fora do porte (07/10/2026)

- **Decisão do usuário:** o agente `echo` do Fator R (FR-23), ferramenta de dev para exercitar o tracer, não é portado. O tracer fica coberto por testes automatizados na TASK-009.

### D-011 — Capacidades não citadas pela spec entram no escopo (07/10/2026)

- **Decisão do usuário:** "se precisamos das capacidades, temos que desenvolver." Toda capacidade da origem não citada pela spec (`fontes.md` §4) é desenvolvida.
- **Distribuição:** `docs/ai-spec/13-implementation-plan.md` §12, assim:
  - FR-03 → TASK-014;
  - FR-07 → TASK-011/014;
  - FR-15 → TASK-013;
  - FR-19 → TASK-013/014;
  - CC-08 → TASK-016;
  - CC-09 → TASK-021;
  - CC-12 → TASK-017/021;
  - CC-22 → TASK-021/024.
- **Nova TASK-031 (M6, antes da TASK-030):** execução local completa fora do modo dev + backup e restauração de todos os armazenamentos (FR-21). A lacuna veio do próprio projeto-base: o `docker-compose.prod.yml` sobe só `app` + `db`, e o `scripts/backup.sh` cobre só o banco e os arquivos de procurações. Sem essa TASK, o produto consolidado instalado fora do modo dev ficaria sem worker, MinIO, LiteLLM, n8n e Langfuse.

### D-010 — Corrigir o baseline antes da TASK-003 (07/10/2026)

- **Decisão do usuário:** corrigir BL-001 (acessibilidade `target-size` no Dashboard) e BL-002 (`source-map-js`) agora, num commit `fix` separado, antes da TASK-003.
- **Motivo:** o teste de acessibilidade varre todas as telas num caso só, então enquanto falha esconde violações novas. Com o baseline todo verde, qualquer falha futura é regressão.
- **Execução:** a verificação da correção revelou mais dois problemas pré-existentes que também impediam um baseline verde e confiável. Os dois entraram no mesmo commit, com a mesma finalidade:
  - BL-003: asserção instável de aviso no E2E;
  - BL-004: detecção do Docker pelo testcontainers no Windows sob paralelismo.

  Detalhes e evidências em `baseline.md` §7.

### D-009 — Parser PGDAS-D determinístico: C-001 resolvido com a opção A (07/10/2026)

- **Decisão do usuário:** opção A, ou seja, portar o parser determinístico (regex), sem chamada de modelo em nenhuma etapa. `parser_pgdas` continua sendo o agente de trace, decisão e avaliação, mas não tem alias no LiteLLM.
- **Registro:** ADR-006. Documentos da spec ajustados: `CLAUDE.md` (IA), `04-system-design` §8, `07-flows` F-002, `10-env-and-secrets` §5, `ai-behavior-spec` §2, `prompt-spec` §3, `eval-plan` §2, `13-implementation-plan` (TASK-004).

### D-008 — Alias `classificador` (proposta técnica, TASK-004)

- **Contexto:** a spec lista `classificador` "quando necessário". No Conciliador, o agente de classificação usa o alias `resolvedor` (`app/agentes/classificador.py`).
- **Proposta:** criar o alias `classificador` apontando, por padrão, para o mesmo modelo do `resolvedor`. O comportamento de origem não muda, e o modelo passa a ser trocável de forma independente pela tela de IA.
- **Status:** aplicado na TASK-004 (D-014).

### D-007 — Precedência sobre o `AGENTS.md` do projeto-base (07/10/2026)

- **Contexto:** o `AGENTS.md` do `controle-tarefas` proíbe filas, dependências fora da lista aprovada e integrações externas (§6.1, §6.2), e define commits em inglês no formato `T-Mx-NN` (§8.8). A consolidação exige fila/worker, LiteLLM, Langfuse, MinIO e n8n.
- **Decisão:** vale a ordem de prioridade do `CLAUDE.md`: instrução do usuário > `docs/ai-spec/*` > `CLAUDE.md` > documentos existentes do `controle-tarefas`. O `AGENTS.md` continua valendo para o que a consolidação não altera: regras de domínio (§7), convenções de código, API, testes e segurança (§6.3–§6.5, §8.2–§8.7). O arquivo não é editado nem apagado.
- **Origem:** `CLAUDE.md` (Prioridade de instruções) e 00-index (Arquivos de agente).

### D-006 — Chave da OpenAI (07/10/2026)

- **Decisão do usuário:** a chave será entregue por arquivo, na hora de usar. Ela nunca entra no repositório, no banco, em log ou na API. Testes padrão não precisam dela (NFR-007).

### D-005 — Design System InovaSala entregue: Q-001 resolvida (07/10/2026)

- **Material:**
  - `docs/design-system/design-system-inovasala.html`, idêntico (mesmo SHA-256 `a48e7fa0…`) a `fator-r/docs/design-system-inovasala.html`;
  - `docs/design-system/design-system-inovasala-luminous.md`, versão operacional com tokens em JSON, regras e checklist.
- **Logos:** ficam em `C:\Users\mauro\OneDrive\Documentos\CEATH\Inovasala\Inova Sala SVG\` (versões branca e preta, com e sem "by Ceath"). Só entram no repositório na TASK-029.
- **Referência de adaptação** do sistema visual (pensado para páginas de marketing) a uma interface operacional densa: o próprio Fator R já fez esse trabalho no commit `31e7fd4` ("aplicar tema visual CEATH com tokens e gate de cores"), documentado em `fator-r/.claude/sdd/features/DESIGN_TEMA_CEATH.md`.
- **Efeito:** o AC-035 deixa de estar bloqueado por falta de material e passa a ser trabalho normal da TASK-029. A ADR-005 foi atualizada.
- **Observação:** o repositório é público. Os dois arquivos ficam públicos no primeiro push; o HTML já é público no `fator-r`.

### D-004 — Cadência de validação (07/10/2026)

- **Decisão do usuário:** parar ao fim de cada TASK para validação. A TASK-001 e a TASK-002 foram executadas juntas, porque só registram estado e não alteram código.

### D-003 — Ambiente de execução no Windows (07/10/2026)

- **Decisão do usuário:** opção A, ou seja, Git Bash + GNU Make no Windows, com o código em `C:\Desenvolvimento`.
- **Aplicado:**
  - GNU Make 4.4.1 (`ezwinports.make`, via winget);
  - Docker Desktop (WSL2);
  - clone com `core.autocrlf=false`;
  - `.env` local com `HTTP_PORT=8088`, porque a 8080 está ocupada por outro processo do usuário.

### D-002 — Git (07/10/2026)

- **Decisão do usuário:**
  - branch local `feat/consolidacao` a partir de `develop`;
  - um commit local por TASK, **depois** da validação do usuário;
  - mensagens em Conventional Commits em português;
  - nenhuma operação remota (push, PR, merge, tag) sem pedido explícito.
- Para o trabalho da consolidação, isso substitui o formato `T-Mx-NN` e as branches `task/...` do `AGENTS.md` §8.8 (instrução do usuário tem prioridade 1). Também é compatível com a DT-02 do projeto-base: nada vai para `main`.

### D-001 — Organização do workspace (07/10/2026)

- **Decisão do usuário:**
  - o repositório final é o `controle-tarefas`, não um repositório novo;
  - o workspace fica assim:

```text
C:\Desenvolvimento\controle-a-fonte\
  controle-tarefas\                  repositório de trabalho (branch feat/consolidacao)
  referencias\fator-r\               só leitura, commit 9dd7a74 (TASK-002)
  referencias\conciliador-contabil\  só leitura, commit 2bc6e26 (TASK-002)
  spec\                              pacote original da spec (cópia versionada aqui)
```

## 2. Conflitos

### C-001 — `parser_pgdas`: extração por LLM (spec) × parser determinístico (origem) — **resolvido (D-009, ADR-006)**

- **Spec de consolidação:**
  - F-002 diz "Worker resolve alias `parser_pgdas` no LiteLLM → extrair campos + confiança";
  - `prompt-spec` §3 define um prompt de extração;
  - `ai-behavior-spec` §2 trata o agente como extrator;
  - o `CLAUDE.md` lista `parser_pgdas` como alias de modelo.
- **Origem (Fator R):**
  - o `CLAUDE.md` §3.2 diz "O parse é determinístico. A extração do PGDAS-D é por regras (regex e heurísticas). O LLM nunca extrai campos de documento na v1";
  - no `plan.md` §7.3, a etapa `plan` do `parser_pgdas` é "sem LLM" e o `render` é template;
  - o código confirma: `parsing/pgdas.py` abre com "regras e regex, sem LLM";
  - a meta de ≥ 80% de acerto ouro mede esse parser determinístico.
- **Por que é material:** usar LLM para extrair é uma mudança de comportamento e de risco que a regra soberana da própria spec proíbe ("não substituir validação determinística por LLM", "PRD de origem é contrato comportamental"). Afeta a TASK-004 (lista de aliases), a TASK-012 (parser) e o eval-plan §2.
- **Opções:**
  - **A (recomendada):** portar o parser determinístico para Go, com a mesma regex, confiança e versão, provado pelo golden das 14 fixtures. `parser_pgdas` continua sendo o nome do agente, mas sem chamada de modelo; o alias não é criado, ou fica reservado e desabilitado.
  - **B:** parser determinístico primeiro; LLM só como sugestão para documento que o regex não lê, sempre em `needs_review` e sem gravar nada. Isso é capacidade nova e exige ADR.
  - **C:** extração por LLM como a spec descreve. Exige ADR, golden novo e aceite explícito da troca de comportamento.
- **Status:** resolvido em 07/10/2026 com a opção A (D-009, ADR-006).

## 3. Pendências

| ID | Pendência | Situação |
|---|---|---|
| P-001 | BL-001 (a11y `target-size` no Dashboard, `baseline.md` §3) | aprovado: corrigir antes da TASK-003 (D-010) |
| P-002 | BL-002 (`source-map-js`, npm audit alto) | aprovado: corrigir antes da TASK-003 (D-010) |
| P-003 | Capacidades não citadas pela spec | aprovado: entram no escopo (D-011) |
| P-004 | FR-23 agente `echo` | aprovado: não portar (D-012) |
| P-005 | D-017: sujeição ao Fator R pelo `companies.factor_r` e carteira só com empresas do Simples no PA | validada pelo usuário com a fatia (08/10/2026) |
| P-006 | Exibição do fator com 2 casas, como na origem: 0,279999 aparece como "28,00%" ao lado de "Anexo V" (o anexo e o semáforo usam o valor exato) | observação para a TASK-029; sem mudança de regra |

### Pendências herdadas (permanecem pendentes; não resolver por suposição)

| Origem | Pendência |
|---|---|
| Controle de Tarefas | Q1–Q8 de `docs/decisoes.md` (valores provisórios em `docs/pendencias.md`), conforme Q-002 do intake |
| Consolidação | ~~Q-003: modelo OpenAI padrão de cada alias~~ — **decidida em 07/10/2026 (D-015)** |
| Fator R | P-04: conferência do CSV das tabelas do Simples contra a LC 123/2006 |
| Fator R | P-05: proporcionalização de empresa com menos de 12 meses (hoje devolve `empresa_nova_regra_pendente`) |
| Fator R | P-07: planilha de aceite (2 empresas × 12 meses); T-707: conferência com empresa real |
| Conciliador | golden com extratos reais (gate M0) e as questões abertas 1, 4, 5 e 6 do PRD |
