# ADR-001 — Aplicação-base no `controle-tarefas`

Status: Accepted

## Decisão

Usar `controle-tarefas` como repositório e stack canônicos da aplicação: Go + React/TypeScript/Vite + PostgreSQL.

## Consequência

Fator R e Conciliação são portados para módulos dessa aplicação. O objetivo é equivalência de comportamento, não reutilização literal de FastAPI/Next/Jinja/HTMX.
