-- +goose Up
-- TASK-016 (D-027): processamento dos documentos da Conciliação (triagem e extração do extrato).
-- A triagem e a importação ficam ligadas à corrida do agente que as decidiu [FR-16, D-022]: o
-- documento aponta a corrida do triador; a importação, a do extrator. Ficam nulas nos caminhos
-- determinísticos (leitor do CSV e do texto nativo do PDF), que não têm agente.
ALTER TABLE reconciliation_documents
    ADD COLUMN triage_trace_id text NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT;

ALTER TABLE statement_imports
    ADD COLUMN trace_id text NULL REFERENCES ai_runs (trace_id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE statement_imports DROP COLUMN trace_id;
ALTER TABLE reconciliation_documents DROP COLUMN triage_trace_id;
