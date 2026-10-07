# ADR-002 — Preservar serviços auxiliares dos módulos

Status: Accepted

## Decisão

Manter MinIO, LiteLLM, n8n e Langfuse quando fazem parte do funcionamento existente. Consolidação não autoriza sua remoção por simplificação.

## Consequência

A stack local permanece composta por múltiplos serviços, mesmo com uma única aplicação principal.
