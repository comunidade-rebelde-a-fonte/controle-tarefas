-- +goose Up
-- Otimização de consultas [T-M7-02, PRD §51]: períodos ainda abertos (backlog, "competências
-- anteriores abertas" no dashboard e no "Exige atenção"). Com o tempo, quase todos os
-- períodos antigos ficam fechados; o índice parcial cobre só os abertos.
CREATE INDEX company_module_periods_open_idx ON company_module_periods (competence_id, company_id) WHERE state = 'OPEN';

-- +goose Down
DROP INDEX company_module_periods_open_idx;
