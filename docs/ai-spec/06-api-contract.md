# 06 — Contrato de API

## 1. Princípios

- Base: `/api/v1`.
- OpenAPI 3.1 existente permanece a fonte de verdade frontend/backend.
- Endpoints atuais do Controle de Tarefas não devem ser renomeados sem necessidade.
- Novos endpoints usam o mesmo envelope/padrão de erro do projeto-base: `{code, message, details}`.
- Sem autenticação da aplicação principal.
- Operações em período fechado devem usar erro explícito (`423` quando compatível com padrão atual).

## 2. Empresas

Reusar CRUD atual. Adicionar views/endpoints de extensão sem duplicar identidade.

### `GET /companies/{id}/overview`

Retorna agregação para ficha:
- dados gerais;
- competência selecionada;
- resumo de obrigações;
- resumo Fator R;
- resumo Conciliação;
- documentos recentes;
- fechamentos.

> **Como foi implementado (TASK-025; D-039).** `GET /companies/{id}/overview?competence=` devolve a identidade da empresa, o progresso por módulo com o estado do período (os fechamentos), os próximos vencimentos, o Fator R do PA com a tendência (só na carteira), a Conciliação da competência e as pendências do "Exige atenção" só da empresa. Numa competência ainda não gerada, vêm só o Fator R e a Conciliação (`competence_generated`). Os dados gerais vêm do `GET /companies/{id}`, e os documentos recentes ficam para a TASK-026. Também novos: `GET /company-indicators?competence=` (os indicadores da listagem) e o filtro `company_id` em `GET /backlog` (a aba Obrigações da ficha).

## 3. Fator R

### `GET /factor-r/portfolio?pa=YYYY-MM`

Retorna carteira ordenada por semáforo e economia.

> **Como foi implementado (TASK-011).** O contrato completo está em `backend/api/openapi.yaml`, tag `factor-r`.
> - `pa` é opcional; sem ele, vale a competência corrente.
> - PA sem tabela do Simples vigente → `422 SIMPLES_TABLE_NOT_IN_FORCE`.
> - Movimentos têm `version` (0 cria; divergência → 409); a listagem filtra por `from`/`to`.
> - Valores decimais são strings exatas.

### `GET /companies/{id}/factor-r/settings`
### `PATCH /companies/{id}/factor-r/settings`

Configuração específica da empresa.

### `GET /companies/{id}/factor-r/movements`
### `PUT /companies/{id}/factor-r/movements/{competence}`

Upsert controlado de movimento mensal.

### `GET /companies/{id}/factor-r/calc?pa=YYYY-MM`

Saída determinística:
- janela início/fim;
- meses preenchidos/faltantes;
- RBT12;
- FS12;
- Fator R;
- anexo;
- folha mínima 28/30;
- gap;
- reforço mensal;
- alíquotas;
- economia estimada;
- status de dados.

### `POST /factor-r/pgdas`

Multipart upload manual. Resposta `202 Accepted`:
```json
{"document_id":"uuid","job_id":"uuid","status":"received"}
```

### `GET /factor-r/pgdas`
### `GET /factor-r/pgdas/{id}`
### `POST /factor-r/pgdas/{id}/link`
### `POST /factor-r/pgdas/{id}/create-company`
### `POST /factor-r/pgdas/{id}/review`

Preservar regras do Inbox original.

> **Como foi implementado (TASK-012; D-023).** O contrato está no OpenAPI, tag `factor-r`.
> - O upload (`multipart/form-data`, campo `file`, até 10 MB, PDF ou TXT pelo conteúdo) responde 202 e o job `pgdas_parse` processa no worker. O mesmo arquivo de novo devolve 200 com o documento existente e `duplicate = true`. Recusas: 413 `FILE_TOO_LARGE` e 415 `UNSUPPORTED_FILE`.
> - `GET /factor-r/pgdas` aceita `status`, `company_id` (extratos vinculados à empresa, para a ficha; TASK-014b), `limit` (1 a 200) e `offset`, e traz `contagem_por_status` do inbox inteiro. O detalhe mostra os campos lidos, a identificação, as séries, a confiança, o estado do job, o texto da última corrida do agente e a `sugestao_cadastro`.
> - Rota a mais: `GET /factor-r/pgdas/{id}/file` baixa o original como anexo (o `/arquivo` da origem).
> - `review` é a rejeição da origem: `{"decisao": "rejeitar", "motivo": "…"}` (3 a 500 caracteres).
> - `create-company` recebe `{"empresa": <CompanyCreate>, "inicio_atividade": "YYYY-MM"}` e responde 201 com `documento` e `empresa`. Recusas antes de gravar: 409 `PGDAS_INVALID_STATE`, 422 `PGDAS_NO_VALID_CNPJ`, 422 `PGDAS_CNPJ_MISMATCH` e 409 `CNPJ_CONFLICT`.

