# ADR-005 — Design System InovaSala

Status: Accepted (artefato entregue em 07/10/2026; aplicado na TASK-029, D-043)

## Decisão

A camada visual final do produto deve usar o Design System InovaSala indicado pelo usuário.

## Artefato

- `docs/design-system/design-system-inovasala.html`: showcase oficial ("InovaSala / Luminous").
- `docs/design-system/design-system-inovasala-luminous.md`: versão operacional, com tokens em JSON, regras "faça / não faça" e checklist de consistência.
- Logos: o SVG oficial InovaSala by Ceath branco está em `docs/design-system/logos/`, com o script que gera dele o PNG do menu (TASK-029, D-043). As outras versões continuam fora do repositório (D-005).

Histórico: até 07/10/2026 o anexo não estava disponível no contexto da spec ([PENDING]). O usuário entregou o arquivo no início da execução (D-005).

## Regra

Não inventar tokens visuais: cores, fontes, espaçamentos, raios, sombras e componentes saem do artefato acima. O sistema foi desenhado para páginas de apresentação; a adaptação a telas operacionais densas (TASK-029) deve preservar os tokens e registrar cada decisão de adaptação. Referência dessa adaptação já feita no Fator R: `fator-r/.claude/sdd/features/DESIGN_TEMA_CEATH.md` (commit `31e7fd4`). Arquitetura de informação e comportamento podem avançar antes da TASK-029.

## Aplicação (TASK-029, D-043)

Os tokens estão em `frontend/src/index.css`, apontando para a paleta Tailwind do material. Cada adaptação (contraste AA, cores de estado e superfícies compostas) está registrada na D-043. Uma guarda (`frontend/src/app/theme.test.ts`) impede a volta da paleta clara antiga.
