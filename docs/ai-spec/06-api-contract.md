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

### `GET /reconciliation/review-queue`

Filtros por bucket, empresa, competência, tipo e estado.

### `POST /reconciliation/review/{id}/manual-match`
### `POST /reconciliation/review/{id}/unmatch`
### `POST /reconciliation/review/{id}/confirm-no-receipt`
### `PATCH /reconciliation/review/{id}/fields`
### `POST /reconciliation/review/{id}/accept-proposal`
### `POST /reconciliation/review/{id}/reject-proposal`

Toda ação registra auditoria.

### `POST /companies/{id}/reconciliation/run?competence=YYYY-MM`

Roda matching determinístico e, se configurado, enfileira resolvedor para remanescentes.

> **Como foi implementado (TASK-018; D-029, ADR-008).** Roda na hora e devolve os baldes da competência (`new_matches`, `by_pass`, `reconciled`, `divergent`, `without_receipt`, `without_statement`, `awaiting_review`).
> - Idempotente; 404 para empresa inexistente, 400 para competência inválida e 423 `PERIOD_CLOSED` com o Contábil fechado.
> - O worker também roda a competência (job `reconciliation_run`) quando importa extrato ou grava comprovante.
> - O resolvedor para os remanescentes chega com a TASK-019.

### `GET /companies/{id}/ledger?from=YYYY-MM&to=YYYY-MM`

Razão como projeção.

### `GET /companies/{id}/ledger/export?format=xlsx|csv`

## 5. Fechamento e sincronização

Usar endpoints atuais do módulo Contábil para fechar/reabrir. A Conciliação deve refletir o estado.

Opcionalmente expor:

### `GET /companies/{id}/reconciliation/closure?competence=YYYY-MM`

Resposta derivada do `company_module_period` Contábil; não persiste segundo fechamento.

Eventos internos:
- `ReconciliationCompleted`
- `ReconciliationReopened`
- `AccountingPeriodClosed`
- `AccountingPeriodReopened`

Podem ser chamadas de serviço transacionais no monólito; não exigem message broker.

## 6. Dashboard e documentos

### `GET /dashboard?competence=YYYY-MM`

Expandir resposta atual com seções `factor_r` e `reconciliation`, sem quebrar campos existentes.

### `GET /documents`

Agregador de documentos de múltiplos backends. Retorna `module`, `storage_backend`, `status`, company, period, timestamps e link de detalhe. Não precisa unificar tabela física.

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
