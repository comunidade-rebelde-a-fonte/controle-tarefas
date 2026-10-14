# ADR-004 — IA via LiteLLM, OpenAI inicial e modelos configuráveis

Status: Accepted

## Decisão

Usar LiteLLM como gateway de todos os agentes portados. Provider inicial OpenAI. Modelo definido por alias/configuração e editável pela UI sem armazenar secrets.

## Consequência

O domínio não referencia nomes de modelos concretos. Cada corrida grava o provider/modelo resolvido.
