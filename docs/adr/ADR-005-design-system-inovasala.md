# ADR-005 — Design System InovaSala

Status: Accepted (artefato entregue em 07/10/2026)

## Decisão

A camada visual final do produto deve usar o Design System InovaSala indicado pelo usuário.

## Artefato

- `docs/design-system/design-system-inovasala.html`: showcase oficial ("InovaSala / Luminous").
- `docs/design-system/design-system-inovasala-luminous.md`: versão operacional, com tokens em JSON, regras "faça / não faça" e checklist de consistência.
- Logos (SVG, versões branca e preta): fora do repositório até a TASK-029; localização em `docs/consolidacao/decisoes.md` (D-005).

Histórico: até 07/10/2026 o anexo não estava disponível no contexto da spec ([PENDING]). O usuário entregou o arquivo no início da execução (D-005).

## Regra

Não inventar tokens visuais: cores, fontes, espaçamentos, raios, sombras e componentes saem do artefato acima. O sistema foi desenhado para páginas de apresentação; a adaptação a telas operacionais densas (TASK-029) deve preservar os tokens e registrar cada decisão de adaptação. Referência dessa adaptação já feita no Fator R: `fator-r/.claude/sdd/features/DESIGN_TEMA_CEATH.md` (commit `31e7fd4`). Arquitetura de informação e comportamento podem avançar antes da TASK-029.
