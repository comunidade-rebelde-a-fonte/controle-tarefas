# 05 — Modelo de Dados Consolidado

## 1. Fonte de verdade

PostgreSQL do `controle-tarefas` passa a ser a base relacional canônica da aplicação. Os bancos separados de Fator R e Conciliador não são migrados, pois contêm apenas dados fictícios e servem como referência de comportamento.

## 2. Regra de identidade

`companies.id` é a FK obrigatória para entidades específicas de Fator R e Conciliação. Não criar nova tabela `cliente`/`firm` representando a mesma empresa.

## 3. Entidades existentes

Preservar todas as entidades atuais do Controle de Tarefas, incluindo, entre outras:

- `companies`
- `company_tax_regime_history`
- `company_module_settings`
- `company_obligation_overrides`
- `status_sets`
- `status_definitions`
- `obligation_types`
- `competencies`
- `company_competence_snapshots`
- `company_module_periods`
- `obligation_instances`
- `obligation_status_history`
- `credential_controls`
- calendários/feriados/regras de vencimento/settings.

Nenhuma migration da consolidação deve recriar essas tabelas com semântica paralela.

## 4. Extensão Fator R

### `company_factor_r_settings`

Uma linha por empresa quando o módulo tiver configuração específica.

Campos mínimos:
- `company_id` PK/FK `companies`.
- `cnae` nullable.
- `atividade` nullable.
- `sujeita_factor_r` boolean.
- `quantidade_socios` nullable.
- `contato` nullable.
- `pacote_comercial` enum/lookup `monitoramento|correcao|retainer|none`.
- `honorario_mensal` numeric nullable.
- `inicio_atividade` date nullable.
- `notas` text nullable.
- `created_at`, `updated_at`.

> **Como foi implementado (TASK-006a; D-017 e D-018).** Sem `sujeita_factor_r`: a sujeição é o `companies.factor_r` do cadastro (D-017). `quantidade_socios` e `honorario_mensal` são NOT NULL com os padrões da origem (1 e 0). `pacote_comercial` nulo = sem pacote. Há `version` para concorrência otimista.

### `factor_r_movements`

Unique `(company_id, competence)`.

Campos:
- `id`
- `company_id`
- `competence` DATE no dia 1
- `receita_bruta` NUMERIC
- `pro_labore` NUMERIC
- `salarios_13_ferias` NUMERIC
- `cpp_recolhida` NUMERIC
- `fgts_recolhido` NUMERIC
- `folha_total_declarada_pgdas` NUMERIC nullable
- `origem` `manual|pgdas|folha|agente`
- `observacao`
- timestamps

Regra: movimento PGDAS que traz folha total declarada não deve inventar split.

> **Como foi implementado (TASK-006a; D-018).**
> - `folha_total_declarada_pgdas` só existe com `origem = pgdas` e a divisão zerada (CHECK). Quando existe, é a folha do mês na FS12.
> - Outras colunas: `folha_mes` (gerada), `pgdas_document_id` (a FK chega na TASK-006b) e `version`.
> - As tabelas do Simples por vigência ficam em `factor_r_simples_tables`, carregadas pelo `make seed`.
> - Valores em `numeric`, lidos como decimal exato (ADR-007).

### `factor_r_pgdas_documents`

- `id`
- `company_id` nullable até vínculo
- `file_ref` / caminho no storage local do módulo
- `sha256`
- `status` `received|parsed|needs_review|linked|rejected`
- `cnpj_extraido`
- `pa`
- `rbt12`, `rpa`, `fs12`, `factor_r`, `das_valor`
- `anexo_citado`
- `nome_empresarial_extraido`
- `sugestao_sujeita_factor_r`
- `confidence`
- `extracted_text_ref` ou campo apropriado
- `parsed_json` JSONB
- `trace_id`
- timestamps

### `factor_r_simulations`

Persistir entrada e saída determinística necessária para auditoria:
- company, PA, meta, reforço, proporção pró-labore, INSS, IRRF, horizonte, economia, custo, líquido, veredito, trace_id opcional do consultor, timestamps.

### `factor_r_policy`

Configuração única/local:
- política CPP;
- meta operacional padrão (30% inicialmente);
- piso de economia anual;
- tolerâncias de eval;
- outros parâmetros já configuráveis no projeto original.

## 5. Extensão Conciliação

### `bank_accounts`

- `id`
- `company_id`
- banco/agência/conta
- ativo
- metadados
- unique conforme identidade bancária definida no domínio.

### `source_documents`

Entidade transversal para documentos de Conciliação se o modelo atual permitir; caso usar tabela própria seja mais fiel ao porte, nomear `reconciliation_documents`. Campos mínimos:
- `id`
- `company_id` nullable durante triagem
- `storage_backend` (`minio` para originais do Conciliador)
- bucket/key/version
- `sha256`
- tipo
- status
- período
- banco/conta inferidos
- timestamps

Não misturar arbitrariamente com documentos PGDAS se isso apagar estados específicos. Uma camada `documents` pode agregá-los via view/DTO.

### `statement_imports`

Cabeçalho de importação com saldos, período, banco/conta, resultado de validação, tentativas e documento de origem.

### `statement_transactions`

- `statement_import_id`
- ordem
- data
- descrição
- documento
- valor NUMERIC positivo
- sentido C/D
- saldo após nullable
- confiança
- dedupe_key unique no escopo apropriado.

### `client_entries`