### `POST /companies/{id}/factor-r/simulations`
### `GET /companies/{id}/factor-r/simulations`

### `POST /factor-r/agents/consult`
### `POST /factor-r/agents/prioritize`

Retorno inclui texto, decisão estruturada e `trace_id`.

> **Como foi implementado (TASK-013; D-024).** O contrato está no OpenAPI, tag `factor-r`.
> - Simulação: corpo `{pa, meta?, fracao_pro_labore?, inss?, irrf?, horizonte_meses?}` (decimais como string) e resposta 201 `{simulation_id, trace_id, pa, veredito, texto, parametros, resultado, created_at}`. O histórico aceita `limit` de 1 a 50.
> - Consultor: `{mensagem, company_id?}`. Priorizador: `{mensagem, pa?}`. Os dois respondem `{texto, decisao, trace_id}`.
> - Recusas: 404 para empresa inexistente, 422 `SIMPLES_TABLE_NOT_IN_FORCE` e 409 `RECOMMENDATION_WITHOUT_SIMULATION` ("corrigir" sem simulação).

## 4. Conciliação

### `POST /reconciliation/documents`

Multipart upload manual. Salva original em MinIO e enfileira job. Resposta 202.

### `GET /reconciliation/documents`

Filtros: company, competence/período, type, status, bank.

### `GET /reconciliation/documents/{id}`

Detalhes, diagnóstico e URL/endpoint seguro de visualização.

### `POST /reconciliation/documents/{id}/retry`

Somente para estados retry/review compatíveis; não ignora portões.

> **Como foi implementado (TASK-015; D-026).** O contrato está no OpenAPI, tag `reconciliation`.
> - O envio é multipart, com o arquivo no campo `file` e, opcionais, `company_id` e `competence`: PDF, PNG, JPEG ou texto, validados pelo conteúdo, até 20 MB.
>   - Responde 202 `{document_id, job_id, status}`.
>   - Recusas: 409 `DUPLICATE_DOCUMENT` (com `details.document_id`), 413, 415 e 503 `STORAGE_UNAVAILABLE` (sem gravar nada).
>   - Com o Contábil fechado na competência informada, o documento fica `pendencia_periodo_fechado`, sem job.
> - A lista filtra por `status`, `company_id`, `kind`, `bank` e `competence`, esta pela competência do envio ou pelo período lido na triagem. Traz `count_by_status`.
> - O detalhe traz o hash, a versão do original, a triagem, o diagnóstico e o `job` (AC-014).
> - Rota a mais: `GET /reconciliation/documents/{id}/file` lê a versão gravada do original (inline, ou `?download=1` para baixar).
> - O `retry` deste item vem com o processamento, na TASK-016. Até lá, o job encerrado com falha volta pela `POST /jobs/{id}/retry`.

> **Desde a TASK-016 (D-027):**
> - `POST /reconciliation/documents/{id}/retry` responde 202 com o novo job. Recusas: 409 `DOCUMENT_NOT_RETRYABLE` (importado ou já na fila) e 423 `PERIOD_CLOSED`.
>   - A revisão de triagem e a pendência de período fechado voltam à triagem; a revisão de extração, à extração. O documento com o job encerrado sem sucesso repete o mesmo job.
> - O detalhe traz também `triage_trace_id`, `import` (a importação aprovada) e `attempts` (o log de extração, sem a saída bruta).

> **Desde a TASK-017 (D-028):**
> - O detalhe traz `entry`, o lançamento lido do comprovante. O `retry` vale também para o comprovante em revisão sem lançamento; com lançamento, 409 `DOCUMENT_NOT_RETRYABLE` (a correção é campo a campo, na fila).
> - Rotas a mais, dos lançamentos do cliente (CC-12; `/lancamentos` e `/lancamentos/csv` da origem):
>   - `GET /reconciliation/entries`: filtros `company_id`, `status`, `date_from`, `date_to`, `limit`, `offset`;
>   - `POST /reconciliation/entries`: lançamento manual (201). Recusas: 400 com os campos e 423 `PERIOD_CLOSED`;
>   - `POST /reconciliation/entries/import`: CSV `data;valor;sentido;contraparte` com `company_id`, tudo ou nada (201 `{inserted, ids}`; 400 `INVALID_ENTRIES_CSV` com `details.lines`; 413; 423).

### `GET /companies/{id}/reconciliation?competence=YYYY-MM`

