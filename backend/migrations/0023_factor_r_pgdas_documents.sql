-- +goose Up
-- Inbox PGDAS-D do Fator R [05 §4, TASK-006b/TASK-012; origem: pgdas_documents, migration
-- 20260917_0007 e o agente parser_pgdas]. Um documento por arquivo: o sha256 é único e o reenvio
-- do mesmo arquivo devolve o documento existente, como na origem. O arquivo fica fora do banco,
-- em FILE_STORAGE_DIR/pgdas/<sha256>; o nome enviado é só metadado. Sem exclusão física.
--
-- Fluxo: received (upload; o job pgdas_parse vai processar) → needs_review | linked, decididos
-- pelo agente; da revisão, o analista vincula (linked) ou rejeita (rejected). parsed é o estado
-- intermediário da origem (entre o parse e a decisão, na mesma transação).
CREATE TABLE factor_r_pgdas_documents (
    id             uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Empresa só no documento vinculado (a origem só preenche no vínculo).
    company_id     uuid         NULL REFERENCES companies (id) ON DELETE RESTRICT,
    sha256         text         NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    file_ref       text         NOT NULL,
    mime           text         NOT NULL CHECK (mime IN ('application/pdf', 'text/plain')),
    size_bytes     integer      NOT NULL CHECK (size_bytes > 0),
    original_name  text         NULL CHECK (original_name IS NULL OR char_length(original_name) BETWEEN 1 AND 255),
    status         text         NOT NULL DEFAULT 'received'
                                CHECK (status IN ('received', 'parsed', 'needs_review', 'linked', 'rejected')),
    -- Motivo da revisão (cnpj_nao_encontrado, confianca_baixa (…), sem_camada_texto…) ou o
    -- texto da rejeição pelo analista.
    motivo         text         NULL CHECK (motivo IS NULL OR char_length(motivo) BETWEEN 1 AND 500),
    -- Campos lidos mais usados em consulta; o resultado completo do parser fica em parsed_json
    -- (o campos_json da origem: campos, confianças, motivos, identificação e séries).
    cnpj_extraido  text         NULL,
    pa             date         NULL CHECK (pa IS NULL OR extract(day FROM pa) = 1),
    confidence     numeric(5,4) NULL CHECK (confidence IS NULL OR confidence BETWEEN 0 AND 1),
    parser_version text         NULL,
    parsed_json    jsonb        NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(parsed_json) = 'object'),
    -- Texto extraído (texto_extraido da origem): fica no banco local e nunca vai ao Langfuse.
    extracted_text text         NULL,
    -- Última corrida do agente sobre o documento (upload, vínculo, rejeição, cadastro).
    trace_id       text         NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT,
    -- Job de processamento do upload (estado acompanhado pela UI) [AC-014].
    job_id         uuid         NULL REFERENCES jobs (id) ON DELETE RESTRICT,
    uploaded_by    uuid         NULL REFERENCES users (id) ON DELETE RESTRICT,
    version        integer      NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at     timestamptz  NOT NULL DEFAULT now(),
    updated_at     timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT factor_r_pgdas_documents_sha256_uq UNIQUE (sha256),
    CONSTRAINT factor_r_pgdas_documents_file_ck CHECK (file_ref = 'pgdas/' || sha256),
    CONSTRAINT factor_r_pgdas_documents_company_ck CHECK ((status = 'linked') = (company_id IS NOT NULL)),
    CONSTRAINT factor_r_pgdas_documents_motivo_ck CHECK ((status IN ('needs_review', 'rejected')) = (motivo IS NOT NULL)),
    -- Depois do upload, toda mudança de estado é decisão gravada numa corrida [AC-029].
    CONSTRAINT factor_r_pgdas_documents_trace_ck CHECK (status = 'received' OR trace_id IS NOT NULL)
);

CREATE INDEX factor_r_pgdas_documents_created_idx ON factor_r_pgdas_documents (created_at DESC, id);
CREATE INDEX factor_r_pgdas_documents_status_idx ON factor_r_pgdas_documents (status, created_at DESC);
CREATE INDEX factor_r_pgdas_documents_company_idx ON factor_r_pgdas_documents (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX factor_r_pgdas_documents_trace_idx ON factor_r_pgdas_documents (trace_id) WHERE trace_id IS NOT NULL;

-- A FK prometida na TASK-006a: o movimento criado a partir do extrato aponta o documento.
ALTER TABLE factor_r_movements
    ADD CONSTRAINT factor_r_movements_pgdas_document_fk
    FOREIGN KEY (pgdas_document_id) REFERENCES factor_r_pgdas_documents (id) ON DELETE RESTRICT;
CREATE INDEX factor_r_movements_pgdas_document_idx ON factor_r_movements (pgdas_document_id) WHERE pgdas_document_id IS NOT NULL;

-- +goose Down
DROP INDEX factor_r_movements_pgdas_document_idx;
ALTER TABLE factor_r_movements DROP CONSTRAINT factor_r_movements_pgdas_document_fk;
DROP TABLE factor_r_pgdas_documents;
