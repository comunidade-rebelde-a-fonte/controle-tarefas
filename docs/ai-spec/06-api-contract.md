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

### `POST /companies/{id}/factor-r/simulations`
### `GET /companies/{id}/factor-r/simulations`

### `POST /factor-r/agents/consult`
### `POST /factor-r/agents/prioritize`

Retorno inclui texto, decisão estruturada e `trace_id`.

## 4. Conciliação

### `POST /reconciliation/documents`

Multipart upload manual. Salva original em MinIO e enfileira job. Resposta 202.

### `GET /reconciliation/documents`

Filtros: company, competence/período, type, status, bank.

### `GET /reconciliation/documents/{id}`

Detalhes, diagnóstico e URL/endpoint seguro de visualização.

### `POST /reconciliation/documents/{id}/retry`

Somente para estados retry/review compatíveis; não ignora portões.

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
