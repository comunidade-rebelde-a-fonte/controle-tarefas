-- +goose Up
-- Instância com os metadados do status atual e o módulo do tipo, base dos derivados
-- (atraso, progresso, conclusão) nas grades e no dashboard [PRD §12; plan M2].
-- O atraso depende de "hoje" no fuso de negócio e por isso é calculado na consulta
-- (applicable AND NOT is_final AND due_date_effective < $hoje) [RN-013].
CREATE VIEW v_obligation_instances AS
SELECT
    i.*,
    t.code                             AS type_code,
    t.primary_module,
    t.is_closing_stage,
    t.sort_order                       AS type_sort_order,
    d.label                            AS status_label,
    d.counts_as_started,
    d.counts_as_operational_progress,
    d.is_final,
    d.is_blocking,
    d.excluded_from_denominator,
    (i.applicable AND i.counts_in_progress AND NOT d.excluded_from_denominator) AS in_denominator
FROM obligation_instances i
JOIN obligation_types t ON t.id = i.obligation_type_id
JOIN status_definitions d ON d.status_set_id = t.status_set_id AND d.code = i.status_code;

-- +goose Down
DROP VIEW v_obligation_instances;
