-- +goose Up
-- Ordem de inserção determinística para históricos: linhas gravadas na mesma transação
-- compartilham created_at (mesmo instante do Clock) e o id é UUID aleatório.
ALTER TABLE obligation_status_history ADD COLUMN seq bigint GENERATED ALWAYS AS IDENTITY;
ALTER TABLE company_module_period_events ADD COLUMN seq bigint GENERATED ALWAYS AS IDENTITY;

-- +goose Down
ALTER TABLE company_module_period_events DROP COLUMN seq;
ALTER TABLE obligation_status_history DROP COLUMN seq;
