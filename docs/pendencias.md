# Pendências — valores provisórios aguardando T-000

Valores usados no código ou nos seeds enquanto o escritório não responde às perguntas Q1–Q8 (`docs/plan.md` §9). Cada item está marcado no código com `A VALIDAR (Qn)`. Quando a resposta chegar, siga `prompts/04-registrar-decisoes.md`.

| Pergunta | Tarefa | Valor provisório | Onde está no código |
|---|---|---|---|
| Q1 | T-M1-06 | Regras de vencimento genéricas e provisórias ("Provisória — dia 10/15/20/25 do mês seguinte", "5º dia útil do mês seguinte", "último dia útil do mês seguinte"), atribuídas por tipo. Não são prazos legais. | `backend/seed/catalog.sql` (seção "Regras de vencimento" e coluna de regra dos tipos) |
| Q5 | T-M1-06 | Empresa "Sem folha": Folha e eSocial = Sem movimento; FGTS e INSS/IRRF = Não se aplica | `backend/seed/catalog.sql` (UPDATE `status_when_no_payroll`) |
| Q6 | T-M1-06 | Declarações × regime: REINF, SINTEGRA, DCTFWeb, REINF Retidos e REINF Lucros em todos os regimes; EFD Contribuições, SPED Fiscal e MIT só nos Normais; PGDAS e DeSTDA só no Simples | `backend/seed/catalog.sql` (CTE `regras`, bloco Declarações) |
| Q7 | T-M1-07 | Somente feriados nacionais (fixos + móveis) de 2025 a 2030; calendários de UF e município vazios, cadastráveis pela tela de feriados | `backend/seed/calendar.go`, `backend/internal/calendar/holidays.go` |
| Q2 | T-M2-07 | Coluna Fechamento do Fiscal fora dos percentuais e do "pronto para fechar" (R2/R3): regra de regime com `counts_in_progress = false` e `is_closing_stage` | `backend/internal/obligation/derived.go` (`InDenominator`), `backend/seed/catalog.sql` |
| Q3 | T-M2-07 | ICMS do Simples fora do progresso também não bloqueia o "pronto para fechar" (R4) | `backend/internal/obligation/derived.go` (`InDenominator`), `backend/seed/catalog.sql` |
| Q4 | T-M2-08 | Módulo fechado bloqueia a edição das instâncias (`423 PERIOD_CLOSED`) até a reabertura (R9) | `backend/internal/obligation/service.go` (`errPeriodClosed`) |
| Q8 | T-M7-06 | Produção começa na competência corrente do primeiro boot; competências retroativas não são geradas (as pendências anteriores seguem nas planilhas até a resposta) | `docs/operacao.md` §2 (Competência inicial de produção) |