Resumo da competência: contas, imports, buckets, progresso, fechamento contábil.

> **Como foi implementado (TASK-021c; D-034).**
> - O resumo traz `accounting_closed`, os extratos que cobrem a competência (com a conta, o período, os saldos, as tentativas e as linhas), a conciliação (`matching`, com o percentual automático e a meta), os baldes, os lançamentos e a classificação.
> - As contas bancárias ficam em `GET` e `POST /companies/{id}/bank-accounts`: as cadastradas e as lidas nos extratos sem cadastro (`read`); a identidade é única no sistema (409 `BANK_ACCOUNT_EXISTS`).
> - O painel de métricas é `GET /reconciliation/metrics` (filtros `company_id` e `competence`) e os playbooks, `GET /reconciliation/playbooks`.

### `GET /reconciliation/review-queue`

Filtros por bucket, empresa, competência, tipo e estado.

### `POST /reconciliation/review/{id}/manual-match`
### `POST /reconciliation/review/{id}/unmatch`
### `POST /reconciliation/review/{id}/confirm-no-receipt`
### `PATCH /reconciliation/review/{id}/fields`

> **Como foi implementado (TASK-021a; D-032).**
> - `GET /reconciliation/review-queue`: os itens por balde e as contagens (filtros `company_id`, `competence`, `bucket`). `GET /reconciliation/review/{id}`: a linha, a conciliação ou os candidatos.
> - `manual-match` recebe `{entry_ids}` (N:1) e devolve a diferença de valor; `confirm-no-receipt` como na origem. As duas passam a linha pela classificação.
> - O desfazer é `POST /reconciliation/reconciliations/{id}/undo` com `{reason}` obrigatório (o `{id}` é a conciliação, e vale também para a automática). `GET /reconciliation/reconciliations` lista as recentes.
> - A correção do campo é do comprovante: `GET` e `PATCH /reconciliation/entries/{id}` (o `{id}` é o lançamento), com a rodada da competência na resposta.
> - "Que documento é esse?": `POST /reconciliation/documents/{id}/identify`. O original como imagem: `GET /reconciliation/documents/{id}/pages` e `.../pages/{page}` (PNG).
### `POST /reconciliation/review/{id}/accept-proposal`
### `POST /reconciliation/review/{id}/reject-proposal`

Toda ação registra auditoria.

> **Como foi implementado o resolvedor (TASK-019; D-030).** O `{id}` é a linha do extrato.
> - Rotas a mais: `POST /reconciliation/review/{id}/proposal` pede a hipótese (202 com o job `agent_resolver`; o mesmo job se o pedido já está na fila, com `already_queued`) e `GET /reconciliation/review/{id}/proposals` lista as propostas da linha, da mais recente para a mais antiga.
> - `accept-proposal` e `reject-proposal` recebem `{proposal_id}`. O aceite devolve a proposta e as conciliações criadas; só concilia linhas e lançamentos da empresa ainda livres.
> - Recusas: 404 (`RECONCILIATION_LINE_NOT_FOUND`, `PROPOSAL_NOT_FOUND`), 400 sem a proposta, 409 (`LINE_RECONCILED`, `PROPOSAL_DECIDED`, `PROPOSAL_NOTHING_TO_ACCEPT`) e 423 `PERIOD_CLOSED`.

### `POST /companies/{id}/reconciliation/run?competence=YYYY-MM`

Roda matching determinístico e, se configurado, enfileira resolvedor para remanescentes.

> **Como foi implementado (TASK-018; D-029, ADR-008).** Roda na hora e devolve os baldes da competência (`new_matches`, `by_pass`, `reconciled`, `divergent`, `without_receipt`, `without_statement`, `awaiting_review`).
> - Idempotente; 404 para empresa inexistente, 400 para competência inválida e 423 `PERIOD_CLOSED` com o Contábil fechado.
> - O worker também roda a competência (job `reconciliation_run`) quando importa extrato ou grava comprovante.
> - O resolvedor para os remanescentes chega com a TASK-019.
>
> **Desde a TASK-019 (D-030):** com `RESOLVEDOR_AUTOMATICO` (padrão desligado), a rodada enfileira o resolvedor uma só vez para cada linha livre sem proposta nem pedido anterior e devolve `resolver_requested`.

### `GET /companies/{id}/ledger?from=YYYY-MM&to=YYYY-MM`

Razão como projeção.

### `GET /companies/{id}/ledger/export?format=xlsx|csv`

