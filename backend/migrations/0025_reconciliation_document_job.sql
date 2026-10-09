-- +goose Up
-- TASK-015: o documento da Conciliação aponta para o job que o processa (triagem), para a
-- interface mostrar o estado até sucesso, revisão ou falha [AC-014], como o documento do Inbox
-- PGDAS-D (0023). Nulo quando não há job: documento parado por período fechado.
ALTER TABLE reconciliation_documents
    ADD COLUMN job_id uuid NULL REFERENCES jobs (id) ON DELETE RESTRICT;

CREATE INDEX reconciliation_documents_job_idx ON reconciliation_documents (job_id) WHERE job_id IS NOT NULL;

-- +goose Down
DROP INDEX reconciliation_documents_job_idx;
ALTER TABLE reconciliation_documents DROP COLUMN job_id;
