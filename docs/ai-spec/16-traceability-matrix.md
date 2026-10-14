# 16 — Matriz de Rastreabilidade

## Requisitos → critérios → tarefas → testes

| Requisito | Critério(s) | Tarefa(s) | Teste(s) |
|---|---|---|---|
| REQ-001 | AC-001, AC-004 | TASK-001, TASK-030, TASK-031 | TEST-001, TEST-030, TEST-031 |
| REQ-002 | AC-002, AC-034 | TASK-006, TASK-007, TASK-025 | TEST-006, TEST-007, TEST-025 |
| REQ-003 | AC-003 | TASK-030 | TEST-030 |
| REQ-004 | AC-004 | TASK-001, TASK-030, TASK-031 | TEST-001, TEST-030, TEST-031 |
| REQ-005 | AC-002, AC-023, AC-025 | TASK-022, TASK-023 | TEST-022, TEST-023 |
| REQ-006 | AC-004, AC-026 | TASK-001, TASK-030, TASK-031 | TEST-001, TEST-030, TEST-031 |
| REQ-007 | AC-004 | TASK-001 | TEST-001 |
| REQ-008 | AC-002, AC-005 | TASK-006, TASK-011 | TEST-006, TEST-011 |
| REQ-009 | AC-005, AC-006 | TASK-006, TASK-010 | TEST-006, TEST-010 |
| REQ-010 | AC-005, AC-006 | TASK-010 | TEST-010 |
| REQ-011 | AC-007, AC-012 | TASK-011, TASK-014, TASK-024 | TEST-011, TEST-014, TEST-024 |
| REQ-012 | AC-008, AC-009 | TASK-012 | TEST-012 |
| REQ-013 | AC-010 | TASK-010, TASK-013 | TEST-010, TEST-013 |
| REQ-014 | AC-008, AC-010, AC-029 | TASK-012, TASK-013 | TEST-012, TEST-013 |
| REQ-015 | AC-031 | TASK-012, TASK-013 | TEST-012, TEST-013 |
| REQ-016 | AC-011 | TASK-014, TASK-030 | TEST-014, TEST-030 |
| REQ-017 | AC-012 | TASK-014, TASK-024 | TEST-014, TEST-024 |
| REQ-018 | AC-013, AC-014 | TASK-015, TASK-021 | TEST-015, TEST-021 |
| REQ-019 | AC-013, AC-014 | TASK-016 | TEST-016 |
| REQ-020 | AC-015, AC-016 | TASK-016 | TEST-016 |
| REQ-021 | AC-015, AC-016 | TASK-016 | TEST-016 |
| REQ-022 | AC-017 | TASK-016 | TEST-016 |
| REQ-023 | AC-020 | TASK-017, TASK-021 | TEST-017, TEST-021 |
| REQ-024 | AC-018 | TASK-018 | TEST-018 |
| REQ-025 | AC-019 | TASK-019 | TEST-019 |
| REQ-026 | AC-020 | TASK-020 | TEST-020 |
| REQ-027 | AC-020 | TASK-020 | TEST-020 |
| REQ-028 | AC-021 | TASK-020 | TEST-020 |
| REQ-029 | AC-022 | TASK-020, TASK-028 | TEST-020, TEST-028 |
| REQ-030 | AC-018, AC-020 | TASK-017, TASK-018 | TEST-017, TEST-018 |
| REQ-031 | AC-023 | TASK-022 | TEST-022 |
| REQ-032 | AC-024 | TASK-022 | TEST-022 |
| REQ-033 | AC-025, AC-026 | TASK-007, TASK-023 | TEST-007, TEST-023 |
| REQ-034 | AC-025 | TASK-023 | TEST-023 |
| REQ-035 | AC-012, AC-033 | TASK-024 | TEST-024 |
| REQ-036 | AC-034 | TASK-014, TASK-021, TASK-025 | TEST-014, TEST-021, TEST-025 |
| REQ-037 | AC-034 | TASK-026 | TEST-026 |
| REQ-038 | AC-027, AC-028 | TASK-003, TASK-004, TASK-019 | TEST-003, TEST-004, TEST-019 |
| REQ-039 | AC-028 | TASK-004, TASK-008 | TEST-004, TEST-008 |
| REQ-040 | AC-027 | TASK-008 | TEST-008 |
| REQ-041 | AC-030, AC-031 | TASK-003, TASK-009 | TEST-003, TEST-009 |
| REQ-042 | AC-029, AC-030 | TASK-009 | TEST-009 |
| REQ-043 | AC-013 | TASK-003, TASK-015, TASK-026, TASK-031 | TEST-003, TEST-015, TEST-026, TEST-031 |
| REQ-044 | AC-013, AC-034 | TASK-026, TASK-031 | TEST-026, TEST-031 |
| REQ-045 | AC-014 | TASK-003, TASK-015, TASK-031 | TEST-003, TEST-015, TEST-031 |
| REQ-046 | AC-014 | TASK-005, TASK-012, TASK-015 | TEST-005, TEST-012, TEST-015 |
| REQ-047 | AC-027, AC-029 | TASK-008, TASK-009 | TEST-008, TEST-009 |
| REQ-048 | AC-032 | TASK-027 | TEST-027 |
| REQ-049 | AC-032 | TASK-027, TASK-030 | TEST-027, TEST-030 |
| REQ-050 | AC-033 | TASK-014, TASK-021, TASK-029 | TEST-014, TEST-021, TEST-029 |
| REQ-051 | AC-035 | TASK-029 | TEST-029 |
| REQ-052 | AC-035 | TASK-014, TASK-021, TASK-029 | TEST-014, TEST-021, TEST-029 |

