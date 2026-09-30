# Prompt 03 — Fechar um marco

Você vai verificar e encerrar um marco do projeto seguindo o `AGENTS.md` (em especial os §7, §10.2 e §6).

## Parâmetro

`MARCO`: `M0` … `M7`. Obrigatório. Se não for informado, deduza pelo último marco com todas as tarefas `[x]` e confirme em uma linha.

---

## 1. Pré-condições

1. Releia o `AGENTS.md`.
2. Em `develop`, com a árvore de trabalho limpa (`git status`).
3. Liste as tarefas do marco no `docs/tasks.md`. Se alguma estiver `[ ]`, **pare** e informe quais faltam; recomende o `prompts/02-executar-tarefas.md`.

## 2. Verificação completa a partir do zero

Recrie o ambiente e rode tudo, registrando as saídas resumidas:

```bash
make db-reset        # apaga e recria os bancos locais (controle, controle_e2e, controle_perf)
make migrate
make seed            # e make seed-demo, a partir de M1
make lint
make test
make e2e             # a partir de M3; usa o banco controle_e2e
make migrate-down && make migrate     # as migrations revertem e reaplicam sem erro
make dev             # a stack completa sobe (db + api + web) e /api/v1/health responde 200
```

Avise no relatório que o `db-reset` apagou os dados locais de desenvolvimento.

Qualquer falha: investigue com o método do `prompts/05-destravar.md`. O marco **não fecha** com nada vermelho.

## 3. Demonstrar o "Pronto quando" do marco

Abra `docs/plan.md` §5, no marco correspondente, e demonstre a frase **"Pronto quando"** com evidência objetiva: testes específicos, chamadas `curl` à API com a resposta resumida, fluxo E2E ou medição.

A partir de M3, inclua a medição de performance registrada em `docs/perf.md`.

## 4. Revisão de conformidade com o contrato

Revise o diff do marco: `git diff <commit de início do marco>..develop`, ou `git log --oneline` para localizar os commits `T-Mx-*`. Cheque cada item e aponte arquivo e linha em caso de violação:

| # | Verificação | Como checar (sugestão) |
|---|---|---|
| 1 | Nenhuma migration commitada foi editada depois do commit original | `git log --follow --format=%h -- backend/migrations/<arquivo>`: cada migration deve ter um único commit de conteúdo |
| 2 | Dependências só da lista aprovada (AGENTS.md §6.1) | Revisar `backend/go.mod` e `frontend/package.json` |
| 3 | Sem DELETE físico de dados históricos; sem endpoint DELETE fora dos previstos (arquivo de credencial, feriado custom) | `grep -rn "DELETE" backend/queries backend/internal` |
| 4 | Sem regra de negócio no frontend (atraso, progresso, aplicabilidade, dependências calculados no cliente) | Revisar `frontend/src/features`, `lib` |
| 5 | SQL parametrizado; nada de concatenação com entrada do usuário | `grep -rn "fmt.Sprintf" backend/internal \| grep -i -E "select\|insert\|update\|where"` |
| 6 | Tempo via `Clock` em regras de negócio | `grep -rn "time.Now()" backend/internal` (fora de `platform/clock`) |
| 7 | Sem testes pulados ou lint desligado sem justificativa | `grep -rn -E "t.Skip\|\.skip\(\|it.only\|@ts-ignore\|nolint\|eslint-disable"` |
| 8 | Sem segredos, `.env`, chaves ou dados reais versionados | `git ls-files \| grep -i -E "\.env$\|\.pfx\|\.p12\|\.key$"` |
| 9 | Todo CA do marco tem teste com o ID no nome | Cruzar `tasks.md` §4.1 com `grep -rn "CA0\|CA-0" backend frontend e2e` |
| 10 | Valores provisórios marcados **e** registrados | `grep -rn "A VALIDAR" backend frontend` × `docs/pendencias.md` |
| 11 | OpenAPI coerente com os handlers e tipos gerados atualizados | `make gen` não produz diff (`git status` limpo após rodar) |
| 12 | Nada do escopo excluído (AGENTS.md §6.2) foi implementado | Revisão do diff |
| 13 | Imagens Docker oficiais com versão fixada; Postgres Debian, não alpine; nada de `latest` | `grep -n "image:\|FROM" docker-compose*.yml Dockerfile` |
| 14 | E2E e carga apontam para `controle_e2e` e `controle_perf`, nunca para `controle` | `grep -rn "controle" Makefile e2e backend/cmd/loadgen docker` |
| 15 | Compose de produção (a partir de M7): `db` sem `ports:`, app como não-root, `.env.prod` não versionado | `docker compose -f docker-compose.prod.yml config`, `grep -n "USER" Dockerfile`, `git ls-files \| grep .env.prod` |
| 16 | Compose válido e stack sobe | `docker compose config -q` e `make dev` com os 3 serviços healthy |

**Correções:**
- Pequenas e dentro do escopo de tarefas do marco: corrija em commit `fix(T-Mx-NN): …` com teste de regressão.
- Qualquer coisa maior ou fora de escopo: **não corrija**; liste como proposta.

## 5. Relatório do marco

Entregue ao humano:

```markdown
# Fechamento do marco Mx — <nome>

**Resultado:** aprovado | aprovado com ressalvas | reprovado

## Tarefas
- T-Mx-01 … T-Mx-NN: todas [x]

## Verificação
- lint / test / e2e: <resumo com contagens>
- migrations up/down: ok
- "Pronto quando": <evidência>
- performance (se aplicável): <números>

## Conformidade (16 itens)
- <ok / violação encontrada → corrigida em <commit> / pendente>

## Critérios de aceite cobertos neste marco
- CA-0xx → <teste>

## Pendências abertas
- valores provisórios: <lista de docs/pendencias.md>
- propostas fora de escopo: <lista>

## Próximo marco
- primeira tarefa disponível: T-…
```

## 6. Integração em `main`

**Não** faça merge em `main`, push nem PR por conta própria. Termine perguntando:

> "Marco Mx verificado. Autoriza o merge de `develop` em `main` (e/ou push para o remoto)?"

Execute somente após um "sim" explícito, com `git checkout main && git merge --no-ff develop -m "Milestone Mx: <nome>"`, e depois volte para `develop`.

### Fechamento do M7 (projeto)

Além do acima, verifique o **AGENTS.md §10.3**:
- cada critério do PRD §54 com evidência na matriz do `tasks.md` §4.2;
- `docs/pendencias.md` vazio, ou cada item aceito explicitamente pelo humano;
- implantação reproduzível conforme `docs/operacao.md`;
- ensaio completo de produção **local**, com os dados de prova sempre fictícios:
  1. subir com `docker compose -f docker-compose.prod.yml --env-file .env.prod up -d` (o `.env.prod` é local e fica fora do git);
  2. executar UC-01;
  3. gerar backup com `scripts/backup.sh`;
  4. derrubar com `down -v`;
  5. restaurar com `scripts/restore.sh`;
  6. conferir que empresas, instâncias e arquivos de credenciais voltaram.
