-- +goose Up
-- TASK-017 (D-028): comprovantes extraídos e lançamentos manuais do cliente.
-- O lançamento escrito pelo extrator aponta a corrida dele [FR-16, D-022]; fica nulo no leitor
-- determinístico e no lançamento manual.
ALTER TABLE client_entries
    ADD COLUMN trace_id text NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT;

-- Um comprovante, um lançamento (persistir_comprovante da origem): o reprocessamento do mesmo
-- documento não cria outro.
CREATE UNIQUE INDEX client_entries_document_uq ON client_entries (document_id) WHERE document_id IS NOT NULL;

-- +goose Down
DROP INDEX client_entries_document_uq;
ALTER TABLE client_entries DROP COLUMN trace_id;