> **Como foi implementado (TASK-020; D-031).**
> - `GET /companies/{id}/ledger`: uma linha por conciliação ativa, com a conta bancária, a conta contábil e a origem da classificação, a contraparte, débito e crédito, os totais e os ids do drill-down; filtro `account` da origem. `GET /reconciliation/reconciliations/{id}` é o drill-down (classificação com a regra, transação e comprovantes com os originais).
> - `export`: `format=csv` (as colunas da origem e as do AC-022); o XLSX chega com a TASK-028.
> - Rotas a mais, do plano e das regras (M5.1, M5.2): `GET`/`POST /companies/{id}/chart-accounts`, `PATCH .../chart-accounts/{accountId}`, `POST .../chart-accounts/import` (CSV `codigo;nome`, tudo ou nada) e `POST .../chart-accounts/copy`; `GET`/`POST /companies/{id}/classification-rules` e `POST .../classification-rules/{ruleId}/deactivate`.
> - Da classificação: `POST /companies/{id}/classification/run?competence=`; na revisão (o `{id}` é a linha), `PUT .../classification` (correção, com a regra derivada proposta), `POST .../classification-suggestion` (202, job `agent_classifier`), `GET .../classification-suggestions` e `POST .../accept-classification-suggestion` e `.../reject-classification-suggestion`.
> - A rodada do matching devolve também `classification` (por regra, por histórico, pendentes) e `classifier_requested`.

## 5. Fechamento e sincronização

Usar endpoints atuais do módulo Contábil para fechar/reabrir. A Conciliação deve refletir o estado.

Opcionalmente expor:

> **Como foi implementado (TASK-022; D-036).** `POST /companies/{id}/reconciliation/complete?competence=` (409 `RECONCILIATION_NOT_READY` com `details.blocking`; 422 `DEPENDENCY_BLOCKED`; 423 `PERIOD_CLOSED`; idempotente) e `POST .../reopen?competence=` com `{reason}` (409 `RECONCILIATION_NOT_COMPLETED`; 423). Os eventos `ReconciliationCompleted` e `ReconciliationReopened` são chamadas de serviço na mesma transação. O estado da conclusão vem no resumo `GET /companies/{id}/reconciliation`. Concluída, toda mutação da Conciliação na competência responde 423 `RECONCILIATION_COMPLETED`.

### `GET /companies/{id}/reconciliation/closure?competence=YYYY-MM`

Resposta derivada do `company_module_period` Contábil; não persiste segundo fechamento.

Eventos internos:
- `ReconciliationCompleted`
- `ReconciliationReopened`
- `AccountingPeriodClosed`
- `AccountingPeriodReopened`

> **Como foi implementado (TASK-023; D-037).** `GET /companies/{id}/reconciliation/closure?competence=` devolve o período do Contábil (nulo se não foi gerado), os eventos (abertura, fechamento e reabertura com o motivo) e `requires_reconciliation`. Fechar e reabrir são os endpoints do Controle (`POST /company-module-periods/{id}/close` e `.../reopen`). Quando a empresa usa a Conciliação na competência, o fechamento do Contábil exige a obrigação Conciliação final (422 `RECONCILIATION_NOT_FINAL`). `AccountingPeriodClosed` e `AccountingPeriodReopened` são os eventos do período no Controle (`company_module_period_events`).

Podem ser chamadas de serviço transacionais no monólito; não exigem message broker.

## 6. Dashboard e documentos

### `GET /dashboard?competence=YYYY-MM`

Expandir resposta atual com seções `factor_r` e `reconciliation`, sem quebrar campos existentes.

> **Como foi implementado (TASK-024; D-038).** `factor_r` é a carteira no PA igual à competência, com os números por semáforo, a economia em jogo, as que exigem ação e as cinco prioridades. `reconciliation` traz os documentos de todas as competências (processando, em revisão, parados por período fechado), a fila da competência e as empresas prontas para concluir e com o Contábil bloqueado. Os filtros do Dashboard valem nas duas: o responsável é o do Fiscal no Fator R e o do Contábil na Conciliação. Cada número traz o drill-down com os filtros. As listas aceitam os mesmos filtros: `GET /factor-r/portfolio` (e o semáforo na tela), `GET /reconciliation/review-queue`, `GET /reconciliation/documents` (vários status e `filter_competence`) e a nova `GET /reconciliation/companies`.

### `GET /documents`

Agregador de documentos de múltiplos backends. Retorna `module`, `storage_backend`, `status`, company, period, timestamps e link de detalhe. Não precisa unificar tabela física.

