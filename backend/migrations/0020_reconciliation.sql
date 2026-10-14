-- +goose Up
-- Extensão Conciliação [docs/ai-spec/05 §5, TASK-007]: porte do schema do Conciliador
-- (app/db/models.py, commit 2bc6e26) com cliente → companies(id) [REQ-002] e usuario → users.
-- Não existe tabela de fechamento própria: a trava da Conciliação é o fechamento do módulo
-- Contábil em company_module_periods [REQ-033, 05 §6]. Nomes em inglês (convenção do projeto);
-- os vocabulários da origem (estados, C/D, métodos, origens) são mantidos. Mapeamento: D-020.
-- Dinheiro numeric(14,2) (MONEY da origem); confiança numeric(5,4) (CONFIDENCE).

-- conta_bancaria. A conta é única no sistema (a origem buscava a conta só por banco, agência e
-- conta, então duplicá-la entre clientes tornava a importação ambígua).
CREATE TABLE bank_accounts (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    bank       text        NOT NULL CHECK (btrim(bank) <> ''),
    branch     text        NOT NULL CHECK (btrim(branch) <> ''),
    account    text        NOT NULL CHECK (btrim(account) <> ''),
    active     boolean     NOT NULL DEFAULT true,
    created_by uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bank_accounts_identity_uq UNIQUE (bank, branch, account),
    CONSTRAINT bank_accounts_id_company_uq UNIQUE (id, company_id)
);

CREATE INDEX bank_accounts_company_idx ON bank_accounts (company_id);

-- documento_origem. Original imutável no MinIO versionado [REQ-018]; hash SHA-256 único (upload
-- repetido = duplicado, como na origem). A empresa pode ficar em aberto até a triagem.
CREATE TABLE reconciliation_documents (
    id                uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        uuid         NULL REFERENCES companies (id) ON DELETE RESTRICT,
    -- Competência informada no envio, quando houver (a origem a usava na checagem de período fechado).
    competence        date         NULL CHECK (competence IS NULL OR extract(day FROM competence) = 1),
    storage_backend   text         NOT NULL DEFAULT 'minio' CHECK (storage_backend IN ('minio')),
    bucket            text         NOT NULL CHECK (btrim(bucket) <> ''),
    object_key        text         NOT NULL CHECK (btrim(object_key) <> ''),
    object_version_id text         NULL,
    sha256            char(64)     NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    file_name         text         NOT NULL CHECK (btrim(file_name) <> ''),
    content_type      text         NOT NULL,
    size_bytes        bigint       NULL CHECK (size_bytes IS NULL OR size_bytes >= 0),
    -- tipo: definido pela triagem.
    kind              text         NULL CHECK (kind IN ('extrato', 'comprovante', 'desconhecido')),
    bank              text         NULL,
    branch            text         NULL,
    account           text         NULL,
    period_start      date         NULL,
    period_end        date         NULL,
    -- Estados congelados da origem: pendente → triado → extraido → importado, com os desvios.
    status            text         NOT NULL DEFAULT 'pendente' CHECK (status IN (
                          'pendente', 'triado', 'extraido', 'importado', 'revisao_triagem',
                          'revisao_extracao', 'duplicado', 'pendencia_periodo_fechado')),
    triage_confidence numeric(5,4) NULL CHECK (triage_confidence IS NULL OR triage_confidence BETWEEN 0 AND 1),
    -- Diagnóstico da triagem ou do validador (ResultadoValidacao) para a revisão.
    diagnosis         jsonb        NULL,
    uploaded_by       uuid         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at        timestamptz  NOT NULL DEFAULT now(),
    updated_at        timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT reconciliation_documents_sha256_uq UNIQUE (sha256),
    CONSTRAINT reconciliation_documents_object_uq UNIQUE (bucket, object_key),
    CONSTRAINT reconciliation_documents_period_ck CHECK (period_start IS NULL OR period_end IS NULL OR period_end >= period_start)
);

CREATE INDEX reconciliation_documents_status_idx ON reconciliation_documents (status, created_at);
CREATE INDEX reconciliation_documents_company_idx ON reconciliation_documents (company_id, created_at);