Dados normalizados de comprovantes, com confiança por campo e referências/bbox quando aplicável.

### `reconciliations`

- transações e lançamentos vinculados (modelar N:1 sem perder pass 4)
- score
- método/passe
- origem `auto|manual`
- ator
- timestamps.

Se N:1 exigir tabela associativa, usar `reconciliation_items` em vez de arrays JSON sem integridade.

### `chart_accounts`

Plano de contas por empresa, com capacidade de copiar padrão.

### `classification_rules`

Regra por empresa/padrão/cnpj/contraparte/regex → conta.

### `bank_playbooks`

Preservar schema lógico congelado do Conciliador: banco, versão, localização da tabela/saldos, formato de data, separador decimal, sinal, páginas ignoradas, estratégia vencedora, valido, validado_em, métricas.

### `extraction_logs`

Por tentativa:
- documento/hash
- agente/alias
- provider/modelo resolvido
- prompt hash/version
- saída bruta ou referência segura
- resultado do validador
- tentativa
- custo/tokens quando disponível
- duração
- trace_id
- timestamps.

> **Como foi implementado (TASK-007; D-020).** A migration `0020_reconciliation.sql` cria `bank_accounts`, `reconciliation_documents`, `statement_imports`, `statement_transactions`, `client_entries`, `reconciliations` e `reconciliation_items`, `chart_accounts`, `classification_rules`, `bank_playbooks`, `extraction_logs` e `resolver_proposals`.
> - Tudo referencia `companies(id)`, com FKs compostas por empresa.
> - O banco impede importação reprovada pelo portão.
> - A chave de dedupe é a da origem, portada e provada por golden.
> - Não há tabela de fechamento.
> - O mapeamento completo origem → consolidado está na D-020.

## 6. Fechamento canônico

**Não criar** `periodo_fechamento` com autoridade independente para Conciliação.

A trava usa `company_module_periods` da empresa/competência/módulo Contábil. Podem existir eventos específicos de Conciliação, mas não um segundo boolean/status de fechamento que possa divergir.

## 7. Jobs

### `jobs`

Se ainda não existir no banco-base, portar a tabela do Conciliador:
- `id`
- `kind`
- `status` `pending|running|retry|done|dead`
- payload referencial JSONB
- `attempts`
- `max_attempts`
- `run_after`
- `locked_at`
- `locked_by`
- `last_error_code`
- `last_error_message` sanitizada
- timestamps.

Índices para aquisição eficiente por status/run_after.

> **Como foi implementado (TASK-005; D-019).**
> - Colunas a mais: `dedupe_key` (um job ativo por `kind` + chave), `finished_at` e `retried_by`/`retried_at` (reenvio manual).
> - O estado terminal é `dead`.
> - A posse (`locked_at`/`locked_by`) tem prazo e é renovada pelo worker.
> - A tabela `worker_heartbeats` registra o batimento de cada worker.

## 8. IA e configuração

### `ai_model_aliases`

- `alias` PK
- `provider`
- `model`
- parâmetros permitidos JSONB (somente não secretos)
- `enabled`
- `version`
- timestamps.

Seeds com aliases; valores concretos podem vir de env e ser substituídos na UI.

> **Como foi implementado (TASK-008; D-021).** Migration `0021_ai_model_aliases.sql`.
> - As 7 linhas do `ai.Catalog` nascem na própria migration (versão 1).
> - `provider`/`model` nulos = padrão do `.env` (`<ALIAS>_MODEL`).
> - Os parâmetros são uma lista fechada (`temperature`, `max_tokens`), garantida também por CHECK, e nome de modelo com cara de chave é recusado no banco.
> - `updated_by` registra quem alterou, e `ai_model_alias_history` guarda cada versão.

### `ai_runs`

Auditoria local mínima:
- `trace_id` unique
- `agent`
- `trigger`
- company/competence/PA nullable
- alias/model/provider resolvidos
- status
- confidence nullable
- latency
- tokens/cost quando disponível
- decision JSONB sanitizado
- `langfuse_sync_status`
- timestamps.

### `ai_human_evals`

Para feedback humano quando aplicável, preservando rubricas do Fator R.

## 9. Integração com obrigação `Conciliação`

A instância já existente em `obligation_instances` continua canônica. Criar vínculo lógico pelo `obligation_type` correspondente, não FK duplicada em todas as tabelas de Conciliação.

Serviço de integração deve localizar empresa + competência + tipo e aplicar transição via o serviço existente de obrigações, para preservar histórico/dependências/locks.

## 10. Constraints

- Dinheiro: `NUMERIC`, nunca float.
- Competência: DATE no primeiro dia do mês, API `YYYY-MM`.
- CNPJ continua único em `companies`.
- SHA-256 de original conforme escopo.
- Dedupe de transação exatamente conforme contrato original do Conciliador.
- Originais não podem ser apagados por fluxo normal.
- FK `ON DELETE RESTRICT` para registros auditáveis.
- Uniques explícitos para movimentos e identidades naturais.

## 11. Migrations

Estratégia aditiva:
1. AI/settings/jobs compartilhados.
2. extensão Fator R.
3. extensão Conciliação.
4. índices/views.
5. seeds/fixtures.

Cada migration:
- aplica em banco vazio;
- aplica sobre schema atual do Controle;
- não dropa tabela/coluna existente sem aprovação;
- possui rollback seguro quando possível.

[SECURITY][BLOCKER] Qualquer migration destrutiva exige aprovação humana explícita.