## Testes

- **TEST-001** baseline das suítes do Controle de Tarefas.
- **TEST-002** refs/fontes de comportamento registradas.
- **TEST-003** stack/healthchecks de infraestrutura consolidada.
- **TEST-004** aliases LiteLLM/model routing.
- **TEST-005** fila PostgreSQL/worker/retry.
- **TEST-006** migrations e domínio Fator R básico.
- **TEST-007** migrations/constraints da Conciliação.
- **TEST-008** configuração de modelo sem secrets.
- **TEST-009** auditoria local + Langfuse sync/falha.
- **TEST-010** golden do motor Fator R.
- **TEST-011** carteira/API/performance Fator R.
- **TEST-012** Inbox/PGDAS/parser/review.
- **TEST-013** simulador + agentes consultor/priorizador.
- **TEST-014** UI/E2E Fator R.
- **TEST-015** MinIO/upload/dedupe/falha de storage.
- **TEST-016** extrato/validador/playbook/golden.
- **TEST-017** comprovantes/baixa confiança/bordas.
- **TEST-018** matching passes 1–4/ambiguidade.
- **TEST-019** resolvedor sem persistência autônoma.
- **TEST-020** classificação/razão/export source.
- **TEST-021** fila de revisão/UI/a11y.
- **TEST-022** integração Conciliação ↔ obrigação.
- **TEST-023** fechamento Contábil/trava/reabertura.
- **TEST-024** dashboard/drill-down.
- **TEST-025** ficha empresa/identidade canônica.
- **TEST-026** agregador de documentos multi-storage.
- **TEST-027** seed-demo reproduzível.
- **TEST-028** exportação XLSX/CSV.
- **TEST-029** Design System/visual/a11y.
- **TEST-030** E2E consolidado + revisão final.
- **TEST-031** execução local completa + backup e restauração de todos os armazenamentos (adendo D-011, `13-implementation-plan.md` §12).

As capacidades incluídas pelo adendo D-011 (FR-03, FR-07, FR-15, FR-19, FR-21, CC-08, CC-09, CC-12, CC-22) entram no aceite das TASKs indicadas em `13-implementation-plan.md` §12 e são conferidas na TASK-030 contra `docs/consolidacao/fontes.md` §4.

## Baseline de fontes

- TASK-001 registra o baseline funcional do sistema-base.
- TASK-002 registra as referencias imutaveis dos projetos Fator R e Conciliador usadas para testes de equivalencia.