-- extrato_importacao. Só existe importação aprovada pelo portão aritmético [REQ-021, AC-015]:
-- a origem nunca persiste importação reprovada, e o CHECK garante isso no banco.
CREATE TABLE statement_imports (
    id              uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id      uuid          NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    document_id     uuid          NOT NULL REFERENCES reconciliation_documents (id) ON DELETE RESTRICT,
    bank_account_id uuid          NULL,
    bank            text          NOT NULL,
    branch          text          NOT NULL,
    account         text          NOT NULL,
    period_start    date          NOT NULL,
    period_end      date          NOT NULL,
    opening_balance numeric(14,2) NOT NULL,
    closing_balance numeric(14,2) NOT NULL,
    validation_ok   boolean       NOT NULL CHECK (validation_ok),
    -- ResultadoValidacao (somas, saldo calculado e esperado, diferença, linhas rejeitadas…).
    validation      jsonb         NOT NULL,
    -- Tentativas do extrator (1 a 3); 0 = importação determinística sem agente (CSV/texto nativo, CC-08).
    attempts        integer       NOT NULL CHECK (attempts BETWEEN 0 AND 3),
    created_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT statement_imports_document_uq UNIQUE (document_id),
    CONSTRAINT statement_imports_id_company_uq UNIQUE (id, company_id),
    CONSTRAINT statement_imports_period_ck CHECK (period_end >= period_start),
    CONSTRAINT statement_imports_bank_account_fk FOREIGN KEY (bank_account_id, company_id)
        REFERENCES bank_accounts (id, company_id) ON DELETE RESTRICT
);

CREATE INDEX statement_imports_company_period_idx ON statement_imports (company_id, period_start);

-- extrato_transacao. Valor sempre positivo com sentido C/D. A chave de dedupe é o contrato
-- congelado da origem: sha256(conta|data|valor|sentido|descrição normalizada|ordem no dia),
-- única no sistema, como na origem [05 §10].
CREATE TABLE statement_transactions (
    id                     uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id             uuid          NOT NULL,
    statement_import_id    uuid          NOT NULL,
    position               integer       NOT NULL CHECK (position >= 1),
    txn_date               date          NOT NULL,
    competence             date          GENERATED ALWAYS AS (date_trunc('month', txn_date::timestamp)::date) STORED,
    description            text          NOT NULL,
    document_ref           text          NULL,
    amount                 numeric(14,2) NOT NULL CHECK (amount > 0),
    direction              char(1)       NOT NULL CHECK (direction IN ('C', 'D')),
    balance_after          numeric(14,2) NULL,
    dedupe_key             char(64)      NOT NULL CHECK (dedupe_key ~ '^[0-9a-f]{64}$'),
    confidence             numeric(5,4)  NULL CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1),
    -- marcas: casos de borda e divergências sinalizados para a revisão, nunca resolvidos sozinhos [REQ-030].
    flags                  jsonb         NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(flags) = 'array'),
    account_code           text          NULL,
    classification_origin  text          NULL CHECK (classification_origin IN ('regra', 'historico', 'agente', 'manual')),
    classification_pending boolean       NOT NULL DEFAULT false,
    created_at             timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT statement_transactions_dedupe_uq UNIQUE (dedupe_key),
    CONSTRAINT statement_transactions_id_company_uq UNIQUE (id, company_id),
    CONSTRAINT statement_transactions_import_fk FOREIGN KEY (statement_import_id, company_id)
        REFERENCES statement_imports (id, company_id) ON DELETE RESTRICT
);

CREATE INDEX statement_transactions_import_idx ON statement_transactions (statement_import_id, position);
CREATE INDEX statement_transactions_company_competence_idx ON statement_transactions (company_id, competence, txn_date);

-- lancamento_cliente: comprovante extraído (confiança e origem por campo) ou lançamento à mão
-- (formulário ou CSV, CC-12). Campo de baixa confiança fica nulo e vai à revisão [REQ-023].
CREATE TABLE client_entries (
    id                     uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id             uuid          NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    document_id            uuid          NULL REFERENCES reconciliation_documents (id) ON DELETE RESTRICT,
    entry_date             date          NULL,
    amount                 numeric(14,2) NULL CHECK (amount IS NULL OR amount > 0),
    direction              char(1)       NULL CHECK (direction IN ('C', 'D')),
    counterparty           text          NULL,
    counterparty_doc       text          NULL CHECK (counterparty_doc IS NULL OR counterparty_doc ~ '^([0-9]{11}|[0-9]{14})$'),
    document_ref           text          NULL,
    operation_type         text          NULL,
    status                 text          NOT NULL DEFAULT 'pendente' CHECK (status IN ('pendente', 'parcial', 'extraido', 'manual')),
    field_confidence       jsonb         NULL CHECK (field_confidence IS NULL OR jsonb_typeof(field_confidence) = 'object'),
    field_origin           jsonb         NULL CHECK (field_origin IS NULL OR jsonb_typeof(field_origin) = 'object'),
    duplicate_flagged      boolean       NOT NULL DEFAULT false,
    account_code           text          NULL,
    classification_origin  text          NULL CHECK (classification_origin IN ('regra', 'historico', 'agente', 'manual')),
    classification_pending boolean       NOT NULL DEFAULT false,
    flags                  jsonb         NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(flags) = 'array'),
    created_by             uuid          NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at             timestamptz   NOT NULL DEFAULT now(),
    updated_at             timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT client_entries_id_company_uq UNIQUE (id, company_id)
);