> **Como foi implementado (TASK-026; D-040).** Uma consulta sobre as tabelas do Fator R (`factor_r_pgdas_documents`, armazenamento local), da Conciliação (`reconciliation_documents`, MinIO) e do Controle (`credential_controls` com arquivo, armazenamento local). Cada item traz `module`, `storage_backend`, `kind`, o `status` do módulo e a `situation` comum de leitura (em processamento, em revisão, concluído, descartado; decisão do usuário), a empresa, a competência ou o período, o banco, a validade da credencial, as datas e o `detail` com a tela que abre o documento. Filtros: `module`, `company_id`, `competence`, `kind`, `status` (exige `module`), `situation`, `bank`, `limit` e `offset`. Vêm também as contagens por origem, situação e status.

## 7. Configuração de IA

### `GET /settings/ai/models`

Retorna aliases e configuração não secreta.

### `PUT /settings/ai/models/{alias}`

Request exemplo:
```json
{
  "provider": "openai",
  "model": "<nome-configuravel>",
  "parameters": {
    "temperature": 0
  },
  "enabled": true,
  "version": 3
}
```

Regras:
- não aceitar `api_key`;
- validar parâmetros permitidos;
- optimistic concurrency via `version` se padrão do projeto-base for aplicado;
- alteração vale somente para novas execuções.

### `POST /settings/ai/models/{alias}/validate`

Valida formato/resolução no LiteLLM sem persistir secret. Se exigir chamada real, deve ser ação explícita do usuário e claramente sinalizada.

> **Como foi implementado (TASK-008; D-021).** O contrato está no OpenAPI, tag `settings`.
> - A validação consulta só a tabela de modelos do LiteLLM (`/utils/model_info`): sem chamada ao provedor e sem custo.
> - O `PUT` repete a validação no servidor quando o modelo efetivo ou os parâmetros mudam: 422 `AI_MODEL_INVALID`, com as conferências em `details.checks`. Gateway indisponível ou não configurado: 503.
> - `provider` e `model` nulos voltam ao padrão do ambiente. `api_key` e qualquer outro campo fora do contrato dão 400.

## 8. Jobs

### `GET /jobs/{id}`

Status, tentativas, timestamps, erro sanitizado.

### `POST /jobs/{id}/retry`

Somente para estados permitidos e sem burlar revisão/portões.

> **Como foi implementado (TASK-005).** O contrato está no OpenAPI, tag `jobs`.
> - O reenvio vale só para `dead` e zera as tentativas.
> - Nos demais estados, `409 JOB_NOT_RETRYABLE`; com outro job ativo para o mesmo item, `409 JOB_ALREADY_ACTIVE`.
> - `GET` não expõe o payload e informa `worker_online`.

## 9. Observabilidade

### `GET /ai/runs`
### `GET /ai/runs/{trace_id}`
### `POST /ai/runs/{trace_id}/evaluation`

Permitem consultar auditoria local e feedback humano.

Link para Langfuse pode ser retornado como URL de configuração não secreta.

> **Como foi implementado (TASK-009; D-022).** O contrato está no OpenAPI, tag `ai`.
> - `GET /ai/runs`: filtros `agent`, `status`, `company_id` e `without_human_eval`, com paginação.
> - `GET /ai/runs/{trace_id}`: spans e decisões vêm da trilha local; inclui o `langfuse_url`.
> - `POST /ai/runs/{trace_id}/evaluation`: nota `acerto`, `parcial` ou `erro`; `erro` sem comentário dá 422 `EVAL_COMMENT_REQUIRED`.
> - `GET /ai/summary`: painel da origem (FR-18) com dados locais.

## 10. Erros relevantes

- `400 VALIDATION_ERROR`
- `404 NOT_FOUND`
- `409 CONFLICT`
- `409 DUPLICATE_DOCUMENT`
- `409 DEPENDENCY_WARNING`
- `422 DOMAIN_RULE_VIOLATION`
- `423 PERIOD_CLOSED`
- `429` apenas se rate limit do gateway/modelo for propagado como erro recuperável interno; UI deve mostrar job retry, não cru.
- `503 LLM_UNAVAILABLE`
- `503 STORAGE_UNAVAILABLE`

Erros de provider devem ser sanitizados; nunca retornar credencial/header.

## 11. Idempotência

- Upload: SHA-256/dedupe conforme módulo.
- Geração/sincronização de obrigação `Conciliação`: idempotente por company+competence+obligation_type.
- Jobs: execução efetiva uma vez por aquisição, com efeitos protegidos por unique/transaction.
- PGDAS: não sobrescrever movimentos existentes onde o PRD proíbe.

## 12. Contrato de dinheiro e datas

- Valores na API como string decimal quando necessário para preservar precisão.
- Competência `YYYY-MM`.
- Datas ISO 8601.
- Timestamps UTC; datas de negócio com `APP_TIMEZONE`.
