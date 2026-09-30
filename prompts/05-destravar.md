# Prompt 05 — Destravar: falha, bug ou bloqueio

Algo impede o avanço: teste ou CI vermelho, bug encontrado, erro de ambiente ou tarefa bloqueada. Resolva **pela causa**, seguindo o `AGENTS.md`.

## Entrada (fornecida pelo humano junto com este prompt)

- Descrição do problema, mensagem de erro, saída do comando ou passos para reproduzir.
- Se não houver descrição, investigue o estado atual: `git status`, `make lint`, `make test`.

---

## 1. Reproduzir

1. Releia o `AGENTS.md` (§6, §7 e §9).
2. Reproduza a falha com o **menor comando possível** (um teste, um pacote, uma requisição `curl`). Registre o comando e a saída relevante.
3. Se não reproduzir, documente as tentativas e investigue as diferenças de ambiente: versões, variáveis, banco sujo, relógio, ordem dos testes.

## 2. Encontrar a causa raiz

- Leia a mensagem de erro inteira e o código envolvido. Não chute.
- Localize **qual tarefa introduziu** o comportamento: `git log -S "<trecho>"`, `git blame` ou `git bisect` entre commits de `develop`.
- Compare o comportamento atual com a **fonte de verdade**: tarefa (Aceite), PRD (RN/CA) e decisões do `plan.md`. Às vezes o teste está certo e o código errado; às vezes o contrário. Decida com base nos documentos, não no que for mais fácil.
- Classifique a causa:

| Tipo | Exemplos |
|---|---|
| Código | Bug de lógica, query errada, race |
| Teste | Expectativa errada em relação ao PRD, teste dependente de data real ou de ordem (flaky) |
| Ambiente | Container, versão, porta, variável ausente |
| Especificação | Documentos em conflito, requisito ambíguo |

### Diagnóstico de Docker e banco (falhas de ambiente)

Rode, nesta ordem, e analise as saídas:

```bash
docker info >/dev/null && echo "docker ok"   # Docker está rodando?
test -f .env && echo ".env ok"               # .env existe? (senão: cp .env.example .env)
docker compose ps                            # serviços e status de health
make db-logs                                 # erros do Postgres
docker compose config -q                     # compose válido?
lsof -nP -iTCP:${POSTGRES_PORT:-5432} -sTCP:LISTEN   # porta ocupada por outro processo?
```

Causas frequentes:

| Sintoma | Causa provável | Correção |
|---|---|---|
| `connection refused` no host | Porta errada ou ocupada por outro Postgres | Ajustar `POSTGRES_PORT` no `.env` e o `DATABASE_URL` |
| API no container não conecta | `DATABASE_URL` com `localhost` em vez de `db` | Usar o host `db` dentro do compose |
| Banco `controle_e2e` ou `controle_perf` não existe | Volume criado antes do script de init (o init só roda com volume vazio) | `make db-reset`, avisando que apaga os dados de dev |
| Ordenação com acentos errada, ou timezone diferente de UTC | Volume antigo, criado sem `INITDB_ARGS` e `TZ` | `make db-reset` |
| testcontainers falha ao subir | Docker parado, ou Colima sem `DOCKER_HOST`/`TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE` | Informar o humano (configuração da máquina) |
| Vite não recarrega no container | Watch sem polling em bind mount | Conferir a configuração de polling (T-M0-05) |
| Migration falha "pela metade" | Erro no SQL. O goose roda cada migration em transação | Corrigir: numa migration **nova** se ela já foi commitada; no próprio arquivo, se ainda não foi |

**Proibido em diagnóstico de ambiente:**
- `docker system prune`, `docker volume prune`, ou remover containers e volumes de outros projetos;
- parar serviços da máquina (outro Postgres, por exemplo) para liberar porta. Mude a porta do projeto;
- conectar em bancos que não sejam os locais do projeto ou os containers de teste.

## 3. Corrigir dentro das regras

**Proibido:**
- pular, desabilitar ou apagar teste;
- afrouxar asserção ou lint;
- aumentar timeout para mascarar flakiness;
- editar migration commitada;
- adicionar dependência fora da lista;
- mudar decisão D*/R*.

Por tipo de causa:
- **Código ou teste:**
  - escreva primeiro um teste que **falha** reproduzindo o bug (regressão) e depois corrija;
  - se estiver em uma tarefa em andamento, corrija na própria branch da tarefa;
  - se for uma tarefa já concluída, use a branch `fix/T-Mx-NN-slug` a partir de `develop` e o commit `fix(T-Mx-NN): <descrição>`, depois merge `--no-ff` em `develop`;
  - se o bug mostrar que o **Aceite** da tarefa concluída nunca foi de fato atendido, desmarque a tarefa (`[ ]`), explique no relatório e a conclua de novo após a correção.
- **Schema:** correção via **migration nova**.
- **Ambiente:** corrija a configuração versionada (`docker-compose.yml`, `Makefile`, `.env.example`) quando for do projeto. Se for da máquina (software ausente, Docker parado), **não** instale nem altere o sistema: informe o humano do que precisa ser feito.
- **Especificação:** **pare e escale** (AGENTS.md §9) com o problema, as opções e uma recomendação. Não escolha sozinho.

## 4. Verificar

```bash
make lint
make test
make e2e    # se a falha envolvia UI/fluxo
```

Tudo verde, incluindo o novo teste de regressão. Para flakiness, rode o teste afetado ao menos 10 vezes seguidas (`go test -count=10 -run …`, ou o equivalente) e confirme a estabilidade.

## 5. Relatório

```markdown
## Destravamento — <resumo do problema>

**Sintoma:** <erro/comportamento, com comando de reprodução>
**Causa raiz:** <explicação> (tipo: código | teste | ambiente | especificação)
**Origem:** T-Mx-NN / commit <hash>
**Correção:** <o que mudou> — commit <hash>
**Teste de regressão:** <nome do teste>
**Verificação:** lint/test/e2e <resultado>
**Tarefas desmarcadas/reabertas:** <nenhuma | lista com motivo>
**Dados locais apagados (`make db-reset`):** <não | sim, motivo>
**Ação necessária do humano:** <nenhuma | o que precisa ser decidido/instalado>
```

Depois disso, retome o trabalho normal com o `prompts/02-executar-tarefas.md`.