CREATE INDEX client_entries_company_date_idx ON client_entries (company_id, entry_date);
CREATE INDEX client_entries_document_idx ON client_entries (document_id);

-- proposta_resolvedor: o resolvedor só propõe [REQ-025, AC-019]. Nenhuma conciliação nasce aqui;
-- o aceite humano cria a conciliação manual que referencia a proposta.
CREATE TABLE resolver_proposals (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               uuid        NOT NULL,
    statement_transaction_id uuid        NOT NULL,
    -- HipoteseResolvedor: hipótese, justificativa, pares propostos, confiança.
    payload                  jsonb       NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    status                   text        NOT NULL DEFAULT 'pendente' CHECK (status IN ('pendente', 'aceita', 'rejeitada')),
    trace_id                 text        NULL,
    decided_by               uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    decided_at               timestamptz NULL,
    created_by               uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT resolver_proposals_txn_fk FOREIGN KEY (statement_transaction_id, company_id)
        REFERENCES statement_transactions (id, company_id) ON DELETE RESTRICT,
    CONSTRAINT resolver_proposals_decision_ck CHECK ((status = 'pendente') = (decided_at IS NULL))
);

CREATE INDEX resolver_proposals_pending_idx ON resolver_proposals (statement_transaction_id) WHERE status = 'pendente';

-- conciliacao: a transação do extrato e os lançamentos conciliados com ela. N:1 (passe 4, aceite
-- do resolvedor) fica em reconciliation_items; "sem comprovante" é uma conciliação sem itens.
-- Desfazer não apaga (a origem apagava a linha): marca undone_at e libera os itens, para a
-- auditoria e o razão, que é projeção das conciliações ativas [REQ-028].
CREATE TABLE reconciliations (
    id                       uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id               uuid         NOT NULL,
    statement_transaction_id uuid         NOT NULL,
    score                    numeric(5,4) NULL CHECK (score IS NULL OR score BETWEEN 0 AND 1),
    pass                     smallint     NULL CHECK (pass BETWEEN 1 AND 4),
    -- passe_1..passe_4 (cascata), manual, sem_comprovante, resolvedor.
    method                   text         NOT NULL CHECK (btrim(method) <> ''),
    origin                   text         NOT NULL CHECK (origin IN ('auto', 'manual')),
    proposal_id              uuid         NULL REFERENCES resolver_proposals (id) ON DELETE RESTRICT,
    created_by               uuid         NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at               timestamptz  NOT NULL DEFAULT now(),
    undone_at                timestamptz  NULL,
    undone_by                uuid         NULL REFERENCES users (id) ON DELETE RESTRICT,
    undo_reason              text         NULL,
    CONSTRAINT reconciliations_txn_fk FOREIGN KEY (statement_transaction_id, company_id)
        REFERENCES statement_transactions (id, company_id) ON DELETE RESTRICT,
    CONSTRAINT reconciliations_id_company_uq UNIQUE (id, company_id),
    -- Origem auto só nos passes determinísticos; ação humana tem autor.
    CONSTRAINT reconciliations_origin_ck CHECK (
        (origin = 'auto' AND pass IS NOT NULL AND created_by IS NULL) OR (origin = 'manual' AND created_by IS NOT NULL)
    ),
    CONSTRAINT reconciliations_undo_ck CHECK ((undone_at IS NULL) = (undone_by IS NULL))
);

-- Uma conciliação ativa por transação do extrato.
CREATE UNIQUE INDEX reconciliations_active_txn_uq ON reconciliations (statement_transaction_id) WHERE undone_at IS NULL;
CREATE INDEX reconciliations_company_idx ON reconciliations (company_id, created_at);

CREATE TABLE reconciliation_items (
    reconciliation_id uuid        NOT NULL,
    company_id        uuid        NOT NULL,
    client_entry_id   uuid        NOT NULL,
    -- Preenchido quando a conciliação é desfeita: o lançamento volta a ficar livre.
    released_at       timestamptz NULL,
    PRIMARY KEY (reconciliation_id, client_entry_id),
    CONSTRAINT reconciliation_items_reconciliation_fk FOREIGN KEY (reconciliation_id, company_id)
        REFERENCES reconciliations (id, company_id) ON DELETE RESTRICT,
    CONSTRAINT reconciliation_items_entry_fk FOREIGN KEY (client_entry_id, company_id)
        REFERENCES client_entries (id, company_id) ON DELETE RESTRICT
);

-- Um lançamento do cliente em no máximo uma conciliação ativa.
CREATE UNIQUE INDEX reconciliation_items_active_entry_uq ON reconciliation_items (client_entry_id) WHERE released_at IS NULL;

