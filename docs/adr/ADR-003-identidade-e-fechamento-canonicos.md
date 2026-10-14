# ADR-003 — Empresa e fechamento Contábil canônicos

Status: Accepted

## Decisão

- `companies.id` é a identidade de empresa de todos os módulos.
- `company_module_periods` do módulo Contábil é a única autoridade de fechamento da Conciliação.

## Consequência

Não criar tabelas paralelas de cliente ou fechamento com autoridade concorrente.
