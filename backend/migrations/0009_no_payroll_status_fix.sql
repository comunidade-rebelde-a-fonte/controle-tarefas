-- +goose Up
-- Correção da 0007: status_when_no_payroll = 'NOT_APPLICABLE' significa "gerar a instância como
-- não aplicável" (applicable = false), e não um código de status — o conjunto PAYROLL não tem
-- NOT_APPLICABLE [PRD §11.5, plan D7]. A FK composta impedia esse valor. O CHECK
-- (NO_MOVEMENT | NOT_APPLICABLE) continua valendo; a existência de NO_MOVEMENT no conjunto
-- é validada pela aplicação.
ALTER TABLE obligation_types DROP CONSTRAINT obligation_types_no_payroll_status_fk;

-- +goose Down
-- NOT VALID: não revalida linhas existentes (podem conter NOT_APPLICABLE).
ALTER TABLE obligation_types
    ADD CONSTRAINT obligation_types_no_payroll_status_fk FOREIGN KEY (status_set_id, status_when_no_payroll)
    REFERENCES status_definitions (status_set_id, code) ON DELETE RESTRICT NOT VALID;