-- plano_contas por empresa [REQ-026]. Copiar de um padrão = copiar as linhas de outra empresa.
CREATE TABLE chart_accounts (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    code       text        NOT NULL CHECK (btrim(code) <> ''),
    name       text        NOT NULL CHECK (btrim(name) <> ''),
    active     boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chart_accounts_code_uq UNIQUE (company_id, code)
);

-- regra_classificacao: CNPJ, contraparte ou regex do descritivo → conta [REQ-026, REQ-027].
CREATE TABLE classification_rules (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id   uuid        NOT NULL REFERENCES companies (id) ON DELETE RESTRICT,
    rule_type    text        NOT NULL CHECK (rule_type IN ('cnpj', 'contraparte', 'regex_descritivo')),
    pattern      text        NOT NULL CHECK (btrim(pattern) <> ''),
    account_code text        NOT NULL CHECK (btrim(account_code) <> ''),
    active       boolean     NOT NULL DEFAULT true,
    created_by   uuid        NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX classification_rules_company_idx ON classification_rules (company_id) WHERE active;

-- playbook_banco: schema lógico congelado da origem (M1.5.1). Por banco, versionado; no máximo
-- um válido por banco (a origem buscava o vigente com scalar_one_or_none).
CREATE TABLE bank_playbooks (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    bank                text        NOT NULL CHECK (btrim(bank) <> ''),
    version             integer     NOT NULL CHECK (version >= 1),
    table_start_page    integer     NOT NULL CHECK (table_start_page >= 1),
    ignored_pages       jsonb       NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(ignored_pages) = 'array'),
    date_format         text        NOT NULL,
    decimal_separator   text        NOT NULL,
    sign_representation text        NOT NULL,
    balance_location    text        NOT NULL,
    winning_strategy    text        NOT NULL,
    validated_at        timestamptz NULL,
    valid               boolean     NOT NULL DEFAULT true,
    successes           integer     NOT NULL DEFAULT 0 CHECK (successes >= 0),
    failures            integer     NOT NULL DEFAULT 0 CHECK (failures >= 0),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT bank_playbooks_version_uq UNIQUE (bank, version)
);

CREATE UNIQUE INDEX bank_playbooks_valid_uq ON bank_playbooks (bank) WHERE valid;

-- extracao_log: uma linha por tentativa de extração e por intervenção humana (alias "humano").
-- Custo em dólares com 8 casas: o MONEY de 2 casas da origem zerava o custo por chamada.
CREATE TABLE extraction_logs (
    id                uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id        uuid          NULL REFERENCES companies (id) ON DELETE RESTRICT,
    document_id       uuid          NULL REFERENCES reconciliation_documents (id) ON DELETE RESTRICT,
    file_sha256       char(64)      NULL CHECK (file_sha256 IS NULL OR file_sha256 ~ '^[0-9a-f]{64}$'),
    alias             text          NOT NULL CHECK (btrim(alias) <> ''),
    -- provedor/modelo resolvido no início da corrida (snapshot, D-014).
    resolved_model    text          NULL,
    prompt_hash       char(64)      NULL,
    prompt_version    text          NULL,
    raw_output        text          NULL,
    validator_result  jsonb         NULL,
    attempt           integer       NOT NULL CHECK (attempt BETWEEN 0 AND 3),
    tokens_in         integer       NULL CHECK (tokens_in IS NULL OR tokens_in >= 0),
    tokens_out        integer       NULL CHECK (tokens_out IS NULL OR tokens_out >= 0),
    cost_usd          numeric(18,8) NULL CHECK (cost_usd IS NULL OR cost_usd >= 0),
    latency_ms        integer       NULL CHECK (latency_ms IS NULL OR latency_ms >= 0),
    trace_id          text          NULL,
    job_id            uuid          NULL REFERENCES jobs (id) ON DELETE RESTRICT,
    actor_id          uuid          NULL REFERENCES users (id) ON DELETE RESTRICT,
    change            text          NULL,
    error             text          NULL,
    created_at        timestamptz   NOT NULL DEFAULT now()
);

CREATE INDEX extraction_logs_document_idx ON extraction_logs (document_id, created_at);

-- +goose Down
DROP TABLE extraction_logs;
DROP TABLE bank_playbooks;
DROP TABLE classification_rules;
DROP TABLE chart_accounts;
DROP TABLE reconciliation_items;
DROP TABLE reconciliations;
DROP TABLE resolver_proposals;
DROP TABLE client_entries;
DROP TABLE statement_transactions;
DROP TABLE statement_imports;
DROP TABLE reconciliation_documents;
DROP TABLE bank_accounts;
