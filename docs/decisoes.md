# Decisões do escritório (T-000)

Respostas às perguntas em aberto do `docs/plan.md` §9. Este documento complementa o PRD quando ele é omisso (AGENTS.md §2). **Somente o humano responsável define as respostas.** O agente apenas as registra, a partir do que o humano informar (`prompts/04-registrar-decisoes.md`).

---

## Q1 — Vencimentos

**Pergunta:** qual a regra concreta (dia, deslocamento, útil/corrido, ajuste) de cada obrigação do catálogo? O PRD delega isso ao catálogo [§21.1]; o seed trará valores provisórios.

**Status:** sem resposta

---

## Q2 — Fechamento Fiscal (R2/R3)

**Pergunta:** confirmar que a coluna Fechamento não entra nos percentuais e que "Pronto para fechar" considera só os controles.

**Status:** sem resposta

---

## Q3 — ICMS Simples (R4)

**Pergunta:** confirmar que não bloqueia a sugestão de fechamento.

**Status:** sem resposta

---

## Q4 — Edição em período fechado (R9)

**Pergunta:** bloquear edição até a reabertura, ou apenas alertar?

**Status:** sem resposta

---

## Q5 — Folha "Sem folha" (D7)

**Pergunta:** para cada controle (Folha, eSocial, FGTS, INSS/IRRF), o padrão é `Sem movimento` ou `Não se aplica`?

**Status:** sem resposta

---

## Q6 — Aplicabilidade padrão por regime

**Pergunta:** matriz tipo × regime para Declarações (ex.: PGDAS e DeSTDA só no Simples? EFD Contribuições só em Normais?).

**Status:** sem resposta

---

## Q7 — Feriados estaduais e municipais

**Pergunta:** quais UFs e municípios concentram a carteira, para o seed inicial? (Pode ficar para M7.)

**Status:** sem resposta

---

## Q8 — Competência de início

**Pergunta:** a partir de qual competência o sistema entra em produção, e há necessidade de gerar competências retroativas vazias para lançar backlog real?

**Status:** sem resposta

---

# Decisões técnicas do responsável (fora de T-000)

Decisões tomadas pelo humano durante a execução, registradas pelo agente.

## DT-01 — Versão do Go (29/09/2026)

**Contexto:** as versões estáveis atuais das bibliotecas aprovadas (`pgx/v5`, `goose`, `maroto` v2, `validator`, `x/text`, `sqlc`, `golangci-lint`) exigem Go ≥ 1.26, enquanto o AGENTS.md §6.1 cita a imagem `golang:1.23` ("Go 1.23+").

**Decisão do responsável:** usar Go 1.26 com as bibliotecas atuais. O `go.mod` declara `go 1.26` e as imagens Docker usam `golang:1.26.x` com versão fixada (dev, CI e build de produção).

## DT-02 — Integração em `main` (29/09/2026)

**Decisão do responsável:** todo o trabalho fica em `develop`. Nenhum merge em `main` nem push sem pedido explícito.
